package focus

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type ActionInput struct {
	Action  string `json:"action"`
	Comment string `json:"comment"`
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
func (s *Service) closeSegment(ctx context.Context, tx pgx.Tx, owner, id string, end time.Time) error {
	tag, e := tx.Exec(ctx, "UPDATE focus_segments SET ended_at=GREATEST(started_at,$3) WHERE owner_id=$1 AND session_id=$2 AND ended_at IS NULL", owner, id, end)
	if e == nil && tag.RowsAffected() != 1 {
		e = errors.New("running focus session requires one open segment")
	}
	return e
}

// The server settles expired timers at their boundary, even if no client was open.
func (s *Service) get(ctx context.Context, tx pgx.Tx, owner, id string) (Session, error) {
	f, e := s.load(ctx, tx, owner, id)
	if e != nil {
		return f, e
	}
	now := s.now()
	wall := f.StartedAt.Add(time.Duration(MaxElapsedSeconds) * time.Second)
	if f.State == "paused" && !now.Before(wall) {
		_, e = tx.Exec(ctx, "UPDATE focus_sessions SET state='awaiting_confirmation',phase='review',ended_at=$3,phase_deadline=NULL,phase_remaining_seconds=NULL,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2", owner, id, wall, now)
		if e != nil {
			return f, e
		}
		f, e = s.load(ctx, tx, owner, id)
		if e == nil {
			e = s.event(ctx, tx, owner, "elapsed_limit", f)
		}
		return f, e
	}
	if f.State != "running" {
		return f, nil
	}
	if len(f.Segments) == 0 || f.Segments[len(f.Segments)-1].EndedAt != nil {
		return f, errors.New("missing open focus segment")
	}
	seg := f.Segments[len(f.Segments)-1]
	cut := wall
	closedWork := f.ActiveSeconds
	if seg.Kind == "work" {
		closedWork -= seg.Seconds
		cut = minTime(cut, seg.StartedAt.Add(time.Duration(max(0, MaxActiveSeconds-closedWork))*time.Second))
	}
	if f.PhaseDeadline != nil {
		cut = minTime(cut, *f.PhaseDeadline)
	}
	if now.Before(cut) {
		return f, nil
	}
	if e = s.closeSegment(ctx, tx, owner, id, cut); e != nil {
		return f, e
	}
	state, phase, kind := "awaiting_confirmation", "review", "duration_limit"
	var ended *time.Time = &cut
	active := closedWork
	if seg.Kind == "work" {
		active += max(0, int64(cut.Sub(seg.StartedAt)/time.Second))
	}
	if now.Before(wall) && active < MaxActiveSeconds && f.Mode != "free" {
		state = "paused"
		phase = f.Phase + "_complete"
		kind = "interval_complete"
		ended = nil
	}
	_, e = tx.Exec(ctx, "UPDATE focus_sessions SET state=$3,phase=$4,ended_at=$5,phase_deadline=NULL,phase_remaining_seconds=NULL,version=version+1,updated_at=$6 WHERE owner_id=$1 AND id=$2", owner, id, state, phase, ended, now)
	if e != nil {
		return f, e
	}
	f, e = s.load(ctx, tx, owner, id)
	if e == nil {
		e = s.event(ctx, tx, owner, kind, f)
	}
	return f, e
}
func (s *Service) action(ctx context.Context, tx pgx.Tx, owner, id string, in ActionInput, r *http.Request) (Session, error) {
	f, e := s.get(ctx, tx, owner, id)
	if e != nil {
		return f, e
	}
	in.Comment = strings.TrimSpace(in.Comment)
	if utf8.RuneCountInString(in.Comment) > 2000 || strings.ContainsRune(in.Comment, 0) {
		return f, problem(422, "VALIDATION_ERROR", "Комментарий до 2000 символов")
	}
	if (in.Action == "pause" && f.State == "paused") || (in.Action == "finish" && f.State == "awaiting_confirmation") || (in.Action == "cancel" && f.State == "cancelled") {
		return f, nil
	}
	if f.State == "finished" || f.State == "cancelled" {
		return f, problem(409, "INVALID_TRANSITION", "Сессия уже закрыта")
	}
	if p := platform.MatchVersion(r, f.Version); p != nil {
		return f, p
	}
	now := s.now()
	if now.Before(f.UpdatedAt) {
		now = f.UpdatedAt
	}
	state, phase := f.State, f.Phase
	deadline, remaining := f.PhaseDeadline, f.PhaseRemainingSeconds
	ended := f.EndedAt
	comment := f.Comment
	switch in.Action {
	case "pause":
		if f.State != "running" {
			return f, problem(409, "INVALID_TRANSITION", "Пауза сейчас недоступна")
		}
		if e = s.closeSegment(ctx, tx, owner, id, now); e != nil {
			return f, e
		}
		state = "paused"
		if deadline != nil {
			seconds := max(0, int64(deadline.Sub(now)/time.Second))
			remaining = &seconds
			deadline = nil
		}
	case "resume":
		if f.State != "paused" || (phase != "work" && phase != "break") {
			return f, problem(409, "INVALID_TRANSITION", "Выберите следующий интервал или завершите сессию")
		}
		state = "running"
		if remaining != nil {
			end := now.Add(time.Duration(*remaining) * time.Second)
			deadline = &end
			remaining = nil
		}
		if _, e = tx.Exec(ctx, "INSERT INTO focus_segments(owner_id,session_id,kind,started_at) VALUES($1,$2,$3,$4)", owner, id, phase, now); e != nil {
			return f, e
		}
	case "finish", "cancel":
		if in.Action == "finish" && f.State != "running" && f.State != "paused" {
			return f, problem(409, "INVALID_TRANSITION", "Завершение сейчас недоступно")
		}
		if f.State == "running" {
			if e = s.closeSegment(ctx, tx, owner, id, now); e != nil {
				return f, e
			}
		}
		state = "awaiting_confirmation"
		phase = "review"
		deadline = nil
		remaining = nil
		if ended == nil {
			ended = &now
		}
		if in.Action == "cancel" {
			state = "cancelled"
			comment = in.Comment
		}
	default:
		return f, problem(422, "VALIDATION_ERROR", "Выберите действие с таймером")
	}
	_, e = tx.Exec(ctx, "UPDATE focus_sessions SET state=$3,phase=$4,phase_deadline=$5,phase_remaining_seconds=$6,ended_at=$7,comment=$8,version=version+1,updated_at=$9 WHERE owner_id=$1 AND id=$2", owner, id, state, phase, deadline, remaining, ended, comment, now)
	if e != nil {
		return f, e
	}
	f, e = s.get(ctx, tx, owner, id)
	if e == nil {
		e = s.event(ctx, tx, owner, in.Action, f)
	}
	return f, e
}
func (s *Service) mountActions(r chi.Router) {
	r.Post("/api/v1/focus/sessions/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in ActionInput) (any, error) {
			return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.action(ctx, tx, owner, chi.URLParam(r, "id"), in, r)
			})
		})
	})
}
