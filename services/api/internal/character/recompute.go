package character

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
)

// RecomputeAll walks owner IDs in stable bounded batches; grants have permanent unique keys.
func (s *Service) RecomputeAll(ctx context.Context) (int, error) {
	count := 0
	var after *string
	for {
		rows, e := s.pool.Query(ctx, "SELECT id::text FROM users WHERE ($1::uuid IS NULL OR id>$1::uuid) ORDER BY id LIMIT 500", after)
		if e != nil {
			return count, e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return count, e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return count, e
		}
		if len(ids) == 0 {
			return count, nil
		}
		for _, id := range ids {
			if _, e = s.RecomputeOwner(ctx, id); e != nil {
				return count, e
			}
			count++
		}
		last := ids[len(ids)-1]
		after = &last
	}
}
func ValidOwner(id string) bool { var value pgtype.UUID; return value.Scan(id) == nil && value.Valid }

func (s *Service) RecomputeOwner(ctx context.Context, owner string) (Profile, error) {
	for range 1000 {
		p, e := s.Get(ctx, owner)
		if e != nil || !p.Recalculating {
			return p, e
		}
	}
	return Profile{}, errors.New("habit evidence backfill requires continuation")
}
