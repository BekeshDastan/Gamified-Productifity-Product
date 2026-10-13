package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type WeekChange struct {
	ID          string  `json:"id"`
	Version     int64   `json:"version"`
	Action      string  `json:"action"`
	PlannedDate *string `json:"planned_date"`
	Reason      string  `json:"reason"`
}
type WeekPlan struct {
	Date    string         `json:"date"`
	Version int64          `json:"version"`
	Input   planning.Input `json:"input"`
}
type WeekApplyInput struct {
	PreviewID string       `json:"preview_id"`
	Changes   []WeekChange `json:"changes"`
	Plans     []WeekPlan   `json:"plans"`
	Reason    string       `json:"reason"`
}
type WeekResult struct {
	PreviewID  string    `json:"preview_id"`
	Week       string    `json:"week"`
	Version    int64     `json:"version"`
	ChangedIDs []string  `json:"changed_ids"`
	PlanIDs    []string  `json:"plan_ids"`
	AppliedAt  time.Time `json:"applied_at"`
}

func (s *Service) WithQuests(q *quests.Service) *Service { s.quests = q; return s }
func (s *Service) applyWeek(r *http.Request, tx pgx.Tx, owner, date string, in WeekApplyInput) (any, error) {
	if !privateapi.UUID(in.PreviewID) || len(in.Changes) > 100 || len(in.Plans) > 7 || strings.TrimSpace(in.Reason) == "" || utf8.RuneCountInString(in.Reason) > 1000 || strings.ContainsRune(in.Reason, 0) {
		return nil, privateapi.Invalid("Выберите предпросмотр, до 100 изменений, до 7 планов и причину до 1000 символов")
	}
	if e := privateapi.Day(date); e != nil {
		return nil, e
	}
	ctx := r.Context()
	var snapshot, decision, result []byte
	var expires time.Time
	e := tx.QueryRow(ctx, "SELECT snapshot,decision,result,expires_at FROM weekly_review_previews WHERE owner_id=$1 AND id=$2 AND local_date=$3::text::date FOR UPDATE", owner, in.PreviewID, date).Scan(&snapshot, &decision, &result, &expires)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, privateapi.Problem(404, "NOT_FOUND", "Предпросмотр не найден")
	}
	if e != nil {
		return nil, e
	}
	body, e := json.Marshal(in)
	if e != nil {
		return nil, e
	}
	if result != nil {
		var previous WeekApplyInput
		if e = json.Unmarshal(decision, &previous); e != nil {
			return nil, e
		}
		canonical, e := json.Marshal(previous)
		if e != nil {
			return nil, e
		}
		if !bytes.Equal(body, canonical) {
			return nil, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Этот обзор уже применён с другим решением")
		}
		return json.RawMessage(result), nil
	}
	if !s.Clock.Now().Before(expires) {
		return nil, privateapi.Problem(409, "PREVIEW_EXPIRED", "Предпросмотр истёк. Подготовьте новый")
	}
	var frozen WeekSnapshot
	if e = json.Unmarshal(snapshot, &frozen); e != nil {
		return nil, e
	}
	if p := privateapi.MatchVersion(r, frozen.ReviewVersion); p != nil {
		return nil, p
	}
	var currentVersion int64
	var zone string
	var weekStart int
	if e = tx.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM weekly_reviews WHERE owner_id=$1 AND local_date=$2::text::date),0),(SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1),(SELECT (preferences->>'week_start')::int FROM user_settings WHERE owner_id=$1)", owner, date).Scan(&currentVersion, &zone, &weekStart); e != nil {
		return nil, e
	}
	weekDate, _ := time.Parse("2006-01-02", date)
	weekday := int(weekDate.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	if currentVersion != frozen.ReviewVersion || zone != frozen.Timezone || weekday != weekStart {
		return nil, privateapi.Problem(409, "PREVIEW_STALE", "Обзор или часовой пояс изменился. Подготовьте новый")
	}
	byID := map[string]WeekItem{}
	ids := []string{}
	for _, item := range frozen.Items {
		byID[item.ID] = item
		ids = append(ids, item.ID)
	}
	rows, e := tx.Query(ctx, "SELECT id::text,version FROM quests WHERE owner_id=$1 AND id::text=ANY($2::text[])", owner, ids)
	if e != nil {
		return nil, e
	}
	matched := 0
	for rows.Next() {
		var id string
		var version int64
		if e = rows.Scan(&id, &version); e != nil {
			rows.Close()
			return nil, e
		}
		if byID[id].Version != version {
			rows.Close()
			return nil, privateapi.Problem(409, "PREVIEW_STALE", "Задача изменилась. Обновите предпросмотр")
		}
		matched++
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if matched != len(ids) {
		return nil, privateapi.Problem(409, "PREVIEW_STALE", "Исходная задача отсутствует")
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return nil, e
	}
	today := s.Clock.Now().In(loc).Format("2006-01-02")
	next, e := time.Parse("2006-01-02", frozen.NextWeek)
	if e != nil {
		return nil, e
	}
	end := next.AddDate(0, 0, 7).Format("2006-01-02")
	plans := map[string]planning.Plan{}
	for _, p := range frozen.Plans {
		plans[p.Date] = p
	}
	// Validate all decisions before applying any of them.
	selected := map[string]bool{}
	for _, change := range in.Changes {
		item, ok := byID[change.ID]
		if !ok || selected[change.ID] || change.Version != item.Version || (change.Action != "move" && change.Action != "archive") || strings.TrimSpace(change.Reason) == "" || utf8.RuneCountInString(change.Reason) > 1000 || strings.ContainsRune(change.Reason, 0) {
			return nil, privateapi.Invalid("Проверьте выбранные задачи, версии и причины")
		}
		selected[change.ID] = true
		if change.Action == "move" && (change.PlannedDate == nil || privateapi.Day(*change.PlannedDate) != nil || *change.PlannedDate < frozen.NextWeek || *change.PlannedDate >= end || *change.PlannedDate < today) {
			return nil, privateapi.Invalid("Перенос возможен только на будущий день следующей недели")
		}
		if change.Action == "archive" && change.PlannedDate != nil {
			return nil, privateapi.Invalid("Для архива дата переноса не нужна")
		}
	}
	dates := map[string]bool{}
	for _, p := range in.Plans {
		source, ok := plans[p.Date]
		if !ok || dates[p.Date] || p.Date < today || p.Version != source.Version {
			return nil, privateapi.Invalid("Проверьте дни и версии следующего плана")
		}
		dates[p.Date] = true
		live, e := s.plans.ViewInTx(ctx, tx, owner, p.Date)
		if e != nil {
			return nil, e
		}
		if live.Version != source.Version {
			return nil, privateapi.Problem(409, "PREVIEW_STALE", "План дня изменился. Обновите предпросмотр")
		}
		if _, e = planning.Validate(p.Input); e != nil {
			return nil, e
		}
	}
	out := WeekResult{PreviewID: in.PreviewID, Week: date, ChangedIDs: []string{}, PlanIDs: []string{}, AppliedAt: s.Clock.Now().UTC()}
	for _, change := range in.Changes {
		if s.quests == nil {
			return nil, privateapi.Problem(503, "UNAVAILABLE", "Задачи недоступны")
		}
		if change.Action == "move" {
			_, e = s.quests.ScheduleInTx(ctx, tx, owner, change.ID, change.Version, *change.PlannedDate, change.Reason)
		} else {
			_, e = s.quests.ReviewArchiveInTx(ctx, tx, owner, change.ID, change.Version, change.Reason)
		}
		if e != nil {
			return nil, e
		}
		out.ChangedIDs = append(out.ChangedIDs, change.ID)
	}
	for _, p := range in.Plans {
		saved, e := s.plans.MorningInTx(r, tx, owner, p.Date, p.Input, p.Version, in.Reason)
		if e != nil {
			return nil, e
		}
		out.PlanIDs = append(out.PlanIDs, saved.ID)
	}
	payload, e := json.Marshal(struct {
		Decision WeekApplyInput `json:"decision"`
		Snapshot WeekSnapshot   `json:"snapshot"`
	}{in, frozen})
	if e != nil {
		return nil, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO weekly_reviews(owner_id,local_date,payload,updated_at) VALUES($1,$2::text::date,$3,$4) ON CONFLICT(owner_id,local_date) DO UPDATE SET payload=excluded.payload,version=weekly_reviews.version+1,updated_at=excluded.updated_at RETURNING version`, owner, date, payload, out.AppliedAt).Scan(&out.Version)
	if e != nil {
		return nil, e
	}
	result, e = json.Marshal(out)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, "UPDATE weekly_review_previews SET decision=$3,result=$4 WHERE owner_id=$1 AND id=$2", owner, in.PreviewID, body, result)
	return out, e
}
func (s *Service) mountWeekApply(r chi.Router) {
	r.Post("/api/v1/reviews/week/{date}/apply-plan", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in WeekApplyInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.applyWeek(r, tx, owner, chi.URLParam(r, "date"), in)
			})
		})
	})
}
