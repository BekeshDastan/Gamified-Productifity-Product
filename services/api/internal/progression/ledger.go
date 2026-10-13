package progression

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var ErrInvalidSource = errors.New("invalid reward source")

type Service struct {
	pool  *pgxpool.Pool
	clock platform.Clock
}

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{pool: pool, clock: clock}
}

type Source struct {
	Kind, Key                      string
	Difficulty                     int
	AreaID                         *string
	Complete, Eligible, AllowFirst bool
}
type Entitlement struct {
	ID                                   string
	Kind                                 string
	Key                                  string
	EconomyVersion                       string
	Difficulty                           int
	AreaID                               *string
	RequestedXP, AwardedXP, AwardedCoins int64
	FirstAwardedAt                       time.Time
	Active                               bool
	Generation                           int64
}
type QuotaState struct {
	TaskUsed      int64     `json:"task_used"`
	HabitUsed     int64     `json:"habit_used"`
	ChallengeUsed int64     `json:"challenge_used"`
	TotalUsed     int64     `json:"total_used"`
	TaskCap       int64     `json:"task_cap"`
	HabitCap      int64     `json:"habit_cap"`
	ChallengeCap  int64     `json:"challenge_cap"`
	TotalCap      int64     `json:"total_cap"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
}
type Balance struct {
	TotalXP        int64      `json:"total_xp"`
	Coins          int64      `json:"coins"`
	AvailableCoins int64      `json:"available_coins"`
	CoinCorrection int64      `json:"coin_correction"`
	Level          Level      `json:"level"`
	Version        int64      `json:"version"`
	EconomyVersion string     `json:"economy_version"`
	AsOf           time.Time  `json:"as_of"`
	Quota          QuotaState `json:"quota"`
}
type Reward struct {
	EntitlementID  string `json:"entitlement_id"`
	LedgerID       string `json:"ledger_id"`
	SourceKind     string `json:"source_kind"`
	SourceKey      string `json:"source_key"`
	EconomyVersion string `json:"economy_version"`
	Kind           string `json:"kind"`
	RequestedXP    int64  `json:"requested_xp"`
	AwardedXP      int64  `json:"awarded_xp"`
	AwardedCoins   int64  `json:"awarded_coins"`
	XPDelta        int64  `json:"xp_delta"`
	CoinsDelta     int64  `json:"coins_delta"`
	TotalXP        int64  `json:"total_xp"`
	AvailableCoins int64  `json:"available_coins"`
	CoinCorrection int64  `json:"coin_correction"`
	LevelBefore    int64  `json:"level_before"`
	LevelAfter     int64  `json:"level_after"`
	Reason         string `json:"reason"`
}

func (s *Service) Lock(ctx context.Context, tx pgx.Tx, owner string) error {
	if _, e := tx.Exec(ctx, `INSERT INTO user_progress(owner_id,updated_at) VALUES($1,$2) ON CONFLICT(owner_id) DO NOTHING`, owner, s.clock.Now().UTC()); e != nil {
		return e
	}
	var id string
	return tx.QueryRow(ctx, `SELECT owner_id::text FROM user_progress WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&id)
}

// LockOwner preserves the common profile -> user_progress -> domain entity order.
func (s *Service) LockOwner(ctx context.Context, tx pgx.Tx, owner string) error {
	var id string
	if e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&id); e != nil {
		return e
	}
	return s.Lock(ctx, tx, owner)
}
func (s *Service) WithinOwner(ctx context.Context, owner string, fn func(pgx.Tx) error) error {
	return storage.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if e := s.LockOwner(ctx, tx, owner); e != nil {
			return e
		}
		return fn(tx)
	})
}
func (s *Service) quota(ctx context.Context, tx pgx.Tx, owner string) (QuotaState, error) {
	q := QuotaState{TaskCap: 200, HabitCap: 50, ChallengeCap: 50, TotalCap: 300, WindowStart: s.clock.Now().UTC().Add(-24 * time.Hour), WindowEnd: s.clock.Now().UTC()}
	e := tx.QueryRow(ctx, `SELECT COALESCE(sum(xp) FILTER(WHERE source_kind='task'),0),COALESCE(sum(xp) FILTER(WHERE source_kind='habit'),0),COALESCE(sum(xp) FILTER(WHERE source_kind='challenge'),0),COALESCE(sum(xp),0) FROM reward_quota_usage WHERE owner_id=$1 AND awarded_at>$2 AND awarded_at<=$3`, owner, q.WindowStart, q.WindowEnd).Scan(&q.TaskUsed, &q.HabitUsed, &q.ChallengeUsed, &q.TotalUsed)
	return q, e
}
func (s *Service) Balance(ctx context.Context, tx pgx.Tx, owner string) (Balance, error) {
	b := Balance{EconomyVersion: Version, AsOf: s.clock.Now().UTC()}
	e := tx.QueryRow(ctx, `SELECT total_xp,coins,version FROM user_progress WHERE owner_id=$1`, owner).Scan(&b.TotalXP, &b.Coins, &b.Version)
	if e != nil {
		return b, e
	}
	b.Level = LevelFor(b.TotalXP)
	b.AvailableCoins = max(0, b.Coins)
	b.CoinCorrection = max(0, -b.Coins)
	b.Quota, e = s.quota(ctx, tx, owner)
	return b, e
}
func validSource(in Source) bool {
	var id pgtype.UUID
	return (in.Kind == "task" || in.Kind == "habit" || in.Kind == "challenge") && id.Scan(in.Key) == nil && id.Valid && BaseXP(in.Kind, in.Difficulty) > 0
}
func (s *Service) Entitlement(ctx context.Context, tx pgx.Tx, owner, kind, key string) (*Entitlement, error) {
	var v Entitlement
	e := tx.QueryRow(ctx, `SELECT id::text,source_kind,source_key,economy_version,difficulty,area_id::text,requested_xp,awarded_xp,awarded_coins,first_awarded_at,active,generation FROM reward_entitlements WHERE owner_id=$1 AND source_kind=$2 AND source_key=$3 FOR UPDATE`, owner, kind, key).Scan(&v.ID, &v.Kind, &v.Key, &v.EconomyVersion, &v.Difficulty, &v.AreaID, &v.RequestedXP, &v.AwardedXP, &v.AwardedCoins, &v.FirstAwardedAt, &v.Active, &v.Generation)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, e
}
func (s *Service) result(ctx context.Context, tx pgx.Tx, owner string, v Entitlement, kind, ledger, reason string, xp, coins int64) (*Reward, error) {
	b, e := s.Balance(ctx, tx, owner)
	if e != nil {
		return nil, e
	}
	return &Reward{EntitlementID: v.ID, LedgerID: ledger, SourceKind: v.Kind, SourceKey: v.Key, EconomyVersion: v.EconomyVersion, Kind: kind, RequestedXP: v.RequestedXP, AwardedXP: v.AwardedXP, AwardedCoins: v.AwardedCoins, XPDelta: xp, CoinsDelta: coins, TotalXP: b.TotalXP, AvailableCoins: b.AvailableCoins, CoinCorrection: b.CoinCorrection, LevelBefore: LevelFor(b.TotalXP - xp).Number, LevelAfter: b.Level.Number, Reason: reason}, nil
}
func (s *Service) append(ctx context.Context, tx pgx.Tx, owner string, v Entitlement, kind, reason string, xp, coins int64) (string, error) {
	var id string
	e := tx.QueryRow(ctx, `INSERT INTO reward_ledger(owner_id,entitlement_id,operation_key,source_kind,source_key,kind,economy_version,area_id,xp_delta,coins_delta,reason,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id::text`, owner, v.ID, fmt.Sprintf("reward/%s/%d", v.ID, v.Generation), v.Kind, v.Key, kind, v.EconomyVersion, v.AreaID, xp, coins, reason, s.clock.Now().UTC()).Scan(&id)
	if e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `UPDATE user_progress SET total_xp=total_xp+$2,coins=coins+$3,version=version+1,updated_at=$4 WHERE owner_id=$1`, owner, xp, coins, s.clock.Now().UTC()); e != nil {
		return "", e
	}
	payload, e := json.Marshal(map[string]string{"ledger_id": id, "source_kind": v.Kind, "source_key": v.Key})
	if e != nil {
		return "", e
	}
	e = (jobs.Store{Clock: s.clock}).AddOutbox(ctx, tx, jobs.Event{ID: id, OwnerID: &owner, Kind: "progression.reward_changed", Payload: payload})
	return id, e
}

// Award must run in the domain transaction after Lock, before its history is committed.
func (s *Service) Award(ctx context.Context, tx pgx.Tx, owner string, in Source) (*Reward, error) {
	if !validSource(in) {
		return nil, ErrInvalidSource
	}
	v, e := s.Entitlement(ctx, tx, owner, in.Kind, in.Key)
	if e != nil {
		return nil, e
	}
	if v != nil {
		return s.result(ctx, tx, owner, *v, "unchanged", "", "Этот источник уже учтён. Исходная награда сохранена.", 0, 0)
	}
	if !in.Complete || !in.Eligible || !in.AllowFirst {
		return nil, nil
	}
	q, e := s.quota(ctx, tx, owner)
	if e != nil {
		return nil, e
	}
	used := q.TaskUsed
	if in.Kind == "habit" {
		used = q.HabitUsed
	}
	if in.Kind == "challenge" {
		used = q.ChallengeUsed
	}
	base := BaseXP(in.Kind, in.Difficulty)
	xp := Limited(base, used, q.TotalUsed, Quota(in.Kind))
	v = &Entitlement{Kind: in.Kind, Key: in.Key, EconomyVersion: Version, Difficulty: in.Difficulty, AreaID: in.AreaID, RequestedXP: base, AwardedXP: xp, AwardedCoins: CoinsForAward(xp), FirstAwardedAt: s.clock.Now().UTC(), Active: true, Generation: 1}
	e = tx.QueryRow(ctx, `INSERT INTO reward_entitlements(owner_id,source_kind,source_key,economy_version,difficulty,area_id,requested_xp,awarded_xp,awarded_coins,first_awarded_at,active,generation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true,1) RETURNING id::text`, owner, v.Kind, v.Key, v.EconomyVersion, v.Difficulty, v.AreaID, v.RequestedXP, v.AwardedXP, v.AwardedCoins, v.FirstAwardedAt).Scan(&v.ID)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO reward_quota_usage(owner_id,entitlement_id,source_kind,xp,awarded_at) VALUES($1,$2,$3,$4,$5)`, owner, v.ID, v.Kind, v.AwardedXP, v.FirstAwardedAt); e != nil {
		return nil, e
	}
	reason := "Награда за подтверждённое выполнение"
	if xp < base {
		reason = fmt.Sprintf("По квоте за скользящие 24 часа выдано %d из %d XP. Остаток не переносится.", xp, base)
	}
	ledger, e := s.append(ctx, tx, owner, *v, "grant", reason, xp, v.AwardedCoins)
	if e != nil {
		return nil, e
	}
	return s.result(ctx, tx, owner, *v, "grant", ledger, reason, xp, v.AwardedCoins)
}
func (s *Service) ExplainNoReward(ctx context.Context, tx pgx.Tx, owner string, source Source, reason string) (*Reward, error) {
	b, e := s.Balance(ctx, tx, owner)
	if e != nil {
		return nil, e
	}
	return &Reward{SourceKind: source.Kind, SourceKey: source.Key, EconomyVersion: Version, Kind: "not_awarded", RequestedXP: BaseXP(source.Kind, source.Difficulty), TotalXP: b.TotalXP, AvailableCoins: b.AvailableCoins, CoinCorrection: b.CoinCorrection, LevelBefore: b.Level.Number, LevelAfter: b.Level.Number, Reason: reason}, nil
}
