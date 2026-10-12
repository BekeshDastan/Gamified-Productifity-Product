package platform

import (
	"github.com/go-chi/chi/v5"
	"log/slog"
)

// NewRouter creates the HTTP boundary. Domain routes are registered by modules.
func NewRouter(logger *slog.Logger, c Clock) *chi.Mux {
	r := chi.NewRouter()
	r.Use(requestMetadata(logger, c))
	return r
}
