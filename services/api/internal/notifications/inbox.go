// Package notifications owns the private inbox and consent-based delivery history.
package notifications

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
)

type Service struct {
	*privateapi.Service
	habits          *habits.Service
	schedulingClock platform.Clock
	mailer          mail.Mailer
	origin          string
	unsubscribeKey  []byte
}

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{Service: privateapi.New(pool, clock)}
}

type Event struct {
	Kind           string  `json:"kind"`
	SourceKind     string  `json:"source_kind"`
	SourceID       string  `json:"source_id"`
	SourceRevision int64   `json:"source_revision"`
	EventKey       string  `json:"event_key"`
	URL            string  `json:"url"`
	ReminderID     *string `json:"reminder_id"`
}
type Notification struct {
	Event
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
	Version   int64      `json:"version"`
	Active    bool       `json:"active"`
	Muted     bool       `json:"muted"`
}
type Page struct {
	Items      []Notification `json:"items"`
	Next       string         `json:"next"`
	Unread     int            `json:"unread"`
	ComputedAt time.Time      `json:"computed_at"`
}

var titles = map[string]string{"deadline": "Напоминание о сроке задачи", "habit": "Окно привычки", "achievement": "Новое достижение", "invite": "Новое приглашение", "report": "Готов отчёт", "focus_result": "Подтвердите итоги фокуса"}
var messages = map[string]string{"deadline": "Проверьте задачу и свободные окна. Решение об изменении плана остаётся за вами.", "habit": "В текущем запланированном периоде ещё осталась норма. Можно открыть привычку.", "achievement": "В приложении появилось достижение по подтверждённым записям.", "invite": "Приглашение доступно в приложении. Доступ появляется только после вашего согласия.", "report": "Сохранён личный отчёт. Откройте его в приложении.", "focus_result": "Таймер завершён; рабочие минуты ждут вашего подтверждения."}

const columns = `id::text,kind,source_kind,source_id,source_revision,event_key,source_url,reminder_id::text,title,message,created_at,read_at,version,active,muted`

func scan(row pgx.Row) (Notification, error) {
	var n Notification
	e := row.Scan(&n.ID, &n.Kind, &n.SourceKind, &n.SourceID, &n.SourceRevision, &n.EventKey, &n.URL, &n.ReminderID, &n.Title, &n.Message, &n.CreatedAt, &n.ReadAt, &n.Version, &n.Active, &n.Muted)
	return n, e
}
func validateEvent(in Event) error {
	expected := map[string]string{"deadline": "task", "habit": "habit_period", "achievement": "achievement", "invite": "invite", "report": "report", "focus_result": "focus"}
	if expected[in.Kind] == "" || in.SourceKind != expected[in.Kind] {
		return privateapi.Invalid("Неверный тип источника уведомления")
	}
	if _, ok := titles[in.Kind]; !ok || len(in.SourceID) < 1 || len(in.SourceID) > 200 || in.SourceRevision < 1 || len(in.EventKey) < 1 || len(in.EventKey) > 300 || !strings.HasPrefix(in.URL, "/app/") || strings.ContainsAny(in.URL, "\r\n\\") || strings.Contains(in.URL, "..") || len(in.URL) > 500 {
		return privateapi.Invalid("Некорректный источник уведомления")
	}
	return nil
}

// EmitInTx accepts trusted structured domain events; no client route can emit arbitrary cards.
func (s *Service) EmitInTx(ctx context.Context, tx pgx.Tx, owner string, in Event) (Notification, bool, error) {
	if e := validateEvent(in); e != nil {
		return Notification{}, false, e
	}
	prefs, e := s.preferences(ctx, tx, owner)
	if e != nil {
		return Notification{}, false, e
	}
	n, e := scan(tx.QueryRow(ctx, `INSERT INTO notifications(owner_id,kind,source_kind,source_id,source_revision,event_key,source_url,reminder_id,title,message,created_at,muted) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(owner_id,event_key,source_revision) DO NOTHING RETURNING `+columns, owner, in.Kind, in.SourceKind, in.SourceID, in.SourceRevision, in.EventKey, in.URL, in.ReminderID, titles[in.Kind], messages[in.Kind], s.Clock.Now(), !prefs.Channel(in.Kind).InApp))
	if e == nil {
		if e = s.queueDeliveries(ctx, tx, owner, n); e != nil {
			return n, true, e
		}
		return n, true, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return n, false, e
	}
	n, e = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM notifications WHERE owner_id=$1 AND event_key=$2 AND source_revision=$3`, owner, in.EventKey, in.SourceRevision))
	if e == nil && (n.Kind != in.Kind || n.SourceKind != in.SourceKind || n.SourceID != in.SourceID || n.URL != in.URL) {
		return n, false, privateapi.Problem(409, "DEDUPE_MISMATCH", "Источник уведомления изменился")
	}
	return n, false, e
}
func (s *Service) list(ctx context.Context, tx pgx.Tx, owner, before string) (Page, error) {
	out := Page{Items: []Notification{}, ComputedAt: s.Clock.Now()}
	var cursorTime *time.Time
	var cursorID *string
	if before != "" {
		if !privateapi.UUID(before) {
			return out, privateapi.Problem(404, "NOT_FOUND", "Страница уведомлений не найдена")
		}
		var at time.Time
		e := tx.QueryRow(ctx, `SELECT created_at FROM notifications WHERE owner_id=$1 AND id=$2`, owner, before).Scan(&at)
		if errors.Is(e, pgx.ErrNoRows) {
			return out, privateapi.Problem(404, "NOT_FOUND", "Страница уведомлений не найдена")
		}
		if e != nil {
			return out, e
		}
		cursorTime = &at
		cursorID = &before
	}
	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM notifications WHERE owner_id=$1 AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3::uuid)) ORDER BY created_at DESC,id DESC LIMIT 26`, owner, cursorTime, cursorID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		n, e := scan(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, n)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		out.Next = out.Items[24].ID
	}
	e = tx.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE owner_id=$1 AND read_at IS NULL AND active AND NOT muted`, owner).Scan(&out.Unread)
	return out, e
}
func (s *Service) get(ctx context.Context, tx pgx.Tx, owner, id string) (Notification, error) {
	if !privateapi.UUID(id) {
		return Notification{}, privateapi.Problem(404, "NOT_FOUND", "Уведомление не найдено")
	}
	n, e := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM notifications WHERE owner_id=$1 AND id=$2`, owner, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = privateapi.Problem(404, "NOT_FOUND", "Уведомление не найдено")
	}
	return n, e
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	s.mountUnsubscribe(router)
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountPreferences(r)
		s.mountReminders(r)
		s.mountDeliveries(r)
		r.Get("/api/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.list(ctx, tx, owner, r.URL.Query().Get("before"))
			})
			s.Reply(w, r, out, e)
		})
		r.Get("/api/v1/notifications/{id}", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.get(ctx, tx, owner, chi.URLParam(r, "id"))
			})
			s.Reply(w, r, out, e)
		})
		r.Patch("/api/v1/notifications/{id}/read", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in struct {
				Read bool `json:"read"`
			}) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					n, e := s.get(ctx, tx, owner, chi.URLParam(r, "id"))
					if e != nil {
						return nil, e
					}
					if e := privateapi.MatchVersion(r, n.Version); e != nil {
						return nil, e
					}
					if in.Read == (n.ReadAt != nil) {
						return n, nil
					}
					var at *time.Time
					if in.Read {
						now := s.Clock.Now()
						at = &now
					}
					return scan(tx.QueryRow(ctx, `UPDATE notifications SET read_at=$3,version=version+1 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, owner, n.ID, at))
				})
			})
		})
		r.Post("/api/v1/notifications/read-all", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in struct {
				SeenBefore time.Time `json:"seen_before"`
			}) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					now := s.Clock.Now()
					if in.SeenBefore.IsZero() || in.SeenBefore.After(now) {
						return nil, privateapi.Invalid("Обновите список перед отметкой прочитанного")
					}
					tag, e := tx.Exec(ctx, `UPDATE notifications SET read_at=$3,version=version+1 WHERE owner_id=$1 AND created_at<=$2 AND read_at IS NULL`, owner, in.SeenBefore, now)
					return map[string]int64{"updated": tag.RowsAffected()}, e
				})
			})
		})
	})
}
