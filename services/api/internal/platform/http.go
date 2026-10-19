package platform

import (
	"context"
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
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")
			limit := int64(1 << 20)
			if req.Method == http.MethodPost && req.URL.Path == "/api/v1/attachments" {
				limit = 11 << 20
			}
			if req.ContentLength > limit {
				WriteError(w, req, problem(413, "BODY_TOO_LARGE", "Запрос слишком большой"))
				return
			}
			req.Body = http.MaxBytesReader(w, req.Body, limit)
			defer func() {
				if recover() != nil {
					logger.Error("http_panic", "request_id", RequestMeta(req.Context()).RequestID)
					WriteError(w, req, problem(500, "INTERNAL_ERROR", "Не удалось выполнить запрос"))
				}
			}()
			next.ServeHTTP(w, req)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := context.WithTimeout(req.Context(), 20*time.Second)
			defer cancel()
			recorder := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			next.ServeHTTP(recorder, req.WithContext(ctx))
			if ctx.Err() == context.DeadlineExceeded && recorder.Status() == 0 {
				WriteError(w, req, problem(504, "DEADLINE_EXCEEDED", "Время ожидания истекло"))
			}
		})
	})
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
			WriteData(w, req, c, status, struct {
				Status string `json:"status"`
			}{state})
		}
	}
	r.Get("/health/live", health(false))
	r.Get("/health/ready", health(true))
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, problem(404, "NOT_FOUND", "Не найдено"))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, problem(405, "METHOD_NOT_ALLOWED", "Метод недоступен"))
	})
	return r
}
