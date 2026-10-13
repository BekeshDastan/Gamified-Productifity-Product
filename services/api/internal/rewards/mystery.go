package rewards

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"math/big"
	"net/http"
	"time"
)

type MysteryGrant struct {
	ID             string                   `json:"id"`
	Source         string                   `json:"source"`
	CatalogVersion string                   `json:"catalog_version"`
	Weights        []character.WeightedItem `json:"weights"`
	ItemID         *string                  `json:"item_id"`
	Compensation   *int64                   `json:"compensation"`
	CreatedAt      time.Time                `json:"created_at"`
	OpenedAt       *time.Time               `json:"opened_at"`
}
type MysteryCatalog struct {
	Enabled      bool                     `json:"enabled"`
	Version      string                   `json:"version"`
	Compensation int64                    `json:"compensation"`
	Items        []character.WeightedItem `json:"items"`
	Grants       []MysteryGrant           `json:"grants"`
}

const mysteryColumns = "id::text,source,catalog_version,weights,item_id,compensation,created_at,opened_at"

func scanMystery(row pgx.Row) (MysteryGrant, error) {
	var g MysteryGrant
	var weights []byte
	e := row.Scan(&g.ID, &g.Source, &g.CatalogVersion, &weights, &g.ItemID, &g.Compensation, &g.CreatedAt, &g.OpenedAt)
	if e == nil {
		e = json.Unmarshal(weights, &g.Weights)
	}
	return g, e
}

// ChooseWeighted uses an integer draw in [0,total). There is no client randomness.
func ChooseWeighted(items []character.WeightedItem, draw int64) (string, error) {
	if draw < 0 {
		return "", errors.New("invalid draw")
	}
	for _, i := range items {
		if i.Weight <= 0 {
			return "", errors.New("invalid weight")
		}
		if draw < i.Weight {
			return i.ID, nil
		}
		draw -= i.Weight
	}
	return "", errors.New("draw out of range")
}
func (s *Service) openMystery(ctx context.Context, tx pgx.Tx, owner, id string) (MysteryGrant, error) {
	if !validID(id) {
		return MysteryGrant{}, problem(404, "NOT_FOUND", "Подарок не найден")
	}
	if e := s.character.EvaluateInTx(ctx, tx, owner); e != nil {
		return MysteryGrant{}, e
	}
	g, e := scanMystery(tx.QueryRow(ctx, "SELECT "+mysteryColumns+" FROM mystery_grants WHERE owner_id=$1 AND id=$2 FOR UPDATE", owner, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return g, problem(404, "NOT_FOUND", "Подарок не найден")
	}
	if e != nil || g.OpenedAt != nil {
		return g, e
	}
	p, e := s.character.ViewInTx(ctx, tx, owner)
	if e != nil {
		return g, e
	}
	owned := map[string]bool{}
	for _, i := range p.Inventory {
		owned[i.ID] = i.Owned
	}
	pool := []character.WeightedItem{}
	var total int64
	for _, i := range g.Weights {
		if i.Weight <= 0 || i.Weight > 1000 {
			return g, errors.New("invalid stored weight")
		}
		if !owned[i.ID] {
			pool = append(pool, i)
			total += i.Weight
		}
	}
	if total == 0 {
		amount := character.MysteryCompensation
		g.Compensation = &amount
		if _, e = s.progress.CreditInTx(ctx, tx, owner, g.ID, "bonus", amount); e != nil {
			return g, e
		}
	} else {
		n, e := rand.Int(rand.Reader, big.NewInt(total))
		if e != nil {
			return g, e
		}
		item, e := ChooseWeighted(pool, n.Int64())
		if e != nil {
			return g, e
		}
		g.ItemID = &item
		if e = s.character.GrantPermanentInTx(ctx, tx, owner, item, "mystery", g.ID); e != nil {
			return g, e
		}
	}
	now := s.clock.Now()
	g.OpenedAt = &now
	_, e = tx.Exec(ctx, "UPDATE mystery_grants SET item_id=$3,compensation=$4,opened_at=$5 WHERE owner_id=$1 AND id=$2", owner, id, g.ItemID, g.Compensation, now)
	return g, e
}
func (s *Service) mountMystery(r chi.Router) {
	r.Get("/api/v1/rewards/mystery", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		out := MysteryCatalog{character.MysteryEnabled(), character.MysteryVersion, character.MysteryCompensation, character.MysteryItems(), []MysteryGrant{}}
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), id, func(tx pgx.Tx) error {
				if e := s.character.EvaluateInTx(r.Context(), tx, id); e != nil {
					return e
				}
				rows, e := tx.Query(r.Context(), "SELECT "+mysteryColumns+" FROM mystery_grants WHERE owner_id=$1 ORDER BY created_at,id", id)
				if e != nil {
					return e
				}
				defer rows.Close()
				for rows.Next() {
					g, e := scanMystery(rows)
					if e != nil {
						return e
					}
					out.Grants = append(out.Grants, g)
				}
				return rows.Err()
			})
		}
		s.reply(w, r, out, e)
	})
	r.Post("/api/v1/rewards/mystery/{id}/open", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in struct{}) (any, error) {
			return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.openMystery(ctx, tx, owner, chi.URLParam(r, "id"))
			})
		})
	})
}
