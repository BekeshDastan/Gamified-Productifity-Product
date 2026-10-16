package social

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
	"time"
)

type CommunityAction struct {
	Action        string `json:"action"`
	Recipient     string `json:"recipient"`
	Enabled       bool   `json:"enabled"`
	Version       int64  `json:"version"`
	Template      string `json:"template"`
	SourceKey     string `json:"source_key"`
	SourceVersion string `json:"source_version"`
	Consent       bool   `json:"consent"`
}
type Post struct {
	ID        string    `json:"id"`
	Owner     string    `json:"owner"`
	Nickname  string    `json:"nickname"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}
type Publishable struct {
	Key     string `json:"key"`
	Version string `json:"version"`
	Title   string `json:"title"`
}
type Nudge struct {
	ID        string    `json:"id"`
	Template  string    `json:"template"`
	Nickname  string    `json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
}
type Community struct {
	Partner     *string       `json:"partner"`
	Nudges      bool          `json:"nudges"`
	Version     int64         `json:"version"`
	Feed        []Post        `json:"feed"`
	Publishable []Publishable `json:"publishable"`
	Inbox       []Nudge       `json:"inbox"`
}

func (s *Service) communityAction(ctx context.Context, tx pgx.Tx, owner string, in CommunityAction) (any, error) {
	now := s.Clock.Now()
	switch in.Action {
	case "preferences":
		var partner *string
		if in.Recipient != "" {
			if e := friends(ctx, tx, owner, in.Recipient); e != nil {
				return nil, e
			}
			partner = &in.Recipient
		}
		if in.Enabled && partner == nil {
			return nil, privateapi.Invalid("Сначала выберите партнёра")
		}
		tag, e := tx.Exec(ctx, "UPDATE social_profiles SET partner_id=$2,nudges=$3,version=version+1 WHERE owner_id=$1 AND version=$4", owner, partner, in.Enabled, in.Version)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return nil, conflict()
		}
	case "nudge":
		if e := verified(ctx, tx, owner); e != nil {
			return nil, e
		}
		if e := friends(ctx, tx, owner, in.Recipient); e != nil {
			return nil, e
		}
		if in.Template != "small_step" && in.Template != "rest" && in.Template != "celebrate" {
			return nil, privateapi.Invalid("Выберите шаблон поддержки")
		}
		var yes bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM social_profiles WHERE owner_id=$1 AND partner_id=$2 AND nudges)", in.Recipient, owner).Scan(&yes); e != nil {
			return nil, e
		}
		if !yes {
			return nil, privateapi.Problem(403, "NUDGES_DISABLED", "Партнёр не включил напоминания от вас")
		}
		var n int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM social_nudges WHERE (sender=$1 OR recipient=$2) AND created_at>$3::timestamptz-interval '24 hours'", owner, in.Recipient, now).Scan(&n); e != nil {
			return nil, e
		}
		if n >= 3 {
			return nil, limit()
		}
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM social_nudges WHERE sender=$1 AND recipient=$2 AND created_at>$3::timestamptz-interval '24 hours'", owner, in.Recipient, now).Scan(&n); e != nil {
			return nil, e
		}
		if n > 0 {
			return nil, limit()
		}
		_, e := tx.Exec(ctx, "INSERT INTO social_nudges(sender,recipient,template,created_at) VALUES($1,$2,$3,$4)", owner, in.Recipient, in.Template, now)
		if e != nil {
			return nil, e
		}
	case "publish":
		if !in.Consent {
			return nil, privateapi.Invalid("Публикация требует согласия")
		}
		var yes bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM user_achievements WHERE owner_id=$1 AND definition_id=$2 AND definition_version=$3 AND active)", owner, in.SourceKey, in.SourceVersion).Scan(&yes); e != nil {
			return nil, e
		}
		if !yes {
			return nil, missing()
		}
		tag, e := tx.Exec(ctx, "INSERT INTO social_posts(owner_id,source_key,source_version,created_at) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,source_key,source_version) DO UPDATE SET hidden=false WHERE NOT social_posts.moderated", owner, in.SourceKey, in.SourceVersion, now)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return nil, conflict()
		}
	default:
		return nil, privateapi.Invalid("Неизвестное действие")
	}
	return receipt(owner), audit(ctx, tx, owner, in.Action, owner, now)
}
func (s *Service) community(ctx context.Context, tx pgx.Tx, owner string) (Community, error) {
	out := Community{Feed: []Post{}, Publishable: []Publishable{}, Inbox: []Nudge{}}
	if e := tx.QueryRow(ctx, "SELECT partner_id::text,nudges,version FROM social_profiles WHERE owner_id=$1", owner).Scan(&out.Partner, &out.Nudges, &out.Version); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT p.id::text,p.owner_id::text,COALESCE(NULLIF(u.nickname,''),'Участник'),d.snapshot->>'name',p.created_at FROM social_posts p JOIN profiles u ON u.owner_id=p.owner_id JOIN user_achievements a ON a.owner_id=p.owner_id AND a.definition_id=p.source_key AND a.definition_version=p.source_version AND a.active JOIN achievement_definitions d ON d.id=p.source_key AND d.version=p.source_version WHERE NOT p.hidden AND NOT p.moderated AND (p.owner_id=$1 OR EXISTS(SELECT 1 FROM social_friends f WHERE f.state='accepted' AND ((f.sender=$1 AND f.recipient=p.owner_id) OR(f.recipient=$1 AND f.sender=p.owner_id)))) AND NOT EXISTS(SELECT 1 FROM social_blocks b WHERE (b.owner_id=$1 AND b.target_id=p.owner_id) OR(b.target_id=$1 AND b.owner_id=p.owner_id)) ORDER BY p.created_at DESC,p.id LIMIT 30`, owner)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p Post
		if e = rows.Scan(&p.ID, &p.Owner, &p.Nickname, &p.Title, &p.CreatedAt); e != nil {
			rows.Close()
			return out, e
		}
		out.Feed = append(out.Feed, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(ctx, "SELECT a.definition_id,a.definition_version,d.snapshot->>'name' FROM user_achievements a JOIN achievement_definitions d ON d.id=a.definition_id AND d.version=a.definition_version WHERE a.owner_id=$1 AND a.active ORDER BY a.awarded_at DESC LIMIT 100", owner)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p Publishable
		if e = rows.Scan(&p.Key, &p.Version, &p.Title); e != nil {
			rows.Close()
			return out, e
		}
		out.Publishable = append(out.Publishable, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(ctx, `SELECT n.id::text,n.template,COALESCE(NULLIF(p.nickname,''),'Участник'),n.created_at FROM social_nudges n JOIN profiles p ON p.owner_id=n.sender JOIN social_profiles sp ON sp.owner_id=n.recipient AND sp.nudges AND sp.partner_id=n.sender WHERE n.recipient=$1 AND NOT EXISTS(SELECT 1 FROM social_blocks b WHERE (b.owner_id=n.sender AND b.target_id=n.recipient) OR(b.target_id=n.sender AND b.owner_id=n.recipient)) ORDER BY n.created_at DESC LIMIT 20`, owner)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var n Nudge
		if e = rows.Scan(&n.ID, &n.Template, &n.Nickname, &n.CreatedAt); e != nil {
			return out, e
		}
		out.Inbox = append(out.Inbox, n)
	}
	return out, rows.Err()
}
