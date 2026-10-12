package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func scanSeries(row pgx.Row) (Series, error) {
	var s Series
	var data []byte
	e := row.Scan(&s.ID, &data, &s.Version, &s.EndsBefore)
	if e == nil {
		e = json.Unmarshal(data, &s.SeriesInput)
	}
	return s, e
}
func (s *Service) getSeries(ctx context.Context, tx pgx.Tx, id, key string) (Series, error) {
	if !uuid(key) {
		return Series{}, problem(404, "NOT_FOUND", "Серия не найдена")
	}
	v, e := scanSeries(tx.QueryRow(ctx, `SELECT id::text,payload,version,ends_before::text FROM recurrence_series WHERE owner_id=$1 AND id=$2`, id, key))
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Серия не найдена")
	}
	return v, e
}
func (s *Service) insertSeries(ctx context.Context, tx pgx.Tx, id string, in SeriesInput) (Series, error) {
	if e := s.labels(ctx, tx, id, in.Event); e != nil {
		return Series{}, e
	}
	var total int
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM recurrence_series WHERE owner_id=$1 AND ends_before IS NULL`, id).Scan(&total); e != nil {
		return Series{}, e
	}
	if total >= 500 {
		return Series{}, invalid("rule", "Не более 500 активных серий")
	}
	data, e := json.Marshal(in)
	if e != nil {
		return Series{}, e
	}
	return scanSeries(tx.QueryRow(ctx, `INSERT INTO recurrence_series(owner_id,payload,created_at) VALUES($1,$2,$3) RETURNING id::text,payload,version,ends_before::text`, id, data, s.clock.Now().UTC()))
}
func (s *Service) materialize(ctx context.Context, tx pgx.Tx, id string, from, to time.Time) (bool, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,payload,version,ends_before::text FROM recurrence_series WHERE owner_id=$1 ORDER BY created_at,id`, id)
	if e != nil {
		return false, e
	}
	series := []Series{}
	for rows.Next() {
		v, e := scanSeries(rows)
		if e != nil {
			rows.Close()
			return false, e
		}
		series = append(series, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, e
	}
	created := 0
	more := false
	for _, v := range series {
		occurrences, e := Generate(v.SeriesInput, from, to, v.EndsBefore)
		if e != nil {
			return false, e
		}
		for _, o := range occurrences {
			var exists bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM calendar_events WHERE owner_id=$1 AND series_id=$2 AND occurrence_key=$3::text::date UNION ALL SELECT 1 FROM recurrence_exclusions WHERE owner_id=$1 AND series_id=$2 AND occurrence_key=$3::text::date)`, id, v.ID, o.Key).Scan(&exists)
			if e != nil {
				return false, e
			}
			if exists {
				continue
			}
			if created >= 400 {
				more = true
				continue
			}
			if v.Kind == "task" {
				planned := o.Key
				q, e := s.quests.CreateInTx(ctx, tx, id, quests.Input{Title: o.Event.Title, AreaID: o.Event.AreaID, Priority: "normal", Difficulty: 1, PlannedDate: &planned, Status: "todo", Required: true, Tags: []string{}})
				if e != nil {
					return false, e
				}
				o.Event.QuestID = &q.ID
			}
			event, e := s.insert(ctx, tx, id, o.Event)
			if e != nil {
				return false, e
			}
			_, e = tx.Exec(ctx, `UPDATE calendar_events SET series_id=$3,occurrence_key=$4::text::date WHERE owner_id=$1 AND id=$2`, id, event.ID, v.ID, o.Key)
			if e != nil {
				return false, e
			}
			created++
		}
	}
	return more, nil
}
func (s *Service) mountSeries(r chi.Router) {
	r.Post("/api/v1/calendar/series", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in SeriesInput) (any, error) {
			valid, e := validateSeries(in)
			if e != nil {
				return nil, e
			}
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			return s.idempotent(r, valid, func(tx pgx.Tx) (any, error) { return s.insertSeries(r.Context(), tx, id, valid) })
		})
	})
	r.Get("/api/v1/calendar/series/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		var out Series
		e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
			var e error
			out, e = s.getSeries(r.Context(), tx, id, chi.URLParam(r, "id"))
			return e
		})
		s.reply(w, r, out, e)
	})
}
