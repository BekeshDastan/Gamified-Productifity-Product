// Package social exposes only explicitly shared, allowlisted summaries.
package social

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ *privateapi.Service }

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{privateapi.New(pool, clock)}
}
func missing() error {
	return privateapi.Problem(404, "NOT_FOUND", "Запись недоступна")
}
func conflict() error {
	return privateapi.Problem(409, "SOCIAL_CONFLICT", "Обновите раздел: состояние изменилось")
}
func limit() error {
	return privateapi.Problem(429, "SOCIAL_LIMIT", "Достигнут предел действий. Попробуйте позже")
}

// All social reads and writes serialize their authorization check and projection.
// No response cache stores another person's data; mutation receipts contain only IDs.
func lock(ctx context.Context, tx pgx.Tx, owner string) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(210021)"); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO social_profiles(owner_id) VALUES($1) ON CONFLICT DO NOTHING", owner)
	return e
}
func verified(ctx context.Context, tx pgx.Tx, owner string) error {
	var yes bool
	if e := tx.QueryRow(ctx, "SELECT email_verified_at IS NOT NULL AND disabled_at IS NULL FROM users WHERE id=$1", owner).Scan(&yes); e != nil {
		return e
	}
	if !yes {
		return privateapi.Problem(403, "EMAIL_VERIFICATION_REQUIRED", "Подтвердите почту в настройках безопасности")
	}
	return nil
}
func blocked(ctx context.Context, tx pgx.Tx, a, b string) (bool, error) {
	var yes bool
	e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM social_blocks WHERE (owner_id=$1 AND target_id=$2) OR (owner_id=$2 AND target_id=$1))", a, b).Scan(&yes)
	return yes, e
}
func friends(ctx context.Context, tx pgx.Tx, a, b string) error {
	if !privateapi.UUID(b) || a == b {
		return missing()
	}
	deny, e := blocked(ctx, tx, a, b)
	if e != nil {
		return e
	}
	if deny {
		return missing()
	}
	var yes bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM social_friends WHERE state='accepted' AND ((sender=$1 AND recipient=$2) OR(sender=$2 AND recipient=$1)))", a, b).Scan(&yes)
	if e != nil {
		return e
	}
	if !yes {
		return missing()
	}
	return nil
}
func audit(ctx context.Context, tx pgx.Tx, owner, action, id string, now time.Time) error {
	_, e := tx.Exec(ctx, "INSERT INTO audit_events(actor_id,action,entity_type,entity_id,created_at) VALUES($1,$2,'social',$3,$4)", owner, "social."+action, id, now)
	return e
}

type Receipt struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
}

func receipt(id string) Receipt { return Receipt{id, true} }

type FriendAction struct {
	Action  string `json:"action"`
	Code    string `json:"code"`
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type Friend struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	State    string `json:"state"`
	Incoming bool   `json:"incoming"`
	Version  int64  `json:"version"`
}
type Friends struct {
	Code     string   `json:"code"`
	Verified bool     `json:"verified"`
	Items    []Friend `json:"items"`
}

func (s *Service) listFriends(ctx context.Context, tx pgx.Tx, owner string) (Friends, error) {
	out := Friends{Items: []Friend{}}
	e := tx.QueryRow(ctx, "SELECT p.invite_code::text,u.email_verified_at IS NOT NULL FROM social_profiles p JOIN users u ON u.id=p.owner_id WHERE p.owner_id=$1", owner).Scan(&out.Code, &out.Verified)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT f.id::text,p.owner_id::text,COALESCE(NULLIF(p.nickname,''),'Участник'),f.state,f.recipient=$1,f.version FROM social_friends f JOIN profiles p ON p.owner_id=CASE WHEN f.sender=$1 THEN f.recipient ELSE f.sender END WHERE $1 IN(f.sender,f.recipient) AND f.state IN('pending','accepted') AND NOT EXISTS(SELECT 1 FROM social_blocks b WHERE (b.owner_id=$1 AND b.target_id=p.owner_id) OR(b.target_id=$1 AND b.owner_id=p.owner_id)) ORDER BY f.created_at DESC LIMIT 200`, owner)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Friend
		if e = rows.Scan(&v.ID, &v.UserID, &v.Nickname, &v.State, &v.Incoming, &v.Version); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
func (s *Service) friendAction(ctx context.Context, tx pgx.Tx, owner string, in FriendAction) (any, error) {
	now := s.Clock.Now()
	if in.Action == "rotate" {
		_, e := tx.Exec(ctx, "UPDATE social_profiles SET invite_code=gen_random_uuid(),version=version+1 WHERE owner_id=$1", owner)
		return receipt(owner), e
	}
	if in.Action == "request" {
		if e := verified(ctx, tx, owner); e != nil {
			return nil, e
		}
		if !privateapi.UUID(in.Code) {
			return nil, missing()
		}
		var target string
		e := tx.QueryRow(ctx, "SELECT owner_id::text FROM social_profiles WHERE invite_code=$1 AND owner_id<>$2", in.Code, owner).Scan(&target)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, missing()
		}
		if e != nil {
			return nil, e
		}
		deny, e := blocked(ctx, tx, owner, target)
		if e != nil {
			return nil, e
		}
		if deny {
			return nil, missing()
		}
		var count int
		e = tx.QueryRow(ctx, "SELECT count(*) FROM social_friends WHERE (sender=$1 AND created_at>$3::timestamptz-interval '24 hours') OR (recipient=$2 AND state='pending') OR($1 IN(sender,recipient) AND state='accepted')", owner, target, now).Scan(&count)
		if e != nil {
			return nil, e
		}
		if count >= 50 {
			return nil, limit()
		}
		var exists bool
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM social_friends WHERE ((sender=$1 AND recipient=$2) OR(sender=$2 AND recipient=$1)) AND (state IN('pending','accepted') OR updated_at>$3::timestamptz-interval '24 hours'))", owner, target, now).Scan(&exists)
		if e != nil {
			return nil, e
		}
		if exists {
			return nil, conflict()
		}
		var id string
		e = tx.QueryRow(ctx, "INSERT INTO social_friends(sender,recipient,state,created_at,updated_at) VALUES($1,$2,'pending',$3,$3) RETURNING id::text", owner, target, now).Scan(&id)
		if e != nil {
			return nil, e
		}
		return receipt(id), audit(ctx, tx, owner, "request", id, now)
	}
	if !privateapi.UUID(in.ID) {
		return nil, missing()
	}
	var a, b, state string
	var version int64
	e := tx.QueryRow(ctx, "SELECT sender::text,recipient::text,state,version FROM social_friends WHERE id=$1 AND $2 IN(sender,recipient)", in.ID, owner).Scan(&a, &b, &state, &version)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, missing()
	}
	if e != nil {
		return nil, e
	}
	if version != in.Version {
		return nil, conflict()
	}
	next := ""
	switch in.Action {
	case "accept":
		if state == "pending" && b == owner {
			next = "accepted"
		}
	case "reject":
		if state == "pending" && b == owner {
			next = "rejected"
		}
	case "remove":
		if state == "accepted" || state == "pending" && a == owner {
			next = "removed"
		}
	}
	if next == "" {
		return nil, conflict()
	}
	deny, e := blocked(ctx, tx, a, b)
	if e != nil {
		return nil, e
	}
	if deny {
		return nil, missing()
	}
	_, e = tx.Exec(ctx, "UPDATE social_friends SET state=$2,version=version+1,updated_at=$3 WHERE id=$1", in.ID, next, now)
	if e != nil {
		return nil, e
	}
	return receipt(in.ID), audit(ctx, tx, owner, in.Action, in.ID, now)
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		r.Get("/api/v1/social/friends", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, o string) (any, error) {
				if e := lock(ctx, tx, o); e != nil {
					return nil, e
				}
				return s.listFriends(ctx, tx, o)
			})
			s.Reply(w, r, out, e)
		})
		r.Post("/api/v1/social/friends", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in FriendAction) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, o string) (any, error) {
					if e := lock(ctx, tx, o); e != nil {
						return nil, e
					}
					return s.friendAction(ctx, tx, o, in)
				})
			})
		})
	})
}
