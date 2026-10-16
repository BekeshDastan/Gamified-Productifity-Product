package social

import (
	"context"
	"github.com/jackc/pgx/v5"
	"sort"
)

func rankStandings(rows []Standing) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Amount == rows[j].Amount {
			return rows[i].UserID < rows[j].UserID
		}
		return rows[i].Amount > rows[j].Amount
	})
	for i := range rows {
		rows[i].Rank = i + 1
		if i > 0 && rows[i].Amount == rows[i-1].Amount {
			rows[i].Rank = rows[i-1].Rank
		}
	}
}
func (s *Service) settle(ctx context.Context, tx pgx.Tx, r Room) error {
	now := s.Clock.Now()
	if now.Before(r.EndsAt) {
		return conflict()
	}
	// Rebuild only consented source totals. Replays update the same reward receipt,
	// and later source corrections or withdrawal revoke its active earned flag.
	if _, e := tx.Exec(ctx, "UPDATE social_results SET earned=false WHERE room_id=$1", r.ID); e != nil {
		return e
	}
	for _, m := range r.Members {
		earned := m.Amount > 0 && ((r.Mode == "coop" && r.Total >= r.Target) || (r.Mode == "pvp" && m.Rank == 1 && len(r.Members) > 1))
		_, e := tx.Exec(ctx, "INSERT INTO social_results(room_id,owner_id,amount,rank,earned,updated_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(room_id,owner_id) DO UPDATE SET amount=excluded.amount,rank=excluded.rank,earned=excluded.earned,updated_at=excluded.updated_at", r.ID, m.UserID, m.Amount, m.Rank, earned, now)
		if e != nil {
			return e
		}
	}
	_, e := tx.Exec(ctx, "UPDATE social_rooms SET finalized_at=COALESCE(finalized_at,$2) WHERE id=$1", r.ID, now)
	return e
}
