package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const verifyEmail = "verify_email"

type TokenInput struct {
	Token string `json:"token"`
}
type tokenMail struct {
	ID string `json:"token_id"`
}

// The opaque token is a domain-separated PRF of a random UUID. Only its SHA256
// is persisted. Outbox/worker payloads contain an ID, never a raw link or token.
func (s *Sessions) emailToken(kind, id string) string { return s.mac("one-time-token:"+kind, id) }

func (s *Sessions) issueToken(ctx context.Context, tx pgx.Tx, userID, kind string, lifetime time.Duration) error {
	var id string
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		return errors.New("email token entropy unavailable")
	}
	digest := sha256.Sum256([]byte(s.emailToken(kind, id)))
	now := s.service.clock.Now()
	// A new request invalidates earlier links of the same purpose.
	if _, err := tx.Exec(ctx, `UPDATE auth_one_time_tokens SET used_at=$3 WHERE user_id=$1 AND kind=$2 AND used_at IS NULL`, userID, kind, now); err != nil {
		return errors.New("email token invalidation failed")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO auth_one_time_tokens(id,user_id,kind,token_hash,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, id, userID, kind, digest[:], now, now.Add(lifetime)); err != nil {
		return errors.New("email token storage failed")
	}
	payload, _ := json.Marshal(tokenMail{ID: id})
	return (jobs.Store{Pool: s.service.pool, Clock: s.service.clock}).AddOutbox(ctx, tx, jobs.Event{ID: id, Kind: "auth.mail." + kind, OwnerID: &userID, Payload: payload})
}

func (s *Sessions) Register(ctx context.Context, input RegisterInput) (Account, error) {
	return s.service.register(ctx, input, func(tx pgx.Tx, account Account) error {
		return s.issueToken(ctx, tx, account.ID, verifyEmail, 24*time.Hour)
	})
}

func invalidToken() *platform.APIError {
	return &platform.APIError{Status: 422, Code: "TOKEN_INVALID", Message: "Ссылка недействительна или её срок истёк. Запросите новую"}
}

func (s *Sessions) tokenOwner(ctx context.Context, raw, kind string) (string, error) {
	if len(raw) != 43 {
		return "", invalidToken()
	}
	digest := sha256.Sum256([]byte(raw))
	var owner string
	err := s.service.pool.QueryRow(ctx, `SELECT user_id::text FROM auth_one_time_tokens WHERE token_hash=$1 AND kind=$2`, digest[:], kind).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", invalidToken()
	}
	if err != nil {
		return "", errors.New("email token unavailable")
	}
	return owner, nil
}

func (s *Sessions) VerifyEmail(ctx context.Context, raw string) error {
	owner, err := s.tokenOwner(ctx, raw, verifyEmail)
	if err != nil {
		return err
	}
	return storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		var disabled pgtype.Timestamptz
		if err := tx.QueryRow(ctx, `SELECT disabled_at FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&disabled); err != nil {
			return errors.New("verification account unavailable")
		}
		if disabled.Valid {
			return invalidToken()
		}
		if err := s.consumeToken(ctx, tx, raw, verifyEmail, owner); err != nil {
			return err
		}
		now := s.service.clock.Now()
		if _, err := tx.Exec(ctx, `UPDATE users SET email_verified_at=coalesce(email_verified_at,$2),updated_at=$2 WHERE id=$1`, owner, now); err != nil {
			return errors.New("verification storage failed")
		}
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,metadata) VALUES($1,'auth.email_verified','user',$1,'{}')`, owner)
		return err
	})
}

func (s *Sessions) consumeToken(ctx context.Context, tx pgx.Tx, raw, kind, owner string) error {
	digest := sha256.Sum256([]byte(raw))
	tag, err := tx.Exec(ctx, `UPDATE auth_one_time_tokens SET used_at=$4 WHERE token_hash=$1 AND kind=$2 AND user_id=$3 AND used_at IS NULL AND expires_at>$4`, digest[:], kind, owner, s.service.clock.Now())
	if err != nil {
		return errors.New("email token consumption failed")
	}
	if tag.RowsAffected() != 1 {
		return invalidToken()
	}
	return nil
}

func (s *Sessions) ResendVerification(ctx context.Context, identity platform.Identity) error {
	return storage.WithinTx(ctx, s.service.pool, func(tx pgx.Tx) error {
		var verified, disabled pgtype.Timestamptz
		if err := tx.QueryRow(ctx, `SELECT email_verified_at,disabled_at FROM users WHERE id=$1 FOR UPDATE`, identity.UserID).Scan(&verified, &disabled); err != nil {
			return errors.New("verification account unavailable")
		}
		if disabled.Valid {
			return unauthorized()
		}
		if verified.Valid {
			return nil
		}
		return s.issueToken(ctx, tx, identity.UserID, verifyEmail, 24*time.Hour)
	})
}

// RequireVerified is used by publication/invitation routes added in later phases.
func (s *Sessions) RequireVerified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := platform.CurrentIdentity(r.Context())
		if !ok {
			s.writeError(w, r, unauthorized())
			return
		}
		var verified bool
		err := s.service.pool.QueryRow(r.Context(), `SELECT email_verified_at IS NOT NULL FROM users WHERE id=$1 AND disabled_at IS NULL`, identity.UserID).Scan(&verified)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		if !verified {
			s.writeError(w, r, &platform.APIError{Status: 403, Code: "EMAIL_UNVERIFIED", Message: "Подтвердите email"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Sessions) MailHandlers(mailer mail.Mailer) map[string]jobs.Handler {
	return map[string]jobs.Handler{"auth.mail.verify_email": s.mailHandler(mailer, verifyEmail, "Подтвердите почту ASCEND", "verify-email")}
}

func (s *Sessions) mailHandler(mailer mail.Mailer, kind, subject, page string) jobs.Handler {
	return func(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
		var payload tokenMail
		if json.Unmarshal(job.Payload, &payload) != nil || job.OwnerID == nil {
			return jobs.ErrPermanent
		}
		var email string
		var hash []byte
		var used, disabled pgtype.Timestamptz
		var expires time.Time
		err := tx.QueryRow(ctx, `SELECT u.email,t.token_hash,t.used_at,t.expires_at,u.disabled_at FROM auth_one_time_tokens t JOIN users u ON u.id=t.user_id WHERE t.id=$1 AND t.user_id=$2 AND t.kind=$3`, payload.ID, *job.OwnerID, kind).Scan(&email, &hash, &used, &expires, &disabled)
		if errors.Is(err, pgx.ErrNoRows) {
			return jobs.ErrPermanent
		}
		if err != nil {
			return errors.New("email delivery lookup failed")
		}
		if used.Valid || disabled.Valid || !expires.After(s.service.clock.Now()) {
			return nil
		}
		token := s.emailToken(kind, payload.ID)
		digest := sha256.Sum256([]byte(token))
		if !hmac.Equal(digest[:], hash) {
			return jobs.ErrPermanent
		}
		// Fragments are never sent in HTTP requests or Referer. The form consumes it
		// via POST; opening a link never changes account state.
		link := s.config.Origin + "/auth/" + page + "#token=" + token
		if err := mailer.Send(ctx, mail.Message{To: email, Subject: subject, Text: "Откройте ссылку и подтвердите действие:\n\n" + link + "\n\nЕсли вы не запрашивали это письмо, просто проигнорируйте его."}); err != nil {
			return errors.New("email delivery failed")
		}
		return nil
	}
}

func (s *Sessions) mountVerification(r chi.Router) {
	r.Post("/verify-email", func(w http.ResponseWriter, r *http.Request) {
		if !s.checkLimit(w, r, []bucket{{scope: "verify-ip:" + remoteIP(r), limit: 30, window: time.Hour}}) {
			return
		}
		input, problem := platform.DecodeJSON[TokenInput](r)
		if problem != nil {
			s.writeError(w, r, problem)
			return
		}
		if err := s.VerifyEmail(r.Context(), input.Token); err != nil {
			s.writeError(w, r, err)
			return
		}
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
}

func (s *Sessions) mountResend(r chi.Router) {
	r.Post("/resend-verification", func(w http.ResponseWriter, r *http.Request) {
		if _, problem := platform.DecodeJSON[struct{}](r); problem != nil {
			s.writeError(w, r, problem)
			return
		}
		identity, _ := platform.CurrentIdentity(r.Context())
		if !s.checkLimit(w, r, []bucket{{scope: "resend-user:" + identity.UserID, limit: 3, window: time.Hour}, {scope: "resend-ip:" + remoteIP(r), limit: 3, window: time.Hour}}) {
			return
		}
		if err := s.ResendVerification(r.Context(), identity); err != nil {
			s.writeError(w, r, err)
			return
		}
		platform.WriteData(w, r, s.service.clock, 200, struct {
			OK bool `json:"ok"`
		}{true})
	})
}
