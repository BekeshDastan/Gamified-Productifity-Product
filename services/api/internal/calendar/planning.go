package calendar

import (
	"context"
	"github.com/jackc/pgx/v5"
	"sort"
	"time"
)

// Planning calls these methods inside its transaction while holding the owner lock.
func (s *Service) CreateInTx(ctx context.Context, tx pgx.Tx, id string, in Input) (Event, error) {
	valid, e := Validate(in)
	if e != nil {
		return Event{}, e
	}
	return s.insert(ctx, tx, id, valid)
}

// Preview projects unmaterialized repeats without writing events or tasks.
func (s *Service) PreviewTimedInTx(ctx context.Context, tx pgx.Tx, id string, fromDate, toDate, from, to time.Time) ([]Event, error) {
	out, e := s.TimedInTx(ctx, tx, id, from, to)
	if e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,payload,version,ends_before::text FROM recurrence_series WHERE owner_id=$1 ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	series := []Series{}
	for rows.Next() {
		v, e := scanSeries(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		series = append(series, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for _, v := range series {
		if v.Event.AllDay {
			continue
		}
		occurrences, e := Generate(v.SeriesInput, fromDate, toDate.AddDate(0, 0, 2), v.EndsBefore)
		if e != nil {
			return nil, e
		}
		for _, o := range occurrences {
			start, _ := time.Parse(time.RFC3339, o.Event.Start)
			end, _ := time.Parse(time.RFC3339, o.Event.End)
			if !start.Before(to) || !end.After(from) {
				continue
			}
			var exists bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM calendar_events WHERE owner_id=$1 AND series_id=$2 AND occurrence_key=$3::text::date UNION ALL SELECT 1 FROM recurrence_exclusions WHERE owner_id=$1 AND series_id=$2 AND occurrence_key=$3::text::date)`, id, v.ID, o.Key).Scan(&exists); e != nil {
				return nil, e
			}
			if exists {
				continue
			}
			source := "event"
			if v.Kind == "task" {
				source = "block"
			}
			key := o.Key
			out = append(out, Event{Input: o.Event, ID: "preview-" + v.ID + "-" + key, Version: v.Version, Source: source, Status: "planned", SeriesID: &v.ID, OccurrenceKey: &key, SeriesVersion: v.Version})
			if len(out) > 2000 {
				return nil, problem(422, "RANGE_LIMIT", "Слишком много блоков для предпросмотра")
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].ID < out[j].ID
		}
		return out[i].Start < out[j].Start
	})
	return out, nil
}
func (s *Service) TimedInTx(ctx context.Context, tx pgx.Tx, id string, from, to time.Time) ([]Event, error) {
	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND NOT e.all_day AND NOT e.cancelled AND (q.id IS NULL OR q.archived_at IS NULL) AND e.starts_at<$3 AND e.ends_at>$2 ORDER BY e.starts_at,e.id LIMIT 2001`, id, from, to)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 2000 {
		return nil, problem(422, "RANGE_LIMIT", "Слишком много блоков в дне")
	}
	return out, nil
}
func (s *Service) MaterializeInTx(ctx context.Context, tx pgx.Tx, id string, from, to time.Time) (bool, error) {
	return s.materialize(ctx, tx, id, from, to)
}
