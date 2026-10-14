package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
	"time"
)

type reportSweepInput struct {
	After  string `json:"after"`
	Bucket string `json:"bucket"`
}

func (s *Service) StartReportSchedule(ctx context.Context) error {
	now := s.Clock.Now().UTC()
	bucket := now.Truncate(time.Hour).Format(time.RFC3339)
	body, _ := json.Marshal(reportSweepInput{Bucket: bucket})
	_, e := s.Pool.Exec(ctx, `INSERT INTO jobs(kind,payload,dedupe_key,next_run_at,created_at) VALUES('analytics.reports_sweep',$1,$2,$3,$3) ON CONFLICT(dedupe_key) DO NOTHING`, body, "report-sweep/"+bucket+"/start", now)
	return e
}
func (s *Service) reportSweep(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
	var in reportSweepInput
	var keys map[string]json.RawMessage
	if j.OwnerID != nil || json.Unmarshal(j.Payload, &in) != nil || json.Unmarshal(j.Payload, &keys) != nil || len(keys) != 2 || keys["after"] == nil || keys["bucket"] == nil || (in.After != "" && !privateapi.UUID(in.After)) {
		return jobs.ErrPermanent
	}
	bucket, e := time.Parse(time.RFC3339, in.Bucket)
	if e != nil || bucket.UTC().Truncate(time.Hour).Format(time.RFC3339) != in.Bucket {
		return jobs.ErrPermanent
	}
	var after *string
	if in.After != "" {
		after = &in.After
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM users WHERE ($1::uuid IS NULL OR id>$1::uuid) ORDER BY id LIMIT 100`, after)
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
		if _, e = tx.Exec(ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'analytics.reports_due','{}',$2,$3,$3) ON CONFLICT(dedupe_key) DO NOTHING`, id, "report-owner/"+in.Bucket+"/"+id, now); e != nil {
			return e
		}
	}
	next := reportSweepInput{Bucket: in.Bucket}
	due := now
	key := "report-sweep/" + in.Bucket + "/"
	if len(ids) == 100 {
		next.After = ids[len(ids)-1]
		key += next.After
	} else {
		due = now.UTC().Truncate(time.Hour).Add(time.Hour)
		next.Bucket = due.Format(time.RFC3339)
		key = "report-sweep/" + next.Bucket + "/start"
	}
	body, _ := json.Marshal(next)
	_, e = tx.Exec(ctx, `INSERT INTO jobs(kind,payload,dedupe_key,next_run_at,created_at) VALUES('analytics.reports_sweep',$1,$2,$3,$4) ON CONFLICT(dedupe_key) DO NOTHING`, body, key, due, now)
	return e
}
func (s *Service) reportsDue(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
	var body map[string]json.RawMessage
	if j.OwnerID == nil || !privateapi.UUID(*j.OwnerID) || json.Unmarshal(j.Payload, &body) != nil || body == nil || len(body) != 0 {
		return jobs.ErrPermanent
	}
	owner := *j.OwnerID
	var locked string
	e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&locked)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	state, e := s.state(ctx, tx, owner)
	if e != nil {
		return e
	}
	if state.From != nil {
		if _, e = s.read(ctx, tx, owner, Range{*state.From, *state.To}, true, false); e != nil {
			return e
		}
	}
	var timezone string
	if e = tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC') FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&timezone); e != nil {
		return e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return e
	}
	now := s.Clock.Now().In(zone)
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	week := date.AddDate(0, 0, -(int(date.Weekday())+6)%7).AddDate(0, 0, -7)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, zone).AddDate(0, -1, 0)
	for _, in := range []ReportInput{{"week", week.Format(time.DateOnly), timezone}, {"month", month.Format(time.DateOnly), timezone}} {
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM report_snapshots WHERE owner_id=$1 AND kind=$2 AND starts_on=$3::text::date AND timezone=$4)`, owner, in.Kind, in.StartsOn, in.Timezone).Scan(&exists); e != nil {
			return e
		}
		if exists {
			continue
		}
		snapshot, e := s.reportSnapshot(ctx, tx, owner, in)
		if e != nil {
			var p *platform.APIError
			if errors.As(e, &p) && (p.Code == "PERIOD_NOT_CLOSED" || p.Code == "PROJECTION_PENDING") {
				continue
			}
			return e
		}
		if !snapshot.CurrentObserved {
			continue
		}
		b, e := json.Marshal(snapshot)
		if e != nil {
			return e
		}
		var reportID string
		var revision int64
		if e = tx.QueryRow(ctx, `INSERT INTO report_snapshots(owner_id,id,kind,starts_on,ends_before,timezone,revision,automatic,snapshot,created_at) SELECT $1,gen_random_uuid(),$2,$3::text::date,$4::text::date,$5,COALESCE(max(revision),0)+1,true,$6,$7 FROM report_snapshots WHERE owner_id=$1 AND kind=$2 AND starts_on=$3::text::date AND timezone=$5 ON CONFLICT DO NOTHING RETURNING id::text,revision`, owner, in.Kind, snapshot.From, snapshot.To, timezone, b, s.Clock.Now()).Scan(&reportID, &revision); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			return e
		}
		if s.reportObserver != nil {
			if e = s.reportObserver(ctx, tx, owner, reportID, revision); e != nil {
				return e
			}
		}
	}
	return nil
}
