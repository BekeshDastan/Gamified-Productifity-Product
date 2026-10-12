package profile

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool  *pgxpool.Pool
	clock platform.Clock
}

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service { return &Service{pool, clock} }

type Input struct {
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}
type Profile struct {
	Input
	OwnerID   string    `json:"owner_id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var nicknamePattern = regexp.MustCompile(`^[a-z0-9_]{3,24}$`)

func invalid(field, message string) *platform.APIError {
	return &platform.APIError{Status: 422, Code: "VALIDATION_ERROR", Message: "Проверьте поля формы", Fields: map[string]string{field: message}}
}
func Validate(input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Nickname = strings.ToLower(strings.TrimSpace(input.Nickname))
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 80 || strings.IndexFunc(input.Name, unicode.IsControl) >= 0 {
		return Input{}, invalid("name", "До 80 символов, без управляющих символов")
	}
	if input.Nickname != "" && !nicknamePattern.MatchString(input.Nickname) {
		return Input{}, invalid("nickname", "От 3 до 24 символов: латинские буквы, цифры и _")
	}
	switch input.Avatar {
	case "mountain", "compass", "sprout", "sun", "moon", "star":
	default:
		return Input{}, invalid("avatar", "Выберите аватар из каталога")
	}
	return input, nil
}

const columns = `name,nickname,avatar,owner_id::text,version,created_at,updated_at`

func scan(row pgx.Row) (Profile, error) {
	var value Profile
	err := row.Scan(&value.Name, &value.Nickname, &value.Avatar, &value.OwnerID, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}
func owner(ctx context.Context) (string, error) {
	id, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", &platform.APIError{Status: 401, Code: "UNAUTHORIZED", Message: "Войдите в аккаунт"}
	}
	return id.UserID, nil
}
func (s *Service) Get(ctx context.Context) (Profile, error) {
	id, err := owner(ctx)
	if err != nil {
		return Profile{}, err
	}
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM profiles WHERE owner_id=$1`, id))
}
func (s *Service) Update(r *http.Request, input Input) (Profile, error) {
	id, err := owner(r.Context())
	if err != nil {
		return Profile{}, err
	}
	input, err = Validate(input)
	if err != nil {
		return Profile{}, err
	}
	var result Profile
	err = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		current, err := scan(tx.QueryRow(r.Context(), `SELECT `+columns+` FROM profiles WHERE owner_id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if problem := platform.MatchVersion(r, current.Version); problem != nil {
			return problem
		}
		result, err = scan(tx.QueryRow(r.Context(), `UPDATE profiles SET name=$2,nickname=$3,avatar=$4,version=version+1,updated_at=$5 WHERE owner_id=$1 RETURNING `+columns, id, input.Name, input.Nickname, input.Avatar, s.clock.Now().UTC()))
		if err != nil {
			var db *pgconn.PgError
			if errors.As(err, &db) && db.Code == "23505" {
				return invalid("nickname", "Этот ник уже занят")
			}
			return err
		}
		return audit(r.Context(), tx, id, "profile.updated", "profile", id)
	})
	return result, err
}
func audit(ctx context.Context, tx pgx.Tx, id, action, kind, entity string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,metadata) VALUES($1,$2,$3,$4,'{}')`, id, action, kind, entity)
	return err
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		var problem *platform.APIError
		if !errors.As(err, &problem) {
			problem = &platform.APIError{Status: 500, Code: "INTERNAL_ERROR", Message: "Не удалось выполнить запрос"}
		}
		platform.WriteError(w, r, problem)
		return
	}
	platform.WriteData(w, r, s.clock, 200, value)
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountSettings(r)
		s.mountAreas(r)
		s.mountOnboarding(r)
		r.Get("/api/v1/me", func(w http.ResponseWriter, r *http.Request) {
			value, err := s.Get(r.Context())
			s.reply(w, r, value, err)
		})
		r.Patch("/api/v1/me", func(w http.ResponseWriter, r *http.Request) {
			input, problem := platform.DecodeJSON[Input](r)
			if problem != nil {
				platform.WriteError(w, r, problem)
				return
			}
			value, err := s.Update(r, input)
			s.reply(w, r, value, err)
		})
	})
}
