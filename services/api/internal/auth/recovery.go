package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const resetPassword = "reset_password"

type ForgotInput struct {
	Email string `json:"email"`
}
type ResetInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}
type resetRequest struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
}
type sealedRequest struct {
	Sealed string `json:"sealed"`
}

func (s *Sessions) resetCipher() cipher.AEAD {
	key, _ := base64.RawURLEncoding.DecodeString(s.mac("reset-request-encryption", "v1"))
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("reset encryption unavailable")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic("reset encryption unavailable")
	}
	return aead
}

// Account lookup happens asynchronously. Known and unknown addresses have the
// same HTTP response and the same database/queue path, without account probing.
func (s *Sessions) ForgotPassword(ctx context.Context, input ForgotInput) error {
	email, invalid := NormalizeEmail(input.Email)
	if invalid != nil {
		return invalid
	}
	payload, _ := json.Marshal(resetRequest{Email: email, ExpiresAt: s.service.clock.Now().Add(30 * time.Minute)})
	aead := s.resetCipher()
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return errors.New("reset request entropy unavailable")
	}
	sealed := aead.Seal(nonce, nonce, payload, []byte("auth.reset_request.v1"))
	body, _ := json.Marshal(sealedRequest{Sealed: base64.RawURLEncoding.EncodeToString(sealed)})
	return storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
			return errors.New("reset request unavailable")
		}
		return (jobs.Store{Pool: s.service.pool, Clock: s.service.clock}).AddOutbox(ctx, tx, jobs.Event{ID: id, Kind: "auth.reset_request", Payload: body})
	})
}

func (s *Sessions) handleResetRequest(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	var payload sealedRequest
	if json.Unmarshal(job.Payload, &payload) != nil {
		return jobs.ErrPermanent
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(payload.Sealed)
	aead := s.resetCipher()
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return jobs.ErrPermanent
	}
	data, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("auth.reset_request.v1"))
	if err != nil {
		return jobs.ErrPermanent
	}
	var request resetRequest
	if json.Unmarshal(data, &request) != nil {
		return jobs.ErrPermanent
	}
	if !request.ExpiresAt.After(s.service.clock.Now()) {
		return nil
	}
	var id string
	var disabled pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT id::text,disabled_at FROM users WHERE email=$1 FOR UPDATE`, request.Email).Scan(&id, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errors.New("reset request account unavailable")
	}
	if disabled.Valid {
		return nil
	}
	return s.issueToken(ctx, tx, id, resetPassword, 30*time.Minute)
}

func (s *Sessions) ResetPassword(ctx context.Context, input ResetInput) error {
	if invalid := ValidatePassword(input.Password); invalid != nil {
		return invalid
	}
	owner, err := s.tokenOwner(ctx, input.Token, resetPassword)
	if err != nil {
		return err
	}
	hash, err := s.service.passwords.Hash(ctx, input.Password)
	if err != nil {
		return err
	}
	return storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		var disabled pgtype.Timestamptz
		if err := tx.QueryRow(ctx, `SELECT disabled_at FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&disabled); err != nil {
			return errors.New("reset account unavailable")
		}
		if disabled.Valid {
			return invalidToken()
		}
		if err := s.consumeToken(ctx, tx, input.Token, resetPassword, owner); err != nil {
			return err
		}
		now := s.service.clock.Now()
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2,updated_at=$3 WHERE id=$1`, owner, hash, now); err != nil {
			return errors.New("password reset storage failed")
		}
		if _, err := tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, owner, now); err != nil {
			return errors.New("reset session invalidation failed")
		}
		if _, err := tx.Exec(ctx, `UPDATE auth_one_time_tokens SET used_at=$2 WHERE user_id=$1 AND kind='reset_password' AND used_at IS NULL`, owner, now); err != nil {
			return errors.New("reset link invalidation failed")
		}
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,metadata) VALUES($1,'auth.password_reset','user',$1,'{}')`, owner)
		return err
	})
}

func (s *Sessions) mountRecovery(r chi.Router) {
	r.Post("/forgot-password", func(w http.ResponseWriter, r *http.Request) {
		input, problem := platform.DecodeJSON[ForgotInput](r)
		if problem != nil {
			s.writeError(w, r, problem)
			return
		}
		email, invalid := NormalizeEmail(input.Email)
		if invalid != nil {
			s.writeError(w, r, invalid)
			return
		}
		if !s.checkLimit(w, r, []bucket{{scope: "forgot-ip:" + remoteIP(r), limit: s.limits.MailRequests, window: time.Hour}, {scope: "forgot-email:" + email, limit: s.limits.MailRequests, window: time.Hour}}) {
			return
		}
		if err := s.ForgotPassword(r.Context(), input); err != nil {
			s.writeError(w, r, err)
			return
		}
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
	r.Post("/reset-password", func(w http.ResponseWriter, r *http.Request) {
		if !s.checkLimit(w, r, []bucket{{scope: "reset-ip:" + remoteIP(r), limit: s.limits.TokenAttempts, window: time.Hour}}) {
			return
		}
		input, problem := platform.DecodeJSON[ResetInput](r)
		if problem != nil {
			s.writeError(w, r, problem)
			return
		}
		if err := s.ResetPassword(r.Context(), input); err != nil {
			s.writeError(w, r, err)
			return
		}
		s.clearCookies(w)
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
}
