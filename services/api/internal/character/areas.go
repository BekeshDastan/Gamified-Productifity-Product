package character

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/jackc/pgx/v5"
)

type Area struct {
	Skills            []Skill           `json:"skills"`
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	XP                int64             `json:"xp"`
	Level             progression.Level `json:"level"`
	SuccessfulPeriods int64             `json:"successful_periods"`
	Archived          bool              `json:"archived"`
}

func (s *Service) areas(ctx context.Context, tx pgx.Tx, owner string) ([]Area, error) {
	out := []Area{}
	rows, e := tx.Query(ctx, `SELECT a.id::text,a.name,a.archived,COALESCE((SELECT sum(xp_delta) FROM reward_ledger l WHERE l.owner_id=a.owner_id AND l.area_id=a.id),0), (SELECT count(*) FROM reward_entitlements e WHERE e.owner_id=a.owner_id AND e.area_id=a.id AND e.source_kind='habit' AND e.active) FROM life_areas a WHERE a.owner_id=$1 ORDER BY a.created_at,a.id`, owner)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var a Area
		if e = rows.Scan(&a.ID, &a.Name, &a.Archived, &a.XP, &a.SuccessfulPeriods); e != nil {
			return out, e
		}
		a.Level = progression.LevelFor(a.XP)
		a.Skills = skillsFor(a)
		out = append(out, a)
	}
	return out, rows.Err()
}
