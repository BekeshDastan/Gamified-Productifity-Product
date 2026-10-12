package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLeaseLost = errors.New("job lease lost")
var ErrPermanent = errors.New("permanent handler failure")
var ErrDedupeMismatch = errors.New("job dedupe mismatch")

type Clock interface{ Now() time.Time }
type Store struct {
	Pool   *pgxpool.Pool
	Clock  Clock
	Jitter func(time.Duration) time.Duration
}
type Job struct {
	ID, Kind, LeaseToken  string
	OwnerID               *string
	Payload               []byte
	Attempts, MaxAttempts int
	LeaseUntil            time.Time
}
type Event struct {
	ID, Kind string
	OwnerID  *string
	Payload  []byte
}
type Handler func(context.Context, pgx.Tx, Job) error

func (s Store) AddOutbox(ctx context.Context, tx pgx.Tx, event Event) error {
	_, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,owner_id,kind,payload,occurred_at)VALUES($1,$2,$3,$4,$5)ON CONFLICT(id)DO NOTHING`, event.ID, event.OwnerID, event.Kind, event.Payload, s.Clock.Now())
	if err != nil {
		return err
	}
	var matches bool
	err = tx.QueryRow(ctx, `SELECT kind=$2 AND payload=$3::jsonb AND owner_id IS NOT DISTINCT FROM $4::uuid FROM outbox_events WHERE id=$1`, event.ID, event.Kind, event.Payload, event.OwnerID).Scan(&matches)
	if err != nil {
		return err
	}
	if !matches {
		return ErrDedupeMismatch
	}
	return nil
}
func (s Store) Enqueue(ctx context.Context, tx pgx.Tx, event Event, dedupe string) error {
	now := s.Clock.Now()
	_, err := tx.Exec(ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at)VALUES($1,$2,$3,$4,$5,$5)ON CONFLICT(dedupe_key)DO NOTHING`, event.OwnerID, event.Kind, event.Payload, dedupe, now)
	if err != nil {
		return err
	}
	var matches bool
	err = tx.QueryRow(ctx, `SELECT kind=$2 AND payload=$3::jsonb AND owner_id IS NOT DISTINCT FROM $4::uuid FROM jobs WHERE dedupe_key=$1`, dedupe, event.Kind, event.Payload, event.OwnerID).Scan(&matches)
	if err != nil {
		return err
	}
	if !matches {
		return ErrDedupeMismatch
	}
	return nil
}
func (s Store) Dispatch(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, errors.New("outbox batch must be 1..100")
	}
	count := 0
	err := storage.WithinTx(ctx, s.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text,owner_id::text,kind,payload FROM outbox_events WHERE dispatched_at IS NULL ORDER BY occurred_at,id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
		if err != nil {
			return err
		}
		var events []Event
		for rows.Next() {
			var event Event
			if err := rows.Scan(&event.ID, &event.OwnerID, &event.Kind, &event.Payload); err != nil {
				rows.Close()
				return err
			}
			events = append(events, event)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := s.Enqueue(ctx, tx, event, "outbox:"+event.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE outbox_events SET dispatched_at=$2 WHERE id=$1`, event.ID, s.Clock.Now()); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
func (s Store) Claim(ctx context.Context) (Job, error) {
	now := s.Clock.Now()
	var job Job
	_, err := s.Pool.Exec(ctx, `UPDATE jobs SET state='dead',lease_until=NULL,lease_token=NULL,last_error_code='ATTEMPTS_EXHAUSTED',completed_at=$1 WHERE state='running' AND lease_until<=$1 AND attempts>=max_attempts`, now)
	if err != nil {
		return job, err
	}
	err = s.Pool.QueryRow(ctx, `WITH candidate AS (
        SELECT id FROM jobs WHERE attempts<max_attempts AND
        ((state IN ('pending','retry_wait') AND next_run_at<=$1) OR (state='running' AND lease_until<=$1))
        ORDER BY next_run_at,id FOR UPDATE SKIP LOCKED LIMIT 1
    ) UPDATE jobs SET state='running',attempts=attempts+1,lease_until=$2,lease_token=gen_random_uuid()
      FROM candidate WHERE jobs.id=candidate.id
      RETURNING jobs.id::text,owner_id::text,kind,payload,attempts,max_attempts,lease_until,lease_token::text`, now, now.Add(30*time.Second)).Scan(&job.ID, &job.OwnerID, &job.Kind, &job.Payload, &job.Attempts, &job.MaxAttempts, &job.LeaseUntil, &job.LeaseToken)
	return job, err
}
