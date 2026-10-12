package calendar

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"time"
)

// FillHorizon uses the same owner lock and unique occurrence identity as HTTP.
// A restart repeats this operation safely; no task is created before its marker check.
func (s *Service) FillHorizon(ctx context.Context) error {
	rows, e := s.pool.Query(ctx, `SELECT DISTINCT owner_id::text FROM recurrence_series`)
	if e != nil {
		return e
	}
	owners := []string{}
	for rows.Next() {
		var id string
		if e := rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		owners = append(owners, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range owners {
		e = storage.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
			if e := s.lock(ctx, tx, id); e != nil {
				return e
			}
			var name string
			if e := tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC') FROM user_settings WHERE owner_id=$1`, id).Scan(&name); e != nil {
				return e
			}
			zone, e := time.LoadLocation(name)
			if e != nil {
				return e
			}
			now := s.clock.Now().In(zone)
			from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			_, e = s.materialize(ctx, tx, id, from, from.AddDate(0, 0, 60))
			return e
		})
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if e := s.FillHorizon(ctx); e != nil && ctx.Err() == nil {
			logger.Warn("calendar_horizon_failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
