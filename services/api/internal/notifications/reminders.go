package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func (s *Service) WithHabits(h *habits.Service) *Service { s.habits = h; return s }
func (s *Service) WithSchedulingClock(clock platform.Clock) *Service {
	s.schedulingClock = clock
	return s
}
func (s *Service) jobTimes(due time.Time) (time.Time, time.Time) {
	now := s.Clock.Now()
	if s.schedulingClock != nil {
		scheduled := s.schedulingClock.Now()
		return scheduled.Add(max(0, due.Sub(now))), scheduled
	}
	return due, now
}

type Reminder struct {
	ID             string     `json:"id"`
	SourceKind     string     `json:"source_kind"`
	SourceID       string     `json:"source_id"`
	ParentID       *string    `json:"parent_id"`
	SourceRevision int64      `json:"source_revision"`
	SourceDueAt    time.Time  `json:"source_due_at"`
	NextRunAt      time.Time  `json:"next_run_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Version        int64      `json:"version"`
	State          string     `json:"state"`
	UrgentOverride bool       `json:"urgent_override"`
	SnoozedUntil   *time.Time `json:"snoozed_until"`
	NotificationID *string    `json:"notification_id"`
	URL            string     `json:"url"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

const reminderColumns = `id::text,source_kind,source_id::text,parent_id::text,source_revision,source_due_at,next_run_at,expires_at,version,state,urgent_override,snoozed_until,notification_id::text,source_url,created_at,updated_at`

func scanReminder(row pgx.Row) (Reminder, error) {
	var r Reminder
	e := row.Scan(&r.ID, &r.SourceKind, &r.SourceID, &r.ParentID, &r.SourceRevision, &r.SourceDueAt, &r.NextRunAt, &r.ExpiresAt, &r.Version, &r.State, &r.UrgentOverride, &r.SnoozedUntil, &r.NotificationID, &r.URL, &r.CreatedAt, &r.UpdatedAt)
	return r, e
}

type reminderJob struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type reminderSource struct {
	Kind, ID          string
	Parent            *string
	Revision          int64
	Due, Run, Expires time.Time
	URL               string
}

func (s *Service) enqueueReminder(ctx context.Context, tx pgx.Tx, owner string, r Reminder) error {
	due, created := s.jobTimes(r.NextRunAt)
	raw, e := json.Marshal(reminderJob{r.ID, r.Version})
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'notifications.reminder',$2,$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING`, owner, raw, fmt.Sprintf("reminder/%s/%s/%d", owner, r.ID, r.Version), due, created)
	return e
}
func (s *Service) cancelReminder(ctx context.Context, tx pgx.Tx, owner string, r Reminder) error {
	if r.State == "cancelled" {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE reminders SET state='cancelled',version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2`, owner, r.ID, s.Clock.Now())
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE notifications SET active=false,version=version+1 WHERE owner_id=$1 AND reminder_id=$2 AND active`, owner, r.ID)
	return e
}
func (s *Service) upsertReminder(ctx context.Context, tx pgx.Tx, owner string, source reminderSource) error {
	now := s.Clock.Now()
	run := source.Run
	if run.Before(now) {
		run = now
	}
	r, e := scanReminder(tx.QueryRow(ctx, `SELECT `+reminderColumns+` FROM reminders WHERE owner_id=$1 AND source_kind=$2 AND source_id=$3 AND source_revision=$4 AND source_due_at=$5`, owner, source.Kind, source.ID, source.Revision, source.Due))
	if e == nil {
		if r.State == "fired" {
			return nil
		}
		if r.SnoozedUntil != nil {
			run = *r.SnoozedUntil
		}
		if r.State == "scheduled" && (r.NextRunAt.Equal(run) || (r.SnoozedUntil == nil && !r.NextRunAt.After(now) && !source.Run.After(now))) {
			return nil
		}
		r, e = scanReminder(tx.QueryRow(ctx, `UPDATE reminders SET next_run_at=$3,expires_at=$4,state='scheduled',version=version+1,updated_at=$5 WHERE owner_id=$1 AND id=$2 RETURNING `+reminderColumns, owner, r.ID, run, source.Expires, now))
		if e != nil {
			return e
		}
		return s.enqueueReminder(ctx, tx, owner, r)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	r, e = scanReminder(tx.QueryRow(ctx, `INSERT INTO reminders(owner_id,source_kind,source_id,parent_id,source_revision,source_due_at,next_run_at,expires_at,state,source_url,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'scheduled',$9,$10,$10) RETURNING `+reminderColumns, owner, source.Kind, source.ID, source.Parent, source.Revision, source.Due, run, source.Expires, source.URL, now))
	if e != nil {
		return e
	}
	return s.enqueueReminder(ctx, tx, owner, r)
}
func (s *Service) reminder(ctx context.Context, tx pgx.Tx, owner, id string) (Reminder, error) {
	if !privateapi.UUID(id) {
		return Reminder{}, privateapi.Problem(404, "NOT_FOUND", "Напоминание не найдено")
	}
	r, e := scanReminder(tx.QueryRow(ctx, `SELECT `+reminderColumns+` FROM reminders WHERE owner_id=$1 AND id=$2`, owner, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = privateapi.Problem(404, "NOT_FOUND", "Напоминание не найдено")
	}
	return r, e
}
func (s *Service) currentReminderSource(ctx context.Context, tx pgx.Tx, owner string, r Reminder, p Preferences) (reminderSource, bool, error) {
	out := reminderSource{Kind: r.SourceKind, ID: r.SourceID, Parent: r.ParentID, URL: r.URL}
	now := s.Clock.Now()
	var enabled bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND disabled_at IS NULL)`, owner).Scan(&enabled); e != nil || !enabled {
		return out, false, e
	}
	switch r.SourceKind {
	case "task":
		var due *time.Time
		var version int64
		var valid bool
		e := tx.QueryRow(ctx, `SELECT deadline,version,archived_at IS NULL AND status NOT IN('completed','failed','skipped') AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) FROM quests q WHERE owner_id=$1 AND id=$2`, owner, r.SourceID).Scan(&due, &version, &valid)
		if errors.Is(e, pgx.ErrNoRows) {
			return out, false, nil
		}
		if e != nil {
			return out, false, e
		}
		if due == nil || !valid {
			return out, false, nil
		}
		out.Revision = version
		out.Due = *due
		out.Run = due.Add(-time.Duration(p.DeadlineLeadMinutes) * time.Minute)
		out.Expires = due.Add(24 * time.Hour)
		out.URL = "/app/quests/" + r.SourceID
		return out, now.Before(out.Expires), nil
	case "habit_period":
		if s.habits == nil || r.ParentID == nil || p.HabitLeadMinutes == 0 {
			return out, false, nil
		}
		period, version, valid, e := s.habits.ReminderPeriodInTx(ctx, tx, owner, *r.ParentID, r.SourceID)
		if e != nil || !valid {
			return out, false, e
		}
		out.Revision = version
		out.Due = period.End
		out.Expires = period.End
		out.Run = period.End.Add(-time.Duration(p.HabitLeadMinutes) * time.Minute)
		if out.Run.Before(period.Start) {
			out.Run = period.Start
		}
		out.URL = "/app/habits/" + *r.ParentID
		return out, true, nil
	}
	return out, false, nil
}
func (s *Service) projectReminders(ctx context.Context, tx pgx.Tx, owner, after string) error {
	var locked string
	if e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&locked); e != nil {
		return e
	}
	p, e := s.preferences(ctx, tx, owner)
	if e != nil {
		return e
	}
	now := s.Clock.Now()
	// Cancel stale rows without editing leased jobs; their handlers check generation.
	rows, e := tx.Query(ctx, `SELECT `+reminderColumns+` FROM reminders r WHERE owner_id=$1 AND state<>'cancelled' AND (expires_at<=$2 OR (source_kind='task' AND NOT EXISTS(SELECT 1 FROM quests q WHERE q.owner_id=r.owner_id AND q.id=r.source_id AND q.version=r.source_revision AND q.deadline=r.source_due_at AND q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped') AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id))) OR (source_kind='habit_period' AND NOT EXISTS(SELECT 1 FROM habits h WHERE h.owner_id=r.owner_id AND h.id=r.parent_id AND h.version=r.source_revision AND h.archived_at IS NULL))) ORDER BY id`, owner, now)
	if e != nil {
		return e
	}
	cancel := []Reminder{}
	for rows.Next() {
		r, e := scanReminder(rows)
		if e != nil {
			rows.Close()
			return e
		}
		cancel = append(cancel, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, r := range cancel {
		if e = s.cancelReminder(ctx, tx, owner, r); e != nil {
			return e
		}
	}
	var cursor *string
	if after != "" {
		cursor = &after
	}
	rows, e = tx.Query(ctx, `SELECT q.id::text,q.version,q.deadline FROM quests q WHERE q.owner_id=$1 AND ($2::uuid IS NULL OR q.id>$2::uuid) AND q.deadline>$3 AND q.deadline<=$4 AND q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped') AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) ORDER BY q.id LIMIT 501`, owner, cursor, now.Add(-24*time.Hour), now.AddDate(0, 0, 14))
	if e != nil {
		return e
	}
	sources := []reminderSource{}
	for rows.Next() {
		v := reminderSource{Kind: "task"}
		if e = rows.Scan(&v.ID, &v.Revision, &v.Due); e != nil {
			rows.Close()
			return e
		}
		v.Run = v.Due.Add(-time.Duration(p.DeadlineLeadMinutes) * time.Minute)
		v.Expires = v.Due.Add(24 * time.Hour)
		v.URL = "/app/quests/" + v.ID
		sources = append(sources, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	more := len(sources) > 500
	if more {
		sources = sources[:500]
	}
	for _, v := range sources {
		if e = s.upsertReminder(ctx, tx, owner, v); e != nil {
			return e
		}
	}
	if more {
		raw, _ := json.Marshal(struct {
			After string `json:"after"`
		}{sources[499].ID})
		_, e = tx.Exec(ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'notifications.reminders_continue',$2,$3,$4,$4) ON CONFLICT(dedupe_key) DO NOTHING`, owner, raw, fmt.Sprintf("reminder-continuation/%s/%s/%s", owner, sources[499].ID, now.UTC().Truncate(time.Minute).Format(time.RFC3339)), now)
		if e != nil {
			return e
		}
	}
	if after != "" || s.habits == nil {
		return nil
	}
	evidence, complete, e := s.habits.EvidenceInTx(ctx, tx, owner)
	if e != nil {
		return e
	}
	validIDs := []string{}
	if p.HabitLeadMinutes > 0 {
		for _, h := range evidence {
			for _, period := range h.Periods {
				if period.Excluded || period.Amount >= period.Target || !now.Before(period.End) || now.Before(period.Start) {
					continue
				}
				validIDs = append(validIDs, period.ID)
				parent := h.ID
				v := reminderSource{Kind: "habit_period", ID: period.ID, Parent: &parent, Revision: h.Version, Due: period.End, Expires: period.End, URL: "/app/habits/" + h.ID, Run: period.End.Add(-time.Duration(p.HabitLeadMinutes) * time.Minute)}
				if v.Run.Before(period.Start) {
					v.Run = period.Start
				}
				if e = s.upsertReminder(ctx, tx, owner, v); e != nil {
					return e
				}
			}
		}
	}
	if complete {
		rows, e = tx.Query(ctx, `SELECT `+reminderColumns+` FROM reminders WHERE owner_id=$1 AND source_kind='habit_period' AND state<>'cancelled' AND NOT(source_id::text=ANY($2::text[]))`, owner, validIDs)
		if e != nil {
			return e
		}
		cancel = []Reminder{}
		for rows.Next() {
			r, e := scanReminder(rows)
			if e != nil {
				rows.Close()
				return e
			}
			cancel = append(cancel, r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, r := range cancel {
			if e = s.cancelReminder(ctx, tx, owner, r); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) fireReminder(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
	var in reminderJob
	var keys map[string]json.RawMessage
	if j.OwnerID == nil || !privateapi.UUID(*j.OwnerID) || json.Unmarshal(j.Payload, &in) != nil || json.Unmarshal(j.Payload, &keys) != nil || len(keys) != 2 || !privateapi.UUID(in.ID) || in.Version < 1 {
		return jobs.ErrPermanent
	}
	owner := *j.OwnerID
	var locked string
	if e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&locked); e != nil {
		return e
	}
	r, e := s.reminder(ctx, tx, owner, in.ID)
	if e != nil {
		return e
	}
	if r.Version != in.Version || r.State != "scheduled" {
		return nil
	}
	p, e := s.preferences(ctx, tx, owner)
	if e != nil {
		return e
	}
	source, valid, e := s.currentReminderSource(ctx, tx, owner, r, p)
	if e != nil {
		return e
	}
	if !valid || source.Revision != r.SourceRevision || !source.Due.Equal(r.SourceDueAt) {
		return s.cancelReminder(ctx, tx, owner, r)
	}
	now := s.Clock.Now()
	if now.Before(r.NextRunAt) {
		r.Version++
		_, e = tx.Exec(ctx, `UPDATE reminders SET version=$3,updated_at=$4 WHERE owner_id=$1 AND id=$2`, owner, r.ID, r.Version, now)
		if e != nil {
			return e
		}
		return s.enqueueReminder(ctx, tx, owner, r)
	}
	kind := "deadline"
	if r.SourceKind == "habit_period" {
		kind = "habit"
	}
	id := r.ID
	n, _, e := s.EmitInTx(ctx, tx, owner, Event{Kind: kind, SourceKind: r.SourceKind, SourceID: r.SourceID, SourceRevision: r.SourceRevision, EventKey: fmt.Sprintf("reminder/%s/%d", r.ID, r.Version), URL: r.URL, ReminderID: &id})
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE reminders SET state='fired',version=version+1,notification_id=$3,updated_at=$4 WHERE owner_id=$1 AND id=$2`, owner, r.ID, n.ID, now)
	return e
}
func (s *Service) mountReminders(r chi.Router) {
	r.Get("/api/v1/reminders", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			rows, e := tx.Query(ctx, `SELECT `+reminderColumns+` FROM reminders WHERE owner_id=$1 AND state IN('scheduled','fired') ORDER BY next_run_at DESC,id DESC LIMIT 100`, owner)
			if e != nil {
				return nil, e
			}
			defer rows.Close()
			items := []Reminder{}
			for rows.Next() {
				v, e := scanReminder(rows)
				if e != nil {
					return nil, e
				}
				items = append(items, v)
			}
			return map[string]any{"items": items}, rows.Err()
		})
		s.Reply(w, r, out, e)
	})
	r.Get("/api/v1/reminders/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.reminder(ctx, tx, owner, chi.URLParam(r, "id"))
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/reminders/{id}/snooze", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in struct {
			Minutes        int  `json:"minutes"`
			UrgentOverride bool `json:"urgent_override"`
			Confirm        bool `json:"confirm"`
		}) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				old, e := s.reminder(ctx, tx, owner, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if e := privateapi.MatchVersion(r, old.Version); e != nil {
					return nil, e
				}
				if !in.Confirm || in.Minutes < 5 || in.Minutes > 1440 || in.UrgentOverride && old.SourceKind != "task" {
					return nil, privateapi.Invalid("Подтвердите отсрочку 5–1440 минут; срочное исключение доступно только задаче")
				}
				p, e := s.preferences(ctx, tx, owner)
				if e != nil {
					return nil, e
				}
				source, valid, e := s.currentReminderSource(ctx, tx, owner, old, p)
				if e != nil {
					return nil, e
				}
				if !valid || source.Revision != old.SourceRevision || !source.Due.Equal(old.SourceDueAt) {
					return nil, privateapi.Problem(409, "SOURCE_CHANGED", "Источник напоминания изменился")
				}
				next := s.Clock.Now().Add(time.Duration(in.Minutes) * time.Minute)
				if !next.Before(source.Expires) {
					return nil, privateapi.Invalid("К этому времени окно напоминания уже завершится")
				}
				updated, e := scanReminder(tx.QueryRow(ctx, `UPDATE reminders SET next_run_at=$3,snoozed_until=$3,urgent_override=$4,version=version+1,state='scheduled',updated_at=$5 WHERE owner_id=$1 AND id=$2 RETURNING `+reminderColumns, owner, old.ID, next, in.UrgentOverride, s.Clock.Now()))
				if e != nil {
					return nil, e
				}
				if e = s.enqueueReminder(ctx, tx, owner, updated); e != nil {
					return nil, e
				}
				if old.NotificationID != nil {
					if _, e = tx.Exec(ctx, `UPDATE notifications SET active=false,version=version+1 WHERE owner_id=$1 AND id=$2 AND active`, owner, *old.NotificationID); e != nil {
						return nil, e
					}
				}
				return updated, nil
			})
		})
	})
}
