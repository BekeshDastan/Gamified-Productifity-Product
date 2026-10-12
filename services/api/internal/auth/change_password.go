package auth

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"time"
	"unicode/utf8"
)

type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	Password        string `json:"password"`
}

func wrongCurrentPassword() *platform.APIError {
	return &platform.APIError{Status: 422, Code: "CURRENT_PASSWORD_INVALID", Message: "Проверьте текущий пароль", Fields: map[string]string{"current_password": "Неверный текущий пароль"}}
}
func (s *Sessions) ChangePassword(ctx context.Context, input ChangePasswordInput) error {
	identity, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return unauthorized()
	}
	if input.CurrentPassword == "" || len(input.CurrentPassword) > 512 || !utf8.ValidString(input.CurrentPassword) {
		return wrongCurrentPassword()
	}
	if problem := ValidatePassword(input.Password); problem != nil {
		return problem
	}
	var original string
	if err := s.service.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1 AND disabled_at IS NULL`, identity.UserID).Scan(&original); err != nil {
		return unauthorized()
	}
	valid, err := s.service.passwords.Verify(ctx, input.CurrentPassword, original)
	if err != nil {
		return errors.New("password verification unavailable")
	}
	if !valid {
		return wrongCurrentPassword()
	}
	replacement, err := s.service.passwords.Hash(ctx, input.Password)
	if err != nil {
		return err
	}
	return storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		var current string
		var disabled pgtype.Timestamptz
		if err := tx.QueryRow(ctx, `SELECT password_hash,disabled_at FROM users WHERE id=$1 FOR UPDATE`, identity.UserID).Scan(&current, &disabled); err != nil {
			return errors.New("password account unavailable")
		}
		if disabled.Valid {
			return unauthorized()
		}
		if current != original {
			return wrongCurrentPassword()
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth_sessions WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>$3)`, identity.SessionID, identity.UserID, s.service.clock.Now()).Scan(&active); err != nil {
			return err
		}
		if !active {
			return unauthorized()
		}
		now := s.service.clock.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2,updated_at=$3 WHERE id=$1`, identity.UserID, replacement, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, identity.UserID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE auth_one_time_tokens SET used_at=$2 WHERE user_id=$1 AND kind='reset_password' AND used_at IS NULL`, identity.UserID, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,metadata) VALUES($1,'auth.password_changed','user',$1,'{}')`, identity.UserID)
		return err
	})
}
func (s *Sessions) mountPasswordChange(router chi.Router) {
	router.With(s.Protect).Patch("/api/v1/me/password", func(w http.ResponseWriter, r *http.Request) {
		identity, _ := platform.CurrentIdentity(r.Context())
		if !s.checkLimit(w, r, []bucket{{scope: "password-change-user:" + identity.UserID, limit: s.limits.LoginFailures, window: 15 * time.Minute}, {scope: "password-change-ip:" + remoteIP(r), limit: s.limits.IPFailures, window: 15 * time.Minute}}) {
			return
		}
		input, problem := platform.DecodeJSON[ChangePasswordInput](r)
		if problem != nil {
			s.writeError(w, r, problem)
			return
		}
		if err := s.ChangePassword(r.Context(), input); err != nil {
			s.writeError(w, r, err)
			return
		}
		s.clearCookies(w)
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
}
