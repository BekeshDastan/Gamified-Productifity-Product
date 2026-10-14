package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
	"time"
)

type sweepInput struct {
	After  string `json:"after"`
	Bucket string `json:"bucket"`
}

func (s *Service) StartReminderSchedule(ctx context.Context) error {
	now := s.Clock.Now()
	bucket := now.UTC().Truncate(time.Minute).Format(time.RFC3339)
	raw, _ := json.Marshal(sweepInput{Bucket: bucket})
	_, e := s.Pool.Exec(ctx, `INSERT INTO jobs(kind,payload,dedupe_key,next_run_at,created_at) VALUES('notifications.reminders_sweep',$1,$2,$3,$3) ON CONFLICT(dedupe_key) DO NOTHING`, raw, "reminder-sweep/"+bucket+"/start", now)
	return e
}
func (s *Service) reminderSweep(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
	var in sweepInput
	var keys map[string]json.RawMessage
	if j.OwnerID != nil || json.Unmarshal(j.Payload, &in) != nil || json.Unmarshal(j.Payload, &keys) != nil || len(keys) != 2 || keys["after"] == nil || keys["bucket"] == nil || in.After != "" && !privateapi.UUID(in.After) {
		return jobs.ErrPermanent
	}
	bucket, e := time.Parse(time.RFC3339, in.Bucket)
	if e != nil || bucket.UTC().Truncate(time.Minute).Format(time.RFC3339) != in.Bucket {
		return jobs.ErrPermanent
	}
	var after *string
	if in.After != "" {
		after = &in.After
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM users WHERE disabled_at IS NULL AND ($1::uuid IS NULL OR id>$1::uuid) ORDER BY id LIMIT 100`, after)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	now := s.Clock.Now()
	for _, id := range ids {
		_, e = tx.Exec(ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'notifications.changed','{}',$2,$3,$3) ON CONFLICT(dedupe_key) DO NOTHING`, id, "reminder-owner/"+in.Bucket+"/"+id, now)
		if e != nil {
			return e
		}
	}
	next := sweepInput{Bucket: in.Bucket}
	due := now
	key := "reminder-sweep/" + in.Bucket + "/"
	if len(ids) == 100 {
		next.After = ids[99]
		key += next.After
	} else {
		due = now.UTC().Truncate(time.Minute).Add(time.Minute)
		next.Bucket = due.Format(time.RFC3339)
		key = "reminder-sweep/" + next.Bucket + "/start"
	}
	raw, _ := json.Marshal(next)
	_, e = tx.Exec(ctx, `INSERT INTO jobs(kind,payload,dedupe_key,next_run_at,created_at) VALUES('notifications.reminders_sweep',$1,$2,$3,$4) ON CONFLICT(dedupe_key) DO NOTHING`, raw, key, due, now)
	return e
}
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{
		"notifications.reminders_sweep": s.reminderSweep, "notifications.reminder": s.fireReminder,
		"notifications.changed": func(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
			var keys map[string]json.RawMessage
			if j.OwnerID == nil || !privateapi.UUID(*j.OwnerID) || json.Unmarshal(j.Payload, &keys) != nil || keys == nil || len(keys) != 0 {
				return jobs.ErrPermanent
			}
			return s.projectReminders(ctx, tx, *j.OwnerID, "")
		},
		"notifications.reminders_continue": func(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
			var in struct {
				After string `json:"after"`
			}
			var keys map[string]json.RawMessage
			if j.OwnerID == nil || !privateapi.UUID(*j.OwnerID) || json.Unmarshal(j.Payload, &in) != nil || json.Unmarshal(j.Payload, &keys) != nil || len(keys) != 1 || !privateapi.UUID(in.After) {
				return jobs.ErrPermanent
			}
			return s.projectReminders(ctx, tx, *j.OwnerID, in.After)
		},
	}
}
func (s *Service) ProjectReminders(ctx context.Context, tx pgx.Tx, owner string) error {
	if !privateapi.UUID(owner) {
		return fmt.Errorf("invalid owner")
	}
	return s.projectReminders(ctx, tx, owner, "")
}
