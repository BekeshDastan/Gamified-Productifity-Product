package seasons

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const SummaryVersion = "season-summary-v1"

type CloseInput struct {
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
}
type ArchiveInput struct {
	Confirm  bool `json:"confirm"`
	Archived bool `json:"archived"`
}
type Activity struct {
	From         *time.Time `json:"from"`
	Until        time.Time  `json:"until"`
	Tasks        int64      `json:"tasks"`
	HabitNorms   int64      `json:"habit_norms"`
	FocusSeconds int64      `json:"focus_seconds"`
	Reviews      int64      `json:"reviews"`
	XPDelta      int64      `json:"xp_delta"`
	CoinsDelta   int64      `json:"coins_delta"`
	LevelAtClose int64      `json:"level_at_close"`
}
type Summary struct {
	Version            string    `json:"version"`
	Kind               string    `json:"kind"`
	At                 time.Time `json:"at"`
	Early              bool      `json:"early"`
	Reason             string    `json:"reason"`
	Overview           Overview  `json:"overview"`
	Activity           Activity  `json:"activity"`
	GoalsAchieved      int       `json:"goals_achieved"`
	GoalsUnavailable   int       `json:"goals_unavailable"`
	ChallengesAchieved int       `json:"challenges_achieved"`
}

func (s *Service) WithSchedulingClock(clock platform.Clock) *Service {
	s.schedulingClock = clock
	return s
}
func seasonBounds(v Season) (time.Time, time.Time, error) {
	loc, e := time.LoadLocation(v.Timezone)
	if e != nil {
		return time.Time{}, time.Time{}, e
	}
	a, e := time.ParseInLocation("2006-01-02", v.StartsOn, loc)
	if e != nil {
		return a, a, e
	}
	b, e := time.ParseInLocation("2006-01-02", v.EndsOn, loc)
	return a.UTC(), b.AddDate(0, 0, 1).UTC(), e
}
func (s *Service) schedule(ctx context.Context, tx pgx.Tx, owner string, v Season) error {
	_, ends, e := seasonBounds(v)
	if e != nil {
		return e
	}
	now := s.Clock.Now()
	due := ends
	if s.schedulingClock != nil {
		now = s.schedulingClock.Now()
		due = now.Add(ends.Sub(s.Clock.Now()))
	}
	body, e := json.Marshal(struct {
		SeasonID string `json:"season_id"`
	}{v.ID})
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'seasons.close',$2,$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING", owner, body, "season-close/"+owner+"/"+v.ID, due, now)
	return e
}
func (s *Service) activity(ctx context.Context, tx pgx.Tx, owner string, v Season, until time.Time) (Activity, error) {
	out := Activity{Until: until}
	var xp int64
	if e := tx.QueryRow(ctx, "SELECT COALESCE((SELECT total_xp FROM user_progress WHERE owner_id=$1),0)", owner).Scan(&xp); e != nil {
		return out, e
	}
	out.LevelAtClose = progression.LevelFor(xp).Number
	if v.ActivatedAt == nil {
		return out, nil
	}
	start, _, e := seasonBounds(v)
	if e != nil {
		return out, e
	}
	if v.ActivatedAt.After(start) {
		start = *v.ActivatedAt
	}
	out.From = &start
	e = tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM reward_entitlements e JOIN quests q ON q.owner_id=e.owner_id AND q.id::text=e.source_key WHERE e.owner_id=$1 AND e.source_kind='task' AND e.active AND q.status='completed' AND e.first_awarded_at>=$2 AND e.first_awarded_at<$3 AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id)),
 (SELECT count(*) FROM reward_entitlements e JOIN habit_periods p ON p.owner_id=e.owner_id AND p.id::text=e.source_key WHERE e.owner_id=$1 AND e.source_kind='habit' AND e.active AND NOT p.excluded AND e.first_awarded_at>=$2 AND e.first_awarded_at<$3),
 (SELECT COALESCE(sum(floor(extract(epoch FROM(seg.ended_at-seg.started_at)))),0)::bigint FROM focus_sessions f JOIN focus_segments seg ON seg.owner_id=f.owner_id AND seg.session_id=f.id AND seg.kind='work' AND seg.ended_at IS NOT NULL WHERE f.owner_id=$1 AND f.state='finished' AND f.result<>'rest' AND f.started_at>=$2 AND f.updated_at<$3),
 (SELECT count(*) FROM daily_reviews WHERE owner_id=$1 AND created_at>=$2 AND created_at<$3),
 (SELECT COALESCE(sum(xp_delta),0)::bigint FROM reward_ledger WHERE owner_id=$1 AND occurred_at>=$2 AND occurred_at<$3),
 (SELECT COALESCE(sum(coins_delta),0)::bigint FROM reward_ledger WHERE owner_id=$1 AND occurred_at>=$2 AND occurred_at<$3)`, owner, start, until).Scan(&out.Tasks, &out.HabitNorms, &out.FocusSeconds, &out.Reviews, &out.XPDelta, &out.CoinsDelta)
	return out, e
}
func (s *Service) close(ctx context.Context, tx pgx.Tx, owner, id string, early bool, why string) (Summary, error) {
	var out Summary
	v, e := s.get(ctx, tx, owner, id)
	if e != nil {
		return out, e
	}
	if v.FinalSnapshot != nil {
		e = json.Unmarshal(v.FinalSnapshot, &out)
		return out, e
	}
	_, ends, e := seasonBounds(v)
	if e != nil {
		return out, e
	}
	now := s.Clock.Now()
	beforeEnd := now.Before(ends)
	if beforeEnd && !early {
		return out, privateapi.Problem(409, "SEASON_NOT_DUE", "Срок сезона ещё не окончен")
	}
	if beforeEnd && strings.TrimSpace(why) == "" {
		return out, privateapi.Invalid("Для досрочного завершения укажите причину")
	}
	overview, e := s.overview(ctx, tx, owner, id)
	if e != nil {
		return out, e
	}
	until := now
	if ends.Before(until) {
		until = ends
	}
	activity, e := s.activity(ctx, tx, owner, v, until)
	if e != nil {
		return out, e
	}
	out = Summary{Version: SummaryVersion, Kind: "season_result", At: now, Early: beforeEnd, Reason: why, Overview: overview, Activity: activity}
	for _, g := range overview.Goals {
		if g.State == "achieved" {
			out.GoalsAchieved++
		}
		if g.Value == nil {
			out.GoalsUnavailable++
		}
	}
	for _, c := range overview.Challenges {
		if c.Result != nil && *c.Result == "achieved" {
			out.ChallengesAchieved++
		}
	}
	state := "completed"
	if v.ActivatedAt == nil {
		state = "cancelled"
	}
	v.State = state
	v.ClosedAt = &now
	v.Version++
	v.FinalSnapshot = nil
	out.Overview.Season = v
	body, e := json.Marshal(out)
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, "UPDATE personal_seasons SET state=$3,closed_at=$4,version=$5,final_snapshot=$6 WHERE owner_id=$1 AND id=$2", owner, id, state, now, v.Version, body); e != nil {
		return out, e
	}
	saved, e := s.get(ctx, tx, owner, id)
	if e != nil {
		return out, e
	}
	if e = s.event(ctx, tx, owner, "closed", saved); e != nil {
		return out, e
	}
	return out, nil
}
func (s *Service) archive(r *http.Request, tx pgx.Tx, owner, id string, in ArchiveInput) (Season, error) {
	v, e := s.get(r.Context(), tx, owner, id)
	if e != nil {
		return v, e
	}
	if !in.Confirm {
		return v, privateapi.Invalid("Подтвердите изменение архива")
	}
	if v.State != "completed" && v.State != "cancelled" {
		return v, privateapi.Problem(409, "SEASON_OPEN", "Сначала завершите сезон")
	}
	if (v.ArchivedAt != nil) == in.Archived {
		return v, nil
	}
	if p := privateapi.MatchVersion(r, v.Version); p != nil {
		return v, p
	}
	var at *time.Time
	if in.Archived {
		now := s.Clock.Now()
		at = &now
	}
	v, e = scan(tx.QueryRow(r.Context(), "UPDATE personal_seasons SET archived_at=$3,version=version+1 WHERE owner_id=$1 AND id=$2 RETURNING "+columns, owner, id, at))
	if e != nil {
		return v, e
	}
	return v, s.event(r.Context(), tx, owner, "archive_changed", v)
}
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{"seasons.close": s.expired}
}
func (s *Service) expired(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	if job.OwnerID == nil {
		return jobs.ErrPermanent
	}
	var in struct {
		SeasonID string `json:"season_id"`
	}
	if e := json.Unmarshal(job.Payload, &in); e != nil || !privateapi.UUID(in.SeasonID) {
		return jobs.ErrPermanent
	}
	var owner string
	if e := tx.QueryRow(ctx, "SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE", *job.OwnerID).Scan(&owner); e != nil {
		return e
	}
	v, e := s.get(ctx, tx, owner, in.SeasonID)
	if e != nil {
		return e
	}
	if v.FinalSnapshot != nil {
		return nil
	}
	if _, e = s.close(ctx, tx, owner, in.SeasonID, false, ""); e != nil {
		return e
	}
	return jobs.OwnerChanged(ctx, tx, owner, s.Clock)
}
func (s *Service) mountClosure(r chi.Router) {
	r.Post("/api/v1/seasons/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in CloseInput) (any, error) {
			if !in.Confirm || utf8.RuneCountInString(in.Reason) > 2000 || strings.ContainsRune(in.Reason, 0) {
				return nil, privateapi.Invalid("Подтвердите завершение; причина до 2000 символов")
			}
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				v, e := s.get(ctx, tx, owner, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if v.FinalSnapshot == nil {
					if p := privateapi.MatchVersion(r, v.Version); p != nil {
						return nil, p
					}
				}
				return s.close(ctx, tx, owner, v.ID, true, in.Reason)
			})
		})
	})
	r.Post("/api/v1/seasons/{id}/archive", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in ArchiveInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.archive(r, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
	r.Get("/api/v1/seasons/{id}/summary", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			v, e := s.get(ctx, tx, owner, chi.URLParam(r, "id"))
			if e != nil {
				return nil, e
			}
			if v.FinalSnapshot == nil {
				return nil, privateapi.Problem(404, "NOT_FOUND", "Итог ещё не сохранён")
			}
			var summary Summary
			e = json.Unmarshal(v.FinalSnapshot, &summary)
			return summary, e
		})
		s.Reply(w, r, out, e)
	})
}
