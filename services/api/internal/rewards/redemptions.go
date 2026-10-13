package rewards

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Service) redemptionAction(ctx context.Context, tx pgx.Tx, owner, id, action, reason string, r *http.Request) (Redemption, error) {
	if !validID(id) {
		return Redemption{}, problem(404, "NOT_FOUND", "Покупка не найдена")
	}
	result, e := scanRedemption(tx.QueryRow(ctx, "SELECT "+redemptionColumns+" FROM reward_redemptions WHERE owner_id=$1 AND id=$2 FOR UPDATE", owner, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Покупка не найдена")
	}
	if e != nil {
		return result, e
	}
	if result.Kind != "custom" {
		return result, problem(409, "INVALID_TRANSITION", "Косметика уже выдана и не возвращается")
	}
	target := "claimed"
	if action == "cancel" {
		target = "cancelled"
	}
	if result.Status == target {
		return result, nil
	}
	if result.Status != "pending" {
		return result, problem(409, "INVALID_TRANSITION", "Полученную или отменённую награду нельзя изменить")
	}
	if p := platform.MatchVersion(r, result.Version); p != nil {
		return result, p
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 1000 {
		return result, problem(422, "VALIDATION_ERROR", "Причина до 1000 символов")
	}
	var ledger *string
	if action == "cancel" {
		id, e := s.progress.CreditInTx(ctx, tx, owner, result.ID, "refund", result.Cost)
		if e != nil {
			return result, e
		}
		ledger = &id
	}
	return scanRedemption(tx.QueryRow(ctx, "UPDATE reward_redemptions SET status=$3,reason=$4,refund_ledger_id=$5,version=version+1,updated_at=$6 WHERE owner_id=$1 AND id=$2 RETURNING "+redemptionColumns, owner, result.ID, target, reason, ledger, s.clock.Now()))
}
func (s *Service) mountRedemptionActions(r chi.Router) {
	for _, action := range []string{"claim", "cancel"} {
		r.Post("/api/v1/rewards/redemptions/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in struct {
				Reason string `json:"reason"`
			}) (any, error) {
				return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					return s.redemptionAction(ctx, tx, owner, chi.URLParam(r, "id"), action, in.Reason, r)
				})
			})
		})
	}
}
