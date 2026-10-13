package challenges

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

const selectedGoalTree = `WITH RECURSIVE selected AS (SELECT quest_id id FROM goal_quests WHERE owner_id=$1 AND goal_id=$6 UNION ALL SELECT q.id FROM quests q JOIN selected t ON q.parent_id=t.id WHERE q.owner_id=$1) `

func sourceQuery(metric string) string {
	switch metric {
	case "tasks":
		return selectedGoalTree + `SELECT 'task'::text source_kind,q.id source_key,q.payload->>'title' title,'/app/quests/'||q.id::text source_url,1::bigint amount,e.first_awarded_at source_time,e.first_awarded_at source_start,q.version source_version FROM quests q JOIN reward_entitlements e ON e.owner_id=q.owner_id AND e.source_kind='task' AND e.source_key=q.id::text WHERE q.owner_id=$1 AND q.status='completed' AND e.active AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) AND ($5::uuid IS NULL OR q.area_id=$5) AND ($6::uuid IS NULL OR EXISTS(SELECT 1 FROM selected t WHERE t.id=q.id))`
	case "habits":
		return `SELECT 'habit'::text source_kind,p.id source_key,h.payload->>'title'||' · '||p.period_key title,'/app/habits/'||h.id::text||'?period_id='||p.id::text source_url,1::bigint amount,e.first_awarded_at source_time,e.first_awarded_at source_start,e.generation source_version FROM habit_periods p JOIN habits h ON h.owner_id=p.owner_id AND h.id=p.habit_id JOIN reward_entitlements e ON e.owner_id=p.owner_id AND e.source_kind='habit' AND e.source_key=p.id::text WHERE p.owner_id=$1 AND p.habit_id=$7 AND NOT p.excluded AND e.active AND p.starts_at>=$3 AND p.ends_at<=$4 AND ($5::uuid IS NULL OR h.area_id=$5)`
	case "focus":
		return selectedGoalTree + `SELECT 'focus'::text source_kind,f.id source_key,f.title,'/app/focus?session_id='||f.id::text source_url,s.amount,f.updated_at source_time,f.started_at source_start,f.version source_version FROM focus_sessions f CROSS JOIN LATERAL (SELECT COALESCE(sum(floor(extract(epoch FROM(ended_at-started_at)))),0)::bigint amount FROM focus_segments WHERE owner_id=f.owner_id AND session_id=f.id AND kind='work' AND ended_at IS NOT NULL) s WHERE f.owner_id=$1 AND f.state='finished' AND f.result<>'rest' AND f.started_at>=$3 AND ($5::uuid IS NULL OR f.area_id=$5) AND ($6::uuid IS NULL OR f.goal_id=$6 OR EXISTS(SELECT 1 FROM selected t WHERE t.id=f.quest_id))`
	case "reviews":
		return `SELECT 'review'::text source_kind,d.id source_key,'Вечерний разбор '||d.local_date::text title,'/app/journal?section=evening&review_date='||d.local_date::text source_url,1::bigint amount,d.created_at source_time,d.created_at source_start,d.version source_version FROM daily_reviews d WHERE d.owner_id=$1 AND d.local_date BETWEEN $8::text::date AND $9::text::date`
	case "stages":
		return `SELECT 'stage'::text source_kind,m.id source_key,m.title,'/app/goals/'||m.goal_id::text||'#milestone-'||m.id::text source_url,1::bigint amount,m.challenge_first_qualified_at source_time,m.challenge_first_qualified_at source_start,g.version source_version FROM milestones m JOIN goals g ON g.owner_id=m.owner_id AND g.id=m.goal_id WHERE m.owner_id=$1 AND m.goal_id=$6 AND m.challenge_completed AND m.challenge_first_qualified_at IS NOT NULL AND ($5::uuid IS NULL OR g.area_id=$5)`
	default:
		return `SELECT 'manual'::text source_kind,NULL::uuid source_key,''::text title,''::text source_url,0::bigint amount,NULL::timestamptz source_time,NULL::timestamptz source_start,0::bigint source_version WHERE false`
	}
}
func (s *Service) syncCredits(ctx context.Context, tx pgx.Tx, owner string, run Run, member Member, now time.Time) error {
	query := `WITH args AS (SELECT $1::uuid owner_id,$2::uuid participant_id,$3::timestamptz starts_at,$4::timestamptz ends_at,$5::uuid area_id,$6::uuid goal_id,$7::uuid habit_id,$8::text starts_on,$9::text ends_on,$10::timestamptz as_of,$11::text timezone,$12::uuid run_id), sources AS (` + sourceQuery(run.Rules.Metric) + `), eligible AS MATERIALIZED (
 SELECT s.* FROM sources s WHERE amount>0 AND source_time>=$3 AND source_time<$4 AND source_time<=$10 AND source_start>=$3 AND EXISTS(SELECT 1 FROM challenge_membership_windows w WHERE w.owner_id=$1 AND w.participant_id=$2 AND s.source_start>=w.joined_at AND s.source_time>=w.joined_at AND (w.left_at IS NULL OR s.source_time<w.left_at))
 ), inserted AS (
 INSERT INTO challenge_source_credits AS c(owner_id,participant_id,source_kind,source_key,title,source_url,amount,source_time,source_start,source_version,active,updated_at)
 SELECT $1,$2,source_kind,source_key,title,source_url,amount,source_time,source_start,source_version,true,$10 FROM eligible
 ON CONFLICT(owner_id,participant_id,source_kind,source_key) DO UPDATE SET title=excluded.title,source_url=excluded.source_url,amount=excluded.amount,source_time=excluded.source_time,source_start=excluded.source_start,source_version=excluded.source_version,active=true,updated_at=excluded.updated_at
 WHERE ROW(c.title,c.source_url,c.amount,c.source_time,c.source_start,c.source_version,c.active) IS DISTINCT FROM ROW(excluded.title,excluded.source_url,excluded.amount,excluded.source_time,excluded.source_start,excluded.source_version,true) RETURNING 1)
 UPDATE challenge_source_credits c SET active=false,updated_at=$10 WHERE c.owner_id=$1 AND c.participant_id=$2 AND c.active AND NOT EXISTS(SELECT 1 FROM eligible e WHERE e.source_kind=c.source_kind AND e.source_key=c.source_key)`
	_, e := tx.Exec(ctx, query, owner, member.ID, run.StartsAt, run.EndsAt, run.Rules.AreaID, run.Rules.GoalID, run.Rules.HabitID, run.Rules.StartsOn, run.Rules.EndsOn, now, run.Rules.Timezone, run.ID)
	return e
}

type Source struct {
	Kind       string    `json:"kind"`
	Key        string    `json:"key"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	Amount     float64   `json:"amount"`
	At         time.Time `json:"at"`
	Version    int64     `json:"version"`
	Confidence string    `json:"confidence"`
}
type SourcePage struct {
	Items    []Source `json:"items"`
	Next     string   `json:"next"`
	Total    int64    `json:"total"`
	Unit     string   `json:"unit"`
	Timezone string   `json:"timezone"`
}

func (s *Service) sourcePage(r *http.Request, tx pgx.Tx, owner string, run Run, member Member) (SourcePage, error) {
	out := SourcePage{Items: []Source{}, Unit: run.Rules.Unit, Timezone: run.Rules.Timezone}
	q := r.URL.Query()
	before := q.Get("before")
	date := q.Get("date")
	if date != "" && (privateapi.Day(date) != nil || date < run.Rules.StartsOn || date > run.Rules.EndsOn) {
		return out, privateapi.Invalid("Дата источников вне периода испытания")
	}
	var dateParam *string
	if date != "" {
		dateParam = &date
	}
	var at *time.Time
	var key *string
	if before != "" {
		if !privateapi.UUID(before) {
			return out, privateapi.Invalid("Некорректная страница")
		}
		var t time.Time
		e := tx.QueryRow(r.Context(), "SELECT source_time FROM challenge_source_credits WHERE owner_id=$1 AND participant_id=$2 AND source_key=$3", owner, member.ID, before).Scan(&t)
		if errors.Is(e, pgx.ErrNoRows) {
			return out, privateapi.Problem(404, "NOT_FOUND", "Источник не найден")
		}
		if e != nil {
			return out, e
		}
		at = &t
		key = &before
	}
	const where = ` FROM challenge_source_credits WHERE owner_id=$1 AND participant_id=$2 AND active AND ($3::text IS NULL OR (source_time AT TIME ZONE $4)::date=$3::text::date)`
	if e := tx.QueryRow(r.Context(), "SELECT count(*)"+where, owner, member.ID, dateParam, run.Rules.Timezone).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := tx.Query(r.Context(), "SELECT source_kind,source_key::text,title,source_url,amount,source_time,source_version"+where+" AND ($5::timestamptz IS NULL OR (source_time,source_key)<($5,$6::uuid)) ORDER BY source_time DESC,source_key DESC LIMIT 101", owner, member.ID, dateParam, run.Rules.Timezone, at, key)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Source
		var raw int64
		if e = rows.Scan(&v.Kind, &v.Key, &v.Title, &v.URL, &raw, &v.At, &v.Version); e != nil {
			return out, e
		}
		v.Amount = float64(raw) / float64(scale(run.Rules.Unit))
		v.Confidence = "recorded"
		if run.Rules.Metric == "manual" {
			v.Confidence = "self_report"
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > 100 {
		out.Items = out.Items[:100]
		out.Next = out.Items[99].Key
	}
	return out, nil
}
