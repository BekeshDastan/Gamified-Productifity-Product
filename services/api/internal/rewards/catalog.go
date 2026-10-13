package rewards

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Offer struct {
	character.InventoryItem
	Cost int64 `json:"cost"`
}
type RedemptionPage struct {
	Items      []Redemption `json:"items"`
	NextCursor *string      `json:"next_cursor"`
}
type Catalog struct {
	Version     string              `json:"version"`
	Balance     progression.Balance `json:"balance"`
	Cosmetics   []Offer             `json:"cosmetics"`
	Custom      []Definition        `json:"custom"`
	Redemptions RedemptionPage      `json:"redemptions"`
}

func (s *Service) redemptionPage(ctx context.Context, tx pgx.Tx, owner, cursor string) (RedemptionPage, error) {
	out := RedemptionPage{Items: []Redemption{}}
	before := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	beforeID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursor != "" {
		if !validID(cursor) {
			return out, problem(422, "VALIDATION_ERROR", "Некорректная граница истории")
		}
		e := tx.QueryRow(ctx, "SELECT created_at,id::text FROM reward_redemptions WHERE owner_id=$1 AND id=$2", owner, cursor).Scan(&before, &beforeID)
		if errors.Is(e, pgx.ErrNoRows) {
			return out, problem(404, "NOT_FOUND", "Покупка не найдена")
		}
		if e != nil {
			return out, e
		}
	}
	rows, e := tx.Query(ctx, "SELECT "+redemptionColumns+" FROM reward_redemptions WHERE owner_id=$1 AND (created_at,id)<($2,$3::uuid) ORDER BY created_at DESC,id DESC LIMIT 51", owner, before, beforeID)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		r, e := scanRedemption(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	if len(out.Items) > 50 {
		out.Items = out.Items[:50]
		id := out.Items[49].ID
		out.NextCursor = &id
	}
	return out, rows.Err()
}
func (s *Service) catalog(ctx context.Context, tx pgx.Tx, owner string) (Catalog, error) {
	out := Catalog{Version: "shop-v1", Cosmetics: []Offer{}}
	p, e := s.character.ViewInTx(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	for _, item := range p.Inventory {
		if !item.Starter {
			out.Cosmetics = append(out.Cosmetics, Offer{item, CosmeticPrice(item.Item)})
		}
	}
	out.Balance, e = s.progress.Balance(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	out.Custom, e = s.definitions(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	out.Redemptions, e = s.redemptionPage(ctx, tx, owner, "")
	return out, e
}
func (s *Service) mountCatalog(r chi.Router) {
	r.Get("/api/v1/rewards/catalog", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		var out Catalog
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), id, func(tx pgx.Tx) error { var e error; out, e = s.catalog(r.Context(), tx, id); return e })
		}
		s.reply(w, r, out, e)
	})
}
