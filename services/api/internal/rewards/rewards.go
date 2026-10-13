package rewards

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"unicode/utf8"
)

type Service struct {
	pool      *pgxpool.Pool
	clock     platform.Clock
	progress  *progression.Service
	character *character.Service
}

func NewService(pool *pgxpool.Pool, clock platform.Clock, p *progression.Service, c *character.Service) *Service {
	return &Service{pool, clock, p, c}
}
func problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func validID(v string) bool { var id pgtype.UUID; return id.Scan(v) == nil && id.Valid }
func owner(ctx context.Context) (string, error) {
	i, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return i.UserID, nil
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, out any, e error) {
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) {
			p = problem(500, "INTERNAL_ERROR", "Не удалось выполнить действие")
		}
		platform.WriteError(w, r, p)
		return
	}
	platform.WriteData(w, r, s.clock, 200, out)
}
func (s *Service) mutate(r *http.Request, in any, fn func(context.Context, pgx.Tx, string) (any, error)) (any, error) {
	id, e := owner(r.Context())
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(struct {
		Input   any
		Version string
	}{in, r.Header.Get("If-Match")})
	if e != nil {
		return nil, e
	}
	result, e := (storage.IdempotencyStore{DB: s.pool, Now: s.clock.Now}).Execute(r.Context(), storage.Scope{UserID: id, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, b, func(tx pgx.Tx) (storage.Result, error) {
		ctx := r.Context()
		if e := s.progress.LockOwner(ctx, tx, id); e != nil {
			return storage.Result{}, e
		}
		if _, e := s.progress.RebuildInTx(ctx, tx, id); e != nil {
			return storage.Result{}, e
		}
		out, e := fn(ctx, tx, id)
		if e != nil {
			return storage.Result{}, e
		}
		if e = jobs.OwnerChanged(ctx, tx, id, s.clock); e != nil {
			return storage.Result{}, e
		}
		b, e := json.Marshal(out)
		return storage.Result{Status: 200, Body: b}, e
	})
	switch {
	case errors.Is(e, storage.ErrInvalidScope):
		e = problem(422, "VALIDATION_ERROR", "Нужен ключ запроса")
	case errors.Is(e, storage.ErrIdempotencyMismatch):
		e = problem(409, "IDEMPOTENCY_MISMATCH", "Ключ использован для другого запроса")
	case errors.Is(e, storage.ErrOperationInProgress):
		e = problem(409, "OPERATION_IN_PROGRESS", "Запрос выполняется")
	}
	return json.RawMessage(result.Body), e
}
func decode[T any](s *Service, w http.ResponseWriter, r *http.Request, fn func(T) (any, error)) {
	in, p := platform.DecodeJSON[T](r)
	if p != nil {
		platform.WriteError(w, r, p)
		return
	}
	out, e := fn(in)
	s.reply(w, r, out, e)
}

type DefinitionInput struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Cost           int64  `json:"cost"`
	Available      bool   `json:"available"`
	Archived       bool   `json:"archived"`
	PriceConfirmed bool   `json:"price_confirmed"`
}
type Definition struct {
	ID          string `json:"id"`
	Version     int64  `json:"version"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Cost        int64  `json:"cost"`
	Available   bool   `json:"available"`
	Archived    bool   `json:"archived"`
}

const definitionColumns = "id::text,version,title,description,cost,available,archived"

func scanDefinition(row pgx.Row) (Definition, error) {
	var d Definition
	e := row.Scan(&d.ID, &d.Version, &d.Title, &d.Description, &d.Cost, &d.Available, &d.Archived)
	return d, e
}
func definition(ctx context.Context, tx pgx.Tx, owner, id string) (Definition, error) {
	if !validID(id) {
		return Definition{}, problem(404, "NOT_FOUND", "Награда не найдена")
	}
	d, e := scanDefinition(tx.QueryRow(ctx, "SELECT "+definitionColumns+" FROM custom_rewards WHERE owner_id=$1 AND id=$2 FOR UPDATE", owner, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Награда не найдена")
	}
	return d, e
}
func (s *Service) saveDefinition(ctx context.Context, tx pgx.Tx, owner, id string, in DefinitionInput, r *http.Request) (Definition, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Title) < 1 || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Description) > 2000 || strings.ContainsRune(in.Title+in.Description, 0) || in.Cost < 0 || in.Cost > 1000000 {
		return Definition{}, problem(422, "VALIDATION_ERROR", "Название до 120 символов, описание до 2000, цена от 0 до 1000000")
	}
	var d Definition
	var e error
	if id == "" {
		var count int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM custom_rewards WHERE owner_id=$1", owner).Scan(&count); e != nil {
			return d, e
		}
		if count >= 200 {
			return d, problem(409, "LIMIT_REACHED", "Не более 200 личных наград, включая архив")
		}
		d, e = scanDefinition(tx.QueryRow(ctx, "INSERT INTO custom_rewards(owner_id,title,description,cost,available,archived,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7) RETURNING "+definitionColumns, owner, in.Title, in.Description, in.Cost, in.Available, in.Archived, s.clock.Now()))
	} else {
		old, err := definition(ctx, tx, owner, id)
		if err != nil {
			return d, err
		}
		if p := platform.MatchVersion(r, old.Version); p != nil {
			return d, p
		}
		if old.Cost != in.Cost && !in.PriceConfirmed {
			return d, problem(422, "PRICE_CONFIRMATION_REQUIRED", "Подтвердите изменение цены. Старые покупки сохранят исходную стоимость.")
		}
		d, e = scanDefinition(tx.QueryRow(ctx, "UPDATE custom_rewards SET title=$3,description=$4,cost=$5,available=$6,archived=$7,version=version+1,updated_at=$8 WHERE owner_id=$1 AND id=$2 RETURNING "+definitionColumns, owner, id, in.Title, in.Description, in.Cost, in.Available, in.Archived, s.clock.Now()))
	}
	if e != nil {
		return d, e
	}
	snapshot, e := json.Marshal(d)
	if e != nil {
		return d, e
	}
	_, e = tx.Exec(ctx, "INSERT INTO custom_reward_versions(owner_id,reward_id,version,snapshot,created_at) VALUES($1,$2,$3,$4,$5)", owner, d.ID, d.Version, snapshot, s.clock.Now())
	return d, e
}
func (s *Service) definitions(ctx context.Context, tx pgx.Tx, id string) ([]Definition, error) {
	out := []Definition{}
	rows, e := tx.Query(ctx, "SELECT "+definitionColumns+" FROM custom_rewards WHERE owner_id=$1 ORDER BY created_at DESC,id", id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		d, e := scanDefinition(rows)
		if e != nil {
			return out, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		r.Get("/api/v1/rewards/custom", func(w http.ResponseWriter, r *http.Request) {
			id, e := owner(r.Context())
			var out []Definition
			if e == nil {
				e = s.progress.WithinOwner(r.Context(), id, func(tx pgx.Tx) error { var err error; out, err = s.definitions(r.Context(), tx, id); return err })
			}
			s.reply(w, r, out, e)
		})
		for _, method := range []string{"POST", "PATCH"} {
			path := "/api/v1/rewards/custom"
			if method == "PATCH" {
				path += "/{id}"
			}
			r.MethodFunc(method, path, func(w http.ResponseWriter, r *http.Request) {
				decode(s, w, r, func(in DefinitionInput) (any, error) {
					return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, id string) (any, error) {
						return s.saveDefinition(ctx, tx, id, chi.URLParam(r, "id"), in, r)
					})
				})
			})
		}
	})
}
