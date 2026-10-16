package social

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type InviteObserver func(context.Context, pgx.Tx, string, string, int64) error

func (s *Service) WithInvites(observer InviteObserver) *Service { s.invites = observer; return s }
func (s *Service) inviteNotification(ctx context.Context, tx pgx.Tx, recipient, id string) error {
	_, e := tx.Exec(ctx, "INSERT INTO notification_invite_sources(recipient_id,id,revision,expires_at,created_at) VALUES($1,$2,1,$3::timestamptz+interval '7 days',$3) ON CONFLICT DO NOTHING", recipient, id, s.Clock.Now())
	if e != nil {
		return e
	}
	if s.invites != nil {
		return s.invites(ctx, tx, recipient, id, 1)
	}
	return nil
}
func closeInvite(ctx context.Context, tx pgx.Tx, recipient, id string) error {
	_, e := tx.Exec(ctx, "UPDATE notification_invite_sources SET active=false WHERE recipient_id=$1 AND id=$2", recipient, id)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "UPDATE notifications SET active=false,version=version+1 WHERE owner_id=$1 AND source_kind='invite' AND source_id=$2 AND active", recipient, id)
	return e
}
