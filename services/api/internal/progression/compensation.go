package progression

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) Award(ctx context.Context, tx pgx.Tx, owner string, in Source) (*Reward, error) {
	return s.Sync(ctx, tx, owner, in)
}
func (s *Service) Sync(ctx context.Context, tx pgx.Tx, owner string, in Source) (*Reward, error) {
	if !validSource(in) {
		return nil, ErrInvalidSource
	}
	v, e := s.Entitlement(ctx, tx, owner, in.Kind, in.Key)
	if e != nil {
		return nil, e
	}
	wanted := in.Complete && in.Eligible
	if v == nil {
		if !wanted || !in.AllowFirst {
			return nil, nil
		}
		return s.awardFirst(ctx, tx, owner, in)
	}
	if v.Active == wanted {
		return s.unchanged(ctx, tx, owner, *v)
	}
	v.Generation++
	v.Active = wanted
	kind, reason := "reversal", "Исправлено выполнение: компенсирована исходная награда. Использованная квота сохраняется."
	xp, coins := -v.AwardedXP, -v.AwardedCoins
	if wanted {
		kind, reason = "restoration", "Восстановлена исходная награда. Новое право и расход квоты не создаются."
		xp, coins = v.AwardedXP, v.AwardedCoins
	}
	if _, e = tx.Exec(ctx, `UPDATE reward_entitlements SET active=$3,generation=$4 WHERE owner_id=$1 AND id=$2`, owner, v.ID, v.Active, v.Generation); e != nil {
		return nil, e
	}
	ledger, e := s.append(ctx, tx, owner, *v, kind, reason, xp, coins)
	if e != nil {
		return nil, e
	}
	return s.result(ctx, tx, owner, *v, kind, ledger, reason, xp, coins)
}
func (s *Service) unchanged(ctx context.Context, tx pgx.Tx, owner string, v Entitlement) (*Reward, error) {
	reason := "Этот источник уже учтён. Повтор не создаёт новую награду."
	if v.AwardedXP < v.RequestedXP {
		reason = fmt.Sprintf("Исходная награда уменьшена квотой до %d из %d XP. Остаток не переносится.", v.AwardedXP, v.RequestedXP)
	}
	if !v.Active {
		reason = "Исходное начисление сейчас отменено; повтор не создаёт новое право."
	}
	return s.result(ctx, tx, owner, v, "unchanged", "", reason, 0, 0)
}
func (s *Service) ExistingReward(ctx context.Context, tx pgx.Tx, owner, kind, key string) (*Reward, error) {
	v, e := s.Entitlement(ctx, tx, owner, kind, key)
	if e != nil || v == nil {
		return nil, e
	}
	return s.unchanged(ctx, tx, owner, *v)
}

// SpendInTx is the internal wallet operation used by purchases. The UUID is permanent.
func (s *Service) SpendInTx(ctx context.Context, tx pgx.Tx, owner, key string, cost int64) (*Reward, error) {
	var uuid pgtype.UUID
	if cost < 0 || cost > MaxSafeXP || uuid.Scan(key) != nil || !uuid.Valid {
		return nil, &platform.APIError{Status: 422, Code: "VALIDATION_ERROR", Message: "Проверьте сумму и постоянный ID операции"}
	}
	if e := s.LockOwner(ctx, tx, owner); e != nil {
		return nil, e
	}
	operation := "spend/" + key
	var ledger string
	var oldCost int64
	e := tx.QueryRow(ctx, `SELECT id::text,-coins_delta FROM reward_ledger WHERE owner_id=$1 AND operation_key=$2`, owner, operation).Scan(&ledger, &oldCost)
	kind, delta, reason := "unchanged", int64(0), "Этот расход уже учтён."
	if e == nil {
		if oldCost != cost {
			return nil, &platform.APIError{Status: 409, Code: "IDEMPOTENCY_MISMATCH", Message: "Операция уже записана с другой суммой"}
		}
	} else if errors.Is(e, pgx.ErrNoRows) {
		b, err := s.Balance(ctx, tx, owner)
		if err != nil {
			return nil, err
		}
		if cost > 0 && b.Coins < cost {
			return nil, &platform.APIError{Status: 409, Code: "INSUFFICIENT_COINS", Message: "Недостаточно доступных виртуальных монет"}
		}
		kind, delta, reason = "spend", -cost, "Расход виртуальных монет"
		e = tx.QueryRow(ctx, `INSERT INTO reward_ledger(owner_id,operation_key,source_kind,source_key,kind,economy_version,xp_delta,coins_delta,reason,occurred_at) VALUES($1,$2,'spend',$3,'spend',$4,0,$5,$6,$7) RETURNING id::text`, owner, operation, key, Version, delta, reason, s.clock.Now().UTC()).Scan(&ledger)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `UPDATE user_progress SET coins=coins+$2,version=version+1,updated_at=$3 WHERE owner_id=$1`, owner, delta, s.clock.Now().UTC()); e != nil {
			return nil, e
		}
		payload, e := json.Marshal(map[string]string{"ledger_id": ledger, "source_kind": "spend", "source_key": key})
		if e != nil {
			return nil, e
		}
		if e = (jobs.Store{Clock: s.clock}).AddOutbox(ctx, tx, jobs.Event{ID: ledger, OwnerID: &owner, Kind: "progression.reward_changed", Payload: payload}); e != nil {
			return nil, e
		}
	} else {
		return nil, e
	}
	b, e := s.Balance(ctx, tx, owner)
	if e != nil {
		return nil, e
	}
	return &Reward{LedgerID: ledger, SourceKind: "spend", SourceKey: key, EconomyVersion: Version, Kind: kind, CoinsDelta: delta, TotalXP: b.TotalXP, AvailableCoins: b.AvailableCoins, CoinCorrection: b.CoinCorrection, LevelBefore: b.Level.Number, LevelAfter: b.Level.Number, Reason: reason}, nil
}
