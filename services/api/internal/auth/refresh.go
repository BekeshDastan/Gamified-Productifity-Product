package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/http"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Sessions) Refresh(ctx context.Context, raw string) (Credentials, error) {
	if len(raw) != 43 {
		return Credentials{}, unauthorized()
	}
	digest := sha256.Sum256([]byte(raw))
	var userID string
	err := s.service.pool.QueryRow(ctx, `SELECT s.user_id::text FROM auth_refresh_tokens t JOIN auth_sessions s ON s.id=t.session_id WHERE t.token_hash=$1`, digest[:]).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, unauthorized()
	}
	if err != nil {
		return Credentials{}, errors.New("refresh unavailable")
	}
	var result Credentials
	var problem *platform.APIError
	err = storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		var disabled, verified pgtype.Timestamptz
		if err := tx.QueryRow(ctx, `SELECT id::text,email,role,email_verified_at,created_at,disabled_at FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&result.Account.ID, &result.Account.Email, &result.Account.Role, &verified, &result.Account.CreatedAt, &disabled); err != nil {
			return errors.New("refresh account unavailable")
		}
		var used, revoked pgtype.Timestamptz
		err := tx.QueryRow(ctx, `SELECT s.id::text,s.expires_at,s.remember_me,s.revoked_at,t.used_at FROM auth_refresh_tokens t JOIN auth_sessions s ON s.id=t.session_id WHERE t.token_hash=$1 AND s.user_id=$2 FOR UPDATE OF s,t`, digest[:], userID).Scan(&result.SessionID, &result.ExpiresAt, &result.RememberMe, &revoked, &used)
		if errors.Is(err, pgx.ErrNoRows) {
			return unauthorized()
		}
		if err != nil {
			return errors.New("refresh session unavailable")
		}
		now := s.service.clock.Now().UTC()
		if disabled.Valid || revoked.Valid || !result.ExpiresAt.After(now) {
			return unauthorized()
		}
		if used.Valid {
			age := now.Sub(used.Time)
			if age >= 0 && age <= 5*time.Second {
				return &platform.APIError{Status: 409, Code: "REFRESH_IN_PROGRESS", Message: "Вход уже обновляется. Повторите исходный запрос"}
			}
			// Persist revocation before returning the business error. Returning an error
			// from WithinTx would roll back the detected-reuse response.
			if _, err := tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=$2 WHERE id=$1`, result.SessionID, now); err != nil {
				return errors.New("refresh revocation failed")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,metadata) VALUES($1,'auth.refresh_reused','session',$2,'{}')`, userID, result.SessionID); err != nil {
				return errors.New("refresh reuse audit failed")
			}
			problem = unauthorized()
			return nil
		}
		result.Account.EmailVerified = verified.Valid
		result.Account.CreatedAt = result.Account.CreatedAt.UTC()
		result.Refresh = randomOpaque()
		next := sha256.Sum256([]byte(result.Refresh))
		if _, err := tx.Exec(ctx, `UPDATE auth_refresh_tokens SET used_at=$2 WHERE token_hash=$1`, digest[:], now); err != nil {
			return errors.New("refresh consumption failed")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO auth_refresh_tokens(token_hash,session_id,created_at) VALUES($1,$2,$3)`, next[:], result.SessionID, now); err != nil {
			return errors.New("refresh rotation failed")
		}
		result.Access, err = s.signAccess(userID, result.SessionID, now)
		return err
	})
	if err != nil {
		return Credentials{}, err
	}
	if problem != nil {
		return Credentials{}, problem
	}
	return result, nil
}

type SessionInfo struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	RememberMe bool      `json:"remember_me"`
	Current    bool      `json:"current"`
}

func (s *Sessions) ListSessions(ctx context.Context, identity platform.Identity) ([]SessionInfo, error) {
	rows, err := s.service.pool.Query(ctx, `SELECT id::text,created_at,expires_at,remember_me FROM auth_sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>$2 ORDER BY created_at DESC,id DESC`, identity.UserID, s.service.clock.Now())
	if err != nil {
		return nil, errors.New("sessions unavailable")
	}
	defer rows.Close()
	result := []SessionInfo{}
	for rows.Next() {
		var item SessionInfo
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.ExpiresAt, &item.RememberMe); err != nil {
			return nil, errors.New("session list unavailable")
		}
		item.Current = item.ID == identity.SessionID
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Sessions) Revoke(ctx context.Context, identity platform.Identity, target string, all bool) error {
	var id pgtype.UUID
	if !all && (id.Scan(target) != nil || !id.Valid) {
		return &platform.APIError{Status: 404, Code: "NOT_FOUND", Message: "Не найдено"}
	}
	return storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1 FOR UPDATE`, identity.UserID).Scan(&locked); err != nil {
			return unauthorized()
		}
		if !all {
			var owner string
			if err := tx.QueryRow(ctx, `SELECT user_id::text FROM auth_sessions WHERE id=$1`, target).Scan(&owner); err != nil || owner != identity.UserID {
				return &platform.APIError{Status: 404, Code: "NOT_FOUND", Message: "Не найдено"}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=$3 WHERE user_id=$1 AND ($2::uuid IS NULL OR id=$2) AND revoked_at IS NULL`, identity.UserID, id, s.service.clock.Now()); err != nil {
			return errors.New("session revocation failed")
		}
		entity := target
		action := "auth.session_revoked"
		if all {
			entity = identity.UserID
			action = "auth.logged_out_all"
		}
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,metadata) VALUES($1,$2,'session',$3,'{}')`, identity.UserID, action, entity)
		return err
	})
}

func (s *Sessions) clearCookies(w http.ResponseWriter) {
	for _, kind := range []string{"access", "refresh", "preauth"} {
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(kind), Value: "", Path: "/", HttpOnly: true, Secure: !s.config.InsecureLocalCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
}

// Refresh and logout also work after access expiry, but require session-bound
// CSRF; a pre-auth context can never authorize either operation.
func (s *Sessions) refreshIdentity(w http.ResponseWriter, r *http.Request) (platform.Identity, string, bool) {
	expected, ok, err := s.sessionCSRF(r)
	if err != nil {
		s.writeError(w, r, err)
		return platform.Identity{}, "", false
	}
	if !ok {
		s.clearCookies(w)
		s.writeError(w, r, unauthorized())
		return platform.Identity{}, "", false
	}
	if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-CSRF-Token"))) {
		s.writeError(w, r, csrfRejected())
		return platform.Identity{}, "", false
	}
	cookie, _ := r.Cookie(s.cookieName("refresh"))
	digest := sha256.Sum256([]byte(cookie.Value))
	var identity platform.Identity
	err = s.service.pool.QueryRow(r.Context(), `SELECT s.user_id::text,s.id::text,u.role FROM auth_refresh_tokens t JOIN auth_sessions s ON s.id=t.session_id JOIN users u ON u.id=s.user_id WHERE t.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>$2 AND u.disabled_at IS NULL`, digest[:], s.service.clock.Now()).Scan(&identity.UserID, &identity.SessionID, &identity.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		err = unauthorized()
	}
	if err != nil {
		s.writeError(w, r, err)
		return platform.Identity{}, "", false
	}
	return identity, cookie.Value, true
}

func (s *Sessions) mountRefresh(r chi.Router) {
	r.Post("/refresh", func(w http.ResponseWriter, r *http.Request) {
		if _, problem := platform.DecodeJSON[struct{}](r); problem != nil {
			s.writeError(w, r, problem)
			return
		}
		identity, raw, ok := s.refreshIdentity(w, r)
		if !ok {
			return
		}
		if !s.userAllowance(w, r, identity) {
			return
		}
		credentials, err := s.Refresh(r.Context(), raw)
		if err != nil {
			var problem *platform.APIError
			if errors.As(err, &problem) && problem.Status == 401 {
				s.clearCookies(w)
			}
			s.writeError(w, r, err)
			return
		}
		s.WriteCookies(w, credentials)
		platform.WriteData(w, r, s.service.clock, 200, credentials)
	})
	r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
		if _, problem := platform.DecodeJSON[struct{}](r); problem != nil {
			s.writeError(w, r, problem)
			return
		}
		identity, _, ok := s.refreshIdentity(w, r)
		if !ok {
			return
		}
		if err := s.Revoke(r.Context(), identity, identity.SessionID, false); err != nil {
			s.writeError(w, r, err)
			return
		}
		s.clearCookies(w)
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
}

func (s *Sessions) mountSessionManagement(r chi.Router) {
	r.Get("/sessions", func(w http.ResponseWriter, r *http.Request) {
		identity, _ := platform.CurrentIdentity(r.Context())
		sessions, err := s.ListSessions(r.Context(), identity)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		platform.WriteData(w, r, s.service.clock, 200, sessions)
	})
	r.Post("/logout-all", func(w http.ResponseWriter, r *http.Request) {
		if _, problem := platform.DecodeJSON[struct{}](r); problem != nil {
			s.writeError(w, r, problem)
			return
		}
		identity, _ := platform.CurrentIdentity(r.Context())
		if err := s.Revoke(r.Context(), identity, "", true); err != nil {
			s.writeError(w, r, err)
			return
		}
		s.clearCookies(w)
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
	r.Delete("/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, problem := platform.DecodeJSON[struct{}](r); problem != nil {
			s.writeError(w, r, problem)
			return
		}
		identity, _ := platform.CurrentIdentity(r.Context())
		target := chi.URLParam(r, "id")
		if err := s.Revoke(r.Context(), identity, target, false); err != nil {
			s.writeError(w, r, err)
			return
		}
		if target == identity.SessionID {
			s.clearCookies(w)
		}
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
}
