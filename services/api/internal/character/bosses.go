package character

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) reconcileBossRewards(ctx context.Context, tx pgx.Tx, owner string) error {
	rows, e := tx.Query(ctx, "SELECT b.id::text,g.version,((b.state='active' OR b.won_at IS NOT NULL) AND g.state='achieved' AND g.progress>=100),b.won_at,EXISTS(SELECT 1 FROM goal_versions v WHERE v.owner_id=b.owner_id AND v.goal_id=b.goal_id AND v.kind='goal.achievement_reverted' AND v.goal_version>COALESCE(b.victory_goal_version,0)) FROM bosses b JOIN goals g ON g.owner_id=b.owner_id AND g.id=b.goal_id WHERE b.owner_id=$1 ORDER BY b.id", owner)
	if e != nil {
		return e
	}
	type result struct {
		id        string
		version   int64
		qualified bool
		won       *time.Time
		corrected bool
	}
	items := []result{}
	for rows.Next() {
		var r result
		if e = rows.Scan(&r.id, &r.version, &r.qualified, &r.won, &r.corrected); e != nil {
			rows.Close()
			return e
		}
		items = append(items, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, r := range items {
		if r.qualified {
			_, e = tx.Exec(ctx, "UPDATE bosses SET won_at=COALESCE(won_at,$3),victory_goal_version=$4 WHERE owner_id=$1 AND id=$2", owner, r.id, s.clock.Now(), r.version)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, "INSERT INTO cosmetic_permanent_grants(owner_id,item_id,source_kind,source_key,active,granted_at) VALUES($1,'frame-2','boss',$2,true,$3) ON CONFLICT(owner_id,source_kind,source_key) DO UPDATE SET active=true", owner, r.id, s.clock.Now())
			if e != nil {
				return e
			}
		} else if r.won != nil && r.corrected {
			if _, e = tx.Exec(ctx, "UPDATE cosmetic_permanent_grants SET active=false WHERE owner_id=$1 AND source_kind='boss' AND source_key=$2", owner, r.id); e != nil {
				return e
			}
		}
	}
	return nil
}
