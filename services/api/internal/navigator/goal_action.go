package navigator

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (s *Service) WithQuests(q *quests.Service) *Service { s.quests = q; return s }
func (s *Service) mountGoalAction(r chi.Router) {
	r.Post("/api/v1/navigator/goals/{id}/first-action", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in struct {
			Task    quests.Input `json:"task"`
			Confirm bool         `json:"confirm"`
		}) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				if !in.Confirm {
					return nil, privateapi.Invalid("Просмотрите первый шаг и подтвердите создание")
				}
				if s.quests == nil {
					return nil, privateapi.Problem(500, "UNAVAILABLE", "Создание шага недоступно")
				}
				return s.goals.AddFirstActionInTx(r, tx, owner, chi.URLParam(r, "id"), in.Task, s.quests)
			})
		})
	})
}
