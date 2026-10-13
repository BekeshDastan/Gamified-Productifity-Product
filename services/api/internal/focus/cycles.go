package focus

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) nextInterval(ctx context.Context, tx pgx.Tx, owner string, f Session, action string) (Session, error) {
	if f.State != "paused" {
		return f, problem(409, "INVALID_TRANSITION", "Сначала завершите текущий интервал")
	}
	phase, minutes, cycle := "work", f.WorkMinutes, f.Cycle
	switch action {
	case "break":
		if f.Mode != "pomodoro" || f.Phase != "work_complete" {
			return f, problem(409, "INVALID_TRANSITION", "Перерыв сейчас недоступен")
		}
		phase = "break"
		minutes = f.BreakMinutes
	case "next":
		if f.Mode != "pomodoro" || (f.Phase != "work_complete" && f.Phase != "break_complete") {
			return f, problem(409, "INVALID_TRANSITION", "Следующий рабочий интервал сейчас недоступен")
		}
		cycle++
	case "extend":
		if f.Mode != "quick" || f.Phase != "work_complete" {
			return f, problem(409, "INVALID_TRANSITION", "Продолжение короткого старта сейчас недоступно")
		}
		minutes = 5
		cycle++
	default:
		return f, problem(422, "VALIDATION_ERROR", "Выберите следующий интервал")
	}
	if cycle > 100 || len(f.Segments) >= 1000 {
		return f, problem(409, "SESSION_LIMIT", "Завершите эту сессию перед новым интервалом")
	}
	now := s.now()
	if now.Before(f.UpdatedAt) {
		now = f.UpdatedAt
	}
	end := now.Add(time.Duration(minutes) * time.Minute)
	_, e := tx.Exec(ctx, "INSERT INTO focus_segments(owner_id,session_id,kind,started_at) VALUES($1,$2,$3,$4)", owner, f.ID, phase, now)
	if e != nil {
		return f, e
	}
	_, e = tx.Exec(ctx, "UPDATE focus_sessions SET state='running',phase=$3,phase_deadline=$4,phase_remaining_seconds=NULL,cycle=$5,version=version+1,updated_at=$6 WHERE owner_id=$1 AND id=$2", owner, f.ID, phase, end, cycle, now)
	if e != nil {
		return f, e
	}
	f, e = s.get(ctx, tx, owner, f.ID)
	if e == nil {
		e = s.event(ctx, tx, owner, action, f)
	}
	return f, e
}
