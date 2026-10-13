package challenges

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type RuleInput struct {
	Config
	Name     string  `json:"name"`
	StartsOn string  `json:"starts_on"`
	EndsOn   string  `json:"ends_on"`
	AreaID   *string `json:"area_id"`
	GoalID   *string `json:"goal_id"`
	HabitID  *string `json:"habit_id"`
	Rejoin   bool    `json:"rejoin"`
}
type Rules struct {
	ScoringVersion string `json:"scoring_version"`
	SourcePolicy   string `json:"source_policy"`
	EconomyVersion string `json:"economy_version"`
	RewardXP       int64  `json:"reward_xp"`
	RuleInput
	Timezone     string `json:"timezone"`
	Visibility   string `json:"visibility"`
	Confirmation string `json:"confirmation"`
}
type CreateInput struct {
	OperationID string    `json:"operation_id"`
	TemplateID  *string   `json:"template_id"`
	Input       RuleInput `json:"input"`
}
type Run struct {
	ID             string          `json:"id"`
	Rules          Rules           `json:"rules"`
	RulesHash      string          `json:"rules_hash"`
	CatalogID      *string         `json:"catalog_id"`
	CatalogVersion string          `json:"catalog_version"`
	State          string          `json:"state"`
	Version        int64           `json:"version"`
	StartsAt       time.Time       `json:"starts_at"`
	EndsAt         time.Time       `json:"ends_at"`
	CreatedAt      time.Time       `json:"created_at"`
	FinalizedAt    *time.Time      `json:"finalized_at"`
	FinalSnapshot  json.RawMessage `json:"final_snapshot"`
}

const runColumns = "id::text,rules,rules_hash,catalog_id,catalog_version,state,version,starts_at,ends_at,created_at,finalized_at,final_snapshot"

func scanRun(row pgx.Row) (Run, error) {
	var out Run
	var data []byte
	e := row.Scan(&out.ID, &data, &out.RulesHash, &out.CatalogID, &out.CatalogVersion, &out.State, &out.Version, &out.StartsAt, &out.EndsAt, &out.CreatedAt, &out.FinalizedAt, &out.FinalSnapshot)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Испытание не найдено")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(data, &out.Rules)
	return out, e
}
func (s *Service) getRun(ctx context.Context, tx pgx.Tx, owner, id string) (Run, error) {
	if !privateapi.UUID(id) {
		return Run{}, privateapi.Problem(404, "NOT_FOUND", "Испытание не найдено")
	}
	return scanRun(tx.QueryRow(ctx, "SELECT "+runColumns+" FROM challenge_runs WHERE owner_id=$1 AND id=$2", owner, id))
}
func validateRules(in RuleInput, zone string, now time.Time) (Rules, time.Time, time.Time, error) {
	out := Rules{RuleInput: in, Timezone: zone, Visibility: "private", Confirmation: "automatic", ScoringVersion: ScoringVersion, SourcePolicy: sourcePolicy(in.Metric), EconomyVersion: progression.Version, RewardXP: progression.BaseXP("challenge", 0)}
	if in.Metric == "manual" {
		out.Confirmation = "self_report"
	}
	out.Name = strings.TrimSpace(in.Name)
	if e := ValidateConfig(in.Config); e != nil {
		return out, time.Time{}, time.Time{}, e
	}
	if privateapi.Day(in.StartsOn) != nil || privateapi.Day(in.EndsOn) != nil || out.Name == "" || utf8.RuneCountInString(out.Name) > 200 || strings.IndexFunc(out.Name, unicode.IsControl) >= 0 {
		return out, time.Time{}, time.Time{}, privateapi.Invalid("Укажите название до 200 символов и календарные даты")
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return out, time.Time{}, time.Time{}, e
	}
	today := now.In(loc).Format("2006-01-02")
	latest := now.In(loc).AddDate(1, 0, 0).Format("2006-01-02")
	from, _ := time.Parse("2006-01-02", in.StartsOn)
	to, _ := time.Parse("2006-01-02", in.EndsOn)
	days := int(to.Sub(from)/(24*time.Hour)) + 1
	if in.StartsOn < today || in.StartsOn > latest || days != in.Days || days < 1 || days > 365 {
		return out, time.Time{}, time.Time{}, privateapi.Invalid("Начало — сегодня или будущий день; срок до 365 дней и должен совпадать с настройкой")
	}
	for _, id := range []*string{in.AreaID, in.GoalID, in.HabitID} {
		if id != nil && !privateapi.UUID(*id) {
			return out, time.Time{}, time.Time{}, privateapi.Invalid("Проверьте источники прогресса")
		}
	}
	if (in.Metric == "habits" && in.HabitID == nil) || (in.Metric != "habits" && in.HabitID != nil) || (in.GoalID != nil && in.Metric != "tasks" && in.Metric != "focus" && in.Metric != "stages") || (in.Metric == "reviews" && in.AreaID != nil) || (in.Metric == "stages" && in.GoalID == nil) {
		return out, time.Time{}, time.Time{}, privateapi.Invalid("Выбранные источники не подходят к метрике")
	}
	starts, e := time.ParseInLocation("2006-01-02", in.StartsOn, loc)
	if e != nil {
		return out, time.Time{}, time.Time{}, e
	}
	ends, e := time.ParseInLocation("2006-01-02", in.EndsOn, loc)
	return out, starts.UTC(), ends.AddDate(0, 0, 1).UTC(), e
}
func (s *Service) createRun(ctx context.Context, tx pgx.Tx, owner string, in CreateInput) (Run, error) {
	if !privateapi.UUID(in.OperationID) {
		return Run{}, privateapi.Invalid("Нужен UUID операции")
	}
	request, e := json.Marshal(in)
	if e != nil {
		return Run{}, e
	}
	var same bool
	e = tx.QueryRow(ctx, "SELECT request=$3::jsonb FROM challenge_runs WHERE owner_id=$1 AND id=$2", owner, in.OperationID, request).Scan(&same)
	if e == nil {
		if !same {
			return Run{}, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Этот запуск создан с другим запросом")
		}
		return s.getRun(ctx, tx, owner, in.OperationID)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Run{}, e
	}
	var count int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM challenge_runs WHERE owner_id=$1 AND state='open'", owner).Scan(&count); e != nil {
		return Run{}, e
	}
	if count >= 50 {
		return Run{}, privateapi.Problem(409, "RUN_LIMIT", "Сначала завершите один из 50 открытых запусков")
	}
	version := "custom-v1"
	if in.TemplateID != nil {
		catalog, e := ReadCatalog()
		if e != nil {
			return Run{}, e
		}
		found := false
		for _, template := range catalog.Challenges {
			if template.ID == *in.TemplateID {
				found = true
				if strings.TrimSpace(in.Input.Name) == "" {
					in.Input.Name = template.Name
				}
				break
			}
		}
		if !found {
			return Run{}, privateapi.Problem(404, "NOT_FOUND", "Шаблон испытания не найден")
		}
		version = catalog.Version
	}
	var zone string
	if e = tx.QueryRow(ctx, "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&zone); e != nil {
		return Run{}, e
	}
	rules, starts, ends, e := validateRules(in.Input, zone, s.Clock.Now())
	if e != nil {
		return Run{}, e
	}
	// Table names are fixed server constants; only owned active sources qualify.
	for _, source := range []struct {
		id    *string
		query string
	}{{in.Input.AreaID, "SELECT EXISTS(SELECT 1 FROM life_areas WHERE owner_id=$1 AND id=$2 AND NOT archived)"}, {in.Input.GoalID, "SELECT EXISTS(SELECT 1 FROM goals WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL AND state NOT IN('cancelled','achieved'))"}, {in.Input.HabitID, "SELECT EXISTS(SELECT 1 FROM habits WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL)"}} {
		if source.id == nil {
			continue
		}
		var exists bool
		if e = tx.QueryRow(ctx, source.query, owner, *source.id).Scan(&exists); e != nil {
			return Run{}, e
		}
		if !exists {
			return Run{}, privateapi.Problem(404, "NOT_FOUND", "Источник не найден")
		}
	}
	body, e := json.Marshal(rules)
	if e != nil {
		return Run{}, e
	}
	hash := sha256.Sum256(body)
	return scanRun(tx.QueryRow(ctx, `INSERT INTO challenge_runs(owner_id,id,request,rules,rules_hash,catalog_id,catalog_version,starts_at,ends_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+runColumns, owner, in.OperationID, request, body, hex.EncodeToString(hash[:]), in.TemplateID, version, starts, ends, s.Clock.Now().UTC()))
}

type RunPage struct {
	Items []Run  `json:"items"`
	Next  string `json:"next"`
}

func (s *Service) listRuns(r *http.Request, tx pgx.Tx, owner string) (RunPage, error) {
	out := RunPage{Items: []Run{}}
	before := r.URL.Query().Get("before")
	var at *time.Time
	var id *string
	if before != "" {
		cursor, e := s.getRun(r.Context(), tx, owner, before)
		if e != nil {
			return out, e
		}
		at = &cursor.CreatedAt
		id = &cursor.ID
	}
	rows, e := tx.Query(r.Context(), "SELECT "+runColumns+" FROM challenge_runs WHERE owner_id=$1 AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3::uuid)) ORDER BY created_at DESC,id DESC LIMIT 26", owner, at, id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		item, e := scanRun(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
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
func (s *Service) mountRuns(r chi.Router) {
	r.Post("/api/v1/challenges/runs", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in CreateInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.createRun(ctx, tx, owner, in)
			})
		})
	})
	r.Get("/api/v1/challenges/runs", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.listRuns(r, tx, owner) })
		s.Reply(w, r, out, e)
	})
	r.Get("/api/v1/challenges/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.getRun(ctx, tx, owner, chi.URLParam(r, "id"))
		})
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/challenges/runs/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			if _, e := s.getRun(ctx, tx, owner, chi.URLParam(r, "id")); e != nil {
				return nil, e
			}
			return nil, privateapi.Problem(409, "RULES_LOCKED", "Правила уже зафиксированы. Создайте новый запуск")
		})
		s.Reply(w, r, out, e)
	})
}
