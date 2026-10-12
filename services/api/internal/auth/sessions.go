package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage/sqlgen"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const accessLifetime = 15 * time.Minute

type SessionConfig struct {
	Origin     string
	SigningKey []byte
	// InsecureLocalCookies is only permitted on loopback in development/test.
	InsecureLocalCookies bool
	Environment          string
}

type Sessions struct {
	service   *Service
	config    SessionConfig
	dummyHash string
}

type LoginInput struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me"`
}

// Credentials never appear in JSON responses or structured logs.
type Credentials struct {
	Account    Account   `json:"account"`
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	Access     string    `json:"-"`
	Refresh    string    `json:"-"`
	RememberMe bool      `json:"-"`
}

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func NewSessions(ctx context.Context, service *Service, config SessionConfig) (*Sessions, error) {
	if config.Environment != "production" && config.Environment != "development" && config.Environment != "test" {
		return nil, errors.New("invalid auth environment")
	}
	origin, err := url.Parse(config.Origin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" || (origin.Scheme != "https" && origin.Scheme != "http") || len(config.SigningKey) != 32 {
		return nil, errors.New("invalid auth configuration")
	}
	if config.InsecureLocalCookies {
		host := origin.Hostname()
		if (config.Environment != "development" && config.Environment != "test") || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
			return nil, errors.New("insecure cookies require explicit local development")
		}
	} else if origin.Scheme != "https" {
		return nil, errors.New("secure cookies require HTTPS")
	}
	config.SigningKey = append([]byte(nil), config.SigningKey...)
	// Equal-cost verification for an unknown email. Never a login credential.
	dummy, err := service.passwords.Hash(ctx, randomOpaque())
	if err != nil {
		return nil, err
	}
	return &Sessions{service: service, config: config, dummyHash: dummy}, nil
}

func randomOpaque() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		panic("secure randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func unauthorized() *platform.APIError {
	return &platform.APIError{Status: 401, Code: "UNAUTHORIZED", Message: "Войдите в аккаунт"}
}

// Login runs behind the HTTP boundary's pre-auth CSRF and attempt limits.
func (s *Sessions) Login(ctx context.Context, input LoginInput) (Credentials, error) {
	email, invalid := NormalizeEmail(input.Email)
	if invalid != nil {
		return Credentials{}, invalid
	}
	user, err := sqlgen.New(s.service.pool).FindUserCredentials(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, errors.New("login storage failed")
	}
	found := err == nil
	hash := s.dummyHash
	if found {
		hash = user.PasswordHash
	}
	valid, verifyErr := s.service.passwords.Verify(ctx, input.Password, hash)
	if verifyErr != nil {
		if ctx.Err() != nil {
			return Credentials{}, ctx.Err()
		}
		return Credentials{}, unauthorized()
	}
	if !valid || !found || user.DisabledAt.Valid {
		return Credentials{}, unauthorized()
	}
	var result Credentials
	err = storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		// Reset/disable must lock this same row before revoking sessions. Recheck
		// after expensive hashing so a concurrent password reset cannot be bypassed.
		var currentHash string
		var disabled pgtype.Timestamptz
		var account Account
		var verified pgtype.Timestamptz
		err := tx.QueryRow(ctx, `SELECT id::text,email,role,email_verified_at,created_at,password_hash,disabled_at FROM users WHERE id=$1 FOR UPDATE`, user.ID).Scan(&account.ID, &account.Email, &account.Role, &verified, &account.CreatedAt, &currentHash, &disabled)
		if err != nil {
			return errors.New("login account unavailable")
		}
		if currentHash != hash || disabled.Valid {
			return unauthorized()
		}
		account.EmailVerified = verified.Valid
		account.CreatedAt = account.CreatedAt.UTC()
		now := s.service.clock.Now().UTC()
		lifetime := 7 * 24 * time.Hour
		if input.RememberMe {
			lifetime = 30 * 24 * time.Hour
		}
		csrf := make([]byte, 32)
		if _, err := rand.Read(csrf); err != nil {
			return errors.New("session entropy unavailable")
		}
		result = Credentials{Account: account, ExpiresAt: now.Add(lifetime), Refresh: randomOpaque(), RememberMe: input.RememberMe}
		err = tx.QueryRow(ctx, `INSERT INTO auth_sessions(user_id,csrf_secret,remember_me,created_at,expires_at) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, account.ID, csrf, input.RememberMe, now, result.ExpiresAt).Scan(&result.SessionID)
		if err != nil {
			return errors.New("session creation failed")
		}
		digest := sha256.Sum256([]byte(result.Refresh))
		if _, err = tx.Exec(ctx, `INSERT INTO auth_refresh_tokens(token_hash,session_id,created_at) VALUES($1,$2,$3)`, digest[:], result.SessionID, now); err != nil {
			return errors.New("session token storage failed")
		}
		result.Access, err = s.signAccess(account.ID, result.SessionID, now)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,metadata) VALUES($1,'auth.logged_in','session',$2,'{}')`, account.ID, result.SessionID)
		return err
	})
	if err != nil {
		return Credentials{}, err
	}
	return result, nil
}

func (s *Sessions) signAccess(userID, sessionID string, now time.Time) (string, error) {
	claims := accessClaims{SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{
		Subject: userID, Issuer: s.config.Origin, Audience: jwt.ClaimStrings{"ascend-web"},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(accessLifetime)), ID: randomOpaque(),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.config.SigningKey)
}

func (s *Sessions) Authenticate(ctx context.Context, raw string) (platform.Identity, error) {
	if len(raw) == 0 || len(raw) > 2048 {
		return platform.Identity{}, unauthorized()
	}
	claims := new(accessClaims)
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) { return s.config.SigningKey, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(s.config.Origin), jwt.WithAudience("ascend-web"), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(s.service.clock.Now), jwt.WithStrictDecoding())
	var userID, sessionID pgtype.UUID
	if err != nil || !token.Valid || claims.IssuedAt == nil || claims.ExpiresAt == nil || !claims.ExpiresAt.After(claims.IssuedAt.Time) || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > accessLifetime || len(claims.ID) != 43 || userID.Scan(claims.Subject) != nil || sessionID.Scan(claims.SessionID) != nil || !userID.Valid || !sessionID.Valid {
		return platform.Identity{}, unauthorized()
	}
	identity := platform.Identity{UserID: claims.Subject, SessionID: claims.SessionID}
	err = s.service.pool.QueryRow(ctx, `SELECT u.role FROM auth_sessions s JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.user_id=$2 AND s.revoked_at IS NULL AND s.expires_at>$3 AND u.disabled_at IS NULL`, identity.SessionID, identity.UserID, s.service.clock.Now()).Scan(&identity.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.Identity{}, unauthorized()
	}
	if err != nil {
		return platform.Identity{}, errors.New("session verification failed")
	}
	return identity, nil
}

func (s *Sessions) cookieName(kind string) string {
	if s.config.InsecureLocalCookies {
		return "ascend_" + kind
	}
	return "__Host-ascend_" + kind
}

func (s *Sessions) WriteCookies(w http.ResponseWriter, credentials Credentials) {
	now := s.service.clock.Now()
	accessExpires := now.Add(accessLifetime)
	for _, item := range []struct {
		kind, value string
		expires     time.Time
	}{{"access", credentials.Access, accessExpires}, {"refresh", credentials.Refresh, credentials.ExpiresAt}} {
		cookie := &http.Cookie{Name: s.cookieName(item.kind), Value: item.value, Path: "/", HttpOnly: true, Secure: !s.config.InsecureLocalCookies, SameSite: http.SameSiteLaxMode}
		if credentials.RememberMe {
			cookie.Expires = item.expires
			cookie.MaxAge = max(1, int(item.expires.Sub(now).Seconds()))
		}
		http.SetCookie(w, cookie)
	}
}
