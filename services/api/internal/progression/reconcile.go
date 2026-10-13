package progression

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrLedgerInvariant = errors.New("ledger totals outside supported range")

type Reconciliation struct {
	OwnerID       string `json:"owner_id"`
	StoredXP      int64  `json:"stored_xp"`
	LedgerXP      int64  `json:"ledger_xp"`
	StoredCoins   int64  `json:"stored_coins"`
	LedgerCoins   int64  `json:"ledger_coins"`
	LedgerEntries int64  `json:"ledger_entries"`
	Changed       bool   `json:"changed"`
}

func (s *Service) compareInTx(ctx context.Context, tx pgx.Tx, owner string) (Reconciliation, error) {
	r := Reconciliation{OwnerID: owner}
	e := tx.QueryRow(ctx, `SELECT total_xp,coins FROM user_progress WHERE owner_id=$1`, owner).Scan(&r.StoredXP, &r.StoredCoins)
	if e != nil {
		return r, e
	}
	e = tx.QueryRow(ctx, `SELECT COALESCE(sum(xp_delta),0),COALESCE(sum(coins_delta),0),count(*) FROM reward_ledger WHERE owner_id=$1`, owner).Scan(&r.LedgerXP, &r.LedgerCoins, &r.LedgerEntries)
	if e != nil {
		return r, e
	}
	if r.LedgerXP < 0 || r.LedgerXP > MaxSafeXP || r.LedgerCoins < -MaxSafeXP || r.LedgerCoins > MaxSafeXP {
		return r, ErrLedgerInvariant
	}
	r.Changed = r.StoredXP != r.LedgerXP || r.StoredCoins != r.LedgerCoins
	return r, nil
}
func (s *Service) RebuildInTx(ctx context.Context, tx pgx.Tx, owner string) (Reconciliation, error) {
	r, e := s.compareInTx(ctx, tx, owner)
	if e != nil || !r.Changed {
		return r, e
	}
	_, e = tx.Exec(ctx, `UPDATE user_progress SET total_xp=$2,coins=$3,version=version+1,updated_at=$4 WHERE owner_id=$1`, owner, r.LedgerXP, r.LedgerCoins, s.clock.Now().UTC())
	return r, e
}
func (s *Service) Check(ctx context.Context, owner string) (Reconciliation, error) {
	var r Reconciliation
	e := s.WithinOwner(ctx, owner, func(tx pgx.Tx) error { var err error; r, err = s.compareInTx(ctx, tx, owner); return err })
	return r, e
}
func (s *Service) Rebuild(ctx context.Context, owner string) (Reconciliation, error) {
	var r Reconciliation
	e := s.WithinOwner(ctx, owner, func(tx pgx.Tx) error {
		var err error
		r, err = s.RebuildInTx(ctx, tx, owner)
		if err == nil && r.Changed && s.projectionObserver != nil {
			err = s.projectionObserver(ctx, tx, owner)
		}
		return err
	})
	return r, e
}
func (s *Service) OwnerBatch(ctx context.Context, after string) ([]string, error) {
	var cursor *string
	if after != "" {
		cursor = &after
	}
	rows, e := s.pool.Query(ctx, `SELECT owner_id::text FROM (SELECT owner_id FROM user_progress UNION SELECT owner_id FROM reward_ledger) owners WHERE ($1::uuid IS NULL OR owner_id>$1::uuid) ORDER BY owner_id LIMIT 500`, cursor)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (s *Service) WithProjectionObserver(fn func(context.Context, pgx.Tx, string) error) *Service {
	s.projectionObserver = fn
	return s
}
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{"progression.reward_changed": s.consumeReward}
}
func (s *Service) consumeReward(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	if job.OwnerID == nil {
		return jobs.ErrPermanent
	}
	var input struct {
		LedgerID string `json:"ledger_id"`
	}
	if json.Unmarshal(job.Payload, &input) != nil {
		return jobs.ErrPermanent
	}
	var id pgtype.UUID
	if id.Scan(input.LedgerID) != nil || !id.Valid {
		return jobs.ErrPermanent
	}
	owner := *job.OwnerID
	if e := s.LockOwner(ctx, tx, owner); e != nil {
		return e
	}
	var exists, processed bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reward_ledger WHERE owner_id=$1 AND id=$2),EXISTS(SELECT 1 FROM reward_projection_events WHERE owner_id=$1 AND ledger_id=$2)`, owner, input.LedgerID).Scan(&exists, &processed); e != nil {
		return e
	}
	if !exists {
		return jobs.ErrPermanent
	}
	if _, e := s.RebuildInTx(ctx, tx, owner); e != nil {
		return e
	}
	if processed {
		return nil
	}
	if s.projectionObserver != nil {
		if e := s.projectionObserver(ctx, tx, owner); e != nil {
			return e
		}
	}
	_, e := tx.Exec(ctx, `INSERT INTO reward_projection_events(owner_id,ledger_id,processed_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, owner, input.LedgerID, s.clock.Now().UTC())
	return e
}
