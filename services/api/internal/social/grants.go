package social

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/jackc/pgx/v5"
)

type GrantAction struct {
	Recipient string `json:"recipient"`
	Metric    string `json:"metric"`
	Scope     string `json:"scope"`
	Allow     bool   `json:"allow"`
}
type Grant struct {
	Owner     string  `json:"owner"`
	Recipient string  `json:"recipient"`
	Metric    string  `json:"metric"`
	Scope     string  `json:"scope"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
}

func (s *Service) setGrant(ctx context.Context, tx pgx.Tx, owner string, in GrantAction) (any, error) {
	if e := friends(ctx, tx, owner, in.Recipient); e != nil {
		return nil, e
	}
	if (in.Metric != "regularity" && in.Metric != "evening" && in.Metric != "challenge") || (in.Metric != "challenge" && in.Scope != "") {
		return nil, privateapi.Invalid("Выберите разрешённый показатель")
	}
	if in.Metric == "challenge" {
		if !privateapi.UUID(in.Scope) {
			return nil, missing()
		}
		var yes bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM challenge_runs WHERE owner_id=$1 AND id=$2)", owner, in.Scope).Scan(&yes); e != nil {
			return nil, e
		}
		if !yes {
			return nil, missing()
		}
	}
	var e error
	if in.Allow {
		_, e = tx.Exec(ctx, "INSERT INTO social_grants(owner_id,recipient_id,metric,scope,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", owner, in.Recipient, in.Metric, in.Scope, s.Clock.Now())
	} else {
		_, e = tx.Exec(ctx, "DELETE FROM social_grants WHERE owner_id=$1 AND recipient_id=$2 AND metric=$3 AND scope=$4", owner, in.Recipient, in.Metric, in.Scope)
	}
	if e != nil {
		return nil, e
	}
	return receipt(in.Recipient), audit(ctx, tx, owner, "grant_changed", in.Recipient, s.Clock.Now())
}
func (s *Service) grants(ctx context.Context, tx pgx.Tx, owner string) ([]Grant, error) {
	out := []Grant{}
	rows, e := tx.Query(ctx, `SELECT g.owner_id::text,g.recipient_id::text,g.metric,g.scope FROM social_grants g WHERE $1 IN(g.owner_id,g.recipient_id) AND EXISTS(SELECT 1 FROM social_friends f WHERE f.state='accepted' AND ((f.sender=g.owner_id AND f.recipient=g.recipient_id) OR(f.sender=g.recipient_id AND f.recipient=g.owner_id))) AND NOT EXISTS(SELECT 1 FROM social_blocks b WHERE (b.owner_id=g.owner_id AND b.target_id=g.recipient_id) OR(b.target_id=g.owner_id AND b.owner_id=g.recipient_id)) ORDER BY g.owner_id,g.metric,g.scope`, owner)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var g Grant
		if e = rows.Scan(&g.Owner, &g.Recipient, &g.Metric, &g.Scope); e != nil {
			rows.Close()
			return out, e
		}
		out = append(out, g)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for i := range out {
		g := &out[i]
		g.Unit = "days"
		switch g.Metric {
		case "regularity":
			e = tx.QueryRow(ctx, "SELECT count(DISTINCT (first_awarded_at AT TIME ZONE 'UTC')::date) FROM reward_entitlements WHERE owner_id=$1 AND active AND first_awarded_at>=(($2::timestamptz AT TIME ZONE 'UTC')::date-6)::timestamp AT TIME ZONE 'UTC' AND first_awarded_at<=$2", g.Owner, s.Clock.Now()).Scan(&g.Value)
		case "evening":
			g.Unit = "review"
			e = tx.QueryRow(ctx, "SELECT CASE WHEN EXISTS(SELECT 1 FROM daily_reviews WHERE owner_id=$1 AND local_date=($2::timestamptz AT TIME ZONE 'UTC')::date) THEN 1 ELSE 0 END", g.Owner, s.Clock.Now()).Scan(&g.Value)
		case "challenge":
			g.Unit = "percent"
			e = tx.QueryRow(ctx, "SELECT COALESCE((SELECT (progress->>'percent')::float8 FROM challenge_participants WHERE owner_id=$1 AND run_owner_id=$1 AND run_id=$2),0)", g.Owner, g.Scope).Scan(&g.Value)
		}
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func revokePair(ctx context.Context, tx pgx.Tx, a, b string) error {
	_, e := tx.Exec(ctx, "DELETE FROM social_grants WHERE (owner_id=$1 AND recipient_id=$2) OR(owner_id=$2 AND recipient_id=$1)", a, b)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "UPDATE social_profiles SET partner_id=NULL,nudges=false,version=version+1 WHERE (owner_id=$1 AND partner_id=$2) OR(owner_id=$2 AND partner_id=$1)", a, b)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "UPDATE social_members m SET state='left',share=false,leaderboard=false,version=m.version+1 FROM social_rooms r WHERE r.id=m.room_id AND ((r.owner_id=$1 AND m.owner_id=$2) OR(r.owner_id=$2 AND m.owner_id=$1)) AND m.state IN('joined','invited')", a, b)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE notification_invite_sources s SET active=false WHERE (s.recipient_id=$1 OR s.recipient_id=$2) AND (EXISTS(SELECT 1 FROM social_friends f WHERE f.id=s.id AND ((f.sender=$1 AND f.recipient=$2) OR(f.sender=$2 AND f.recipient=$1))) OR EXISTS(SELECT 1 FROM social_rooms r WHERE r.id=s.id AND ((r.owner_id=$1 AND s.recipient_id=$2) OR(r.owner_id=$2 AND s.recipient_id=$1))))`, a, b)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE notifications n SET active=false,version=version+1 WHERE n.owner_id IN($1,$2) AND n.source_kind='invite' AND n.active AND EXISTS(SELECT 1 FROM notification_invite_sources s WHERE s.recipient_id=n.owner_id AND s.id::text=n.source_id AND NOT s.active)`, a, b)
	return e
}
