package goals

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/jackc/pgx/v5"
)

// QuestChanged shares the owner lock and transaction with the source event.
// Old and new ancestors are included when a subtree moves between plans.
func (s *Service) QuestChanged(ctx context.Context, tx pgx.Tx, id, kind string, before *quests.Quest, after quests.Quest) error {
	var previousParent *string
	if before != nil {
		previousParent = before.ParentID
	}
	structural := before == nil || !sameID(previousParent, after.ParentID)
	if structural {
		var overlap bool
		if e := tx.QueryRow(ctx, `WITH RECURSIVE roots AS (
   SELECT quest_id root,quest_id id FROM goal_quests WHERE owner_id=$1
   UNION ALL SELECT r.root,q.id FROM roots r JOIN quests q ON q.parent_id=r.id AND q.owner_id=$1
  ) SELECT EXISTS(SELECT 1 FROM roots GROUP BY id HAVING count(DISTINCT root)>1)`, id).Scan(&overlap); e != nil {
			return e
		}
		if overlap {
			return invalid("parent_id", "Родитель и потомок не могут одновременно быть источниками разных связей цели")
		}
	}
	rows, e := tx.Query(ctx, `WITH RECURSIVE ancestors AS (
  SELECT id,parent_id FROM quests WHERE owner_id=$1 AND (id=$2 OR id=NULLIF($3,'')::uuid)
  UNION SELECT q.id,q.parent_id FROM quests q JOIN ancestors a ON q.id=a.parent_id WHERE q.owner_id=$1
 ) SELECT DISTINCT goal_id::text FROM goal_quests WHERE owner_id=$1 AND quest_id IN(SELECT id FROM ancestors) ORDER BY goal_id::text`, id, after.ID, pointerID(previousParent))
	if e != nil {
		return e
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if e = rows.Scan(&key); e != nil {
			break
		}
		keys = append(keys, key)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	for _, key := range keys {
		g, e := s.get(ctx, tx, id, key)
		if e != nil {
			return e
		}
		if _, e = s.touch(ctx, tx, id, g, "quest_"+kind, "Изменена связанная задача «"+after.Title+"»"); e != nil {
			return e
		}
	}
	return nil
}
func pointerID(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func sameID(a, b *string) bool { return pointerID(a) == pointerID(b) }
