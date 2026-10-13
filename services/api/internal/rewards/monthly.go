package rewards

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type MonthlyBoss struct {
	Month    string  `json:"month"`
	Timezone string  `json:"timezone"`
	Version  int64   `json:"version"`
	BossID   *string `json:"boss_id"`
	Boss     *Boss   `json:"boss"`
}
type MonthlyChange struct {
	ID        int64     `json:"id"`
	Month     string    `json:"month"`
	Timezone  string    `json:"timezone"`
	BossID    *string   `json:"boss_id"`
	Title     string    `json:"title"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type MonthlyHistory struct {
	Items      []MonthlyChange `json:"items"`
	NextCursor *string         `json:"next_cursor"`
}

func (s *Service) monthly(ctx context.Context, tx pgx.Tx, owner string) (MonthlyBoss, error) {
	out := MonthlyBoss{}
	var zone string
	e := tx.QueryRow(ctx, "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&zone)
	if e != nil {
		return out, e
	}
	z, e := time.LoadLocation(zone)
	if e != nil {
		return out, e
	}
	out.Month = s.clock.Now().In(z).Format("2006-01")
	out.Timezone = zone
	if _, e = tx.Exec(ctx, "INSERT INTO monthly_bosses(owner_id,month,timezone,updated_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", owner, out.Month, zone, s.clock.Now()); e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, "SELECT version,boss_id::text,timezone FROM monthly_bosses WHERE owner_id=$1 AND month=$2 FOR UPDATE", owner, out.Month).Scan(&out.Version, &out.BossID, &out.Timezone)
	if e != nil {
		return out, e
	}
	if out.BossID != nil {
		b, e := s.boss(ctx, tx, owner, *out.BossID)
		if e != nil {
			return out, e
		}
		out.Boss = &b
	}
	return out, nil
}
func (s *Service) monthlyHistory(ctx context.Context, tx pgx.Tx, owner, cursor string) (MonthlyHistory, error) {
	out := MonthlyHistory{Items: []MonthlyChange{}}
	before := int64(1<<63 - 1)
	if cursor != "" {
		n, e := strconv.ParseInt(cursor, 10, 64)
		if e != nil || n < 1 {
			return out, problem(422, "VALIDATION_ERROR", "Некорректная граница истории")
		}
		var exists bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM monthly_boss_changes WHERE owner_id=$1 AND id=$2)", owner, n).Scan(&exists); e != nil {
			return out, e
		}
		if !exists {
			return out, problem(404, "NOT_FOUND", "Запись не найдена")
		}
		before = n
	}
	rows, e := tx.Query(ctx, "SELECT id,month,timezone,boss_id::text,title,reason,created_at FROM monthly_boss_changes WHERE owner_id=$1 AND id<$2 ORDER BY id DESC LIMIT 51", owner, before)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var r MonthlyChange
		if e = rows.Scan(&r.ID, &r.Month, &r.Timezone, &r.BossID, &r.Title, &r.Reason, &r.CreatedAt); e != nil {
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	if len(out.Items) > 50 {
		out.Items = out.Items[:50]
		cursor := strconv.FormatInt(out.Items[49].ID, 10)
		out.NextCursor = &cursor
	}
	return out, rows.Err()
}
func (s *Service) mountMonthly(r chi.Router) {
	r.Get("/api/v1/bosses/monthly", func(w http.ResponseWriter, r *http.Request) {
		owner, e := owner(r.Context())
		var out MonthlyBoss
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error {
				if e := s.character.EvaluateInTx(r.Context(), tx, owner); e != nil {
					return e
				}
				var e error
				out, e = s.monthly(r.Context(), tx, owner)
				return e
			})
		}
		s.reply(w, r, out, e)
	})
	r.Get("/api/v1/bosses/monthly/history", func(w http.ResponseWriter, r *http.Request) {
		owner, e := owner(r.Context())
		var out MonthlyHistory
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error {
				var e error
				out, e = s.monthlyHistory(r.Context(), tx, owner, r.URL.Query().Get("before"))
				return e
			})
		}
		s.reply(w, r, out, e)
	})
	r.Put("/api/v1/bosses/monthly", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in struct {
			Month  string  `json:"month"`
			BossID *string `json:"boss_id"`
			Reason string  `json:"reason"`
		}) (any, error) {
			return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				current, e := s.monthly(ctx, tx, owner)
				if e != nil {
					return nil, e
				}
				if in.Month != current.Month {
					return nil, problem(409, "VERSION_CONFLICT", "Месяц изменился. Обновите выбор перед сохранением")
				}
				if p := platform.MatchVersion(r, current.Version); p != nil {
					return nil, p
				}
				reason := strings.TrimSpace(in.Reason)
				if len(reason) == 0 || utf8.RuneCountInString(reason) > 1000 {
					return nil, problem(422, "VALIDATION_ERROR", "Укажите причину выбора до 1000 символов")
				}
				title := ""
				if in.BossID != nil {
					b, e := s.boss(ctx, tx, owner, *in.BossID)
					if e != nil {
						return nil, e
					}
					if b.State == "closed" {
						return nil, problem(409, "BOSS_CLOSED", "Сначала возобновите бой")
					}
					title = b.Title
				}
				_, e = tx.Exec(ctx, "UPDATE monthly_bosses SET boss_id=$3,title=$4,version=version+1,updated_at=$5 WHERE owner_id=$1 AND month=$2", owner, current.Month, in.BossID, title, s.clock.Now())
				if e != nil {
					return nil, e
				}
				_, e = tx.Exec(ctx, "INSERT INTO monthly_boss_changes(owner_id,month,timezone,boss_id,title,reason,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)", owner, current.Month, current.Timezone, in.BossID, title, reason, s.clock.Now())
				if e != nil {
					return nil, e
				}
				return s.monthly(ctx, tx, owner)
			})
		})
	})
}
