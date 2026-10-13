package habits

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
)

type Starter struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (s *Service) starter(ctx context.Context, id string) (*Starter, error) {
	var out Starter
	e := s.pool.QueryRow(ctx, `SELECT id::text,title FROM onboarding_starters WHERE owner_id=$1 AND kind='habit' AND promoted_entity_id IS NULL`, id).Scan(&out.ID, &out.Title)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &out, e
}
func (s *Service) mountStarter(r chi.Router) {
	r.Get("/api/v1/habits/starter", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		var out *Starter
		if e == nil {
			out, e = s.starter(r.Context(), id)
		}
		s.reply(w, r, out, e)
	})
}
