package focus

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"unicode/utf8"
)

type ConfirmInput struct {
	Result  string `json:"result"`
	Comment string `json:"comment"`
}

func (s *Service) confirm(ctx context.Context, tx pgx.Tx, owner, id string, in ConfirmInput, r *http.Request) (Session, error) {
	in.Comment = strings.TrimSpace(in.Comment)
	if (in.Result != "progress" && in.Result != "no_progress" && in.Result != "rest") || utf8.RuneCountInString(in.Comment) > 2000 || strings.ContainsRune(in.Comment, 0) {
		return Session{}, problem(422, "VALIDATION_ERROR", "Выберите итог и комментарий до 2000 символов")
	}
	f, e := s.get(ctx, tx, owner, id)
	if e != nil {
		return f, e
	}
	if f.State == "finished" {
		if f.Result != in.Result || f.Comment != in.Comment {
			return f, problem(409, "IDEMPOTENCY_MISMATCH", "Эта сессия уже подтверждена с другим итогом")
		}
		return f, nil
	}
	if f.State != "awaiting_confirmation" {
		return f, problem(409, "INVALID_TRANSITION", "Сначала завершите таймер")
	}
	if p := platform.MatchVersion(r, f.Version); p != nil {
		return f, p
	}
	if in.Result == "progress" && f.ActiveSeconds == 0 {
		return f, problem(422, "VALIDATION_ERROR", "Рабочее время отсутствует. Можно отметить отсутствие результата или отдых")
	}
	_, e = tx.Exec(ctx, "UPDATE focus_sessions SET state='finished',result=$3,comment=$4,version=version+1,updated_at=$5 WHERE owner_id=$1 AND id=$2", owner, id, in.Result, in.Comment, s.now())
	if e != nil {
		return f, e
	}
	f, e = s.get(ctx, tx, owner, id)
	if e == nil {
		e = s.event(ctx, tx, owner, "confirmed", f)
	}
	return f, e
}

type QuestTime struct {
	QuestID           string `json:"quest_id"`
	Mode              string `json:"mode"`
	ManualMinutes     int64  `json:"manual_minutes"`
	FocusSeconds      int64  `json:"focus_seconds"`
	EffectiveSeconds  int64  `json:"effective_seconds"`
	ConfirmedSessions int64  `json:"confirmed_sessions"`
	Overridden        bool   `json:"overridden"`
}

// QuestTimeInTx exposes the mutually exclusive sources for later analytics too.
func QuestTimeInTx(ctx context.Context, tx pgx.Tx, owner, id string) (QuestTime, error) {
	out := QuestTime{QuestID: id}
	if !validID(id) {
		return out, problem(404, "NOT_FOUND", "Задача не найдена")
	}
	e := tx.QueryRow(ctx, "SELECT COALESCE(payload->>'actual_time_mode',CASE WHEN COALESCE((payload->>'actual_minutes')::bigint,0)>0 THEN 'manual_override' ELSE 'from_focus' END),COALESCE((payload->>'actual_minutes')::bigint,0) FROM quests WHERE owner_id=$1 AND id=$2", owner, id).Scan(&out.Mode, &out.ManualMinutes)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, problem(404, "NOT_FOUND", "Задача не найдена")
	}
	if e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT COALESCE(sum(s.seconds),0)::bigint,count(*) FILTER(WHERE s.seconds>0) FROM focus_sessions f CROSS JOIN LATERAL (SELECT COALESCE(sum(floor(extract(epoch FROM(ended_at-started_at)))),0)::bigint seconds FROM focus_segments WHERE owner_id=f.owner_id AND session_id=f.id AND kind='work' AND ended_at IS NOT NULL) s WHERE f.owner_id=$1 AND f.quest_id=$2 AND f.state='finished' AND f.result<>'rest'`, owner, id).Scan(&out.FocusSeconds, &out.ConfirmedSessions)
	out.Overridden = out.Mode == "manual_override"
	out.EffectiveSeconds = out.FocusSeconds
	if out.Overridden {
		out.EffectiveSeconds = out.ManualMinutes * 60
	}
	return out, e
}
func (s *Service) mountReview(r chi.Router) {
	r.Post("/api/v1/focus/sessions/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in ConfirmInput) (any, error) {
			return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.confirm(ctx, tx, owner, chi.URLParam(r, "id"), in, r)
			})
		})
	})
	r.Get("/api/v1/focus/quests/{id}/time", func(w http.ResponseWriter, r *http.Request) {
		owner, e := owner(r.Context())
		var out QuestTime
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error {
				var e error
				out, e = QuestTimeInTx(r.Context(), tx, owner, chi.URLParam(r, "id"))
				return e
			})
		}
		s.reply(w, r, out, e)
	})
}
