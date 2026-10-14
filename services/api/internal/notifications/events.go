package notifications

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *Service) invalidateEvent(ctx context.Context, tx pgx.Tx, owner, kind, id string) error {
	_, e := tx.Exec(ctx, `UPDATE notifications SET active=false,version=version+1 WHERE owner_id=$1 AND source_kind=$2 AND source_id=$3 AND active`, owner, kind, id)
	return e
}
func (s *Service) AchievementChanged(ctx context.Context, tx pgx.Tx, owner, id string, revision int64, active bool) error {
	if e := s.invalidateEvent(ctx, tx, owner, "achievement", id); e != nil {
		return e
	}
	if !active {
		return nil
	}
	_, _, e := s.EmitInTx(ctx, tx, owner, Event{Kind: "achievement", SourceKind: "achievement", SourceID: id, SourceRevision: revision, EventKey: "achievement/" + id, URL: "/app/achievements"})
	return e
}
func (s *Service) ReportCreated(ctx context.Context, tx pgx.Tx, owner, id string, revision int64) error {
	_, _, e := s.EmitInTx(ctx, tx, owner, Event{Kind: "report", SourceKind: "report", SourceID: id, SourceRevision: revision, EventKey: "report/" + id, URL: "/app/analytics?report=" + id})
	return e
}
func (s *Service) FocusChanged(ctx context.Context, tx pgx.Tx, owner, id string, revision int64, pending bool) error {
	if e := s.invalidateEvent(ctx, tx, owner, "focus", id); e != nil {
		return e
	}
	if !pending {
		return nil
	}
	_, _, e := s.EmitInTx(ctx, tx, owner, Event{Kind: "focus_result", SourceKind: "focus", SourceID: id, SourceRevision: revision, EventKey: "focus/" + id, URL: "/app/focus"})
	return e
}

// InviteCreated is the trusted integration point for stage 21. Only a persisted,
// recipient-bound invitation may emit; there is no public arbitrary-send route.
func (s *Service) InviteCreated(ctx context.Context, tx pgx.Tx, recipient, id string, revision int64) error {
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_invite_sources WHERE recipient_id=$1 AND id=$2 AND revision=$3 AND active AND expires_at>$4)`, recipient, id, revision, s.Clock.Now()).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		return nil
	}
	_, _, e := s.EmitInTx(ctx, tx, recipient, Event{Kind: "invite", SourceKind: "invite", SourceID: id, SourceRevision: revision, EventKey: "invite/" + id, URL: "/app/notifications"})
	return e
}
func (s *Service) eventCurrent(ctx context.Context, tx pgx.Tx, owner string, n Notification) (bool, error) {
	var exists bool
	var e error
	switch n.SourceKind {
	case "achievement":
		parts := strings.SplitN(n.SourceID, "/", 2)
		if len(parts) != 2 {
			return false, nil
		}
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_achievements a WHERE a.owner_id=$1 AND a.definition_id=$2 AND a.definition_version=$3 AND a.active AND $4=(SELECT max(id) FROM achievement_changes c WHERE c.owner_id=a.owner_id AND c.definition_id=a.definition_id AND c.definition_version=a.definition_version))`, owner, parts[0], parts[1], n.SourceRevision).Scan(&exists)
	case "report":
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM report_snapshots WHERE owner_id=$1 AND id::text=$2 AND revision=$3)`, owner, n.SourceID, n.SourceRevision).Scan(&exists)
	case "focus":
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM focus_sessions WHERE owner_id=$1 AND id::text=$2 AND version=$3 AND state='awaiting_confirmation')`, owner, n.SourceID, n.SourceRevision).Scan(&exists)
	case "invite":
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_invite_sources WHERE recipient_id=$1 AND id::text=$2 AND revision=$3 AND active AND expires_at>$4)`, owner, n.SourceID, n.SourceRevision, s.Clock.Now()).Scan(&exists)
	}
	return exists, e
}
