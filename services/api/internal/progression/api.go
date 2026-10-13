package progression

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"time"
)

type RulesView struct {
	TaskXP       []int64 `json:"task_xp"`
	HabitXP      int64   `json:"habit_xp"`
	ChallengeXP  int64   `json:"challenge_xp"`
	CoinsDivisor int64   `json:"coins_divisor"`
}

func rulesView() RulesView {
	return RulesView{TaskXP: []int64{TaskXP(1), TaskXP(2), TaskXP(3), TaskXP(4), TaskXP(5)}, HabitXP: BaseXP("habit", 0), ChallengeXP: BaseXP("challenge", 0), CoinsDivisor: 5}
}

type LedgerEntry struct {
	ID             string    `json:"id"`
	Sequence       int64     `json:"sequence"`
	SourceKind     string    `json:"source_kind"`
	SourceKey      string    `json:"source_key"`
	SourceTitle    string    `json:"source_title"`
	SourceURL      *string   `json:"source_url"`
	Kind           string    `json:"kind"`
	EconomyVersion string    `json:"economy_version"`
	AreaID         *string   `json:"area_id"`
	AreaName       *string   `json:"area_name"`
	Difficulty     int       `json:"difficulty"`
	RequestedXP    int64     `json:"requested_xp"`
	AwardedXP      int64     `json:"awarded_xp"`
	XPDelta        int64     `json:"xp_delta"`
	CoinsDelta     int64     `json:"coins_delta"`
	Reason         string    `json:"reason"`
	OccurredAt     time.Time `json:"occurred_at"`
}
type LedgerPage struct {
	Items      []LedgerEntry `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

func (s *Service) history(ctx context.Context, tx pgx.Tx, owner, cursor string) (LedgerPage, error) {
	out := LedgerPage{Items: []LedgerEntry{}}
	before := int64(1<<63 - 1)
	if cursor != "" {
		var id pgtype.UUID
		if id.Scan(cursor) != nil || !id.Valid {
			return out, &platform.APIError{Status: 422, Code: "VALIDATION_ERROR", Message: "Некорректная граница истории"}
		}
		e := tx.QueryRow(ctx, `SELECT sequence FROM reward_ledger WHERE owner_id=$1 AND id=$2`, owner, cursor).Scan(&before)
		if errors.Is(e, pgx.ErrNoRows) {
			return out, &platform.APIError{Status: 404, Code: "NOT_FOUND", Message: "Запись не найдена"}
		}
		if e != nil {
			return out, e
		}
	}
	rows, e := tx.Query(ctx, `SELECT l.id::text,l.sequence,l.source_kind,l.source_key,l.kind,l.economy_version,l.area_id::text,a.name,COALESCE(e.difficulty,0),COALESCE(e.requested_xp,0),COALESCE(e.awarded_xp,0),l.xp_delta,l.coins_delta,l.reason,l.occurred_at,COALESCE(q.payload->>'title',h.payload->>'title',CASE l.source_kind WHEN 'task' THEN 'Задача' WHEN 'habit' THEN 'Привычка' WHEN 'spend' THEN 'Расход монет' ELSE 'Испытание' END),q.id::text,h.id::text FROM reward_ledger l LEFT JOIN reward_entitlements e ON e.owner_id=l.owner_id AND e.id=l.entitlement_id LEFT JOIN life_areas a ON a.owner_id=l.owner_id AND a.id=l.area_id LEFT JOIN quests q ON l.source_kind='task' AND q.owner_id=l.owner_id AND q.id::text=l.source_key LEFT JOIN habit_periods hp ON l.source_kind='habit' AND hp.owner_id=l.owner_id AND hp.id::text=l.source_key LEFT JOIN habits h ON h.owner_id=hp.owner_id AND h.id=hp.habit_id WHERE l.owner_id=$1 AND l.sequence<$2 ORDER BY l.sequence DESC LIMIT 51`, owner, before)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var entry LedgerEntry
		var quest, habit *string
		if e = rows.Scan(&entry.ID, &entry.Sequence, &entry.SourceKind, &entry.SourceKey, &entry.Kind, &entry.EconomyVersion, &entry.AreaID, &entry.AreaName, &entry.Difficulty, &entry.RequestedXP, &entry.AwardedXP, &entry.XPDelta, &entry.CoinsDelta, &entry.Reason, &entry.OccurredAt, &entry.SourceTitle, &quest, &habit); e != nil {
			return out, e
		}
		if quest != nil {
			url := "/app/quests/" + *quest
			entry.SourceURL = &url
		}
		if habit != nil {
			url := "/app/habits#habit-" + *habit
			entry.SourceURL = &url
		}
		out.Items = append(out.Items, entry)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > 50 {
		out.Items = out.Items[:50]
		cursor := out.Items[49].ID
		out.NextCursor = &cursor
	}
	return out, nil
}
func currentOwner(ctx context.Context) (string, error) {
	id, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", &platform.APIError{Status: 401, Code: "UNAUTHORIZED", Message: "Войдите в аккаунт"}
	}
	return id.UserID, nil
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, out any, e error) {
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) {
			p = &platform.APIError{Status: 500, Code: "INTERNAL_ERROR", Message: "Не удалось загрузить прогресс"}
		}
		platform.WriteError(w, r, p)
		return
	}
	platform.WriteData(w, r, s.clock, 200, out)
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		r.Get("/api/v1/progression", func(w http.ResponseWriter, r *http.Request) {
			owner, e := currentOwner(r.Context())
			var out Balance
			if e == nil {
				e = s.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error { var err error; out, err = s.Balance(r.Context(), tx, owner); return err })
			}
			s.reply(w, r, out, e)
		})
		r.Get("/api/v1/progression/history", func(w http.ResponseWriter, r *http.Request) {
			owner, e := currentOwner(r.Context())
			var out LedgerPage
			if e == nil {
				e = s.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error {
					var err error
					out, err = s.history(r.Context(), tx, owner, r.URL.Query().Get("before"))
					return err
				})
			}
			s.reply(w, r, out, e)
		})
	})
}
