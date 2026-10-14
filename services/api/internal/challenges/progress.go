package challenges

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

const ScoringVersion = "challenge-scoring-v1"

type DayAmount struct {
	Date   string
	Amount int64
}
type ChallengePeriod struct {
	Start   string  `json:"start"`
	End     string  `json:"end"`
	Current float64 `json:"current"`
	Target  float64 `json:"target"`
	State   string  `json:"state"`
}
type Progress struct {
	Version     string            `json:"version"`
	Current     float64           `json:"current"`
	Target      float64           `json:"target"`
	Unit        string            `json:"unit"`
	Percent     float64           `json:"percent"`
	TargetMet   bool              `json:"target_met"`
	SourceCount int64             `json:"source_count"`
	Periods     []ChallengePeriod `json:"periods"`
}

func scale(unit string) int64 {
	if unit == "minutes" {
		return 60
	}
	return 1
}
func calculate(rules Rules, days []DayAmount, sources int64, now time.Time) Progress {
	out := Progress{Version: ScoringVersion, Target: float64(rules.Target), Unit: rules.Unit, SourceCount: sources, Periods: []ChallengePeriod{}}
	amounts := map[string]int64{}
	var total int64
	for _, day := range days {
		amount := min(day.Amount, rules.DailyCap*scale(rules.Unit))
		amounts[day.Date] = amount
		total += amount
	}
	if rules.Kind == "cumulative" {
		out.Current = float64(total) / float64(scale(rules.Unit))
	} else {
		out.Unit = "periods"
		start, _ := time.Parse("2006-01-02", rules.StartsOn)
		end, _ := time.Parse("2006-01-02", rules.EndsOn)
		end = end.AddDate(0, 0, 1)
		width := 1
		if rules.Period == "week" {
			width = 7
		}
		loc, _ := time.LoadLocation(rules.Timezone)
		today := now.In(loc).Format("2006-01-02")
		for from := start; from.Before(end); from = from.AddDate(0, 0, width) {
			to := from.AddDate(0, 0, width)
			if to.After(end) {
				to = end
			}
			var amount int64
			for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
				amount += amounts[d.Format("2006-01-02")]
			}
			period := ChallengePeriod{Start: from.Format("2006-01-02"), End: to.AddDate(0, 0, -1).Format("2006-01-02"), Current: float64(amount) / float64(scale(rules.Unit)), Target: float64(rules.PeriodTarget), State: "open"}
			if amount >= rules.PeriodTarget*scale(rules.Unit) {
				period.State = "met"
				out.Current++
			} else if period.Start > today {
				period.State = "future"
			} else if period.End < today {
				period.State = "missed"
			}
			out.Periods = append(out.Periods, period)
		}
	}
	out.TargetMet = out.Current >= out.Target
	out.Percent = min(100, 100*out.Current/out.Target)
	return out
}
func (s *Service) WithProgression(p *progression.Service) *Service { s.progress = p; return s }
func sourcePolicy(metric string) string {
	switch metric {
	case "tasks":
		return "Первые квалифицированные завершения листовых задач после вступления; повтор и восстановление старого источника не создают новое действие."
	case "habits":
		return "Полные нормы привычки в целиком включённых периодах, впервые достигнутые после вступления."
	case "focus":
		return "Подтверждённое рабочее время сессий, начатых после вступления и подтверждённых до конца срока; без пауз, перерывов и отдыха."
	case "reviews":
		return "Первые сохранения вечерних разборов после вступления, не более одного зачёта за день; частный текст не используется."
	case "stages":
		return "Первое полное выполнение этапов выбранной цели после вступления. Старые уже завершённые этапы не переучитываются."
	default:
		return "Самоотчётные отметки в пределах правил и активного участия; достоверность реального поведения не подтверждается платформой."
	}
}
func (s *Service) rebuild(ctx context.Context, tx pgx.Tx, owner string, run Run, member Member) (Member, error) {
	if s.progress == nil {
		return member, privateapi.Problem(503, "UNAVAILABLE", "Прогресс недоступен")
	}
	if e := s.progress.Lock(ctx, tx, owner); e != nil {
		return member, e
	}
	if run.Rules.ScoringVersion != "" && run.Rules.ScoringVersion != ScoringVersion {
		return member, privateapi.Problem(409, "SCORING_VERSION_UNAVAILABLE", "Версия правил сейчас недоступна")
	}
	now := s.Clock.Now().UTC()
	if member.ClosedAt != nil {
		var snapshot FinalSnapshot
		if e := json.Unmarshal(member.FinalSnapshot, &snapshot); e != nil {
			return member, e
		}
		if snapshot.Kind == "result" {
			now = *member.ClosedAt
		}
	}
	if e := s.syncCredits(ctx, tx, owner, run, member, now); e != nil {
		return member, e
	}
	var count int64
	if e := tx.QueryRow(ctx, "SELECT count(*) FROM challenge_source_credits WHERE owner_id=$1 AND participant_id=$2 AND active", owner, member.ID).Scan(&count); e != nil {
		return member, e
	}
	rows, e := tx.Query(ctx, "SELECT (source_time AT TIME ZONE $3)::date::text,sum(amount)::bigint FROM challenge_source_credits WHERE owner_id=$1 AND participant_id=$2 AND active GROUP BY 1 ORDER BY 1", owner, member.ID, run.Rules.Timezone)
	if e != nil {
		return member, e
	}
	days := []DayAmount{}
	for rows.Next() {
		var day DayAmount
		if e = rows.Scan(&day.Date, &day.Amount); e != nil {
			rows.Close()
			return member, e
		}
		days = append(days, day)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return member, e
	}
	progress := calculate(run.Rules, days, count, now)
	body, e := json.Marshal(progress)
	if e != nil {
		return member, e
	}
	_, e = tx.Exec(ctx, "UPDATE challenge_participants SET version=version+CASE WHEN progress IS DISTINCT FROM $3::jsonb THEN 1 ELSE 0 END,progress=$3,computed_at=$4 WHERE owner_id=$1 AND id=$2", owner, member.ID, body, now)
	if e != nil {
		return member, e
	}
	saved, e := s.member(ctx, tx, owner, owner, run.ID)
	if e != nil {
		return member, e
	}
	if saved == nil {
		return member, privateapi.Problem(404, "NOT_FOUND", "Участие не найдено")
	}
	return s.syncClosedReward(ctx, tx, owner, run, *saved, progress)
}
func (s *Service) currentProgress(ctx context.Context, tx pgx.Tx, owner, id string) (Member, error) {
	run, e := s.getRun(ctx, tx, owner, id)
	if e != nil {
		return Member{}, e
	}
	member, e := s.member(ctx, tx, owner, owner, id)
	if e != nil {
		return Member{}, e
	}
	if member == nil {
		return Member{}, privateapi.Problem(404, "NOT_FOUND", "Сначала вступите в испытание")
	}
	return s.rebuild(ctx, tx, owner, run, *member)
}
func (s *Service) mountProgress(r chi.Router) {
	r.Get("/api/v1/challenges/runs/{id}/progress", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.currentProgress(ctx, tx, owner, chi.URLParam(r, "id"))
		})
		s.Reply(w, r, out, e)
	})
	r.Get("/api/v1/challenges/runs/{id}/sources", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			id := chi.URLParam(r, "id")
			run, e := s.getRun(ctx, tx, owner, id)
			if e != nil {
				return nil, e
			}
			member, e := s.currentProgress(ctx, tx, owner, id)
			if e != nil {
				return nil, e
			}
			return s.sourcePage(r, tx, owner, run, member)
		})
		s.Reply(w, r, out, e)
	})
}
