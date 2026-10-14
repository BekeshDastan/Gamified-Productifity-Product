package navigator

import (
	"context"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) facts(ctx context.Context, tx pgx.Tx, owner string) ([]Fact, bool, error) {
	out := []Fact{}
	now := s.Clock.Now()
	var timezone string
	var weekStart int
	if e := tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC'),COALESCE((preferences->>'week_start')::int,1) FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&timezone, &weekStart); e != nil {
		return nil, false, e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return nil, false, e
	}
	today := now.In(zone).Format(time.DateOnly)
	add := func(id, subject string, n map[string]float64, sources ...Evidence) {
		if sources == nil {
			sources = []Evidence{}
		}
		out = append(out, Fact{id, subject, n, sources})
	}
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.version,count(*) FROM task_events e JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND e.kind='postpone' AND e.created_at >= $2 AND q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped') AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) GROUP BY q.id,q.version HAVING count(*)>=3 ORDER BY q.id`, owner, now.Add(-14*24*time.Hour))
	if e != nil {
		return nil, false, e
	}
	for rows.Next() {
		var id string
		var version int64
		var count float64
		if e = rows.Scan(&id, &version, &count); e != nil {
			rows.Close()
			return nil, false, e
		}
		add("R01", id, map[string]float64{"postponements": count, "window_days": 14}, Evidence{"task", id, version, "/app/quests/" + id})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, false, e
	}
	p, e := s.plans.ViewInTx(ctx, tx, owner, today)
	if e != nil {
		return nil, false, e
	}
	complete := p.Availability != nil && !p.Availability.Materializing
	if p.Availability != nil && complete {
		a := p.Availability
		add("R02", today, map[string]float64{"planned_minutes": float64(a.PlannedMinutes), "available_minutes": float64(p.AvailableMinutes), "deficit_minutes": float64(a.PlannedMinutes - p.AvailableMinutes)}, Evidence{"plan", p.ID, p.Version, "/app/calendar"})
		for _, c := range a.Conflicts {
			add("R12", c.First+"/"+c.Second, map[string]float64{"overlap_minutes": c.End.Sub(c.Start).Minutes()}, Evidence{"calendar", c.First, 0, "/app/calendar"}, Evidence{"calendar", c.Second, 0, "/app/calendar"})
		}
	}
	rows, e = tx.Query(ctx, `SELECT q.id::text,q.version,q.deadline,COALESCE((q.payload->>'estimated_minutes')::int,0) FROM quests q WHERE q.owner_id=$1 AND q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped') AND q.deadline>$2 AND q.deadline<$3 AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) ORDER BY q.deadline,q.id`, owner, now, now.Add(24*time.Hour))
	if e != nil {
		return nil, false, e
	}
	type due struct {
		id        string
		version   int64
		at        time.Time
		estimated float64
	}
	dues := []due{}
	for rows.Next() {
		var d due
		if e = rows.Scan(&d.id, &d.version, &d.at, &d.estimated); e != nil {
			rows.Close()
			return nil, false, e
		}
		dues = append(dues, d)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, false, e
	}
	for _, d := range dues {
		if d.estimated <= 0 {
			continue
		}
		free := 0.0
		date := time.Date(now.In(zone).Year(), now.In(zone).Month(), now.In(zone).Day(), 0, 0, 0, 0, zone)
		for !date.After(d.at.In(zone)) {
			slots, e := s.plans.FreeWorkInTx(ctx, tx, owner, date.Format(time.DateOnly), now, d.at)
			if e != nil {
				return nil, false, e
			}
			for _, v := range slots {
				free += v.End.Sub(v.Start).Minutes()
			}
			date = date.AddDate(0, 0, 1)
		}
		add("R03", d.id, map[string]float64{"hours_to_deadline": d.at.Sub(now).Hours(), "estimated_minutes": d.estimated, "free_minutes": free, "deficit_minutes": d.estimated - free}, Evidence{"task", d.id, d.version, "/app/quests/" + d.id})
	}
	evidence, habitsComplete, e := s.habits.EvidenceInTx(ctx, tx, owner)
	if e != nil {
		return nil, false, e
	}
	complete = complete && habitsComplete
	if habitsComplete {
		for _, h := range evidence {
			missed := 0
			for i := len(h.Periods) - 1; i >= 0; i-- {
				v := h.Periods[i]
				if v.Excluded || now.Before(v.End) {
					continue
				}
				if v.Amount >= v.Target {
					break
				}
				missed++
			}
			ref := Evidence{"habit", h.ID, 0, "/app/habits/" + h.ID}
			add("R04", h.ID, map[string]float64{"missed_periods": float64(missed)}, ref)
			add("R11", h.ID, map[string]float64{"streak": float64(h.Stats.Current)}, ref)
		}
	}
	var last time.Time
	e = tx.QueryRow(ctx, `SELECT GREATEST((SELECT created_at FROM users WHERE id=$1),COALESCE((SELECT max(completed_at) FROM quests WHERE owner_id=$1 AND status='completed'),'-infinity'),COALESCE((SELECT max(ended_at) FROM focus_sessions WHERE owner_id=$1 AND state='finished' AND result<>'rest'),'-infinity'),COALESCE((SELECT max(accepted_at) FROM habit_logs WHERE owner_id=$1 AND amount>0),'-infinity'),COALESCE((SELECT max(created_at) FROM goal_metric_entries WHERE owner_id=$1),'-infinity'))`, owner).Scan(&last)
	if e != nil {
		return nil, false, e
	}
	add("R05", "return", map[string]float64{"inactive_days": now.Sub(last).Hours() / 24, "returned": 1}, Evidence{"activity", "return", 0, "/app/analytics"})
	var low bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM low_energy_previews WHERE owner_id=$1 AND applied_at IS NOT NULL AND snapshot->>'date'=$2)`, owner, today).Scan(&low)
	if e != nil {
		return nil, false, e
	}
	if low {
		add("R06", today, map[string]float64{"low_energy": 1}, Evidence{"plan", p.ID, p.Version, "/app/calendar"})
	}
	rows, e = tx.Query(ctx, `SELECT v.quest_id::text,q.version,v.visible_seconds,(SELECT count(*) FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id)+(SELECT count(*) FROM focus_sessions f WHERE f.owner_id=q.owner_id AND f.quest_id=q.id AND f.state IN('running','paused','awaiting_confirmation','finished')) FROM navigator_task_views v JOIN quests q ON q.owner_id=v.owner_id AND q.id=v.quest_id WHERE v.owner_id=$1 AND v.visible AND v.last_seen >= $2 AND q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped') ORDER BY q.id`, owner, now.Add(-45*time.Second))
	if e != nil {
		return nil, false, e
	}
	for rows.Next() {
		var id string
		var version int64
		var seconds, steps float64
		if e = rows.Scan(&id, &version, &seconds, &steps); e != nil {
			rows.Close()
			return nil, false, e
		}
		add("R07", id, map[string]float64{"visible_seconds": seconds, "first_steps": steps}, Evidence{"task", id, version, "/app/quests/" + id})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, false, e
	}
	date, _ := time.Parse(time.DateOnly, today)
	week := date.AddDate(0, 0, -(int(date.Weekday())-weekStart+7)%7).AddDate(0, 0, -7).Format(time.DateOnly)
	weekAt, _ := time.ParseInLocation(time.DateOnly, week, zone)
	var review, observed bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM weekly_reviews WHERE owner_id=$1 AND local_date=$2::text::date), (SELECT created_at<=$3 FROM users WHERE id=$1)`, owner, week, weekAt).Scan(&review, &observed)
	if e != nil {
		return nil, false, e
	}
	if observed && !review {
		add("R08", week, map[string]float64{"review_missing": 1}, Evidence{"week", week, 0, "/app/journal"})
	}
	samples, e := s.analytics.TemplateEvidenceInTx(ctx, tx, owner, analytics.Range{From: date.AddDate(0, 0, -365).Format(time.DateOnly), To: date.AddDate(0, 0, 1).Format(time.DateOnly)}, timezone)
	if e != nil {
		return nil, false, e
	}
	for _, v := range samples {
		if v.Factor == nil {
			continue
		}
		refs := []Evidence{}
		for _, id := range v.QuestIDs {
			refs = append(refs, Evidence{"task", id, 0, "/app/quests/" + id})
		}
		add("R09", fmt.Sprintf("%s/%d", v.ID, v.Version), map[string]float64{"samples": float64(v.Count), "actual_estimate_ratio": *v.Factor}, refs...)
	}
	goals, e := s.goals.NavigatorFactsInTx(ctx, tx, owner)
	if e != nil {
		return nil, false, e
	}
	for _, g := range goals {
		ref := Evidence{"goal", g.ID, g.Version, "/app/goals/" + g.ID}
		add("R10", g.ID, map[string]float64{"next_actions": float64(g.NextActions)}, ref)
		if g.Overdue {
			add("R13", g.ID, map[string]float64{"overdue": 1, "progress": g.Progress}, ref)
		}
	}
	month := now.In(zone).Format("2006-01")
	var boss bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monthly_bosses WHERE owner_id=$1 AND month=$2 AND boss_id IS NOT NULL)`, owner, month).Scan(&boss)
	if e != nil {
		return nil, false, e
	}
	if !boss {
		add("R14", month, map[string]float64{"boss_missing": 1}, Evidence{"month", month, 0, "/app/rewards"})
	}
	rows, e = tx.Query(ctx, `SELECT id::text,version FROM focus_sessions WHERE owner_id=$1 AND state='awaiting_confirmation' ORDER BY id`, owner)
	if e != nil {
		return nil, false, e
	}
	for rows.Next() {
		var id string
		var version int64
		if e = rows.Scan(&id, &version); e != nil {
			rows.Close()
			return nil, false, e
		}
		add("R15", id, map[string]float64{"pending_sessions": 1}, Evidence{"focus", id, version, "/app/focus?session_id=" + id})
	}
	e = rows.Err()
	rows.Close()
	return out, complete, e
}
