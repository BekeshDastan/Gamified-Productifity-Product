package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/push"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"time"
)

type PushDevice struct {
	ID        string     `json:"id"`
	Provider  string     `json:"provider"`
	Version   int64      `json:"version"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}
type PushInput struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
	Confirm  bool   `json:"confirm"`
}

func (s *Service) ConfigurePush(publicKey string, sender push.Sender) {
	s.pushPublicKey = publicKey
	s.pushSender = sender
}
func (s *Service) mountPush(r chi.Router) {
	r.Get("/api/v1/notifications/push", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			rows, e := tx.Query(ctx, `SELECT id::text,endpoint,version,expires_at,revoked_at FROM notification_push_subscriptions WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 20`, owner)
			if e != nil {
				return nil, e
			}
			defer rows.Close()
			items := []PushDevice{}
			for rows.Next() {
				var d PushDevice
				var endpoint string
				if e = rows.Scan(&d.ID, &endpoint, &d.Version, &d.ExpiresAt, &d.RevokedAt); e != nil {
					return nil, e
				}
				u, _ := url.Parse(endpoint)
				d.Provider = u.Hostname()
				items = append(items, d)
			}
			return map[string]any{"items": items, "public_key": s.pushPublicKey, "configured": s.pushPublicKey != ""}, rows.Err()
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/notifications/push", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in PushInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				sub := push.Subscription{Endpoint: in.Endpoint}
				sub.Keys.P256dh = in.P256dh
				sub.Keys.Auth = in.Auth
				if !in.Confirm || push.Validate(sub) != nil {
					return nil, privateapi.Invalid("Подтвердите подключение корректного устройства")
				}
				if s.pushPublicKey == "" {
					return nil, privateapi.Problem(409, "PUSH_UNAVAILABLE", "Push пока не настроен на сервере")
				}
				sum := sha256.Sum256([]byte(in.Endpoint))
				hash := hex.EncodeToString(sum[:])
				now := s.Clock.Now()
				var existing, previous, pub, auth string
				var version int64
				// Serialize a capability across owners without locking another owner's profile.
				if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, hash); e != nil {
					return nil, e
				}
				e := tx.QueryRow(ctx, `SELECT id::text,owner_id::text,p256dh,auth,version FROM notification_push_subscriptions WHERE endpoint_hash=$1 AND revoked_at IS NULL FOR UPDATE`, hash).Scan(&existing, &previous, &pub, &auth, &version)
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return nil, e
				}
				if e == nil {
					if pub != in.P256dh || auth != in.Auth {
						return nil, privateapi.Problem(409, "DEVICE_CHANGED", "Переподключите устройство в браузере")
					}
					if previous == owner {
						if versionError := privateapi.MatchVersion(r, version); versionError != nil {
							return nil, versionError
						}
						var d PushDevice
						d.ID = existing
						u, _ := url.Parse(in.Endpoint)
						d.Provider = u.Hostname()
						e = tx.QueryRow(ctx, `UPDATE notification_push_subscriptions SET expires_at=$3,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2 RETURNING version,expires_at,revoked_at`, owner, existing, now.AddDate(0, 3, 0), now).Scan(&d.Version, &d.ExpiresAt, &d.RevokedAt)
						return d, e
					}
					if _, e = tx.Exec(ctx, `UPDATE notification_push_subscriptions SET revoked_at=$2,version=version+1,updated_at=$2 WHERE id=$1`, existing, now); e != nil {
						return nil, e
					}
				}
				if versionError := privateapi.MatchVersion(r, 0); versionError != nil {
					return nil, versionError
				}
				var count int
				if e = tx.QueryRow(ctx, `SELECT count(*) FROM notification_push_subscriptions WHERE owner_id=$1 AND revoked_at IS NULL AND expires_at>$2`, owner, now).Scan(&count); e != nil {
					return nil, e
				}
				if count >= 10 {
					return nil, privateapi.Invalid("Отключите одно из десяти устройств")
				}
				var d PushDevice
				u, _ := url.Parse(in.Endpoint)
				d.Provider = u.Hostname()
				e = tx.QueryRow(ctx, `INSERT INTO notification_push_subscriptions(owner_id,endpoint,endpoint_hash,p256dh,auth,expires_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7) RETURNING id::text,version,expires_at,revoked_at`, owner, in.Endpoint, hash, in.P256dh, in.Auth, now.AddDate(0, 3, 0), now).Scan(&d.ID, &d.Version, &d.ExpiresAt, &d.RevokedAt)
				return d, e
			})
		})
	})
	r.Post("/api/v1/notifications/push/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in struct {
			Confirm bool `json:"confirm"`
		}) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				id := chi.URLParam(r, "id")
				if !privateapi.UUID(id) {
					return nil, privateapi.Problem(404, "NOT_FOUND", "Устройство не найдено")
				}
				var version int64
				e := tx.QueryRow(ctx, `SELECT version FROM notification_push_subscriptions WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&version)
				if errors.Is(e, pgx.ErrNoRows) {
					return nil, privateapi.Problem(404, "NOT_FOUND", "Устройство не найдено")
				}
				if e != nil {
					return nil, e
				}
				if versionError := privateapi.MatchVersion(r, version); versionError != nil {
					return nil, versionError
				}
				if !in.Confirm {
					return nil, privateapi.Invalid("Подтвердите отключение")
				}
				_, e = tx.Exec(ctx, `UPDATE notification_push_subscriptions SET revoked_at=COALESCE(revoked_at,$3),version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2`, owner, id, s.Clock.Now())
				return map[string]bool{"revoked": true}, e
			})
		})
	})
}
func (s *Service) queuePush(ctx context.Context, tx pgx.Tx, owner string, n Notification) error {
	p, e := s.preferences(ctx, tx, owner)
	if e != nil || !p.Channel(n.Kind).Push {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM notification_push_subscriptions WHERE owner_id=$1 AND revoked_at IS NULL AND expires_at>$2 ORDER BY id LIMIT 10`, owner, s.Clock.Now())
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	expires, urgent, e := s.deliveryExpiry(ctx, tx, owner, n)
	if e != nil {
		return e
	}
	used, oldest, e := s.attemptUsage(ctx, tx, owner, "push")
	if e != nil {
		return e
	}
	next, allowed, e := PolicyTime(s.Clock.Now(), expires, p, urgent, used, oldest)
	if e != nil {
		return e
	}
	for _, id := range ids {
		state, code := "queued", ""
		if !allowed {
			state, code = "cancelled", "POLICY_OR_EXPIRY"
		}
		d, e := scanDelivery(tx.QueryRow(ctx, `INSERT INTO notification_deliveries(owner_id,notification_id,channel,subscription_id,state,next_run_at,expires_at,last_error_code,created_at,updated_at) VALUES($1,$2,'push',$3,$4,$5,$6,$7,$8,$8) ON CONFLICT DO NOTHING RETURNING `+deliveryColumns, owner, n.ID, id, state, next, expires, code, s.Clock.Now()))
		if errors.Is(e, pgx.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		if state == "queued" {
			if e = s.enqueueDelivery(ctx, tx, owner, d); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) pushSubscription(ctx context.Context, tx pgx.Tx, owner string, d Delivery) (push.Subscription, error) {
	var sub push.Subscription
	if d.SubscriptionID == nil {
		return sub, push.ErrGone
	}
	e := tx.QueryRow(ctx, `SELECT endpoint,p256dh,auth FROM notification_push_subscriptions WHERE owner_id=$1 AND id=$2 AND revoked_at IS NULL AND expires_at>$3`, owner, *d.SubscriptionID, s.Clock.Now()).Scan(&sub.Endpoint, &sub.Keys.P256dh, &sub.Keys.Auth)
	if errors.Is(e, pgx.ErrNoRows) {
		e = push.ErrGone
	}
	return sub, e
}
func (s *Service) sendPush(ctx context.Context, sub push.Subscription, n Notification) error {
	if s.pushSender == nil {
		return push.ErrUnavailable
	}
	raw, _ := json.Marshal(map[string]string{"title": n.Title, "body": n.Message, "url": "/app/notifications"})
	return s.pushSender.Send(ctx, sub, raw)
}
func classifyPush(e error) (string, string, bool) {
	switch {
	case e == nil:
		return "provider_accepted", "", false
	case errors.Is(e, push.ErrUnknown):
		return "delivery_unknown", "PUSH_OUTCOME_UNKNOWN", false
	case errors.Is(e, push.ErrGone):
		return "failed", "PUSH_REVOKED", false
	case errors.Is(e, push.ErrRetry):
		return "retry_wait", "PUSH_TEMPORARY", true
	case errors.Is(e, push.ErrUnavailable):
		return "failed", "ADAPTER_UNAVAILABLE", false
	default:
		return "failed", "PUSH_REJECTED", false
	}
}
