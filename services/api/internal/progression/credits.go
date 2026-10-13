package progression

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreditInTx accepts only internal refunds and fixed cosmetic compensation; never XP.
func (s *Service) CreditInTx(ctx context.Context, tx pgx.Tx, owner, key, kind string, amount int64) (string, error) {
	if (kind != "refund" && kind != "bonus") || amount < 0 || amount > MaxSafeXP || !validCreditID(key) {
		return "", &platform.APIError{Status: 422, Code: "VALIDATION_ERROR", Message: "Недопустимая компенсация"}
	}
	if e := s.LockOwner(ctx, tx, owner); e != nil {
		return "", e
	}
	var id string
	var original int64
	e := tx.QueryRow(ctx, "SELECT id::text,coins_delta FROM reward_ledger WHERE owner_id=$1 AND operation_key=$2", owner, kind+"/"+key).Scan(&id, &original)
	if e == nil {
		if original != amount {
			return "", &platform.APIError{Status: 409, Code: "IDEMPOTENCY_MISMATCH", Message: "Компенсация уже записана с другой суммой"}
		}
		return id, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return "", e
	}
	balance, e := s.Balance(ctx, tx, owner)
	if e != nil {
		return "", e
	}
	if balance.Coins > MaxSafeXP-amount {
		return "", ErrLedgerInvariant
	}
	reason := "Возврат за отменённую личную награду"
	if kind == "bonus" {
		reason = "Компенсация полного косметического набора"
	}
	e = tx.QueryRow(ctx, "INSERT INTO reward_ledger(owner_id,operation_key,source_kind,source_key,kind,economy_version,xp_delta,coins_delta,reason,occurred_at) VALUES($1,$2,$3,$4,$3,$5,0,$6,$7,$8) RETURNING id::text", owner, kind+"/"+key, kind, key, Version, amount, reason, s.clock.Now()).Scan(&id)
	if e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, "UPDATE user_progress SET coins=coins+$2,version=version+1,updated_at=$3 WHERE owner_id=$1", owner, amount, s.clock.Now()); e != nil {
		return "", e
	}
	payload, _ := json.Marshal(map[string]string{"ledger_id": id, "source_kind": kind, "source_key": key})
	e = (jobs.Store{Clock: s.clock}).AddOutbox(ctx, tx, jobs.Event{ID: id, OwnerID: &owner, Kind: "progression.reward_changed", Payload: payload})
	return id, e
}

func validCreditID(v string) bool { var id pgtype.UUID; return id.Scan(v) == nil && id.Valid }
