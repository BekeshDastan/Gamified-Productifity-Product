package platform

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates the HTTP boundary. Domain routes are registered by modules.
func NewRouter(logger *slog.Logger, c Clock, readiness ...func(context.Context) error) *chi.Mux {
	r := chi.NewRouter()
	r.Use(requestMetadata(logger, c))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			if req.ContentLength > 1<<20 {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			req.Body = http.MaxBytesReader(w, req.Body, 1<<20)
			defer func() {
				if recover() != nil {
					logger.Error("http_panic", "request_id", RequestMeta(req.Context()).RequestID)
					http.Error(w, "internal error", 500)
				}
			}()
			next.ServeHTTP(w, req)
		})
	})
	r.Use(middleware.Timeout(20 * time.Second))
	health := func(ready bool) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			status, state := http.StatusOK, "ok"
			if ready {
				ctx, cancel := context.WithTimeout(req.Context(), time.Second)
				defer cancel()
				if len(readiness) == 0 || readiness[0] == nil || readiness[0](ctx) != nil {
					status, state = http.StatusServiceUnavailable, "unavailable"
				}
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": state}, "meta": map[string]string{"request_id": RequestMeta(req.Context()).RequestID, "server_time": c.Now().UTC().Format(time.RFC3339)}})
		}
	}
	r.Get("/health/live", health(false))
	r.Get("/health/ready", health(true))
	return r
}
