package calendar

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

func (s *Service) addOutcomes(ctx context.Context, tx pgx.Tx, id string, from, to, start, end time.Time, out *Page) error {
	rows, e := tx.Query(ctx, `SELECT id::text,payload->>'title',area_id::text,deadline,status,version,completed_at FROM quests WHERE owner_id=$1 AND archived_at IS NULL AND deadline>=$2 AND deadline<$3 ORDER BY deadline,id LIMIT 2001`, id, start, end)
	if e != nil {
		return e
	}
	for rows.Next() {
		v := Event{Input: Input{Timezone: out.Timezone}, Source: "deadline"}
		var key string
		var deadline time.Time
		var completed *time.Time
		if e := rows.Scan(&key, &v.Title, &v.AreaID, &deadline, &v.Status, &v.Version, &completed); e != nil {
			rows.Close()
			return e
		}
		v.ID = "deadline-" + key
		v.QuestID = &key
		v.QuestVersion = v.Version
		v.Title = "Срок: " + v.Title
		v.Start = deadline.UTC().Format(time.RFC3339)
		v.End = deadline.Add(time.Minute).UTC().Format(time.RFC3339)
		v.Reason = "Дедлайн задачи, отдельно от времени работы"
		if completed != nil {
			v.Reason = "Фактически выполнено: " + completed.In(mustZone(out.Timezone)).Format("2006-01-02 15:04")
			if completed.After(deadline) {
				v.Reason += " · позже дедлайна"
			}
		}
		out.Items = append(out.Items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	rows, e = tx.Query(ctx, `SELECT h.id,h.quest_id::text,h.before_state->>'title',h.before_state->>'area_id',h.before_state->>'planned_date',h.quest_version,h.reason FROM task_events h JOIN quests q ON q.owner_id=h.owner_id AND q.id=h.quest_id WHERE h.owner_id=$1 AND q.archived_at IS NULL AND h.before_state->>'planned_date'>=$2 AND h.before_state->>'planned_date'<$3 AND h.before_state->>'planned_date' IS DISTINCT FROM h.after_state->>'planned_date' ORDER BY h.id LIMIT 2001`, id, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		v := Event{Input: Input{AllDay: true, Timezone: out.Timezone}, Source: "history", Status: "postponed"}
		var n int64
		var key string
		if e := rows.Scan(&n, &key, &v.Title, &v.AreaID, &v.Start, &v.Version, &v.Reason); e != nil {
			return e
		}
		v.ID = "history-" + strconv.FormatInt(n, 10)
		v.QuestID = &key
		v.Title = "Прежний план: " + v.Title
		d, _ := day(v.Start)
		v.End = d.AddDate(0, 0, 1).Format("2006-01-02")
		out.Items = append(out.Items, v)
	}
	return rows.Err()
}
func mustZone(name string) *time.Location {
	zone, e := time.LoadLocation(name)
	if e != nil {
		return time.UTC
	}
	return zone
}
