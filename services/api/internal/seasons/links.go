package seasons

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func (s *Service) WithChallenges(c *challenges.Service) *Service { s.challenges = c; return s }

type Source struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Metric          string          `json:"metric"`
	Value           *float64        `json:"value"`
	Target          float64         `json:"target"`
	Unit            string          `json:"unit"`
	Percent         *float64        `json:"percent"`
	State           string          `json:"state"`
	Result          *string         `json:"result"`
	Version         int64           `json:"version"`
	StagesCompleted int             `json:"stages_completed"`
	StagesTotal     int             `json:"stages_total"`
	URL             string          `json:"url"`
	Baseline        json.RawMessage `json:"baseline"`
}
type SeasonBoss struct {
	BossID        string  `json:"boss_id"`
	GoalID        string  `json:"goal_id"`
	Name          string  `json:"name"`
	Month         string  `json:"month"`
	Timezone      string  `json:"timezone"`
	State         string  `json:"state"`
	Progress      float64 `json:"progress"`
	SourceChanged bool    `json:"source_changed"`
}
type Overview struct {
	Season     Season       `json:"season"`
	MainGoalID *string      `json:"main_goal_id"`
	Goals      []Source     `json:"goals"`
	Challenges []Source     `json:"challenges"`
	Bosses     []SeasonBoss `json:"bosses"`
}
type LinksInput struct {
	GoalIDs      []string `json:"goal_ids"`
	MainGoalID   *string  `json:"main_goal_id"`
	ChallengeIDs []string `json:"challenge_ids"`
}

func (s *Service) goalSource(ctx context.Context, tx pgx.Tx, owner, id string) (Source, error) {
	v := Source{ID: id, URL: "/app/goals/" + id}
	var current, percent float64
	e := tx.QueryRow(ctx, "SELECT payload->>'title',payload->>'progress_type',current,(payload->>'target')::double precision,COALESCE(payload->>'unit',''),progress::double precision,state,version FROM goals WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL", owner, id).Scan(&v.Name, &v.Metric, &current, &v.Target, &v.Unit, &percent, &v.State, &v.Version)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, privateapi.Problem(404, "NOT_FOUND", "Цель не найдена или архивирована")
	}
	if e != nil {
		return v, e
	}
	v.Value = &current
	v.Percent = &percent
	if v.Metric != "metric" {
		v.Value = &percent
		v.Target = 100
		v.Unit = "%"
	}
	e = tx.QueryRow(ctx, "SELECT count(*) FILTER(WHERE challenge_completed),count(*) FROM milestones WHERE owner_id=$1 AND goal_id=$2", owner, id).Scan(&v.StagesCompleted, &v.StagesTotal)
	return v, e
}
func (s *Service) challengeSource(ctx context.Context, tx pgx.Tx, owner, id string) (Source, error) {
	v := Source{ID: id, URL: "/app/challenges?run_id=" + id}
	var body []byte
	e := tx.QueryRow(ctx, "SELECT rules,state,version FROM challenge_runs WHERE owner_id=$1 AND id=$2", owner, id).Scan(&body, &v.State, &v.Version)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, privateapi.Problem(404, "NOT_FOUND", "Испытание не найдено")
	}
	if e != nil {
		return v, e
	}
	var rules challenges.Rules
	if e = json.Unmarshal(body, &rules); e != nil {
		return v, e
	}
	v.Name = rules.Name
	v.Metric = rules.Metric
	v.Target = float64(rules.Target)
	v.Unit = rules.Unit
	if rules.Kind == "periodic" {
		v.Unit = "periods"
	}
	var joined bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM challenge_participants WHERE owner_id=$1 AND run_owner_id=$1 AND run_id=$2)", owner, id).Scan(&joined); e != nil {
		return v, e
	}
	if !joined {
		return v, nil
	}
	if s.challenges == nil {
		return v, privateapi.Problem(503, "UNAVAILABLE", "Источники испытаний недоступны")
	}
	m, e := s.challenges.ProgressInTx(ctx, tx, owner, id)
	if e != nil {
		return v, e
	}
	var p challenges.Progress
	if e = json.Unmarshal(m.Progress, &p); e != nil {
		return v, e
	}
	v.Value = &p.Current
	v.Percent = &p.Percent
	v.State = m.State
	v.Result = m.Result
	v.Version = m.Version
	return v, nil
}
func uniqueIDs(ids []string) bool {
	if len(ids) > 50 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !privateapi.UUID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func (s *Service) overview(ctx context.Context, tx pgx.Tx, owner, id string) (Overview, error) {
	out := Overview{Goals: []Source{}, Challenges: []Source{}, Bosses: []SeasonBoss{}}
	v, e := s.get(ctx, tx, owner, id)
	if e != nil {
		return out, e
	}
	out.Season = v
	if v.FinalSnapshot != nil {
		var saved Summary
		if e = json.Unmarshal(v.FinalSnapshot, &saved); e != nil {
			return out, e
		}
		out = saved.Overview
		out.Season = v
		return out, nil
	}
	type item struct {
		id       string
		baseline json.RawMessage
		main     bool
	}
	goals := []item{}
	rows, e := tx.Query(ctx, "SELECT goal_id::text,baseline,is_main FROM season_goal_links WHERE owner_id=$1 AND season_id=$2 ORDER BY is_main DESC,linked_at,goal_id", owner, id)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var x item
		if e = rows.Scan(&x.id, &x.baseline, &x.main); e != nil {
			rows.Close()
			return out, e
		}
		goals = append(goals, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, x := range goals {
		g, e := s.goalSource(ctx, tx, owner, x.id)
		if e != nil {
			var problem *platform.APIError
			if !errors.As(e, &problem) || problem.Status != 404 {
				return out, e
			}
			var baseline Source
			if json.Unmarshal(x.baseline, &baseline) != nil {
				return out, e
			}
			g = baseline
			g.State = "unavailable"
			g.Value = nil
			g.Percent = nil
			g.StagesCompleted = 0
			g.StagesTotal = 0
		}
		g.Baseline = x.baseline
		out.Goals = append(out.Goals, g)
		if x.main {
			copy := x.id
			out.MainGoalID = &copy
		}
	}
	runs := []item{}
	rows, e = tx.Query(ctx, "SELECT run_id::text,baseline FROM season_challenge_links WHERE owner_id=$1 AND season_id=$2 ORDER BY linked_at,run_id", owner, id)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var x item
		if e = rows.Scan(&x.id, &x.baseline); e != nil {
			rows.Close()
			return out, e
		}
		runs = append(runs, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, x := range runs {
		c, e := s.challengeSource(ctx, tx, owner, x.id)
		if e != nil {
			return out, e
		}
		c.Baseline = x.baseline
		out.Challenges = append(out.Challenges, c)
	}
	rows, e = tx.Query(ctx, "SELECT l.boss_id::text,b.goal_id::text,g.payload->>'title',l.month,l.timezone,b.state,g.progress::double precision,(m.version IS DISTINCT FROM l.source_version OR m.boss_id IS DISTINCT FROM l.boss_id) FROM season_monthly_boss_links l JOIN bosses b ON b.owner_id=l.owner_id AND b.id=l.boss_id JOIN goals g ON g.owner_id=b.owner_id AND g.id=b.goal_id LEFT JOIN monthly_bosses m ON m.owner_id=l.owner_id AND m.month=l.month WHERE l.owner_id=$1 AND l.season_id=$2 ORDER BY l.month", owner, id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b SeasonBoss
		if e = rows.Scan(&b.BossID, &b.GoalID, &b.Name, &b.Month, &b.Timezone, &b.State, &b.Progress, &b.SourceChanged); e != nil {
			return out, e
		}
		out.Bosses = append(out.Bosses, b)
	}
	return out, rows.Err()
}
func (s *Service) sourceEvent(ctx context.Context, tx pgx.Tx, owner, kind string, v Overview) error {
	body, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO personal_season_events(owner_id,season_id,version,kind,snapshot,created_at) VALUES($1,$2,$3,$4,$5,$6)", owner, v.Season.ID, v.Season.Version, kind, body, s.Clock.Now())
	return e
}
func (s *Service) setLinks(r *http.Request, tx pgx.Tx, owner, id string, in LinksInput) (Overview, error) {
	out := Overview{}
	v, e := s.get(r.Context(), tx, owner, id)
	if e != nil {
		return out, e
	}
	if v.State != "planned" && v.State != "active" {
		return out, privateapi.Problem(409, "SEASON_CLOSED", "Источники закрытого сезона не меняются")
	}
	if p := privateapi.MatchVersion(r, v.Version); p != nil {
		return out, p
	}
	if in.GoalIDs == nil {
		in.GoalIDs = []string{}
	}
	if in.ChallengeIDs == nil {
		in.ChallengeIDs = []string{}
	}
	if !uniqueIDs(in.GoalIDs) || !uniqueIDs(in.ChallengeIDs) {
		return out, privateapi.Invalid("Не более 50 уникальных целей и испытаний")
	}
	mainFound := in.MainGoalID == nil
	gs := []Source{}
	cs := []Source{}
	for _, id := range in.GoalIDs {
		g, e := s.goalSource(r.Context(), tx, owner, id)
		if e != nil {
			return out, e
		}
		gs = append(gs, g)
		if in.MainGoalID != nil && *in.MainGoalID == id {
			mainFound = true
		}
	}
	if !mainFound {
		return out, privateapi.Invalid("Главная цель должна входить в выбранные цели")
	}
	for _, id := range in.ChallengeIDs {
		c, e := s.challengeSource(r.Context(), tx, owner, id)
		if e != nil {
			return out, e
		}
		cs = append(cs, c)
	}
	// Existing baselines survive a list reorder or main-goal change.
	for _, table := range []string{"season_goal_links", "season_challenge_links"} {
		column := "goal_id"
		ids := in.GoalIDs
		if table == "season_challenge_links" {
			column = "run_id"
			ids = in.ChallengeIDs
		}
		if _, e = tx.Exec(r.Context(), "DELETE FROM "+table+" WHERE owner_id=$1 AND season_id=$2 AND NOT("+column+"=ANY($3::uuid[]))", owner, id, ids); e != nil {
			return out, e
		}
	}
	if _, e = tx.Exec(r.Context(), "UPDATE season_goal_links SET is_main=false WHERE owner_id=$1 AND season_id=$2", owner, id); e != nil {
		return out, e
	}
	for _, g := range gs {
		body, e := json.Marshal(g)
		if e != nil {
			return out, e
		}
		main := in.MainGoalID != nil && *in.MainGoalID == g.ID
		if _, e = tx.Exec(r.Context(), "INSERT INTO season_goal_links(owner_id,season_id,goal_id,is_main,baseline,linked_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,season_id,goal_id) DO UPDATE SET is_main=excluded.is_main", owner, id, g.ID, main, body, s.Clock.Now()); e != nil {
			return out, e
		}
	}
	for _, c := range cs {
		body, e := json.Marshal(c)
		if e != nil {
			return out, e
		}
		if _, e = tx.Exec(r.Context(), "INSERT INTO season_challenge_links(owner_id,season_id,run_id,baseline,linked_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", owner, id, c.ID, body, s.Clock.Now()); e != nil {
			return out, e
		}
	}
	if _, e = tx.Exec(r.Context(), "UPDATE personal_seasons SET version=version+1 WHERE owner_id=$1 AND id=$2", owner, id); e != nil {
		return out, e
	}
	out, e = s.overview(r.Context(), tx, owner, id)
	if e != nil {
		return out, e
	}
	return out, s.sourceEvent(r.Context(), tx, owner, "links_changed", out)
}

type MonthlyLinkInput struct {
	BossID        string `json:"boss_id"`
	Month         string `json:"month"`
	SourceVersion int64  `json:"source_version"`
	Confirm       bool   `json:"confirm"`
}

func (s *Service) linkMonthly(r *http.Request, tx pgx.Tx, owner, id string, in MonthlyLinkInput) (Overview, error) {
	out := Overview{}
	v, e := s.get(r.Context(), tx, owner, id)
	if e != nil {
		return out, e
	}
	if v.State != "active" && v.State != "planned" {
		return out, privateapi.Problem(409, "SEASON_CLOSED", "Сезон закрыт")
	}
	if p := privateapi.MatchVersion(r, v.Version); p != nil {
		return out, p
	}
	_, monthError := time.Parse("2006-01", in.Month)
	if !in.Confirm || !privateapi.UUID(in.BossID) || monthError != nil || in.Month < v.StartsOn[:7] || in.Month > v.EndsOn[:7] {
		return out, privateapi.Invalid("Подтвердите связь выбранного босса месяца")
	}
	var match bool
	e = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM monthly_bosses m JOIN bosses b ON b.owner_id=m.owner_id AND b.id=m.boss_id JOIN season_goal_links l ON l.owner_id=b.owner_id AND l.goal_id=b.goal_id AND l.season_id=$2 WHERE m.owner_id=$1 AND m.month=$3 AND m.boss_id=$4 AND m.version=$5)", owner, id, in.Month, in.BossID, in.SourceVersion).Scan(&match)
	if e != nil {
		return out, e
	}
	if !match {
		return out, privateapi.Problem(409, "MONTHLY_SOURCE_CHANGED", "Обновите выбор босса месяца; его цель должна входить в сезон")
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO season_monthly_boss_links(owner_id,season_id,month,timezone,boss_id,source_version,linked_at) SELECT owner_id,$2,month,timezone,boss_id,version,$4 FROM monthly_bosses WHERE owner_id=$1 AND month=$3 ON CONFLICT(owner_id,season_id,month) DO UPDATE SET timezone=excluded.timezone,boss_id=excluded.boss_id,source_version=excluded.source_version,linked_at=excluded.linked_at", owner, id, in.Month, s.Clock.Now())
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(r.Context(), "UPDATE personal_seasons SET version=version+1 WHERE owner_id=$1 AND id=$2", owner, id); e != nil {
		return out, e
	}
	out, e = s.overview(r.Context(), tx, owner, id)
	if e != nil {
		return out, e
	}
	return out, s.sourceEvent(r.Context(), tx, owner, "monthly_boss_linked", out)
}
func (s *Service) mountLinks(r chi.Router) {
	r.Get("/api/v1/seasons/{id}/overview", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.overview(ctx, tx, owner, chi.URLParam(r, "id"))
		})
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/seasons/{id}/links", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in LinksInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.setLinks(r, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
	r.Put("/api/v1/seasons/{id}/monthly-boss", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in MonthlyLinkInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.linkMonthly(r, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
}
