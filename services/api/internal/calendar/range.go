package calendar

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

// The public range is calendar dates [from,to) in the account's zone.
// Date-only events never pass through an instant or the browser's local zone.
func (s *Service) List(r *http.Request) (Page, error) {
	id, e := owner(r.Context())
	if e != nil {
		return Page{}, e
	}
	from, e := day(r.URL.Query().Get("from"))
	if e != nil {
		return Page{}, e
	}
	to, e := day(r.URL.Query().Get("to"))
	if e != nil {
		return Page{}, e
	}
	if !to.After(from) || to.Sub(from) > 370*24*time.Hour {
		return Page{}, invalid("to", "Диапазон: от 1 до 370 дней, конец не включён")
	}
	out := Page{Items: []Event{}, Timezone: "UTC"}
	e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		if e := s.lock(r.Context(), tx, id); e != nil {
			return e
		}
		if e := tx.QueryRow(r.Context(), `SELECT COALESCE(preferences->>'timezone','UTC') FROM user_settings WHERE owner_id=$1`, id).Scan(&out.Timezone); e != nil {
			return e
		}
		zone, e := time.LoadLocation(out.Timezone)
		if e != nil {
			return e
		}
		start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, zone)
		end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, zone)
		rows, e := tx.Query(r.Context(), `SELECT `+columns+` FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND (q.id IS NULL OR q.archived_at IS NULL) AND ((e.all_day AND e.start_date<$3::text::date AND e.end_date>$2::text::date) OR (NOT e.all_day AND e.starts_at<$5 AND e.ends_at>$4)) ORDER BY e.created_at,e.id LIMIT 2001`, id, from.Format("2006-01-02"), to.Format("2006-01-02"), start, end)
		if e != nil {
			return e
		}
		for rows.Next() {
			v, e := scan(rows)
			if e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		rows, e = tx.Query(r.Context(), `SELECT id::text,payload->>'title',area_id::text,planned_date::text,status,version FROM quests WHERE owner_id=$1 AND archived_at IS NULL AND planned_date>=$2::text::date AND planned_date<$3::text::date ORDER BY planned_date,id LIMIT 2001`, id, from.Format("2006-01-02"), to.Format("2006-01-02"))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := Event{Input: Input{AllDay: true, Timezone: out.Timezone}, Source: "quest"}
			var key string
			if e := rows.Scan(&key, &v.Title, &v.AreaID, &v.Start, &v.Status, &v.Version); e != nil {
				return e
			}
			v.ID = "quest-" + key
			v.QuestID = &key
			v.QuestVersion = v.Version
			d, _ := day(v.Start)
			v.End = d.AddDate(0, 0, 1).Format("2006-01-02")
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > 2000 {
			return problem(422, "RANGE_LIMIT", "Слишком много событий. Сузьте диапазон")
		}
		return rows.Err()
	})
	return out, e
}
