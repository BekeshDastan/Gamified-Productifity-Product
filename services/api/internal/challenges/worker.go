package challenges

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
)

func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{"challenges.changed": s.ownerChanged, "challenges.close": s.expired}
}
func (s *Service) ownerChanged(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	if job.OwnerID == nil || s.progress == nil {
		return jobs.ErrPermanent
	}
	var request struct {
		After string `json:"after"`
	}
	if e := json.Unmarshal(job.Payload, &request); e != nil {
		return jobs.ErrPermanent
	}
	if request.After != "" && !privateapi.UUID(request.After) {
		return jobs.ErrPermanent
	}
	owner := *job.OwnerID
	if e := s.progress.LockOwner(ctx, tx, owner); e != nil {
		return e
	}
	var after *string
	if request.After != "" {
		after = &request.After
	}
	rows, e := tx.Query(ctx, "SELECT id::text,run_id::text,run_owner_id::text FROM challenge_participants WHERE owner_id=$1 AND ($2::uuid IS NULL OR id>$2::uuid) ORDER BY id LIMIT 11", owner, after)
	if e != nil {
		return e
	}
	type item struct{ id, run, creator string }
	items := []item{}
	for rows.Next() {
		var v item
		if e = rows.Scan(&v.id, &v.run, &v.creator); e != nil {
			rows.Close()
			return e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	more := len(items) > 10
	if more {
		items = items[:10]
	}
	for _, item := range items {
		run, e := s.getRun(ctx, tx, item.creator, item.run)
		if e != nil {
			return e
		}
		member, e := s.member(ctx, tx, owner, item.creator, item.run)
		if e != nil {
			return e
		}
		if member == nil {
			continue
		}
		if _, e = s.rebuild(ctx, tx, owner, run, *member); e != nil {
			return e
		}
	}
	if more {
		body, e := json.Marshal(struct {
			After string `json:"after"`
		}{items[len(items)-1].id})
		if e != nil {
			return e
		}
		return (jobs.Store{Clock: s.Clock}).Enqueue(ctx, tx, jobs.Event{OwnerID: &owner, Kind: "challenges.changed", Payload: body}, "challenge-rebuild/"+job.ID+"/"+items[len(items)-1].id)
	}
	return nil
}
