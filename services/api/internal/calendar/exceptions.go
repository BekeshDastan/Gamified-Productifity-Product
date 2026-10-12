package calendar

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
)

type SeriesEdit struct {
	Action        string       `json:"action"`
	OccurrenceKey string       `json:"occurrence_key"`
	Replacement   *SeriesInput `json:"replacement"`
	Until         *string      `json:"until"`
	Reason        string       `json:"reason"`
}

func (s *Service) seriesHistory(ctx context.Context, tx pgx.Tx, id string, before *Series, after Series, reason string) error {
	old, e := json.Marshal(before)
	if e != nil {
		return e
	}
	data, e := json.Marshal(after)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO recurrence_series_history(owner_id,series_id,before_state,after_state,reason,created_at) VALUES($1,$2,$3,$4,$5,$6)`, id, after.ID, old, data, reason, s.clock.Now().UTC())
	return e
}
func (s *Service) cancelFuture(ctx context.Context, tx pgx.Tx, id, series, from, reason string) error {
	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND e.series_id=$2 AND e.occurrence_key>=$3::text::date AND NOT e.cancelled AND (q.id IS NULL OR (q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped'))) ORDER BY e.occurrence_key`, id, series, from)
	if e != nil {
		return e
	}
	events := []Event{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			rows.Close()
			return e
		}
		events = append(events, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range events {
		before := v
		if v.QuestID != nil {
			q, e := s.quests.CancelOccurrenceInTx(ctx, tx, id, *v.QuestID, v.QuestVersion, reason)
			if e != nil {
				return e
			}
			v.Status = q.Status
			v.QuestVersion = q.Version
		}
		v.Cancelled = true
		v.Version++
		if _, e := tx.Exec(ctx, `UPDATE calendar_events SET cancelled=true,version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2`, id, v.ID, s.clock.Now().UTC()); e != nil {
			return e
		}
		if e := s.history(ctx, tx, id, &before, v, reason); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) changeSeries(r *http.Request, tx pgx.Tx, id, key string, in SeriesEdit) (Series, error) {
	current, e := s.getSeries(r.Context(), tx, id, key)
	if e != nil {
		return current, e
	}
	if p := platform.MatchVersion(r, current.Version); p != nil {
		return current, p
	}
	before := current
	switch in.Action {
	case "end":
		if in.Until == nil {
			return current, invalid("until", "Укажите последний день серии")
		}
		end, e := day(*in.Until)
		if e != nil {
			return current, e
		}
		if current.EndsBefore != nil && *in.Until >= *current.EndsBefore {
			return current, problem(409, "SERIES_SPLIT", "Серия разделена. Изменяйте её продолжение")
		}
		current.Rule.Until = in.Until
		current.Rule.Count = 0
		valid, e := validateSeries(current.SeriesInput)
		if e != nil {
			return current, e
		}
		current.SeriesInput = valid
		data, e := json.Marshal(valid)
		if e != nil {
			return current, e
		}
		_, e = tx.Exec(r.Context(), `UPDATE recurrence_series SET payload=$3,version=version+1 WHERE owner_id=$1 AND id=$2`, id, key, data)
		if e != nil {
			return current, e
		}
		current.Version++
		if e = s.cancelFuture(r.Context(), tx, id, key, end.AddDate(0, 0, 1).Format("2006-01-02"), in.Reason); e != nil {
			return current, e
		}
		return current, s.seriesHistory(r.Context(), tx, id, &before, current, in.Reason)
	case "split":
		split, e := day(in.OccurrenceKey)
		if e != nil {
			return current, e
		}
		if in.Replacement == nil {
			return current, invalid("replacement", "Укажите новое правило будущих повторений")
		}
		if current.EndsBefore != nil && in.OccurrenceKey >= *current.EndsBefore {
			return current, problem(409, "SERIES_SPLIT", "Эта часть серии уже отделена")
		}
		var exists bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND e.series_id=$2 AND e.occurrence_key=$3::text::date AND NOT e.cancelled AND (q.id IS NULL OR (q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped'))))`, id, key, in.OccurrenceKey).Scan(&exists)
		if e != nil {
			return current, e
		}
		if !exists {
			return current, problem(409, "HISTORY_LOCKED", "Выберите незавершённое повторение")
		}
		valid, e := validateSeries(*in.Replacement)
		if e != nil {
			return current, e
		}
		anchor, _, _ := seriesAnchor(valid.Event)
		if anchor.Before(split) {
			return current, invalid("start", "Продолжение не раньше выбранного повторения")
		}
		rows, e := tx.Query(r.Context(), `SELECT e.occurrence_key::text FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND e.series_id=$2 AND e.occurrence_key>=$3::text::date AND (e.cancelled OR q.status IN('completed','failed','skipped') OR q.archived_at IS NOT NULL) UNION SELECT occurrence_key::text FROM recurrence_exclusions WHERE owner_id=$1 AND series_id=$2 AND occurrence_key>=$3::text::date`, id, key, in.OccurrenceKey)
		if e != nil {
			return current, e
		}
		preserved := []string{}
		for rows.Next() {
			var date string
			if e := rows.Scan(&date); e != nil {
				rows.Close()
				return current, e
			}
			preserved = append(preserved, date)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return current, e
		}
		continuation, e := s.insertSeries(r.Context(), tx, id, valid)
		if e != nil {
			return current, e
		}
		for _, date := range preserved {
			if _, e := tx.Exec(r.Context(), `INSERT INTO recurrence_exclusions(owner_id,series_id,occurrence_key,reason) VALUES($1,$2,$3::text::date,$4)`, id, continuation.ID, date, "Сохранена история прежней серии"); e != nil {
				return current, e
			}
		}
		current.EndsBefore = &in.OccurrenceKey
		current.Version++
		if _, e = tx.Exec(r.Context(), `UPDATE recurrence_series SET ends_before=$3::text::date,version=version+1 WHERE owner_id=$1 AND id=$2`, id, key, in.OccurrenceKey); e != nil {
			return current, e
		}
		if e = s.cancelFuture(r.Context(), tx, id, key, in.OccurrenceKey, in.Reason); e != nil {
			return current, e
		}
		if e = s.seriesHistory(r.Context(), tx, id, &before, current, in.Reason); e != nil {
			return current, e
		}
		return continuation, s.seriesHistory(r.Context(), tx, id, nil, continuation, "Продолжение: "+key+". "+in.Reason)
	default:
		return current, invalid("action", "Выберите изменение конца или этого и будущих повторений")
	}
}
func (s *Service) mountExceptions(r chi.Router) {
	r.Patch("/api/v1/calendar/series/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in SeriesEdit) (any, error) {
			if e := validReason(in.Reason); e != nil {
				return nil, e
			}
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			return s.idempotent(r, in, func(tx pgx.Tx) (any, error) { return s.changeSeries(r, tx, id, chi.URLParam(r, "id"), in) })
		})
	})
}
