package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"math"
	"net/http"
	"time"
)

type BalanceTargetInput struct {
	Range
	Metric  string  `json:"metric"`
	Target  float64 `json:"target"`
	Confirm bool    `json:"confirm"`
}
type BalanceTarget struct {
	Range
	AreaID    string    `json:"area_id"`
	Metric    string    `json:"metric"`
	Target    float64   `json:"target"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}
type BalanceAxis struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Archived bool     `json:"archived"`
	Metric   string   `json:"metric"`
	Unit     string   `json:"unit"`
	Target   *float64 `json:"target"`
	Actual   *float64 `json:"actual"`
	Percent  *float64 `json:"percent"`
	Version  int64    `json:"version"`
	Sources  []Source `json:"sources"`
}
type BalanceView struct {
	Range
	Timezone          string        `json:"timezone"`
	ComputedAt        *time.Time    `json:"computed_at"`
	Stale             bool          `json:"stale"`
	SourceRevision    int64         `json:"source_revision"`
	ProjectedRevision int64         `json:"projected_revision"`
	Axes              []BalanceAxis `json:"axes"`
}

func normalizeTarget(actual, target *float64) *float64 {
	if actual == nil || target == nil || *target <= 0 || math.IsNaN(*target) || math.IsInf(*target, 0) || math.IsNaN(*actual) || math.IsInf(*actual, 0) {
		return nil
	}
	v := min(1, max(0, *actual / *target)) * 100
	return &v
}
func validateTarget(in BalanceTargetInput) error {
	if _, _, e := validateRange(in.Range); e != nil {
		return e
	}
	if !in.Confirm || math.IsNaN(in.Target) || math.IsInf(in.Target, 0) || in.Target < 0 || in.Target > 1000000 || math.Abs(in.Target*1000-math.Round(in.Target*1000)) > 0.000001 {
		return privateapi.Invalid("Подтвердите ориентир от 0 до 1 000 000, до трёх знаков после запятой")
	}
	switch in.Metric {
	case "tasks", "focus_minutes", "habit_norms":
		return nil
	}
	return privateapi.Invalid("Выберите задачи, минуты фокуса или полные нормы привычек")
}
func (s *Service) setTarget(r *http.Request, ctx context.Context, tx pgx.Tx, owner, area string, in BalanceTargetInput) (BalanceTarget, error) {
	out := BalanceTarget{}
	if !privateapi.UUID(area) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Направление не найдено")
	}
	var archived bool
	e := tx.QueryRow(ctx, `SELECT archived FROM life_areas WHERE owner_id=$1 AND id=$2`, owner, area).Scan(&archived)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Направление не найдено")
	}
	if e != nil {
		return out, e
	}
	if archived {
		return out, privateapi.Problem(409, "AREA_ARCHIVED", "Сначала восстановите направление")
	}
	if e = validateTarget(in); e != nil {
		return out, e
	}
	var version int64
	e = tx.QueryRow(ctx, `SELECT version FROM personal_area_targets WHERE owner_id=$1 AND area_id=$2 AND starts_on=$3::text::date AND ends_before=$4::text::date`, owner, area, in.From, in.To).Scan(&version)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if p := privateapi.MatchVersion(r, version); p != nil {
		return out, p
	}
	out = BalanceTarget{Range: in.Range, AreaID: area, Metric: in.Metric, Target: in.Target, Version: version + 1, UpdatedAt: s.Clock.Now()}
	_, e = tx.Exec(ctx, `INSERT INTO personal_area_targets(owner_id,area_id,starts_on,ends_before,metric,target,updated_at) VALUES($1,$2,$3::text::date,$4::text::date,$5,$6,$7) ON CONFLICT(owner_id,area_id,starts_on,ends_before) DO UPDATE SET metric=excluded.metric,target=excluded.target,version=personal_area_targets.version+1,updated_at=excluded.updated_at`, owner, area, in.From, in.To, in.Metric, in.Target, out.UpdatedAt)
	if e != nil {
		return out, e
	}
	b, e := json.Marshal(out)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO personal_area_target_events(owner_id,area_id,starts_on,ends_before,version,snapshot,created_at) VALUES($1,$2,$3::text::date,$4::text::date,$5,$6,$7)`, owner, area, in.From, in.To, out.Version, b, out.UpdatedAt)
	return out, e
}
func (s *Service) balance(ctx context.Context, tx pgx.Tx, owner string, in Range) (BalanceView, error) {
	v, e := s.read(ctx, tx, owner, in, false)
	if e != nil {
		return BalanceView{}, e
	}
	return s.balanceFrom(ctx, tx, owner, v)
}
func (s *Service) balanceFrom(ctx context.Context, tx pgx.Tx, owner string, v View) (BalanceView, error) {
	out := BalanceView{Range: v.Range, Timezone: v.Timezone, ComputedAt: v.ComputedAt, Stale: v.Stale, SourceRevision: v.SourceRevision, ProjectedRevision: v.ProjectedRevision, Axes: []BalanceAxis{}}
	rows, e := tx.Query(ctx, `SELECT a.id::text,a.name,a.archived,COALESCE(t.metric,'tasks'),t.target::float8,COALESCE(t.version,0) FROM life_areas a LEFT JOIN personal_area_targets t ON t.owner_id=a.owner_id AND t.area_id=a.id AND t.starts_on=$2::text::date AND t.ends_before=$3::text::date WHERE a.owner_id=$1 ORDER BY a.archived,lower(a.name),a.id`, owner, v.From, v.To)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		axis := BalanceAxis{Sources: []Source{}}
		if e = rows.Scan(&axis.ID, &axis.Name, &axis.Archived, &axis.Metric, &axis.Target, &axis.Version); e != nil {
			rows.Close()
			return out, e
		}
		axis.Unit = map[string]string{"tasks": "tasks", "focus_minutes": "minutes", "habit_norms": "periods"}[axis.Metric]
		value := 0.0
		known := true
		for _, day := range v.Days {
			if axis.Metric == "habit_norms" {
				known = known && day.HabitsComplete
				for _, h := range day.Habits {
					if h.AreaID != nil && *h.AreaID == axis.ID && !h.Excluded && h.Amount >= h.Target {
						value++
						axis.Sources = append(axis.Sources, Source{Date: day.Date, Kind: "habit", ID: h.PeriodID, Name: h.Name, URL: h.URL, At: h.End, Value: 1, Unit: "periods", AreaID: h.AreaID})
					}
				}
				continue
			}
			for _, source := range day.Sources {
				if source.AreaID == nil || *source.AreaID != axis.ID {
					continue
				}
				if axis.Metric == "tasks" && source.Kind == "task" {
					value++
					axis.Sources = append(axis.Sources, source)
				} else if axis.Metric == "focus_minutes" && source.Kind == "focus" {
					value += source.Value / 60
					axis.Sources = append(axis.Sources, source)
				}
			}
		}
		if known {
			axis.Actual = &value
		}
		axis.Percent = normalizeTarget(axis.Actual, axis.Target)
		out.Axes = append(out.Axes, axis)
	}
	e = rows.Err()
	rows.Close()
	return out, e
}
func (s *Service) mountBalance(r chi.Router) {
	r.Get("/api/v1/analytics/balance", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.balance(ctx, tx, owner, Range{r.URL.Query().Get("from"), r.URL.Query().Get("to")})
		})
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/analytics/targets/{area_id}", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in BalanceTargetInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.setTarget(r, ctx, tx, owner, chi.URLParam(r, "area_id"), in)
			})
		})
	})
}
