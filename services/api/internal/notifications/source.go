package notifications

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (s *Service) mountSource(r chi.Router) {
	r.Get("/api/v1/notifications/{id}/source", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			n, e := s.get(ctx, tx, owner, chi.URLParam(r, "id"))
			if e != nil {
				return nil, e
			}
			p, e := s.preferences(ctx, tx, owner)
			if e != nil {
				return nil, e
			}
			current, e := s.notificationCurrent(ctx, tx, owner, n, p)
			if e != nil {
				return nil, e
			}
			var link *string
			message := "Источник больше не доступен или событие уже изменилось. Запись сохранена в истории."
			if current {
				link = &n.URL
				message = "Источник доступен в вашем аккаунте."
			}
			return map[string]any{"available": current, "url": link, "message": message}, nil
		})
		s.Reply(w, r, out, e)
	})
}
