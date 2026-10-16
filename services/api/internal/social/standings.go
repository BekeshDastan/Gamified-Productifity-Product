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

// ReconcileInTx keeps earned co-op evidence current even when nobody opens the
// social page after a source correction. The caller holds only its own profile
// lock; no other participant's profile is locked here.
func (s *Service) ReconcileInTx(ctx context.Context, tx pgx.Tx, owner string) error {
	if e := lock(ctx, tx, owner); e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT r.id::text,r.owner_id::text FROM social_rooms r JOIN social_members m ON m.room_id=r.id AND m.owner_id=$1 WHERE r.finalized_at IS NOT NULL ORDER BY r.id`, owner)
	if e != nil {
		return e
	}
	type identity struct{ id, organizer string }
	ids := []identity{}
	for rows.Next() {
		var v identity
		if e = rows.Scan(&v.id, &v.organizer); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range ids {
		r, e := s.room(ctx, tx, v.organizer, v.id)
		if e != nil {
			return e
		}
		// A departed organizer does not cancel consenting teammates. This internal
		// projection uses the organizer's perspective without granting them a read.
		r.State = "joined"
		if e = s.fillRoom(ctx, tx, v.organizer, &r); e != nil {
			return e
		}
		if e = s.settle(ctx, tx, r); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) settle(ctx context.Context, tx pgx.Tx, r Room) error {
	now := s.Clock.Now()
	if now.Before(r.EndsAt) {
		return conflict()
	}
	// Rebuild only consented source totals. Replays update the same reward receipt,
	// and later source corrections or withdrawal revoke its active earned flag.
	members := []string{}
	for _, m := range r.Members {
		members = append(members, m.UserID)
	}
	if _, e := tx.Exec(ctx, `WITH revoked AS (UPDATE social_results SET earned=false,updated_at=$3 WHERE room_id=$1 AND earned AND owner_id::text<>ALL($2::text[]) RETURNING owner_id) INSERT INTO outbox_events(id,owner_id,kind,payload,occurred_at) SELECT gen_random_uuid(),owner_id,'character.changed','{}',$3 FROM revoked`, r.ID, members, now); e != nil {
		return e
	}
	for _, m := range r.Members {
		earned := m.Amount > 0 && ((r.Mode == "coop" && r.Total >= r.Target && len(r.Members) > 1) || (r.Mode == "pvp" && m.Rank == 1 && len(r.Members) > 1))
		_, e := tx.Exec(ctx, `WITH changed AS (INSERT INTO social_results(room_id,owner_id,amount,rank,earned,updated_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(room_id,owner_id) DO UPDATE SET amount=excluded.amount,rank=excluded.rank,earned=excluded.earned,updated_at=excluded.updated_at WHERE ROW(social_results.amount,social_results.rank,social_results.earned) IS DISTINCT FROM ROW(excluded.amount,excluded.rank,excluded.earned) RETURNING owner_id) INSERT INTO outbox_events(id,owner_id,kind,payload,occurred_at) SELECT gen_random_uuid(),owner_id,'character.changed','{}',$6 FROM changed`, r.ID, m.UserID, m.Amount, m.Rank, earned, now)
		if e != nil {
			return e
		}
	}
	_, e := tx.Exec(ctx, "UPDATE social_rooms SET finalized_at=COALESCE(finalized_at,$2) WHERE id=$1", r.ID, now)
	return e
}
