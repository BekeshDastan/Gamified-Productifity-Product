package seasons

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const RecoveryVersion = "recovery-v1"

type recoveryServices struct {
	goals  *goals.Service
	quests *quests.Service
}

func (s *Service) WithRecovery(q *quests.Service, g *goals.Service) *Service {
	s.recovery = &recoveryServices{g, q}
	return s
}

type RecoveryStep struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	EstimatedMinutes int    `json:"estimated_minutes"`
}
type RecoverySnapshot struct {
	ChallengeRules challenges.Config `json:"challenge_rules"`
	RewardXP       int64             `json:"reward_xp"`
	EconomyVersion string            `json:"economy_version"`
	Rejoin         bool              `json:"rejoin"`
	Visibility     string            `json:"visibility"`
	SourcePolicy   string            `json:"source_policy"`
	CatalogVersion string            `json:"catalog_version"`
	Steps          []RecoveryStep    `json:"steps"`
	LastResultAt   *time.Time        `json:"last_result_at"`
	InactiveSince  time.Time         `json:"inactive_since"`
}
type RecoveryResult struct {
	GoalID   *string        `json:"goal_id"`
	RunID    *string        `json:"run_id"`
	QuestIDs []string       `json:"quest_ids"`
	Steps    []RecoveryStep `json:"steps"`
}
type RecoveryOffer struct {
	RecoverySnapshot
	ID        string          `json:"id"`
	OfferedAt time.Time       `json:"offered_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	State     string          `json:"state"`
	Version   int64           `json:"version"`
	DecidedAt *time.Time      `json:"decided_at"`
	Result    *RecoveryResult `json:"result"`
}
type RecoveryEligibility struct {
	Timezone      string         `json:"timezone"`
	Eligible      bool           `json:"eligible"`
	LastResultAt  *time.Time     `json:"last_result_at"`
	InactiveSince time.Time      `json:"inactive_since"`
	CooldownUntil *time.Time     `json:"cooldown_until"`
	Offer         *RecoveryOffer `json:"offer"`
}
type RecoveryDecision struct {
	Timezone string         `json:"timezone"`
	Accept   bool           `json:"accept"`
	Steps    []RecoveryStep `json:"steps"`
	AreaID   *string        `json:"area_id"`
}

func recoverySteps() []RecoveryStep {
	return []RecoveryStep{{"pick", "Выбрать один небольшой результат", "Назовите посильный результат на ближайшие дни.", 5}, {"step", "Сделать первый небольшой шаг", "Выберите действие, которое помещается в доступное время и энергию.", 10}, {"next", "Записать следующий шаг", "Отметьте доступное продолжение, чтобы было проще вернуться.", 5}}
}

const offerColumns = "id::text,snapshot,offered_at,expires_at,state,version,decided_at,result"

func scanOffer(row pgx.Row) (RecoveryOffer, error) {
	var v RecoveryOffer
	var snapshot, result []byte
	e := row.Scan(&v.ID, &snapshot, &v.OfferedAt, &v.ExpiresAt, &v.State, &v.Version, &v.DecidedAt, &result)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, privateapi.Problem(404, "NOT_FOUND", "Предложение не найдено")
	}
	if e != nil {
		return v, e
	}
	if e = json.Unmarshal(snapshot, &v.RecoverySnapshot); e != nil {
		return v, e
	}
	if result != nil {
		var saved RecoveryResult
		if e = json.Unmarshal(result, &saved); e != nil {
			return v, e
		}
		v.Result = &saved
	}
	return v, nil
}
func (s *Service) getOffer(ctx context.Context, tx pgx.Tx, owner, id string) (RecoveryOffer, error) {
	if !privateapi.UUID(id) {
		return RecoveryOffer{}, privateapi.Problem(404, "NOT_FOUND", "Предложение не найдено")
	}
	return scanOffer(tx.QueryRow(ctx, "SELECT "+offerColumns+" FROM recovery_offers WHERE owner_id=$1 AND id=$2", owner, id))
}
func (s *Service) recoveryAnchor(ctx context.Context, tx pgx.Tx, owner string) (time.Time, *time.Time, error) {
	var created time.Time
	var last *time.Time
	e := tx.QueryRow(ctx, `SELECT created_at,(SELECT max(at) FROM (
 SELECT first_awarded_at at FROM reward_entitlements WHERE owner_id=$1 AND active
 UNION ALL SELECT accepted_at FROM habit_logs WHERE owner_id=$1 AND amount>0
 UNION ALL SELECT updated_at FROM focus_sessions WHERE owner_id=$1 AND state='finished' AND result<>'rest' AND EXISTS(SELECT 1 FROM focus_segments s WHERE s.owner_id=focus_sessions.owner_id AND s.session_id=focus_sessions.id AND s.kind='work' AND s.ended_at>s.started_at)
 UNION ALL SELECT created_at FROM daily_reviews WHERE owner_id=$1
 UNION ALL SELECT created_at FROM goal_metric_entries WHERE owner_id=$1 AND value>0
 UNION ALL SELECT created_at FROM challenge_checkins WHERE owner_id=$1 AND amount>0
 UNION ALL SELECT challenge_first_qualified_at FROM milestones WHERE owner_id=$1 AND challenge_completed AND challenge_first_qualified_at IS NOT NULL
 ) a) FROM users WHERE id=$1`, owner).Scan(&created, &last)
	anchor := created
	if last != nil {
		anchor = *last
	}
	return anchor, last, e
}
func (s *Service) offerEvent(ctx context.Context, tx pgx.Tx, owner string, v RecoveryOffer) error {
	body, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO recovery_offer_events(owner_id,offer_id,version,snapshot,created_at) VALUES($1,$2,$3,$4,$5)", owner, v.ID, v.Version, body, s.Clock.Now())
	return e
}
func offerCooldown(v RecoveryOffer) time.Time {
	at := v.OfferedAt
	if v.DecidedAt != nil && (v.State == "accepted" || v.State == "declined") {
		at = *v.DecidedAt
	}
	return at.Add(14 * 24 * time.Hour)
}
func (s *Service) recoveryOffer(ctx context.Context, tx pgx.Tx, owner string) (RecoveryEligibility, error) {
	out := RecoveryEligibility{}
	anchor, last, e := s.recoveryAnchor(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	out.LastResultAt = last
	out.InactiveSince = anchor
	if e = tx.QueryRow(ctx, "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&out.Timezone); e != nil {
		return out, e
	}
	now := s.Clock.Now()
	inactive := now.Sub(anchor) >= 72*time.Hour
	latest, e := scanOffer(tx.QueryRow(ctx, "SELECT "+offerColumns+" FROM recovery_offers WHERE owner_id=$1 ORDER BY offered_at DESC,id DESC LIMIT 1", owner))
	exists := e == nil
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) || p.Status != 404 {
			return out, e
		}
	}
	if exists {
		if latest.State == "offered" && (!now.Before(latest.ExpiresAt) || !inactive) {
			state := "withdrawn"
			if !now.Before(latest.ExpiresAt) {
				state = "expired"
			}
			latest, e = scanOffer(tx.QueryRow(ctx, "UPDATE recovery_offers SET state=$3,decided_at=$4,version=version+1 WHERE owner_id=$1 AND id=$2 RETURNING "+offerColumns, owner, latest.ID, state, now))
			if e != nil {
				return out, e
			}
			if e = s.offerEvent(ctx, tx, owner, latest); e != nil {
				return out, e
			}
		}
		cooldown := offerCooldown(latest)
		out.CooldownUntil = &cooldown
		out.Offer = &latest
		if latest.State == "offered" && inactive && now.Before(latest.ExpiresAt) {
			out.Eligible = true
			return out, nil
		}
		if now.Before(cooldown) || !inactive {
			return out, nil
		}
	}
	if !inactive {
		return out, nil
	}
	snapshot := RecoverySnapshot{CatalogVersion: RecoveryVersion, Steps: recoverySteps(), LastResultAt: last, InactiveSince: anchor, ChallengeRules: challenges.Config{Metric: "tasks", Unit: "tasks", Kind: "cumulative", Period: "day", Target: 3, PeriodTarget: 1, Days: 7, DailyCap: 3}, RewardXP: progression.BaseXP("challenge", 0), EconomyVersion: progression.Version, Rejoin: true, Visibility: "private", SourcePolicy: "Первые завершения листовых задач созданной цели после вступления; повтор не увеличивает счёт, исправление снимает зачёт. Учитываются записи приложения, без проверки реального поведения."}
	body, e := json.Marshal(snapshot)
	if e != nil {
		return out, e
	}
	v, e := scanOffer(tx.QueryRow(ctx, "INSERT INTO recovery_offers(owner_id,snapshot,offered_at,expires_at) VALUES($1,$2,$3,$4) RETURNING "+offerColumns, owner, body, now, now.Add(14*24*time.Hour)))
	if e != nil {
		return out, e
	}
	if e = s.offerEvent(ctx, tx, owner, v); e != nil {
		return out, e
	}
	out.Offer = &v
	out.Eligible = true
	cooldown := offerCooldown(v)
	out.CooldownUntil = &cooldown
	return out, nil
}
func (s *Service) decideRecovery(r *http.Request, tx pgx.Tx, owner, id string, in RecoveryDecision) (RecoveryOffer, error) {
	v, e := s.getOffer(r.Context(), tx, owner, id)
	if e != nil {
		return v, e
	}
	if in.AreaID != nil && !privateapi.UUID(*in.AreaID) {
		return v, privateapi.Invalid("Проверьте направление")
	}
	if strings.ContainsRune(in.Timezone, 0) {
		return v, privateapi.Invalid("Проверьте часовой пояс")
	}
	for _, step := range in.Steps {
		if strings.ContainsRune(step.Title, 0) || strings.ContainsRune(step.Description, 0) || strings.ContainsRune(step.ID, 0) {
			return v, privateapi.Invalid("Нулевой символ недопустим")
		}
	}
	request, e := json.Marshal(in)
	if e != nil {
		return v, e
	}
	if v.State != "offered" {
		if v.State != "accepted" && v.State != "declined" {
			return v, privateapi.Problem(409, "OFFER_STALE", "Предложение больше не действует")
		}
		var same bool
		if e = tx.QueryRow(r.Context(), "SELECT request=$3::jsonb FROM recovery_offers WHERE owner_id=$1 AND id=$2", owner, id, request).Scan(&same); e != nil {
			return v, e
		}
		if !same {
			return v, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Для этого предложения уже сохранено другое решение")
		}
		return v, nil
	}
	if p := privateapi.MatchVersion(r, v.Version); p != nil {
		return v, p
	}
	now := s.Clock.Now()
	if !now.Before(v.ExpiresAt) {
		return v, privateapi.Problem(409, "OFFER_EXPIRED", "Срок предложения истёк")
	}
	result := RecoveryResult{QuestIDs: []string{}, Steps: []RecoveryStep{}}
	state := "declined"
	if in.Accept {
		if v.CatalogVersion != RecoveryVersion || v.EconomyVersion != progression.Version || v.RewardXP != progression.BaseXP("challenge", 0) {
			return v, privateapi.Problem(409, "RECOVERY_VERSION_UNAVAILABLE", "Версия условий сейчас недоступна")
		}
		anchor, _, e := s.recoveryAnchor(r.Context(), tx, owner)
		if e != nil {
			return v, e
		}
		if now.Sub(anchor) < 72*time.Hour {
			return v, privateapi.Problem(409, "OFFER_STALE", "Уже есть новый записанный результат. Обновите предложение")
		}
		if len(in.Steps) != 3 {
			return v, privateapi.Invalid("Подтвердите три небольших шага")
		}
		if s.recovery == nil || s.challenges == nil {
			return v, privateapi.Problem(503, "UNAVAILABLE", "Создание возврата недоступно")
		}
		steps := make([]RecoveryStep, 3)
		for i, step := range in.Steps {
			if step.ID != v.Steps[i].ID || step.EstimatedMinutes < 1 || step.EstimatedMinutes > 15 || utf8.RuneCountInString(step.Description) > 2000 {
				return v, privateapi.Invalid("Каждый шаг: исходный ID, название и от 1 до 15 минут")
			}
			valid, e := quests.Validate(quests.Input{Title: step.Title, Description: step.Description, AreaID: in.AreaID, Priority: "normal", Difficulty: 1, EstimatedMinutes: step.EstimatedMinutes, Tags: []string{}, Status: "backlog", Required: true})
			if e != nil {
				return v, e
			}
			step.Title = valid.Title
			step.Description = valid.Description
			steps[i] = step
		}
		var zone string
		if e = tx.QueryRow(r.Context(), "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&zone); e != nil {
			return v, e
		}
		if in.Timezone != zone {
			return v, privateapi.Problem(409, "RECOVERY_CONTEXT_CHANGED", "Часовой пояс изменился. Обновите и подтвердите показанные условия")
		}
		loc, e := time.LoadLocation(zone)
		if e != nil {
			return v, e
		}
		start := now.In(loc).Format("2006-01-02")
		end := now.In(loc).AddDate(0, 0, v.ChallengeRules.Days-1).Format("2006-01-02")
		goal, e := s.recovery.goals.CreateInTx(r.Context(), tx, owner, goals.Create{Input: goals.Input{Title: "Возвращение в ритм", Criteria: "Выполнить три выбранных небольших шага", AreaID: in.AreaID, StartDate: &start, TargetDate: &end, ProgressType: "quests", Target: 1}, State: "active"})
		if e != nil {
			return v, e
		}
		for _, step := range steps {
			q, e := s.recovery.quests.CreateInTx(r.Context(), tx, owner, quests.Input{Title: step.Title, Description: step.Description, AreaID: in.AreaID, Priority: "normal", Difficulty: 1, EstimatedMinutes: step.EstimatedMinutes, Tags: []string{}, Status: "backlog", Required: true})
			if e != nil {
				return v, e
			}
			goal, e = s.recovery.goals.AttachNewQuestInTx(r.Context(), tx, owner, goal, q)
			if e != nil {
				return v, e
			}
			result.QuestIDs = append(result.QuestIDs, q.ID)
		}
		run, e := s.challenges.CreateInTx(r.Context(), tx, owner, challenges.CreateInput{OperationID: v.ID, Input: challenges.RuleInput{Config: v.ChallengeRules, Name: "Возвращение: три небольших шага", StartsOn: start, EndsOn: end, AreaID: in.AreaID, GoalID: &goal.ID, Rejoin: v.Rejoin}})
		if e != nil {
			return v, e
		}
		if _, e = s.challenges.JoinInTx(r.Context(), tx, owner, run.ID, challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}); e != nil {
			return v, e
		}
		result.GoalID = &goal.ID
		result.RunID = &run.ID
		result.Steps = steps
		state = "accepted"
	} else if len(in.Steps) > 0 {
		return v, privateapi.Invalid("При отказе список шагов пуст")
	}
	body, e := json.Marshal(result)
	if e != nil {
		return v, e
	}
	v, e = scanOffer(tx.QueryRow(r.Context(), "UPDATE recovery_offers SET state=$3,request=$4,result=$5,decided_at=$6,version=version+1 WHERE owner_id=$1 AND id=$2 RETURNING "+offerColumns, owner, id, state, request, body, now))
	if e != nil {
		return v, e
	}
	return v, s.offerEvent(r.Context(), tx, owner, v)
}

type RecoveryHistory struct {
	Items []RecoveryOffer `json:"items"`
	Next  string          `json:"next"`
}

func (s *Service) recoveryHistory(r *http.Request, tx pgx.Tx, owner string) (RecoveryHistory, error) {
	out := RecoveryHistory{Items: []RecoveryOffer{}}
	var at *time.Time
	var id *string
	if before := r.URL.Query().Get("before"); before != "" {
		v, e := s.getOffer(r.Context(), tx, owner, before)
		if e != nil {
			return out, e
		}
		at = &v.OfferedAt
		id = &v.ID
	}
	rows, e := tx.Query(r.Context(), "SELECT "+offerColumns+" FROM recovery_offers WHERE owner_id=$1 AND ($2::timestamptz IS NULL OR (offered_at,id)<($2,$3::uuid)) ORDER BY offered_at DESC,id DESC LIMIT 26", owner, at, id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanOffer(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		out.Next = out.Items[24].ID
	}
	return out, nil
}
func (s *Service) mountRecovery(r chi.Router) {
	r.Get("/api/v1/recovery/comeback", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.recoveryOffer(ctx, tx, owner)
		})
		s.Reply(w, r, out, e)
	})
	r.Get("/api/v1/recovery/comeback/history", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.recoveryHistory(r, tx, owner)
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/recovery/comeback/{id}/decide", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in RecoveryDecision) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.decideRecovery(r, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
}
