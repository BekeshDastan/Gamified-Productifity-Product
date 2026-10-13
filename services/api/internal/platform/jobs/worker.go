package jobs

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
)

func (s Store) Process(ctx context.Context, job Job, handlers map[string]Handler) error {
	handler, ok := handlers[job.Kind]
	var err error
	if !ok || handler == nil {
		err = ErrPermanent
	} else {
		err = storage.WithinTx(ctx, s.Pool, func(tx pgx.Tx) error {
			var token, state string
			var until time.Time
			if err := tx.QueryRow(ctx, `SELECT lease_token::text,state,lease_until FROM jobs WHERE id=$1 AND state='running' AND lease_token=$2::uuid FOR UPDATE`, job.ID, job.LeaseToken).Scan(&token, &state, &until); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrLeaseLost
				}
				return err
			}
			if state != "running" || token != job.LeaseToken || !until.After(s.Clock.Now()) {
				return ErrLeaseLost
			}
			if err := handler(ctx, tx, job); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE jobs SET state='succeeded',lease_token=NULL,lease_until=NULL,completed_at=$2,last_error_code=NULL WHERE id=$1`, job.ID, s.Clock.Now())
			return err
		})
	}
	if err == nil || errors.Is(err, ErrLeaseLost) {
		return err
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	state, code := "retry_wait", "HANDLER_ERROR"
	if errors.Is(err, ErrPermanent) || job.Attempts >= job.MaxAttempts {
		state, code = "dead", "PERMANENT_OR_EXHAUSTED"
	}
	delay := time.Duration(1<<min(job.Attempts, 8)) * 5 * time.Second
	if s.Jitter != nil {
		delay = s.Jitter(delay)
	} else {
		delay += time.Duration(rand.Int64N(int64(delay/5) + 1))
	}
	var provider interface{ RetryAfter() time.Duration }
	if errors.As(err, &provider) && provider.RetryAfter() > delay {
		delay = provider.RetryAfter()
	}
	now := s.Clock.Now()
	tag, saveErr := s.Pool.Exec(cleanup, `UPDATE jobs SET state=$3,next_run_at=$4,lease_token=NULL,lease_until=NULL,last_error_code=$5,completed_at=CASE WHEN $3='dead' THEN $6::timestamptz ELSE NULL END WHERE id=$1 AND state='running' AND lease_token=$2::uuid`, job.ID, job.LeaseToken, state, now.Add(delay), code, now)
	if saveErr != nil {
		return saveErr
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return err
}

func Run(ctx context.Context, store Store, handlers map[string]Handler, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if _, err := store.Dispatch(ctx, 50); err != nil && ctx.Err() == nil {
			logger.Error("outbox_dispatch_failed")
		}
		for ctx.Err() == nil {
			job, err := store.Claim(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				break
			}
			if err != nil {
				logger.Error("job_claim_failed")
				break
			}
			work, cancel := context.WithTimeout(ctx, 20*time.Second)
			err = store.Process(work, job, handlers)
			cancel()
			if err != nil && !errors.Is(err, ErrLeaseLost) {
				logger.Warn("job_handler_failed", "job_id", job.ID, "kind", job.Kind)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
