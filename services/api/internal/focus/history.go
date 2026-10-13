package focus

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type HistorySummary struct {
	Sessions         int64 `json:"sessions"`
	WorkSeconds      int64 `json:"work_seconds"`
	ConfirmedSeconds int64 `json:"confirmed_seconds"`
	PendingSeconds   int64 `json:"pending_seconds"`
	CancelledSeconds int64 `json:"cancelled_seconds"`
	RestSeconds      int64 `json:"rest_seconds"`
}
type History struct {
	Items      []Session      `json:"items"`
	NextCursor *string        `json:"next_cursor"`
	Summary    HistorySummary `json:"summary"`
	AsOf       time.Time      `json:"as_of"`
	From       string         `json:"from"`
	To         string         `json:"to"`
}

const historyWhere = `f.owner_id=$1 AND (f.started_at AT TIME ZONE f.timezone)::date BETWEEN $2::text::date AND $3::text::date AND ($4::uuid IS NULL OR f.quest_id=$4) AND ($5::uuid IS NULL OR f.area_id=$5) AND ($6='' OR f.state=$6)`

func (s *Service) history(ctx context.Context, tx pgx.Tx, owner string, r *http.Request) (History, error) {
	out := History{Items: []Session{}}
	if _, e := s.current(ctx, tx, owner); e != nil {
		return out, e
	}
	out.AsOf = s.now()
	query := r.URL.Query()
	out.From = query.Get("from")
	out.To = query.Get("to")
	if out.From == "" || out.To == "" {
		var zone string
		if e := tx.QueryRow(ctx, "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&zone); e != nil {
			return out, e
		}
		loc, e := time.LoadLocation(zone)
		if e != nil {
			return out, e
		}
		today := s.now().In(loc)
		if out.To == "" {
			out.To = today.Format(time.DateOnly)
		}
		if out.From == "" {
			out.From = today.AddDate(0, 0, -30).Format(time.DateOnly)
		}
	}
	from, e := time.Parse(time.DateOnly, out.From)
	if e != nil || from.Year() < 1 {
		return out, problem(422, "VALIDATION_ERROR", "Укажите дату начала")
	}
	to, e := time.Parse(time.DateOnly, out.To)
	if e != nil || to.Before(from) || to.Sub(from) > 365*24*time.Hour {
		return out, problem(422, "VALIDATION_ERROR", "Диапазон истории: от 1 до 366 дней")
	}
	var quest, area *string
	for key, dest := range map[string]**string{"quest_id": &quest, "area_id": &area} {
		v := query.Get(key)
		if v != "" {
			if !validID(v) {
				return out, problem(422, "VALIDATION_ERROR", "Выберите задачу или направление")
			}
			*dest = &v
		}
	}
	state := query.Get("state")
	if state != "" && state != "running" && state != "paused" && state != "awaiting_confirmation" && state != "finished" && state != "cancelled" {
		return out, problem(422, "VALIDATION_ERROR", "Выберите состояние сессии")
	}
	args := []any{owner, out.From, out.To, quest, area, state, out.AsOf}
	e = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(s.seconds),0)::bigint,COALESCE(sum(s.seconds) FILTER(WHERE f.state='finished' AND f.result<>'rest'),0)::bigint,COALESCE(sum(s.seconds) FILTER(WHERE f.state IN('running','paused','awaiting_confirmation')),0)::bigint,COALESCE(sum(s.seconds) FILTER(WHERE f.state='cancelled'),0)::bigint,COALESCE(sum(s.seconds) FILTER(WHERE f.state='finished' AND f.result='rest'),0)::bigint FROM focus_sessions f CROSS JOIN LATERAL (SELECT LEAST(14400,COALESCE(sum(GREATEST(0,floor(extract(epoch FROM(COALESCE(ended_at,LEAST($7,f.started_at+interval '8 hours',COALESCE(f.phase_deadline,'infinity'::timestamptz)))-started_at))))),0))::bigint seconds FROM focus_segments WHERE owner_id=f.owner_id AND session_id=f.id AND kind='work') s WHERE `+historyWhere, args...).Scan(&out.Summary.Sessions, &out.Summary.WorkSeconds, &out.Summary.ConfirmedSeconds, &out.Summary.PendingSeconds, &out.Summary.CancelledSeconds, &out.Summary.RestSeconds)
	if e != nil {
		return out, e
	}
	before := query.Get("before")
	beforeAt := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	beforeID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if before != "" {
		if !validID(before) {
			return out, problem(422, "VALIDATION_ERROR", "Некорректная граница истории")
		}
		e = tx.QueryRow(ctx, "SELECT started_at,id::text FROM focus_sessions WHERE owner_id=$1 AND id=$2", owner, before).Scan(&beforeAt, &beforeID)
		if errors.Is(e, pgx.ErrNoRows) {
			return out, problem(404, "NOT_FOUND", "Сессия не найдена")
		}
		if e != nil {
			return out, e
		}
	}
	args = append(args[:6], beforeAt, beforeID)
	rows, e := tx.Query(ctx, "SELECT f.id::text FROM focus_sessions f WHERE "+historyWhere+" AND (f.started_at,f.id)<($7::timestamptz,$8::uuid) ORDER BY f.started_at DESC,f.id DESC LIMIT 26", args...)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return out, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(ids) > 25 {
		cursor := ids[24]
		out.NextCursor = &cursor
		ids = ids[:25]
	}
	for _, id := range ids {
		f, e := s.load(ctx, tx, owner, id, out.AsOf)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, f)
	}
	return out, nil
}
func (s *Service) mountHistory(r chi.Router) {
	r.Get("/api/v1/focus/history", func(w http.ResponseWriter, r *http.Request) {
		owner, e := owner(r.Context())
		var out History
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error { var e error; out, e = s.history(r.Context(), tx, owner, r); return e })
		}
		s.reply(w, r, out, e)
	})
	r.Get("/api/v1/focus/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		owner, e := owner(r.Context())
		var out Current
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error {
				f, e := s.get(r.Context(), tx, owner, chi.URLParam(r, "id"))
				out = Current{ServerNow: s.now(), Session: &f}
				return e
			})
		}
		s.reply(w, r, out, e)
	})
}
