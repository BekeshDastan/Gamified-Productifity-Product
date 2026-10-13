package rewards

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type PurchaseInput struct {
	OperationID     string `json:"operation_id"`
	Kind            string `json:"kind"`
	TargetID        string `json:"target_id"`
	ExpectedCost    int64  `json:"expected_cost"`
	ExpectedVersion int64  `json:"expected_version"`
}
type Redemption struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind"`
	TargetID          string    `json:"target_id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Cost              int64     `json:"cost"`
	DefinitionVersion int64     `json:"definition_version"`
	Status            string    `json:"status"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	Reason            string    `json:"reason"`
}

const redemptionColumns = "id::text,kind,target_id,title,description,cost,definition_version,status,version,created_at,reason"

func scanRedemption(row pgx.Row) (Redemption, error) {
	var r Redemption
	e := row.Scan(&r.ID, &r.Kind, &r.TargetID, &r.Title, &r.Description, &r.Cost, &r.DefinitionVersion, &r.Status, &r.Version, &r.CreatedAt, &r.Reason)
	return r, e
}
func CosmeticPrice(item character.Item) int64 {
	switch item.Rarity {
	case "rare":
		return 30
	case "uncommon":
		return 20
	default:
		return 10
	}
}
func (s *Service) purchase(ctx context.Context, tx pgx.Tx, owner string, in PurchaseInput) (Redemption, error) {
	if !validID(in.OperationID) || (in.Kind != "custom" && in.Kind != "cosmetic") || in.ExpectedCost < 0 || in.ExpectedCost > 1000000 {
		return Redemption{}, problem(422, "VALIDATION_ERROR", "Проверьте операцию и цену")
	}
	request, e := json.Marshal(in)
	if e != nil {
		return Redemption{}, e
	}
	existing, e := scanRedemption(tx.QueryRow(ctx, "SELECT "+redemptionColumns+" FROM reward_redemptions WHERE owner_id=$1 AND id=$2", owner, in.OperationID))
	if e == nil {
		var matches bool
		e = tx.QueryRow(ctx, "SELECT request=$3::jsonb FROM reward_redemptions WHERE owner_id=$1 AND id=$2", owner, in.OperationID, request).Scan(&matches)
		if e != nil {
			return existing, e
		}
		if !matches {
			return existing, problem(409, "IDEMPOTENCY_MISMATCH", "ID покупки использован для другого выбора")
		}
		return existing, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return existing, e
	}
	var title, description string
	cost, version := int64(0), int64(1)
	status := "pending"
	if in.Kind == "custom" {
		d, e := definition(ctx, tx, owner, in.TargetID)
		if e != nil {
			return existing, e
		}
		if d.Archived || !d.Available {
			return existing, problem(409, "REWARD_UNAVAILABLE", "Награда недоступна")
		}
		title, description, cost, version = d.Title, d.Description, d.Cost, d.Version
	} else {
		if e := s.character.EvaluateInTx(ctx, tx, owner); e != nil {
			return existing, e
		}
		found := false
		for _, item := range character.Catalog() {
			if item.ID == in.TargetID && !item.Starter {
				found = true
				title, description, cost = item.Name, "Косметический предмет · "+item.Slot, CosmeticPrice(item)
			}
		}
		if !found {
			return existing, problem(404, "NOT_FOUND", "Предмет не найден")
		}
		var owned bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM character_inventory WHERE owner_id=$1 AND item_id=$2 AND active)", owner, in.TargetID).Scan(&owned); e != nil {
			return existing, e
		}
		if owned {
			return existing, problem(409, "ALREADY_OWNED", "Предмет уже открыт")
		}
		status = "claimed"
	}
	if cost != in.ExpectedCost || version != in.ExpectedVersion {
		return existing, problem(409, "PRICE_CHANGED", "Цена или версия изменилась. Проверьте каталог ещё раз.")
	}
	spend, e := s.progress.SpendInTx(ctx, tx, owner, in.OperationID, cost)
	if e != nil {
		return existing, e
	}
	result, e := scanRedemption(tx.QueryRow(ctx, "INSERT INTO reward_redemptions(owner_id,id,kind,target_id,title,description,cost,definition_version,request,status,spend_ledger_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12) RETURNING "+redemptionColumns, owner, in.OperationID, in.Kind, in.TargetID, title, description, cost, version, request, status, spend.LedgerID, s.clock.Now()))
	if e != nil {
		return result, e
	}
	if in.Kind == "cosmetic" {
		e = s.character.GrantPermanentInTx(ctx, tx, owner, in.TargetID, "purchase", in.OperationID)
	}
	return result, e
}
func (s *Service) mountPurchases(r chi.Router) {
	r.Post("/api/v1/rewards/purchase", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in PurchaseInput) (any, error) {
			return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, id string) (any, error) { return s.purchase(ctx, tx, id, in) })
		})
	})
	r.Get("/api/v1/rewards/redemptions", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		out := RedemptionPage{Items: []Redemption{}}
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), id, func(tx pgx.Tx) error {
				var err error
				out, err = s.redemptionPage(r.Context(), tx, id, r.URL.Query().Get("before"))
				return err
			})
		}
		s.reply(w, r, out, e)
	})
}
func (s *Service) redemptions(ctx context.Context, tx pgx.Tx, owner string) ([]Redemption, error) {
	out := []Redemption{}
	rows, e := tx.Query(ctx, "SELECT "+redemptionColumns+" FROM reward_redemptions WHERE owner_id=$1 ORDER BY created_at DESC,id DESC LIMIT 200", owner)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		r, e := scanRedemption(rows)
		if e != nil {
			return out, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
