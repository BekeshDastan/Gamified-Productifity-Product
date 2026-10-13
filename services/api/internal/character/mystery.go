package character

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"os"
)

const MysteryVersion = "mystery-v1"
const MysteryCompensation int64 = 5

type WeightedItem struct {
	Item
	Weight int64 `json:"weight"`
}

// The switch stops new gifts, while already earned gifts remain usable.
func MysteryEnabled() bool { return os.Getenv("MYSTERY_REWARDS_ENABLED") != "false" }
func MysteryItems() []WeightedItem {
	out := []WeightedItem{}
	for _, item := range Catalog() {
		if item.Starter {
			continue
		}
		weight := int64(3)
		if item.Rarity == "rare" {
			weight = 1
		}
		out = append(out, WeightedItem{item, weight})
	}
	return out
}
func (s *Service) ensureMysteryGrants(ctx context.Context, tx pgx.Tx, owner string) error {
	if !MysteryEnabled() {
		return nil
	}
	weights, e := json.Marshal(MysteryItems())
	if e != nil {
		return e
	}
	// Created-source achievements count archived definitions too, so normal edits
	// and corrections cannot repeatedly earn these lifetime gifts.
	_, e = tx.Exec(ctx, `INSERT INTO mystery_grants(owner_id,source,source_version,catalog_version,weights,created_at)
 SELECT owner_id,definition_id,definition_version,$2,$3,$4 FROM user_achievements
 WHERE owner_id=$1 AND active AND definition_id IN ('goal-created-3','habit-created-3')
 ON CONFLICT(owner_id,source,source_version) DO NOTHING`, owner, MysteryVersion, weights, s.clock.Now())
	return e
}
