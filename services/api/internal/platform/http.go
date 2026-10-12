package platform

import "github.com/go-chi/chi/v5"

// NewRouter creates the HTTP boundary. Domain routes are registered by modules.
func NewRouter() *chi.Mux {
	return chi.NewRouter()
}
