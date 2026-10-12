package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Limits struct{ LoginFailures, IPFailures, Registrations, UserRequests int }

func DefaultLimits() Limits {
	return Limits{LoginFailures: 5, IPFailures: 50, Registrations: 10, UserRequests: 120}
}

func (s *Sessions) mac(label, value string) string {
	hash := hmac.New(sha256.New, s.config.SigningKey)
	_, _ = hash.Write([]byte(label + "\x00" + value))
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
}

func (s *Sessions) preauth(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(s.cookieName("preauth"))
	if err != nil || len(cookie.Value) > 160 {
		return "", false
	}
	pieces := strings.Split(cookie.Value, ".")
	if len(pieces) != 3 || len(pieces[0]) != 43 {
		return "", false
	}
	expires, err := strconv.ParseInt(pieces[1], 10, 64)
	now := s.service.clock.Now().Unix()
	value := pieces[0] + "." + pieces[1]
	return cookie.Value, err == nil && expires > now && expires <= now+1800 && hmac.Equal([]byte(pieces[2]), []byte(s.mac("preauth-cookie", value)))
}

func (s *Sessions) sessionCSRF(r *http.Request) (string, bool, error) {
	cookie, err := r.Cookie(s.cookieName("refresh"))
	if err != nil || len(cookie.Value) != 43 {
		return "", false, nil
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	var secret []byte
	var sid string
	err = s.service.pool.QueryRow(r.Context(), `SELECT s.id::text,s.csrf_secret FROM auth_refresh_tokens t JOIN auth_sessions s ON s.id=t.session_id JOIN users u ON u.id=s.user_id WHERE t.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>$2 AND u.disabled_at IS NULL`, digest[:], s.service.clock.Now()).Scan(&sid, &secret)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("csrf session unavailable")
	}
	return s.mac("session-csrf", sid+"."+base64.RawURLEncoding.EncodeToString(secret)), true, nil
}

func (s *Sessions) csrf(w http.ResponseWriter, r *http.Request) {
	// No cross-origin script can read this response; no CORS headers are emitted.
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.config.Origin {
		s.writeError(w, r, &platform.APIError{Status: 403, Code: "ORIGIN_REJECTED", Message: "Откройте приложение по его адресу"})
		return
	}
	token, ok, err := s.sessionCSRF(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if !ok {
		value, valid := s.preauth(r)
		if !valid {
			expires := s.service.clock.Now().Add(30 * time.Minute)
			unsigned := randomOpaque() + "." + strconv.FormatInt(expires.Unix(), 10)
			value = unsigned + "." + s.mac("preauth-cookie", unsigned)
			http.SetCookie(w, &http.Cookie{Name: s.cookieName("preauth"), Value: value, Path: "/", HttpOnly: true, Secure: !s.config.InsecureLocalCookies, SameSite: http.SameSiteLaxMode, MaxAge: 1800, Expires: expires})
		}
		token = s.mac("preauth-csrf", value)
	}
	platform.WriteData(w, r, s.service.clock, 200, struct {
		Token string `json:"token"`
	}{token})
}

func (s *Sessions) browserMutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Origin") != s.config.Origin {
			s.writeError(w, r, &platform.APIError{Status: 403, Code: "ORIGIN_REJECTED", Message: "Откройте приложение по его адресу"})
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			s.writeError(w, r, &platform.APIError{Status: 415, Code: "UNSUPPORTED_MEDIA_TYPE", Message: "Используйте JSON"})
			return
		}
		supplied := r.Header.Get("X-CSRF-Token")
		if len(supplied) != 43 {
			s.writeError(w, r, csrfRejected())
			return
		}
		token, ok, err := s.sessionCSRF(r)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		valid := ok && hmac.Equal([]byte(token), []byte(supplied))
		if !valid {
			if value, ok := s.preauth(r); ok {
				valid = hmac.Equal([]byte(s.mac("preauth-csrf", value)), []byte(supplied))
			}
		}
		if !valid {
			s.writeError(w, r, csrfRejected())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func csrfRejected() *platform.APIError {
	return &platform.APIError{Status: 403, Code: "CSRF_REJECTED", Message: "Обновите страницу и повторите действие"}
}

func (s *Sessions) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var problem *platform.APIError
	if !errors.As(err, &problem) {
		problem = &platform.APIError{Status: 500, Code: "INTERNAL_ERROR", Message: "Не удалось выполнить запрос"}
	}
	platform.WriteError(w, r, problem)
}

func (s *Sessions) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.cookieName("access"))
		if err != nil {
			s.writeError(w, r, unauthorized())
			return
		}
		identity, err := s.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			var secret []byte
			if err := s.service.pool.QueryRow(r.Context(), `SELECT csrf_secret FROM auth_sessions WHERE id=$1`, identity.SessionID).Scan(&secret); err != nil {
				s.writeError(w, r, err)
				return
			}
			token := s.mac("session-csrf", identity.SessionID+"."+base64.RawURLEncoding.EncodeToString(secret))
			if !hmac.Equal([]byte(token), []byte(r.Header.Get("X-CSRF-Token"))) {
				s.writeError(w, r, csrfRejected())
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(platform.WithIdentity(r.Context(), identity)))
	})
}

// Protect is the boundary for private browser routes in subsequent modules.
func (s *Sessions) Protect(next http.Handler) http.Handler {
	return s.browserMutation(s.RequireSession(next))
}

func remoteIP(r *http.Request) string {
	// Forwarded headers are deliberately ignored. Only a separately configured,
	// trusted reverse proxy may establish client identity in a future deployment.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "unknown"
	}
	return ip.String()
}

type bucket struct {
	scope  string
	limit  int
	window time.Duration
}
type limited struct{ retry int }

func (l *limited) Error() string { return "request limit exceeded" }

// reserve is atomic across all buckets, including parallel API instances.
func (s *Sessions) reserve(ctx context.Context, buckets []bucket) error {
	return storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		now := s.service.clock.Now()
		for _, bucket := range buckets {
			hash := s.mac("rate-limit", bucket.scope)
			key, _ := base64.RawURLEncoding.DecodeString(hash)
			var attempts int
			var expires time.Time
			err := tx.QueryRow(ctx, `INSERT INTO auth_rate_limits(bucket_hash,attempts,expires_at) VALUES($1,1,$2) ON CONFLICT(bucket_hash) DO UPDATE SET attempts=CASE WHEN auth_rate_limits.expires_at<=$3 THEN 1 ELSE auth_rate_limits.attempts+1 END,expires_at=CASE WHEN auth_rate_limits.expires_at<=$3 THEN $2 ELSE auth_rate_limits.expires_at END WHERE auth_rate_limits.expires_at<=$3 OR auth_rate_limits.attempts<$4 RETURNING attempts,expires_at`, key, now.Add(bucket.window), now, bucket.limit).Scan(&attempts, &expires)
			if errors.Is(err, pgx.ErrNoRows) {
				if err := tx.QueryRow(ctx, `SELECT expires_at FROM auth_rate_limits WHERE bucket_hash=$1`, key).Scan(&expires); err != nil {
					return errors.New("rate limit unavailable")
				}
				return &limited{retry: max(1, int(expires.Sub(now).Seconds()+0.999))}
			}
			if err != nil {
				return errors.New("rate limit unavailable")
			}
		}
		return nil
	})
}

func (s *Sessions) release(ctx context.Context, buckets []bucket, startedAt time.Time) {
	// Successful login should not use a failed-attempt allowance. Do not extend
	// windows, and never decrement a new window created while hashing.
	for _, bucket := range buckets {
		key, _ := base64.RawURLEncoding.DecodeString(s.mac("rate-limit", bucket.scope))
		_, _ = s.service.pool.Exec(ctx, `UPDATE auth_rate_limits SET attempts=greatest(0,attempts-1) WHERE bucket_hash=$1 AND expires_at>$2 AND expires_at<=$3`, key, s.service.clock.Now(), startedAt.Add(bucket.window))
	}
}

func (s *Sessions) checkLimit(w http.ResponseWriter, r *http.Request, buckets []bucket) bool {
	err := s.reserve(r.Context(), buckets)
	if err == nil {
		return true
	}
	var limit *limited
	if errors.As(err, &limit) {
		w.Header().Set("Retry-After", strconv.Itoa(limit.retry))
		s.writeError(w, r, &platform.APIError{Status: 429, Code: "RATE_LIMITED", Message: "Слишком много попыток. Попробуйте позже"})
	} else {
		s.writeError(w, r, err)
	}
	return false
}

func (s *Sessions) Mount(router chi.Router, limits Limits) error {
	if limits.LoginFailures < 1 || limits.IPFailures < limits.LoginFailures || limits.Registrations < 1 || limits.UserRequests < 1 {
		return errors.New("invalid auth limits")
	}
	router.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(s.browserMutation)
		r.Get("/csrf", s.csrf)
		r.Post("/register", func(w http.ResponseWriter, r *http.Request) {
			if !s.checkLimit(w, r, []bucket{{scope: "register-ip:" + remoteIP(r), limit: limits.Registrations, window: 15 * time.Minute}}) {
				return
			}
			input, problem := platform.DecodeJSON[RegisterInput](r)
			if problem != nil {
				s.writeError(w, r, problem)
				return
			}
			account, err := s.service.Register(r.Context(), input)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			platform.WriteData(w, r, s.service.clock, 201, account)
		})
		r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
			input, problem := platform.DecodeJSON[LoginInput](r)
			if problem != nil {
				s.writeError(w, r, problem)
				return
			}
			email, invalid := NormalizeEmail(input.Email)
			if invalid != nil {
				s.writeError(w, r, invalid)
				return
			}
			buckets := []bucket{{scope: "login-ip:" + remoteIP(r), limit: limits.IPFailures, window: 15 * time.Minute}, {scope: "login-account-ip:" + email + ":" + remoteIP(r), limit: limits.LoginFailures, window: 15 * time.Minute}}
			startedAt := s.service.clock.Now()
			if !s.checkLimit(w, r, buckets) {
				return
			}
			credentials, err := s.Login(r.Context(), input)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			s.release(r.Context(), buckets, startedAt)
			s.WriteCookies(w, credentials)
			platform.WriteData(w, r, s.service.clock, 200, credentials)
		})
		r.Group(func(r chi.Router) {
			r.Use(s.RequireSession)
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					identity, _ := platform.CurrentIdentity(r.Context())
					if s.checkLimit(w, r, []bucket{{scope: "user-api:" + identity.UserID, limit: limits.UserRequests, window: time.Minute}}) {
						next.ServeHTTP(w, r)
					}
				})
			})
			r.Get("/session", func(w http.ResponseWriter, r *http.Request) {
				identity, _ := platform.CurrentIdentity(r.Context())
				var result Credentials
				result.SessionID = identity.SessionID
				err := s.service.pool.QueryRow(r.Context(), `SELECT u.id::text,u.email,u.role,u.email_verified_at IS NOT NULL,u.created_at,s.expires_at FROM users u JOIN auth_sessions s ON s.user_id=u.id WHERE u.id=$1 AND s.id=$2 AND s.revoked_at IS NULL AND s.expires_at>$3 AND u.disabled_at IS NULL`, identity.UserID, identity.SessionID, s.service.clock.Now()).Scan(&result.Account.ID, &result.Account.Email, &result.Account.Role, &result.Account.EmailVerified, &result.Account.CreatedAt, &result.ExpiresAt)
				if errors.Is(err, pgx.ErrNoRows) {
					err = unauthorized()
				}
				if err != nil {
					s.writeError(w, r, err)
					return
				}
				platform.WriteData(w, r, s.service.clock, 200, result)
			})
		})
	})
	return nil
}

func LoadSessionConfig(origin, environment string, lookup func(string) (string, bool)) (SessionConfig, error) {
	value, _ := lookup("AUTH_SIGNING_KEY")
	key, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(key) != 32 {
		return SessionConfig{}, errors.New("AUTH_SIGNING_KEY must encode 32 random bytes in base64")
	}
	insecure, _ := lookup("AUTH_INSECURE_LOCAL_COOKIES")
	if insecure != "" && insecure != "false" && insecure != "true" {
		return SessionConfig{}, errors.New("AUTH_INSECURE_LOCAL_COOKIES must be true or false")
	}
	return SessionConfig{Origin: origin, Environment: environment, SigningKey: key, InsecureLocalCookies: insecure == "true"}, nil
}

func LoadLimits(lookup func(string) (string, bool)) (Limits, error) {
	limits := DefaultLimits()
	for _, item := range []struct {
		name   string
		target *int
	}{
		{"AUTH_LOGIN_FAILURES", &limits.LoginFailures}, {"AUTH_IP_FAILURES", &limits.IPFailures},
		{"AUTH_REGISTRATIONS", &limits.Registrations}, {"AUTH_USER_REQUESTS", &limits.UserRequests},
	} {
		if value, ok := lookup(item.name); ok {
			number, err := strconv.Atoi(value)
			if err != nil || number < 1 || number > 10000 {
				return Limits{}, errors.New("invalid auth rate limit configuration")
			}
			*item.target = number
		}
	}
	if limits.IPFailures < limits.LoginFailures {
		return Limits{}, errors.New("IP allowance must cover account allowance")
	}
	return limits, nil
}
