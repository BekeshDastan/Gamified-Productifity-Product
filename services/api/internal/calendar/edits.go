package calendar

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

func validReason(reason string) error {
	if strings.TrimSpace(reason) == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 1000 || strings.ContainsRune(reason, 0) {
		return invalid("reason", "Укажите причину: 1–1000 символов")
	}
	return nil
}
func (s *Service) edit(r *http.Request, tx pgx.Tx, id, key string, in Edit) (Event, error) {
	current, e := s.get(r.Context(), tx, id, key)
	if e != nil {
		return current, e
	}
	if p := platform.MatchVersion(r, current.Version); p != nil {
		return current, p
	}
	if (in.QuestID == nil) != (current.QuestID == nil) || (in.QuestID != nil && *in.QuestID != *current.QuestID) {
		return current, invalid("quest_id", "Связь с задачей не изменяется")
	}
	if e = s.labels(r.Context(), tx, id, in.Input); e != nil {
		return current, e
	}
	if current.QuestID != nil {
		if current.Status == "completed" || current.Status == "failed" || current.Status == "skipped" {
			return current, problem(409, "HISTORY_LOCKED", "Завершённая история не переписывается")
		}
		zone, _ := time.LoadLocation(in.Timezone)
		planned := in.Start
		if !in.AllDay {
			d, _ := time.Parse(time.RFC3339, in.Start)
			planned = d.In(zone).Format("2006-01-02")
		}
		q, e := s.quests.ScheduleInTx(r.Context(), tx, id, *in.QuestID, in.QuestVersion, planned, in.Reason)
		if e != nil {
			return current, e
		}
		current.QuestVersion = q.Version
	}
	data, e := json.Marshal(in.Input)
	if e != nil {
		return current, e
	}
	a, b, c, d := dates(in.Input)
	_, e = tx.Exec(r.Context(), `UPDATE calendar_events SET payload=$3,all_day=$4,start_date=$5::text::date,end_date=$6::text::date,starts_at=$7::text::timestamptz,ends_at=$8::text::timestamptz,cancelled=$9,version=version+1,updated_at=$10 WHERE owner_id=$1 AND id=$2`, id, key, data, in.AllDay, a, b, c, d, in.Cancelled, s.clock.Now().UTC())
	if e != nil {
		return current, e
	}
	v, e := s.get(r.Context(), tx, id, key)
	if e == nil {
		e = s.history(r.Context(), tx, id, &current, v, in.Reason)
	}
	return v, e
}
func (s *Service) mountEdits(r chi.Router) {
	r.Patch("/api/v1/calendar/events/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in Edit) (any, error) {
			valid, e := Validate(in.Input)
			if e != nil {
				return nil, e
			}
			in.Input = valid
			if e = validReason(in.Reason); e != nil {
				return nil, e
			}
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			return s.idempotent(r, in, func(tx pgx.Tx) (any, error) { return s.edit(r, tx, id, chi.URLParam(r, "id"), in) })
		})
	})
	r.Patch("/api/v1/calendar/quests/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in struct {
			PlannedDate string `json:"planned_date"`
			Reason      string `json:"reason"`
		}) (any, error) { if _, e := day(in.PlannedDate); e != nil {
			return nil, e
		}; if e := validReason(in.Reason); e != nil {
			return nil, e
		}; id, e := owner(r.Context()); if e != nil {
			return nil, e
		}; return s.idempotent(r, in, func(tx pgx.Tx) (any, error) {
			var version int64
			if !uuid(chi.URLParam(r, "id")) {
				return nil, problem(404, "NOT_FOUND", "Задача не найдена")
			}
			e := tx.QueryRow(r.Context(), `SELECT version FROM quests WHERE owner_id=$1 AND id=$2`, id, chi.URLParam(r, "id")).Scan(&version)
			if e == pgx.ErrNoRows {
				return nil, problem(404, "NOT_FOUND", "Задача не найдена")
			}
			if e != nil {
				return nil, e
			}
			if p := platform.MatchVersion(r, version); p != nil {
				return nil, p
			}
			return s.quests.ScheduleInTx(r.Context(), tx, id, chi.URLParam(r, "id"), version, in.PlannedDate, in.Reason)
		}) })
	})
}
