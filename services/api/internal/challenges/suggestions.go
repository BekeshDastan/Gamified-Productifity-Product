package challenges

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Service) WithQuests(q *quests.Service) *Service { s.quests = q; return s }

type Suggestion struct {
	Template QuestTemplate `json:"template"`
	QuestID  *string       `json:"quest_id"`
	Reason   string        `json:"reason"`
}
type Suggestions struct {
	Period           string       `json:"period"`
	StartsOn         string       `json:"starts_on"`
	Timezone         string       `json:"timezone"`
	AvailableMinutes int          `json:"available_minutes"`
	AreaID           *string      `json:"area_id"`
	Items            []Suggestion `json:"items"`
}
type AcceptSuggestion struct {
	Period           string  `json:"period"`
	StartsOn         string  `json:"starts_on"`
	TemplateID       string  `json:"template_id"`
	AvailableMinutes int     `json:"available_minutes"`
	AreaID           *string `json:"area_id"`
}

func (s *Service) suggestions(ctx context.Context, tx pgx.Tx, owner, period string, budget int, area *string) (Suggestions, error) {
	out := Suggestions{Period: period, AvailableMinutes: budget, AreaID: area, Items: []Suggestion{}}
	if (period != "daily" && period != "weekly") || budget < 5 || budget > 240 {
		return out, privateapi.Invalid("Выберите день или неделю и от 5 до 240 доступных минут")
	}
	var prefs struct {
		Timezone  string `json:"timezone"`
		WeekStart int    `json:"week_start"`
	}
	var data []byte
	if e := tx.QueryRow(ctx, "SELECT preferences FROM user_settings WHERE owner_id=$1", owner).Scan(&data); e != nil {
		return out, e
	}
	if e := json.Unmarshal(data, &prefs); e != nil {
		return out, e
	}
	loc, e := time.LoadLocation(prefs.Timezone)
	if e != nil {
		return out, e
	}
	out.Timezone = prefs.Timezone
	now := s.Clock.Now().In(loc)
	if period == "weekly" {
		now = now.AddDate(0, 0, -(int(now.Weekday())-prefs.WeekStart+7)%7)
	}
	out.StartsOn = now.Format("2006-01-02")
	hints := []string{}
	rows, e := tx.Query(ctx, "SELECT id::text,name FROM life_areas WHERE owner_id=$1 AND NOT archived ORDER BY id", owner)
	if e != nil {
		return out, e
	}
	found := area == nil
	for rows.Next() {
		var id, name string
		if e = rows.Scan(&id, &name); e != nil {
			rows.Close()
			return out, e
		}
		if area == nil || *area == id {
			found = true
			hints = append(hints, strings.ToLower(name))
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if !found {
		return out, privateapi.Problem(404, "NOT_FOUND", "Направление не найдено")
	}
	catalog, e := ReadCatalog()
	if e != nil {
		return out, e
	}
	rank := func(q QuestTemplate) int {
		for _, hint := range hints {
			if strings.Contains(strings.ToLower(q.AreaHint), hint) || strings.Contains(hint, strings.ToLower(q.AreaHint)) {
				return 0
			}
		}
		return 1
	}
	sort.SliceStable(catalog.Quests, func(i, j int) bool {
		a, b := rank(catalog.Quests[i]), rank(catalog.Quests[j])
		if a != b {
			return a < b
		}
		return catalog.Quests[i].ID < catalog.Quests[j].ID
	})
	limit := 5
	if period == "weekly" {
		limit = 10
	}
	for _, q := range catalog.Quests {
		if q.EstimatedMinutes > budget {
			continue
		}
		var id string
		e = tx.QueryRow(ctx, "SELECT quest_id::text FROM accepted_quest_suggestions WHERE owner_id=$1 AND period=$2 AND starts_on=$3 AND template_id=$4", owner, period, out.StartsOn, q.ID).Scan(&id)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return out, e
		}
		v := Suggestion{Template: q, Reason: "Оценка помещается в выбранное время"}
		if rank(q) == 0 {
			v.Reason += "; соответствует активному направлению"
		}
		if id != "" {
			v.QuestID = &id
		}
		out.Items = append(out.Items, v)
		if len(out.Items) >= limit {
			break
		}
	}
	return out, nil
}

type AcceptedSuggestion struct {
	QuestID string `json:"quest_id"`
}

func (s *Service) acceptSuggestion(ctx context.Context, tx pgx.Tx, owner string, in AcceptSuggestion) (AcceptedSuggestion, error) {
	out := AcceptedSuggestion{}
	if privateapi.Day(in.StartsOn) != nil {
		return out, privateapi.Invalid("Проверьте период")
	}
	if (in.Period != "daily" && in.Period != "weekly") || in.TemplateID == "" || len(in.TemplateID) > 100 || strings.ContainsRune(in.TemplateID, 0) || (in.AreaID != nil && !privateapi.UUID(*in.AreaID)) {
		return out, privateapi.Invalid("Проверьте период, шаблон и направление")
	}
	request, e := json.Marshal(in)
	if e != nil {
		return out, e
	}
	var same bool
	e = tx.QueryRow(ctx, "SELECT quest_id::text,request=$5::jsonb FROM accepted_quest_suggestions WHERE owner_id=$1 AND period=$2 AND starts_on=$3 AND template_id=$4", owner, in.Period, in.StartsOn, in.TemplateID, request).Scan(&out.QuestID, &same)
	if e == nil {
		if !same {
			return out, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Это предложение уже принято с другими настройками")
		}
		return out, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	list, e := s.suggestions(ctx, tx, owner, in.Period, in.AvailableMinutes, in.AreaID)
	if e != nil {
		return out, e
	}
	if list.StartsOn != in.StartsOn {
		return out, privateapi.Problem(409, "SUGGESTION_EXPIRED", "Период изменился. Обновите предложения")
	}
	var template *QuestTemplate
	for _, v := range list.Items {
		if v.Template.ID == in.TemplateID {
			copy := v.Template
			template = &copy
			break
		}
	}
	if template == nil {
		return out, privateapi.Invalid("Обновите предложения для выбранного времени")
	}
	if s.quests == nil {
		return out, privateapi.Problem(503, "UNAVAILABLE", "Создание квеста недоступно")
	}
	q, e := s.quests.CreateInTx(ctx, tx, owner, quests.Input{Title: template.Name, Description: template.Description, AreaID: in.AreaID, Priority: "normal", Difficulty: template.Difficulty, EstimatedMinutes: template.EstimatedMinutes, Tags: []string{}, Status: "backlog", Required: true})
	if e != nil {
		return out, e
	}
	out.QuestID = q.ID
	_, e = tx.Exec(ctx, "INSERT INTO accepted_quest_suggestions(owner_id,period,starts_on,template_id,catalog_version,request,quest_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", owner, in.Period, in.StartsOn, in.TemplateID, CatalogVersion, request, q.ID, s.Clock.Now())
	return out, e
}
func (s *Service) mountSuggestions(r chi.Router) {
	r.Get("/api/v1/challenges/suggestions", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			q := r.URL.Query()
			budget, e := strconv.Atoi(q.Get("available_minutes"))
			if e != nil {
				return nil, privateapi.Invalid("Укажите доступные минуты")
			}
			var area *string
			if id := q.Get("area_id"); id != "" {
				if !privateapi.UUID(id) {
					return nil, privateapi.Invalid("Проверьте направление")
				}
				area = &id
			}
			return s.suggestions(ctx, tx, owner, q.Get("period"), budget, area)
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/challenges/suggestions/accept", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in AcceptSuggestion) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.acceptSuggestion(ctx, tx, owner, in)
			})
		})
	})
}
