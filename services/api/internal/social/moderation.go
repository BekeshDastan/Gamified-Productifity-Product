package social

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
)

type ModerationAction struct {
	Action string `json:"action"`
	ID     string `json:"id"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}
type Block struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}
type Report struct {
	ID       string  `json:"id"`
	Target   string  `json:"target"`
	PostID   *string `json:"post_id"`
	Reason   string  `json:"reason"`
	Resolved bool    `json:"resolved"`
}
type Moderation struct {
	Blocks  []Block  `json:"blocks"`
	Reports []Report `json:"reports"`
	Admin   bool     `json:"admin"`
}

func (s *Service) moderate(ctx context.Context, tx pgx.Tx, owner string, in ModerationAction) (any, error) {
	now := s.Clock.Now()
	var admin bool
	if e := tx.QueryRow(ctx, "SELECT role='admin' FROM users WHERE id=$1", owner).Scan(&admin); e != nil {
		return nil, e
	}
	switch in.Action {
	case "resolve":
		if !admin || !privateapi.UUID(in.ID) {
			return nil, missing()
		}
		tag, e := tx.Exec(ctx, "UPDATE social_reports SET resolved=true WHERE id=$1", in.ID)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return nil, missing()
		}
		return receipt(in.ID), audit(ctx, tx, owner, "report_resolved", in.ID, now)
	case "block", "unblock":
		if !privateapi.UUID(in.Target) || in.Target == owner {
			return nil, missing()
		}
		if in.Action == "unblock" {
			_, e := tx.Exec(ctx, "DELETE FROM social_blocks WHERE owner_id=$1 AND target_id=$2", owner, in.Target)
			return receipt(in.Target), e
		}
		var known bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM social_friends WHERE (sender=$1 AND recipient=$2) OR(sender=$2 AND recipient=$1)) OR EXISTS(SELECT 1 FROM social_members a JOIN social_members b ON b.room_id=a.room_id WHERE a.owner_id=$1 AND b.owner_id=$2)", owner, in.Target).Scan(&known); e != nil {
			return nil, e
		}
		if !known {
			return nil, missing()
		}
		if _, e := tx.Exec(ctx, "INSERT INTO social_blocks(owner_id,target_id,created_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", owner, in.Target, now); e != nil {
			return nil, e
		}
		if _, e := tx.Exec(ctx, "UPDATE social_friends SET state='removed',version=version+1,updated_at=$3 WHERE ((sender=$1 AND recipient=$2) OR(sender=$2 AND recipient=$1)) AND state IN('pending','accepted')", owner, in.Target, now); e != nil {
			return nil, e
		}
		if e := revokePair(ctx, tx, owner, in.Target); e != nil {
			return nil, e
		}
		// Leaving all common rooms prevents inferring a blocked participant through totals.
		if _, e := tx.Exec(ctx, "UPDATE social_members m SET state='left',share=false,leaderboard=false,version=version+1 WHERE m.owner_id=$1 AND m.state IN('joined','invited') AND EXISTS(SELECT 1 FROM social_members other WHERE other.room_id=m.room_id AND other.owner_id=$2)", owner, in.Target); e != nil {
			return nil, e
		}
		if _, e := tx.Exec(ctx, "UPDATE social_reviews SET state='withdrawn',version=version+1,updated_at=$3 WHERE ((owner_id=$1 AND reviewer_id=$2) OR(owner_id=$2 AND reviewer_id=$1)) AND state<>'withdrawn'", owner, in.Target, now); e != nil {
			return nil, e
		}
	case "hide":
		if !privateapi.UUID(in.ID) {
			return nil, missing()
		}
		tag, e := tx.Exec(ctx, "UPDATE social_posts SET hidden=true,moderated=moderated OR $3 WHERE id=$1 AND (owner_id=$2 OR $3)", in.ID, owner, admin)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return nil, missing()
		}
		if _, e = tx.Exec(ctx, "UPDATE social_reports SET resolved=true WHERE post_id=$1", in.ID); e != nil {
			return nil, e
		}
		return receipt(in.ID), audit(ctx, tx, owner, "post_hidden", in.ID, now)
	case "report":
		if in.Reason != "spam" && in.Reason != "abuse" && in.Reason != "privacy" && in.Reason != "other" {
			return nil, privateapi.Invalid("Выберите причину жалобы")
		}
		var post *string
		if in.ID != "" {
			if !privateapi.UUID(in.ID) {
				return nil, missing()
			}
			e := tx.QueryRow(ctx, "SELECT owner_id::text FROM social_posts WHERE id=$1 AND NOT hidden AND NOT moderated", in.ID).Scan(&in.Target)
			if errors.Is(e, pgx.ErrNoRows) {
				return nil, missing()
			}
			if e != nil {
				return nil, e
			}
			post = &in.ID
		}
		if post != nil {
			if e := friends(ctx, tx, owner, in.Target); e != nil {
				return nil, e
			}
		} else {
			// Unwanted invitations can be reported without accepting them, including after a block.
			if !privateapi.UUID(in.Target) || in.Target == owner {
				return nil, missing()
			}
			var known bool
			if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM social_friends WHERE (sender=$1 AND recipient=$2) OR(sender=$2 AND recipient=$1)) OR EXISTS(SELECT 1 FROM social_members a JOIN social_members b ON b.room_id=a.room_id WHERE a.owner_id=$1 AND b.owner_id=$2)", owner, in.Target).Scan(&known); e != nil {
				return nil, e
			}
			if !known {
				return nil, missing()
			}
		}
		var count int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM social_reports WHERE owner_id=$1 AND created_at>$2::timestamptz-interval '24 hours'", owner, now).Scan(&count); e != nil {
			return nil, e
		}
		if count >= 5 {
			return nil, limit()
		}
		var id string
		if e := tx.QueryRow(ctx, "INSERT INTO social_reports(owner_id,target_id,post_id,reason,created_at) VALUES($1,$2,$3,$4,$5) RETURNING id::text", owner, in.Target, post, in.Reason, now).Scan(&id); e != nil {
			return nil, e
		}
		return receipt(id), audit(ctx, tx, owner, "reported", id, now)
	default:
		return nil, privateapi.Invalid("Неизвестное действие")
	}
	return receipt(in.Target), audit(ctx, tx, owner, in.Action, in.Target, now)
}
func (s *Service) moderation(ctx context.Context, tx pgx.Tx, owner string) (Moderation, error) {
	out := Moderation{Blocks: []Block{}, Reports: []Report{}}
	if e := tx.QueryRow(ctx, "SELECT role='admin' FROM users WHERE id=$1", owner).Scan(&out.Admin); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, "SELECT b.target_id::text,COALESCE(NULLIF(p.nickname,''),'Участник') FROM social_blocks b JOIN profiles p ON p.owner_id=b.target_id WHERE b.owner_id=$1 ORDER BY b.created_at DESC LIMIT 200", owner)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var b Block
		if e = rows.Scan(&b.ID, &b.Nickname); e != nil {
			rows.Close()
			return out, e
		}
		out.Blocks = append(out.Blocks, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(ctx, "SELECT id::text,target_id::text,post_id::text,reason,resolved FROM social_reports WHERE owner_id=$1 OR $2 ORDER BY created_at DESC LIMIT 100", owner, out.Admin)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Report
		if e = rows.Scan(&v.ID, &v.Target, &v.PostID, &v.Reason, &v.Resolved); e != nil {
			return out, e
		}
		out.Reports = append(out.Reports, v)
	}
	return out, rows.Err()
}
