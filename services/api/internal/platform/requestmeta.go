package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type metadataKey struct{}
type RequestMetadata struct{ RequestID, CorrelationID string }

var validID = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

func RequestMeta(ctx context.Context) RequestMetadata {
	v, _ := ctx.Value(metadataKey{}).(RequestMetadata)
	return v
}
func cleanID(candidate string) string {
	if validID.MatchString(candidate) {
		return candidate
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic("system random source unavailable")
	}
	return hex.EncodeToString(bytes[:])
}

func requestMetadata(logger *slog.Logger, c Clock) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			meta := RequestMetadata{RequestID: cleanID(r.Header.Get("X-Request-ID")), CorrelationID: cleanID(r.Header.Get("X-Correlation-ID"))}
			w.Header().Set("X-Request-ID", meta.RequestID)
			w.Header().Set("X-Correlation-ID", meta.CorrelationID)
			r = r.WithContext(context.WithValue(r.Context(), metadataKey{}, meta))
			started := c.Now()
			recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				pattern := "unmatched"
				if route := chi.RouteContext(r.Context()); route != nil && route.RoutePattern() != "" {
					pattern = route.RoutePattern()
				}
				logger.Info("http_request", "request_id", meta.RequestID, "correlation_id", meta.CorrelationID,
					"method", r.Method, "route", pattern, "status", recorder.Status(), "duration_ms", c.Now().Sub(started).Milliseconds())
			}()
			next.ServeHTTP(recorder, r)
		})
	}
}
