package seasons

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Service struct{ *privateapi.Service }

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{privateapi.New(pool, clock)}
}

type Input struct {
	Title         string `json:"title"`
	Theme         string `json:"theme"`
	DesiredResult string `json:"desired_result"`
	StartsOn      string `json:"starts_on"`
	EndsOn        string `json:"ends_on"`
	Primary       bool   `json:"primary"`
}
type CreateInput struct {
	Input
	OperationID string `json:"operation_id"`
}
type Season struct {
	Input
	ID            string          `json:"id"`
	Timezone      string          `json:"timezone"`
	State         string          `json:"state"`
	Version       int64           `json:"version"`
	ActivatedAt   *time.Time      `json:"activated_at"`
	ClosedAt      *time.Time      `json:"closed_at"`
	FinalSnapshot json.RawMessage `json:"final_snapshot"`
	ArchivedAt    *time.Time      `json:"archived_at"`
	CreatedAt     time.Time       `json:"created_at"`
}

const columns = "id::text,title,theme,desired_result,starts_on::text,ends_on::text,timezone,is_primary,state,version,activated_at,closed_at,final_snapshot,archived_at,created_at"

func scan(row pgx.Row) (Season, error) {
	var v Season
	e := row.Scan(&v.ID, &v.Title, &v.Theme, &v.DesiredResult, &v.StartsOn, &v.EndsOn, &v.Timezone, &v.Primary, &v.State, &v.Version, &v.ActivatedAt, &v.ClosedAt, &v.FinalSnapshot, &v.ArchivedAt, &v.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = privateapi.Problem(404, "NOT_FOUND", "Сезон не найден")
	}
	return v, e
}
func (s *Service) get(ctx context.Context, tx pgx.Tx, owner, id string) (Season, error) {
	if !privateapi.UUID(id) {
		return Season{}, privateapi.Problem(404, "NOT_FOUND", "Сезон не найден")
	}
	return scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM personal_seasons WHERE owner_id=$1 AND id=$2", owner, id))
}
func validText(v string, min, max int) bool {
	return utf8.RuneCountInString(strings.TrimSpace(v)) >= min && utf8.RuneCountInString(v) <= max && strings.IndexFunc(v, unicode.IsControl) < 0
}
func (s *Service) event(ctx context.Context, tx pgx.Tx, owner, kind string, v Season) error {
	body, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO personal_season_events(owner_id,season_id,version,kind,snapshot,created_at) VALUES($1,$2,$3,$4,$5,$6)", owner, v.ID, v.Version, kind, body, s.Clock.Now())
	return e
}
func (s *Service) create(ctx context.Context, tx pgx.Tx, owner string, in CreateInput) (Season, error) {
	if !privateapi.UUID(in.OperationID) || privateapi.Day(in.StartsOn) != nil || privateapi.Day(in.EndsOn) != nil {
		return Season{}, privateapi.Invalid("Проверьте идентификатор и даты")
	}
	for _, v := range []string{in.Title, in.Theme, in.DesiredResult} {
		if strings.ContainsRune(v, 0) {
			return Season{}, privateapi.Invalid("Нулевой символ недопустим")
		}
	}
	body, e := json.Marshal(in)
	if e != nil {
		return Season{}, e
	}
	var same bool
	e = tx.QueryRow(ctx, "SELECT request=$3::jsonb FROM personal_seasons WHERE owner_id=$1 AND id=$2", owner, in.OperationID, body).Scan(&same)
	if e == nil {
		if !same {
			return Season{}, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Этот сезон уже создан с другими настройками")
		}
		return s.get(ctx, tx, owner, in.OperationID)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Season{}, e
	}
	if !validText(in.Title, 1, 200) || !validText(in.Theme, 1, 200) || !validText(in.DesiredResult, 1, 2000) {
		return Season{}, privateapi.Invalid("Укажите название/тему до 200 и желаемый результат до 2000 символов")
	}
	var zone string
	if e = tx.QueryRow(ctx, "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&zone); e != nil {
		return Season{}, e
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return Season{}, e
	}
	now := s.Clock.Now()
	today := now.In(loc).Format("2006-01-02")
	a, _ := time.Parse("2006-01-02", in.StartsOn)
	b, _ := time.Parse("2006-01-02", in.EndsOn)
	days := int(b.Sub(a)/(24*time.Hour)) + 1
	if in.StartsOn < today || in.StartsOn > now.In(loc).AddDate(1, 0, 0).Format("2006-01-02") || days < 1 || days > 365 {
		return Season{}, privateapi.Invalid("Сезон начинается сегодня или позже и длится до 365 дней")
	}
	var count int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM personal_seasons WHERE owner_id=$1 AND state IN('planned','active')", owner).Scan(&count); e != nil {
		return Season{}, e
	}
	if count >= 50 {
		return Season{}, privateapi.Problem(409, "SEASON_LIMIT", "Сначала завершите один из 50 открытых сезонов")
	}
	v, e := scan(tx.QueryRow(ctx, "INSERT INTO personal_seasons(owner_id,id,request,title,theme,desired_result,starts_on,ends_on,timezone,is_primary,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING "+columns, owner, in.OperationID, body, strings.TrimSpace(in.Title), strings.TrimSpace(in.Theme), strings.TrimSpace(in.DesiredResult), in.StartsOn, in.EndsOn, zone, in.Primary, now))
	if e != nil {
		return v, e
	}
	return v, s.event(ctx, tx, owner, "created", v)
}

type Confirmation struct {
	Confirm bool `json:"confirm"`
}

func (s *Service) activate(r *http.Request, tx pgx.Tx, owner, id string, in Confirmation) (Season, error) {
	v, e := s.get(r.Context(), tx, owner, id)
	if e != nil {
		return v, e
	}
	if !in.Confirm {
		return v, privateapi.Invalid("Подтвердите начало сезона")
	}
	if v.State == "active" {
		return v, nil
	}
	if v.State != "planned" || v.ArchivedAt != nil {
		return v, privateapi.Problem(409, "SEASON_CLOSED", "Этот сезон нельзя начать")
	}
	if p := platform.MatchVersion(r, v.Version); p != nil {
		return v, p
	}
	loc, e := time.LoadLocation(v.Timezone)
	if e != nil {
		return v, e
	}
	today := s.Clock.Now().In(loc).Format("2006-01-02")
	if today < v.StartsOn || today > v.EndsOn {
		return v, privateapi.Problem(409, "SEASON_OUTSIDE_DATES", "Начало доступно только в пределах дат сезона")
	}
	if v.Primary {
		var another bool
		if e = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM personal_seasons WHERE owner_id=$1 AND state='active' AND is_primary AND id<>$2)", owner, id).Scan(&another); e != nil {
			return v, e
		}
		if another {
			return v, privateapi.Problem(409, "PRIMARY_SEASON_EXISTS", "Уже есть основной активный сезон. Сначала завершите его")
		}
	}
	v, e = scan(tx.QueryRow(r.Context(), "UPDATE personal_seasons SET state='active',version=version+1,activated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING "+columns, owner, id, s.Clock.Now()))
	if e != nil {
		return v, e
	}
	return v, s.event(r.Context(), tx, owner, "activated", v)
}

type Page struct {
	Items []Season `json:"items"`
	Next  string   `json:"next"`
}

func (s *Service) list(r *http.Request, tx pgx.Tx, owner string) (Page, error) {
	out := Page{Items: []Season{}}
	var at *time.Time
	var id *string
	if before := r.URL.Query().Get("before"); before != "" {
		v, e := s.get(r.Context(), tx, owner, before)
		if e != nil {
			return out, e
		}
		at = &v.CreatedAt
		id = &v.ID
	}
	rows, e := tx.Query(r.Context(), "SELECT "+columns+" FROM personal_seasons WHERE owner_id=$1 AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3::uuid)) ORDER BY created_at DESC,id DESC LIMIT 26", owner, at, id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		out.Next = out.Items[24].ID
	}
	return out, nil
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		r.Get("/api/v1/seasons", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.list(r, tx, owner) })
			s.Reply(w, r, out, e)
		})
		r.Post("/api/v1/seasons", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in CreateInput) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.create(ctx, tx, owner, in) })
			})
		})
		r.Get("/api/v1/seasons/{id}", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.get(ctx, tx, owner, chi.URLParam(r, "id"))
			})
			s.Reply(w, r, out, e)
		})
		r.Post("/api/v1/seasons/{id}/activate", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in Confirmation) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					return s.activate(r, tx, owner, chi.URLParam(r, "id"), in)
				})
			})
		})
	})
}
