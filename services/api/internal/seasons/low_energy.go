package seasons

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"strings"
	"time"
)

type planningServices struct {
	plans    *planning.Service
	calendar *calendar.Service
}

func (s *Service) WithPlanning(p *planning.Service, c *calendar.Service) *Service {
	s.planning = &planningServices{p, c}
	return s
}

type LowEnergyInput struct {
	Date           string `json:"date"`
	MinimumMinutes int    `json:"minimum_minutes"`
}
type LowEnergyCandidate struct {
	planning.Item
	Required bool   `json:"required"`
	Proposed bool   `json:"proposed"`
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason"`
}
type LowEnergyPreview struct {
	LowEnergyInput
	ID          string               `json:"id"`
	ExpiresAt   time.Time            `json:"expires_at"`
	Timezone    string               `json:"timezone"`
	Original    planning.Plan        `json:"original"`
	Candidates  []LowEnergyCandidate `json:"candidates"`
	Commitments []calendar.Event     `json:"commitments"`
	SourceHash  string               `json:"source_hash"`
	AppliedAt   *time.Time           `json:"applied_at"`
	Result      *planning.Plan       `json:"result"`
}
type LowEnergyApply struct {
	Confirm        bool     `json:"confirm"`
	Reason         string   `json:"reason"`
	OptionalIDs    []string `json:"optional_ids"`
	ConfirmUnknown bool     `json:"confirm_unknown"`
}

func hashSource(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) lowEnergySource(ctx context.Context, tx pgx.Tx, owner string, in LowEnergyInput) (LowEnergyPreview, error) {
	out := LowEnergyPreview{LowEnergyInput: in, Candidates: []LowEnergyCandidate{}, Commitments: []calendar.Event{}}
	if privateapi.Day(in.Date) != nil || in.MinimumMinutes < 0 || in.MinimumMinutes > 240 {
		return out, privateapi.Invalid("Дата и минимальный объём: 0–240 минут")
	}
	var settingsVersion int64
	var settingsTimezone string
	if e := tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC'),version FROM user_settings WHERE owner_id=$1`, owner).Scan(&settingsTimezone, &settingsVersion); e != nil {
		return out, e
	}
	var e error
	out.Original, e = s.planning.plans.ViewInTx(ctx, tx, owner, in.Date)
	if e != nil {
		return out, e
	}
	if out.Original.Version == 0 {
		return out, privateapi.Problem(409, "PLAN_REQUIRED", "Сначала составьте план дня")
	}
	out.Timezone = out.Original.Timezone
	zone, e := time.LoadLocation(out.Timezone)
	if e != nil {
		return out, e
	}
	today := s.Clock.Now().In(zone).Format("2006-01-02")
	if in.Date < today || in.Date > s.Clock.Now().In(zone).AddDate(1, 0, 0).Format("2006-01-02") {
		return out, privateapi.Invalid("Облегчить можно сегодняшний или будущий день в пределах года")
	}
	out.Commitments, e = s.planning.calendar.DayCommitmentsInTx(ctx, tx, owner, in.Date, out.Timezone)
	if e != nil {
		return out, e
	}
	fixed := map[string]bool{}
	for _, b := range out.Commitments {
		if b.QuestID != nil {
			fixed[*b.QuestID] = true
		}
	}
	d, _ := time.ParseInLocation("2006-01-02", in.Date, zone)
	end := d.AddDate(0, 0, 1)
	versions := []any{}
	tops := map[string]bool{}
	for _, id := range out.Original.TopIDs {
		tops[id] = true
	}
	for _, item := range out.Original.Items {
		var deadline *time.Time
		var planned *string
		var leaf bool
		if e = tx.QueryRow(ctx, `SELECT deadline,planned_date::text,NOT EXISTS(SELECT 1 FROM quests child WHERE child.owner_id=q.owner_id AND child.parent_id=q.id AND child.archived_at IS NULL) FROM quests q WHERE owner_id=$1 AND id=$2`, owner, item.ID).Scan(&deadline, &planned, &leaf); e != nil {
			return out, e
		}
		c := LowEnergyCandidate{Item: item, Eligible: !item.Archived && leaf && (item.Status == "todo" || item.Status == "backlog" || item.Status == "in_progress") && (planned == nil || *planned == in.Date), Reason: "optional"}
		if !c.Eligible {
			c.Reason = "no_longer_pending"
		} else if fixed[item.ID] {
			c.Required = true
			c.Reason = "fixed_commitment"
		} else if deadline != nil && deadline.Before(end) {
			c.Required = true
			c.Reason = "deadline"
		} else if item.EstimatedMinutes == 0 {
			c.Reason = "unknown_estimate"
		}
		c.Proposed = c.Required
		out.Candidates = append(out.Candidates, c)
		versions = append(versions, []any{item.ID, item.Version, deadline, planned, leaf})
	}
	order := make([]int, 0, len(out.Candidates))
	for i, c := range out.Candidates {
		if c.Eligible && !c.Required && c.EstimatedMinutes > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := out.Candidates[order[i]], out.Candidates[order[j]]
		if tops[a.ID] != tops[b.ID] {
			return tops[a.ID]
		}
		return a.Weight > b.Weight
	})
	remaining := in.MinimumMinutes
	for _, i := range order {
		c := &out.Candidates[i]
		if c.EstimatedMinutes <= remaining {
			c.Proposed = true
			remaining -= c.EstimatedMinutes
		}
	}
	out.SourceHash = hashSource([]any{out.Original.Version, out.Original.Timezone, settingsVersion, settingsTimezone, versions, out.Commitments})
	return out, nil
}
func (s *Service) lowEnergyPreview(ctx context.Context, tx pgx.Tx, owner string, in LowEnergyInput) (LowEnergyPreview, error) {
	v, e := s.lowEnergySource(ctx, tx, owner, in)
	if e != nil {
		return v, e
	}
	v.ExpiresAt = s.Clock.Now().Add(10 * time.Minute)
	b, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO low_energy_previews(owner_id,snapshot,created_at,expires_at) VALUES($1,$2,$3,$4) RETURNING id::text`, owner, b, s.Clock.Now(), v.ExpiresAt).Scan(&v.ID)
	return v, e
}
func (s *Service) getLowEnergy(ctx context.Context, tx pgx.Tx, owner, id string) (LowEnergyPreview, []byte, error) {
	v := LowEnergyPreview{}
	if !privateapi.UUID(id) {
		return v, nil, privateapi.Problem(404, "NOT_FOUND", "Предпросмотр не найден")
	}
	var b, request, result []byte
	var at *time.Time
	e := tx.QueryRow(ctx, `SELECT snapshot,apply_request,result,applied_at FROM low_energy_previews WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&b, &request, &result, &at)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, nil, privateapi.Problem(404, "NOT_FOUND", "Предпросмотр не найден")
	}
	if e != nil {
		return v, nil, e
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return v, nil, e
	}
	v.ID = id
	v.AppliedAt = at
	if result != nil {
		v.Result = &planning.Plan{}
		e = json.Unmarshal(result, v.Result)
	}
	return v, request, e
}
func (s *Service) applyLowEnergy(r *http.Request, ctx context.Context, tx pgx.Tx, owner, id string, in LowEnergyApply) (LowEnergyPreview, error) {
	v, old, e := s.getLowEnergy(ctx, tx, owner, id)
	if e != nil {
		return v, e
	}
	if strings.ContainsRune(in.Reason, 0) {
		return v, privateapi.Invalid("Недопустимый символ")
	}
	b, e := json.Marshal(in)
	if e != nil {
		return v, e
	}
	if v.AppliedAt != nil {
		var same bool
		e = tx.QueryRow(ctx, `SELECT apply_request=$3::jsonb FROM low_energy_previews WHERE owner_id=$1 AND id=$2`, owner, id, b).Scan(&same)
		_ = old
		if e != nil {
			return v, e
		}
		if !same {
			return v, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "План уже применён с другим выбором")
		}
		return v, nil
	}
	if !in.Confirm || !validText(in.Reason, 1, 2000) {
		return v, privateapi.Invalid("Подтвердите изменение и укажите причину")
	}
	if !s.Clock.Now().Before(v.ExpiresAt) {
		return v, privateapi.Problem(409, "PREVIEW_EXPIRED", "Предпросмотр устарел. Составьте новый")
	}
	fresh, e := s.lowEnergySource(ctx, tx, owner, v.LowEnergyInput)
	if e != nil {
		return v, e
	}
	if fresh.SourceHash != v.SourceHash {
		return v, privateapi.Problem(409, "SOURCE_CHANGED", "План, задачи, часовой пояс или события изменились. Обновите предпросмотр")
	}
	selected := map[string]bool{}
	for _, id := range in.OptionalIDs {
		if !privateapi.UUID(id) || selected[id] {
			return v, privateapi.Invalid("Проверьте выбранные задачи")
		}
		selected[id] = true
	}
	next := v.Original.Input
	next.QuestIDs = []string{}
	next.TopIDs = []string{}
	minutes := 0
	for _, c := range fresh.Candidates {
		chosen := selected[c.ID]
		delete(selected, c.ID)
		if chosen && (!c.Eligible || c.Required) {
			return v, privateapi.Invalid("Выберите только доступные гибкие задачи")
		}
		if chosen {
			if c.EstimatedMinutes == 0 && !in.ConfirmUnknown {
				return v, privateapi.Invalid("Подтвердите неопределённость задач без оценки времени")
			}
			minutes += c.EstimatedMinutes
		}
		if c.Eligible && (c.Required || chosen) {
			next.QuestIDs = append(next.QuestIDs, c.ID)
			for _, top := range v.Original.TopIDs {
				if top == c.ID {
					next.TopIDs = append(next.TopIDs, top)
				}
			}
		}
	}
	if len(selected) > 0 || minutes > v.MinimumMinutes {
		return v, privateapi.Invalid("Гибкие задачи должны помещаться в выбранный объём")
	}
	result, e := s.planning.plans.MorningInTx(r, tx, owner, v.Date, next, v.Original.Version, in.Reason)
	if e != nil {
		return v, e
	}
	data, e := json.Marshal(result)
	if e != nil {
		return v, e
	}
	now := s.Clock.Now()
	_, e = tx.Exec(ctx, `UPDATE low_energy_previews SET apply_request=$3,result=$4,applied_at=$5 WHERE owner_id=$1 AND id=$2`, owner, id, b, data, now)
	v.Result = &result
	v.AppliedAt = &now
	return v, e
}
func (s *Service) mountLowEnergy(r chi.Router) {
	if s.planning == nil {
		return
	}
	r.Post("/api/v1/recovery/low-energy/preview", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in LowEnergyInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.lowEnergyPreview(ctx, tx, owner, in)
			})
		})
	})
	r.Get("/api/v1/recovery/low-energy/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			v, _, e := s.getLowEnergy(ctx, tx, owner, chi.URLParam(r, "id"))
			return v, e
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/recovery/low-energy/{id}/apply", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in LowEnergyApply) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.applyLowEnergy(r, ctx, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
}
