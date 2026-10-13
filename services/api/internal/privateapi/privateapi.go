// Package privateapi provides owner-scoped transactions for private domains.
package privateapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

type Service struct {
	Pool  *pgxpool.Pool
	Clock platform.Clock
}
type Operation func(context.Context, pgx.Tx, string) (any, error)

// MatchVersion permits the explicit zero version only for a missing singleton.
func MatchVersion(r *http.Request, current int64) *platform.APIError {
	if current == 0 {
		if r.Header.Get("If-Match") == `"0"` {
			return nil
		}
		return Problem(409, "VERSION_CONFLICT", "Запись ещё не создана. Обновите данные")
	}
	if r.Header.Get("If-Match") == `"0"` {
		return Problem(409, "VERSION_CONFLICT", "Запись уже создана. Обновите данные")
	}
	return platform.MatchVersion(r, current)
}

func New(pool *pgxpool.Pool, clock platform.Clock) *Service { return &Service{pool, clock} }
func Problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func Invalid(message string) error { return Problem(422, "VALIDATION_ERROR", message) }
func UUID(v string) bool           { var id pgtype.UUID; return id.Scan(v) == nil && id.Valid }
func Day(v string) error {
	d, e := time.Parse("2006-01-02", v)
	if e != nil || d.Format("2006-01-02") != v || d.Year() < 1 || d.Year() > 9998 {
		return Invalid("Укажите календарную дату")
	}
	return nil
}
func Owner(ctx context.Context) (string, error) {
	id, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", Problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return id.UserID, nil
}
func (s *Service) Reply(w http.ResponseWriter, r *http.Request, out any, e error) {
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) {
			p = Problem(500, "INTERNAL_ERROR", "Не удалось обработать запрос")
		}
		platform.WriteError(w, r, p)
		return
	}
	platform.WriteData(w, r, s.Clock, 200, out)
}
func (s *Service) Read(r *http.Request, fn Operation) (any, error) {
	id, e := Owner(r.Context())
	if e != nil {
		return nil, e
	}
	var out any
	e = storage.WithinTx(r.Context(), s.Pool, func(tx pgx.Tx) error { var e error; out, e = fn(r.Context(), tx, id); return e })
	return out, e
}
func (s *Service) Mutate(r *http.Request, input any, fn Operation) (any, error) {
	id, e := Owner(r.Context())
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(struct {
		Input   any
		Version string
	}{input, r.Header.Get("If-Match")})
	if e != nil {
		return nil, e
	}
	result, e := (storage.IdempotencyStore{DB: s.Pool, Now: s.Clock.Now}).Execute(r.Context(), storage.Scope{UserID: id, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, b, func(tx pgx.Tx) (storage.Result, error) {
		var locked string
		if e := tx.QueryRow(r.Context(), "SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE", id).Scan(&locked); e != nil {
			return storage.Result{}, e
		}
		out, e := fn(r.Context(), tx, id)
		if e != nil {
			return storage.Result{}, e
		}
		if e = jobs.OwnerChanged(r.Context(), tx, id, s.Clock); e != nil {
			return storage.Result{}, e
		}
		body, e := json.Marshal(out)
		return storage.Result{Status: 200, Body: body}, e
	})
	switch {
	case errors.Is(e, storage.ErrInvalidScope):
		e = Invalid("Нужен ключ запроса")
	case errors.Is(e, storage.ErrIdempotencyMismatch):
		e = Problem(409, "IDEMPOTENCY_MISMATCH", "Ключ использован для другого запроса")
	case errors.Is(e, storage.ErrOperationInProgress):
		e = Problem(409, "OPERATION_IN_PROGRESS", "Запрос выполняется")
	}
	return json.RawMessage(result.Body), e
}
func Decode[T any](s *Service, w http.ResponseWriter, r *http.Request, fn func(T) (any, error)) {
	in, p := platform.DecodeJSON[T](r)
	if p != nil {
		platform.WriteError(w, r, p)
		return
	}
	out, e := fn(in)
	s.Reply(w, r, out, e)
}
