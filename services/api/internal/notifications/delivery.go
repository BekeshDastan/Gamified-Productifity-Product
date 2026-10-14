package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/push"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/textproto"
	"time"
)

type Delivery struct {
	ID                 string     `json:"id"`
	NotificationID     string     `json:"notification_id"`
	Channel            string     `json:"channel"`
	SubscriptionID     *string    `json:"subscription_id"`
	State              string     `json:"state"`
	Version            int64      `json:"version"`
	Attempts           int        `json:"attempts"`
	AutoAttempts       int        `json:"auto_attempts"`
	NextRunAt          time.Time  `json:"next_run_at"`
	ExpiresAt          time.Time  `json:"expires_at"`
	LastAttemptID      *string    `json:"last_attempt_id"`
	LastErrorCode      string     `json:"last_error_code"`
	ProviderAcceptedAt *time.Time `json:"provider_accepted_at"`
	UnknownAt          *time.Time `json:"unknown_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

const deliveryColumns = `id::text,notification_id::text,channel,subscription_id::text,state,version,attempts,auto_attempts,next_run_at,expires_at,last_attempt_id::text,last_error_code,provider_accepted_at,unknown_at,created_at,updated_at`

func scanDelivery(row pgx.Row) (Delivery, error) {
	var d Delivery
	e := row.Scan(&d.ID, &d.NotificationID, &d.Channel, &d.SubscriptionID, &d.State, &d.Version, &d.Attempts, &d.AutoAttempts, &d.NextRunAt, &d.ExpiresAt, &d.LastAttemptID, &d.LastErrorCode, &d.ProviderAcceptedAt, &d.UnknownAt, &d.CreatedAt, &d.UpdatedAt)
	return d, e
}

type deliveryJob struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

func (s *Service) delivery(ctx context.Context, tx pgx.Tx, owner, id string) (Delivery, error) {
	if !privateapi.UUID(id) {
		return Delivery{}, privateapi.Problem(404, "NOT_FOUND", "Отправка не найдена")
	}
	d, e := scanDelivery(tx.QueryRow(ctx, `SELECT `+deliveryColumns+` FROM notification_deliveries WHERE owner_id=$1 AND id=$2`, owner, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = privateapi.Problem(404, "NOT_FOUND", "Отправка не найдена")
	}
	return d, e
}
func (s *Service) enqueueDelivery(ctx context.Context, tx pgx.Tx, owner string, d Delivery) error {
	body, _ := json.Marshal(deliveryJob{d.ID, d.Version})
	due, created := s.jobTimes(d.NextRunAt)
	_, e := tx.Exec(ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'notifications.delivery',$2,$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING`, owner, body, fmt.Sprintf("notification-delivery/%s/%s/%d", owner, d.ID, d.Version), due, created)
	return e
}
func (s *Service) attemptUsage(ctx context.Context, tx pgx.Tx, owner, channel string) (int, *time.Time, error) {
	var used int
	var oldest *time.Time
	e := tx.QueryRow(ctx, `SELECT count(*),min(started_at) FROM notification_delivery_attempts WHERE owner_id=$1 AND channel=$2 AND started_at>$3 AND state IN('sending','provider_accepted','delivery_unknown')`, owner, channel, s.Clock.Now().Add(-24*time.Hour)).Scan(&used, &oldest)
	return used, oldest, e
}
func (s *Service) deliveryExpiry(ctx context.Context, tx pgx.Tx, owner string, n Notification) (time.Time, bool, error) {
	expires := n.CreatedAt.Add(7 * 24 * time.Hour)
	urgent := false
	if n.ReminderID != nil {
		r, e := s.reminder(ctx, tx, owner, *n.ReminderID)
		if e != nil {
			return expires, false, e
		}
		expires = r.ExpiresAt
		urgent = r.SourceKind == "task" && r.UrgentOverride
	}
	return expires, urgent, nil
}
func (s *Service) queueDeliveries(ctx context.Context, tx pgx.Tx, owner string, n Notification) error {
	if e := s.queueEmail(ctx, tx, owner, n); e != nil {
		return e
	}
	return s.queuePush(ctx, tx, owner, n)
}
func (s *Service) queueEmail(ctx context.Context, tx pgx.Tx, owner string, n Notification) error {
	p, e := s.preferences(ctx, tx, owner)
	if e != nil {
		return e
	}
	policy := p.Channel(n.Kind)
	if !policy.Email {
		return nil
	}
	now := s.Clock.Now()
	expires, urgent, e := s.deliveryExpiry(ctx, tx, owner, n)
	if e != nil {
		return e
	}
	used, oldest, e := s.attemptUsage(ctx, tx, owner, "email")
	if e != nil {
		return e
	}
	next, allowed, e := PolicyTime(now, expires, p, urgent, used, oldest)
	if e != nil {
		return e
	}
	state, code := "queued", ""
	if !allowed {
		state, code = "cancelled", "POLICY_OR_EXPIRY"
	}
	var verified bool
	if e = tx.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL AND disabled_at IS NULL FROM users WHERE id=$1`, owner).Scan(&verified); e != nil {
		return e
	}
	if !verified {
		state, code = "cancelled", "EMAIL_UNVERIFIED"
	}
	if len(s.unsubscribeKey) != 32 {
		state, code = "failed", "ADAPTER_UNAVAILABLE"
	}
	d, e := scanDelivery(tx.QueryRow(ctx, `INSERT INTO notification_deliveries(owner_id,notification_id,channel,state,next_run_at,expires_at,last_error_code,created_at,updated_at) VALUES($1,$2,'email',$3,$4,$5,$6,$7,$7) ON CONFLICT DO NOTHING RETURNING `+deliveryColumns, owner, n.ID, state, next, expires, code, now))
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if state == "queued" {
		return s.enqueueDelivery(ctx, tx, owner, d)
	}
	return nil
}
func (s *Service) notificationCurrent(ctx context.Context, tx pgx.Tx, owner string, n Notification, p Preferences) (bool, error) {
	if !n.Active {
		return false, nil
	}
	var active bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND disabled_at IS NULL)`, owner).Scan(&active); e != nil || !active {
		return false, e
	}
	if n.ReminderID != nil {
		r, e := s.reminder(ctx, tx, owner, *n.ReminderID)
		if e != nil {
			return false, e
		}
		if r.State != "fired" || r.NotificationID == nil || *r.NotificationID != n.ID {
			return false, nil
		}
		source, valid, e := s.currentReminderSource(ctx, tx, owner, r, p)
		return valid && source.Revision == n.SourceRevision && source.Due.Equal(r.SourceDueAt), e
	}
	return s.eventCurrent(ctx, tx, owner, n)
}

// deliveryTransaction uses an independent connection so the sending marker is
// durable before I/O, without waiting for another connection in a saturated
// request pool whose transactions may be waiting for this owner's lock.
func (s *Service) deliveryTransaction(ctx context.Context, fn func(pgx.Tx) error) error {
	conn, e := pgx.ConnectConfig(ctx, s.Pool.Config().ConnConfig.Copy())
	if e != nil {
		return e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = conn.Close(cleanup)
	}()
	return storage.WithinTx(ctx, conn, fn)
}
func (s *Service) markSending(ctx context.Context, owner string, d Delivery) (Delivery, bool, error) {
	var result Delivery
	claimed := false
	e := s.deliveryTransaction(ctx, func(tx pgx.Tx) error {
		var e error
		result, e = scanDelivery(tx.QueryRow(ctx, `UPDATE notification_deliveries SET state='sending',attempts=attempts+1,auto_attempts=auto_attempts+1,last_attempt_id=gen_random_uuid(),last_error_code='',updated_at=$4 WHERE owner_id=$1 AND id=$2 AND version=$3 AND state IN('queued','retry_wait') RETURNING `+deliveryColumns, owner, d.ID, d.Version, s.Clock.Now()))
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO notification_delivery_attempts(owner_id,delivery_id,attempt,attempt_id,channel,state,started_at) VALUES($1,$2,$3,$4,$5,'sending',$6)`, owner, result.ID, result.Attempts, result.LastAttemptID, result.Channel, s.Clock.Now())
		claimed = e == nil
		return e
	})
	return result, claimed, e
}
func (s *Service) finishAttempt(ctx context.Context, owner string, d Delivery, state, code string, next time.Time) (Delivery, error) {
	var out Delivery
	e := s.deliveryTransaction(ctx, func(tx pgx.Tx) error {
		now := s.Clock.Now()
		var e error
		out, e = scanDelivery(tx.QueryRow(ctx, `UPDATE notification_deliveries SET state=$4,last_error_code=$5,next_run_at=$6,version=version+1,provider_accepted_at=CASE WHEN $4='provider_accepted' THEN $7 ELSE provider_accepted_at END,unknown_at=CASE WHEN $4='delivery_unknown' THEN $7 ELSE unknown_at END,updated_at=$7 WHERE owner_id=$1 AND id=$2 AND state='sending' AND last_attempt_id=$3 RETURNING `+deliveryColumns, owner, d.ID, d.LastAttemptID, state, code, next, now))
		if errors.Is(e, pgx.ErrNoRows) {
			out = d
			return nil
		}
		if e != nil {
			return e
		}
		attemptState := state
		if state == "retry_wait" {
			attemptState = "failed"
		}
		_, e = tx.Exec(ctx, `UPDATE notification_delivery_attempts SET state=$4,error_code=$5,finished_at=$6 WHERE owner_id=$1 AND delivery_id=$2 AND attempt_id=$3 AND state='sending'`, owner, d.ID, d.LastAttemptID, attemptState, code, now)
		return e
	})
	return out, e
}
func classifyMail(err error) (string, string, bool) {
	if err == nil {
		return "provider_accepted", "", false
	}
	if errors.Is(err, mail.ErrDeliveryUnknown) {
		return "delivery_unknown", "SMTP_OUTCOME_UNKNOWN", false
	}
	if errors.Is(err, mail.ErrUnavailable) || errors.Is(err, mail.ErrTLSRequired) {
		return "failed", "ADAPTER_UNAVAILABLE", false
	}
	var smtp *textproto.Error
	if errors.As(err, &smtp) && smtp.Code >= 500 {
		return "failed", "SMTP_REJECTED", false
	}
	return "retry_wait", "SMTP_TEMPORARY_FAILURE", true
}
func (s *Service) rescheduleDelivery(ctx context.Context, tx pgx.Tx, owner string, d Delivery, next time.Time, code string) error {
	updated, e := scanDelivery(tx.QueryRow(ctx, `UPDATE notification_deliveries SET state='queued',next_run_at=$3,version=version+1,last_error_code=$4,updated_at=$5 WHERE owner_id=$1 AND id=$2 RETURNING `+deliveryColumns, owner, d.ID, next, code, s.Clock.Now()))
	if e != nil {
		return e
	}
	return s.enqueueDelivery(ctx, tx, owner, updated)
}
func (s *Service) sendDelivery(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
	var in deliveryJob
	var keys map[string]json.RawMessage
	if j.OwnerID == nil || !privateapi.UUID(*j.OwnerID) || json.Unmarshal(j.Payload, &in) != nil || json.Unmarshal(j.Payload, &keys) != nil || len(keys) != 2 || !privateapi.UUID(in.ID) || in.Version < 1 {
		return jobs.ErrPermanent
	}
	owner := *j.OwnerID
	var locked string
	if e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&locked); e != nil {
		return e
	}
	d, e := s.delivery(ctx, tx, owner, in.ID)
	if e != nil {
		return e
	}
	if d.State == "sending" {
		_, e = s.finishAttempt(ctx, owner, d, "delivery_unknown", "WORKER_OUTCOME_UNKNOWN", d.NextRunAt)
		return e
	}
	if d.State != "queued" && d.State != "retry_wait" {
		return nil
	}
	if d.Version != in.Version {
		return s.enqueueDelivery(ctx, tx, owner, d)
	}
	p, e := s.preferences(ctx, tx, owner)
	if e != nil {
		return e
	}
	n, e := s.get(ctx, tx, owner, d.NotificationID)
	if e != nil {
		return e
	}
	current, e := s.notificationCurrent(ctx, tx, owner, n, p)
	if e != nil {
		return e
	}
	policy := p.Channel(n.Kind)
	allowedChannel := d.Channel == "email" && policy.Email || d.Channel == "push" && policy.Push
	if !current || !allowedChannel || !s.Clock.Now().Before(d.ExpiresAt) {
		_, e = tx.Exec(ctx, `UPDATE notification_deliveries SET state='cancelled',version=version+1,last_error_code='SOURCE_OR_CONSENT_CHANGED',updated_at=$3 WHERE owner_id=$1 AND id=$2`, owner, d.ID, s.Clock.Now())
		return e
	}
	used, oldest, e := s.attemptUsage(ctx, tx, owner, d.Channel)
	if e != nil {
		return e
	}
	_, urgent, e := s.deliveryExpiry(ctx, tx, owner, n)
	if e != nil {
		return e
	}
	next, allowed, e := PolicyTime(s.Clock.Now(), d.ExpiresAt, p, urgent, used, oldest)
	if e != nil {
		return e
	}
	if !allowed {
		_, e = tx.Exec(ctx, `UPDATE notification_deliveries SET state='cancelled',version=version+1,last_error_code='POLICY_OR_EXPIRY',updated_at=$3 WHERE owner_id=$1 AND id=$2`, owner, d.ID, s.Clock.Now())
		return e
	}
	if next.After(s.Clock.Now()) {
		return s.rescheduleDelivery(ctx, tx, owner, d, next, "QUIET_HOURS_OR_FREQUENCY_CAP")
	}
	if d.NextRunAt.After(s.Clock.Now()) {
		return s.rescheduleDelivery(ctx, tx, owner, d, d.NextRunAt, "WAITING")
	}
	var email string
	var verified bool
	var subscription push.Subscription
	if d.Channel == "push" {
		subscription, e = s.pushSubscription(ctx, tx, owner, d)
		if errors.Is(e, push.ErrGone) {
			_, e = tx.Exec(ctx, `UPDATE notification_deliveries SET state='cancelled',version=version+1,last_error_code='PUSH_REVOKED',updated_at=$3 WHERE owner_id=$1 AND id=$2`, owner, d.ID, s.Clock.Now())
			return e
		}
		if e != nil {
			return e
		}
		verified = true
	} else if e = tx.QueryRow(ctx, `SELECT email,email_verified_at IS NOT NULL AND disabled_at IS NULL FROM users WHERE id=$1`, owner).Scan(&email, &verified); e != nil {
		return e
	}
	if !verified {
		_, e = tx.Exec(ctx, `UPDATE notification_deliveries SET state='cancelled',version=version+1,last_error_code='EMAIL_UNVERIFIED',updated_at=$3 WHERE owner_id=$1 AND id=$2`, owner, d.ID, s.Clock.Now())
		return e
	}
	sending, claimed, e := s.markSending(ctx, owner, d)
	if e != nil {
		return e
	}
	if !claimed {
		return nil
	}
	sender := s.mailer
	if sender == nil {
		sender = mail.Unavailable{}
	}
	var err error
	if d.Channel == "email" {
		err = sender.Send(ctx, s.mailMessage(owner, n, email))
	}
	state, code, retry := classifyMail(err)
	if d.Channel == "push" {
		err = s.sendPush(ctx, subscription, n)
		state, code, retry = classifyPush(err)
		if errors.Is(err, push.ErrGone) {
			if _, e = tx.Exec(ctx, `UPDATE notification_push_subscriptions SET revoked_at=$3,version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2 AND revoked_at IS NULL`, owner, *d.SubscriptionID, s.Clock.Now()); e != nil {
				return e
			}
		}
	}
	run := s.Clock.Now()
	if retry {
		if sending.AutoAttempts >= 3 {
			state = "failed"
		} else {
			run = run.Add(time.Duration(1<<min(sending.AutoAttempts-1, 3)) * time.Minute)
			if !run.Before(d.ExpiresAt) {
				state = "cancelled"
				code = "SOURCE_EXPIRED"
			}
		}
	}
	if state == "cancelled" {
		state = "failed"
	}
	finished, e := s.finishAttempt(ctx, owner, sending, state, code, run)
	if e != nil {
		return e
	}
	if finished.State == "retry_wait" {
		return s.enqueueDelivery(ctx, tx, owner, finished)
	}
	return nil
}
func (s *Service) repairDeliveries(ctx context.Context, tx pgx.Tx, owner string) error {
	rows, e := tx.Query(ctx, `SELECT `+deliveryColumns+` FROM notification_deliveries WHERE owner_id=$1 AND state IN('queued','retry_wait','sending') ORDER BY next_run_at,id LIMIT 100`, owner)
	if e != nil {
		return e
	}
	pending := []Delivery{}
	for rows.Next() {
		d, e := scanDelivery(rows)
		if e != nil {
			rows.Close()
			return e
		}
		pending = append(pending, d)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, d := range pending {
		if d.State == "sending" {
			if s.Clock.Now().Sub(d.UpdatedAt) > 2*time.Minute {
				if _, e = s.finishAttempt(ctx, owner, d, "delivery_unknown", "WORKER_OUTCOME_UNKNOWN", d.NextRunAt); e != nil {
					return e
				}
			}
			continue
		}
		var exists bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE dedupe_key=$1 AND state IN('pending','running','retry_wait'))`, fmt.Sprintf("notification-delivery/%s/%s/%d", owner, d.ID, d.Version)).Scan(&exists)
		if e != nil {
			return e
		}
		if !exists {
			if e = s.rescheduleDelivery(ctx, tx, owner, d, d.NextRunAt, "RECOVERED_QUEUE"); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) mountDeliveries(r chi.Router) {
	r.Get("/api/v1/notifications/{id}/deliveries", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			n, e := s.get(ctx, tx, owner, chi.URLParam(r, "id"))
			if e != nil {
				return nil, e
			}
			rows, e := tx.Query(ctx, `SELECT `+deliveryColumns+` FROM notification_deliveries WHERE owner_id=$1 AND notification_id=$2 ORDER BY created_at,id`, owner, n.ID)
			if e != nil {
				return nil, e
			}
			defer rows.Close()
			items := []Delivery{}
			for rows.Next() {
				d, e := scanDelivery(rows)
				if e != nil {
					return nil, e
				}
				items = append(items, d)
			}
			return map[string]any{"items": items}, rows.Err()
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/notification-deliveries/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in struct {
			Confirm               bool `json:"confirm"`
			AcknowledgeDuplicates bool `json:"acknowledge_duplicates"`
		}) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				d, e := s.delivery(ctx, tx, owner, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if e := privateapi.MatchVersion(r, d.Version); e != nil {
					return nil, e
				}
				if !in.Confirm || d.State == "delivery_unknown" && !in.AcknowledgeDuplicates {
					return nil, privateapi.Invalid("Подтвердите повтор; при неизвестном исходе возможен дубль")
				}
				if d.State != "failed" && d.State != "delivery_unknown" {
					return nil, privateapi.Problem(409, "DELIVERY_STATE_CHANGED", "Этот результат отправки уже изменился")
				}
				p, e := s.preferences(ctx, tx, owner)
				if e != nil {
					return nil, e
				}
				n, e := s.get(ctx, tx, owner, d.NotificationID)
				if e != nil {
					return nil, e
				}
				valid, e := s.notificationCurrent(ctx, tx, owner, n, p)
				if e != nil {
					return nil, e
				}
				if !valid || !(d.Channel == "email" && p.Channel(n.Kind).Email || d.Channel == "push" && p.Channel(n.Kind).Push) || !s.Clock.Now().Before(d.ExpiresAt) {
					return nil, privateapi.Problem(409, "SOURCE_OR_CONSENT_CHANGED", "Источник или согласие изменились")
				}
				updated, e := scanDelivery(tx.QueryRow(ctx, `UPDATE notification_deliveries SET state='queued',version=version+1,auto_attempts=0,next_run_at=$3,last_error_code='',updated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING `+deliveryColumns, owner, d.ID, s.Clock.Now()))
				if e != nil {
					return nil, e
				}
				if e = s.enqueueDelivery(ctx, tx, owner, updated); e != nil {
					return nil, e
				}
				return updated, nil
			})
		})
	})
}
