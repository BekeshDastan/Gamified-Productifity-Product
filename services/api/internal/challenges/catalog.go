package challenges

import (
	_ "embed"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"unicode/utf8"
)

//go:embed catalog.json
var catalogData []byte

const CatalogVersion = "challenges-v1"

type Config struct {
	Metric        string `json:"metric"`
	Unit          string `json:"unit"`
	Kind          string `json:"kind"`
	Period        string `json:"period"`
	Target        int64  `json:"target"`
	PeriodTarget  int64  `json:"period_target"`
	Days          int    `json:"days"`
	DailyCap      int64  `json:"daily_cap"`
	AllowedMisses int    `json:"allowed_misses"`
}
type Template struct {
	Config
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Audience    string `json:"audience"`
	AreaHint    string `json:"area_hint"`
	Conditions  string `json:"conditions"`
	SafeNorm    string `json:"safe_norm"`
}
type QuestTemplate struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Audience         string `json:"audience"`
	AreaHint         string `json:"area_hint"`
	EstimatedMinutes int    `json:"estimated_minutes"`
	Difficulty       int    `json:"difficulty"`
	Metric           string `json:"metric"`
	Unit             string `json:"unit"`
	Target           int    `json:"target"`
	Conditions       string `json:"conditions"`
	SafeNorm         string `json:"safe_norm"`
}
type Catalog struct {
	Version    string          `json:"version"`
	Challenges []Template      `json:"challenges"`
	Quests     []QuestTemplate `json:"quests"`
}

func UnitAllowed(metric, unit string) bool {
	switch metric {
	case "tasks":
		return unit == "tasks"
	case "habits":
		return unit == "periods"
	case "focus":
		return unit == "minutes"
	case "reviews":
		return unit == "reviews"
	case "manual":
		return unit == "count" || unit == "pages" || unit == "minutes"
	}
	return false
}
func ValidateConfig(in Config) error {
	if !UnitAllowed(in.Metric, in.Unit) || (in.Kind != "cumulative" && in.Kind != "periodic") || (in.Period != "day" && in.Period != "week") || in.Days < 1 || in.Days > 365 || in.Target < 1 || in.Target > 100000 || in.PeriodTarget < 1 || in.PeriodTarget > 100000 || in.DailyCap < 1 {
		return privateapi.Invalid("Проверьте метрику, единицу, цель и срок до 365 дней")
	}
	cap := int64(100)
	if in.Metric == "focus" || (in.Metric == "manual" && in.Unit == "minutes") {
		cap = 240
	}
	if in.Metric == "reviews" {
		cap = 1
	}
	if in.Metric == "manual" && in.Unit != "minutes" {
		cap = 1000
	}
	if in.DailyCap > cap {
		return privateapi.Invalid("Дневной предел слишком велик для выбранной метрики")
	}
	periodDays := 1
	if in.Period == "week" {
		periodDays = 7
	}
	periods := (in.Days + periodDays - 1) / periodDays
	if in.Kind == "periodic" {
		possible := in.Days / periodDays
		if remaining := in.Days % periodDays; remaining > 0 && in.PeriodTarget <= in.DailyCap*int64(remaining) {
			possible++
		}
		if in.Target > int64(possible) || in.PeriodTarget > in.DailyCap*int64(periodDays) || in.AllowedMisses != periods-int(in.Target) {
			return privateapi.Invalid("Цель периода должна помещаться в дневной предел; пропуски равны числу периодов минус цель")
		}
	} else if in.Target > int64(in.Days)*in.DailyCap || in.AllowedMisses != 0 {
		return privateapi.Invalid("Накопительная цель должна помещаться в срок и дневные пределы")
	}
	return nil
}
func ReadCatalog() (Catalog, error) {
	var out Catalog
	if e := json.Unmarshal(catalogData, &out); e != nil {
		return out, e
	}
	if out.Version != CatalogVersion || len(out.Challenges) != 12 || len(out.Quests) != 20 {
		return out, privateapi.Invalid("Каталог неполон")
	}
	seen := map[string]bool{}
	for _, t := range out.Challenges {
		if e := ValidateConfig(t.Config); e != nil {
			return out, e
		}
		if t.ID == "" || seen[t.ID] || t.Name == "" || t.Audience == "" || t.Conditions == "" || t.SafeNorm == "" {
			return out, privateapi.Invalid("Проверьте описание испытания")
		}
		seen[t.ID] = true
	}
	for _, q := range out.Quests {
		if q.ID == "" || seen[q.ID] || q.Name == "" || utf8.RuneCountInString(q.Name) > 200 || q.Audience == "" || q.Conditions == "" || q.SafeNorm == "" || q.Metric != "tasks" || q.Unit != "tasks" || q.Target != 1 || q.EstimatedMinutes < 5 || q.EstimatedMinutes > 45 || q.Difficulty < 1 || q.Difficulty > 2 || strings.ContainsRune(q.Description, 0) {
			return out, privateapi.Invalid("Проверьте шаблон квеста")
		}
		seen[q.ID] = true
	}
	return out, nil
}

type Service struct{ *privateapi.Service }

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{privateapi.New(pool, clock)}
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountRuns(r)
		s.mountMembers(r)
		r.Get("/api/v1/challenges/catalog", func(w http.ResponseWriter, r *http.Request) { out, e := ReadCatalog(); s.Reply(w, r, out, e) })
	})
}
