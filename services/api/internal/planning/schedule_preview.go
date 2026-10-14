package planning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"time"
)

type TaskOption struct {
	ID              string `json:"id"`
	AllowSplit      bool   `json:"allow_split"`
	FiveMinuteStart bool   `json:"five_minute_start"`
}
type ScheduleInput struct {
	From           string       `json:"from"`
	To             string       `json:"to"`
	Tasks          []TaskOption `json:"tasks"`
	Breaks         []Interval   `json:"breaks"`
	BetweenMinutes int          `json:"between_minutes"`
}
type InputVersion struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type ScheduleDay struct {
	Date        string           `json:"date"`
	PlanVersion int64            `json:"plan_version"`
	Input       Input            `json:"input"`
	Timezone    string           `json:"timezone"`
	Blocks      []calendar.Event `json:"blocks"`
}
type SchedulePreview struct {
	ID            string         `json:"id"`
	Input         ScheduleInput  `json:"input"`
	Timezone      string         `json:"timezone"`
	CreatedAt     time.Time      `json:"created_at"`
	ExpiresAt     time.Time      `json:"expires_at"`
	InputVersions []InputVersion `json:"input_versions"`
	Hash          string         `json:"hash"`
	Tasks         []ScheduleTask `json:"tasks"`
	Days          []ScheduleDay  `json:"days"`
	Suggestion
}
type ScheduleApply struct {
	OperationID string `json:"operation_id"`
	PreviewID   string `json:"preview_id"`
	Confirm     bool   `json:"confirm"`
}
type ScheduleResult struct {
	OperationID string           `json:"operation_id"`
	PreviewID   string           `json:"preview_id"`
	AppliedAt   time.Time        `json:"applied_at"`
	Blocks      []calendar.Event `json:"blocks"`
	Plans       []Plan           `json:"plans"`
}

func validateSchedule(in ScheduleInput) (time.Time, time.Time, error) {
	a, e := day(in.From)
	if e != nil {
		return a, a, e
	}
	b, e := day(in.To)
	if e != nil || !b.After(a) || b.Sub(a) > 14*24*time.Hour {
		return a, b, invalid("to", "От одного до 14 дней; конечная дата не включается")
	}
	if len(in.Tasks) < 1 || len(in.Tasks) > 100 || len(in.Breaks) > 100 || in.BetweenMinutes < 0 || in.BetweenMinutes > 60 {
		return a, b, invalid("tasks", "Выберите 1–100 задач и перерыв 0–60 минут")
	}
	ids := map[string]bool{}
	for _, q := range in.Tasks {
		if !uuid(q.ID) || ids[q.ID] {
			return a, b, invalid("tasks", "Собственные задачи без повторов")
		}
		ids[q.ID] = true
	}
	for _, v := range in.Breaks {
		if !v.End.After(v.Start) {
			return a, b, invalid("breaks", "Проверьте интервалы перерывов")
		}
	}
	return a, b, nil
}
func (s *Service) scheduleSnapshot(ctx context.Context, tx pgx.Tx, owner string, in ScheduleInput, anchor time.Time) (SchedulePreview, error) {
	if in.Breaks == nil {
		in.Breaks = []Interval{}
	}
	out := SchedulePreview{Input: in, CreatedAt: anchor, ExpiresAt: anchor.Add(10 * time.Minute), InputVersions: []InputVersion{}, Tasks: []ScheduleTask{}, Days: []ScheduleDay{}}
	a, b, e := validateSchedule(in)
	if e != nil {
		return out, e
	}
	var settingsVersion int64
	if e = tx.QueryRow(ctx, `SELECT preferences->>'timezone',version FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&out.Timezone, &settingsVersion); e != nil {
		return out, e
	}
	zone, e := time.LoadLocation(out.Timezone)
	if e != nil {
		return out, e
	}
	start := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, zone)
	end := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, zone)
	if start.Format(time.DateOnly) < anchor.In(zone).Format(time.DateOnly) {
		return out, invalid("from", "Планировать можно с сегодняшнего дня")
	}
	if anchor.After(start) {
		start = anchor
	}
	for _, v := range in.Breaks {
		if v.Start.Before(start) || v.End.After(end) {
			return out, invalid("breaks", "Перерывы должны находиться в будущем внутри диапазона")
		}
	}
	out.InputVersions = append(out.InputVersions, InputVersion{"settings", owner, settingsVersion})
	options := map[string]TaskOption{}
	ids := []string{}
	for _, q := range in.Tasks {
		options[q.ID] = q
		ids = append(ids, q.ID)
	}
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.version,q.payload->>'title',COALESCE((q.payload->>'estimated_minutes')::int,0),q.payload->>'priority',q.created_at,q.deadline,q.status,q.archived_at IS NOT NULL,q.planned_date::text,EXISTS(SELECT 1 FROM quest_dependencies d JOIN quests parent ON parent.owner_id=d.owner_id AND parent.id=d.depends_on WHERE d.owner_id=q.owner_id AND d.quest_id=q.id AND parent.status<>'completed'),EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id),EXISTS(SELECT 1 FROM calendar_events ev WHERE ev.owner_id=q.owner_id AND ev.quest_id=q.id AND NOT ev.cancelled) FROM quests q WHERE q.owner_id=$1 AND q.id::text=ANY($2::text[]) ORDER BY q.id`, owner, ids)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var q ScheduleTask
		var status string
		var archived, blocked, container, hasBlock bool
		var planned *string
		if e = rows.Scan(&q.ID, &q.Version, &q.Title, &q.Minutes, &q.Priority, &q.CreatedAt, &q.Deadline, &status, &archived, &planned, &blocked, &container, &hasBlock); e != nil {
			rows.Close()
			return out, e
		}
		q.AllowSplit = options[q.ID].AllowSplit
		q.FiveMinuteStart = options[q.ID].FiveMinuteStart
		switch {
		case archived || status == "completed" || status == "failed" || status == "skipped":
			q.ExcludedReason = "history"
		case blocked:
			q.ExcludedReason = "blocked"
		case container:
			q.ExcludedReason = "container"
		case hasBlock:
			q.ExcludedReason = "existing_block"
		case planned != nil:
			q.ExcludedReason = "already_planned"
		}
		out.Tasks = append(out.Tasks, q)
		out.InputVersions = append(out.InputVersions, InputVersion{"task", q.ID, q.Version})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Tasks) != len(ids) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Одна из задач недоступна")
	}
	// Dependencies are part of the input contract even if their target task did not change.
	rows, e = tx.Query(ctx, `SELECT d.quest_id::text,p.id::text,p.version FROM quest_dependencies d JOIN quests p ON p.owner_id=d.owner_id AND p.id=d.depends_on WHERE d.owner_id=$1 AND d.quest_id::text=ANY($2::text[]) ORDER BY d.quest_id,p.id`, owner, ids)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var q, id string
		var version int64
		if e = rows.Scan(&q, &id, &version); e != nil {
			rows.Close()
			return out, e
		}
		out.InputVersions = append(out.InputVersions, InputVersion{"dependency", q + "/" + id, version})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	free := []Interval{}
	for d := a; d.Before(b); d = d.AddDate(0, 0, 1) {
		date := d.Format(time.DateOnly)
		p, e := s.get(ctx, tx, owner, date)
		if e != nil {
			return out, e
		}
		if p.Timezone != out.Timezone {
			return out, privateapi.Problem(409, "PLAN_TIMEZONE_CHANGED", "План дня создан в другом часовом поясе. Сначала пересмотрите его")
		}
		blocks, e := s.calendar.DayCommitmentsInTx(ctx, tx, owner, date, p.Timezone)
		if e != nil {
			return out, e
		}
		out.Days = append(out.Days, ScheduleDay{date, p.Version, p.Input, p.Timezone, blocks})
		out.InputVersions = append(out.InputVersions, InputVersion{"plan", date, p.Version})
		for _, v := range blocks {
			out.InputVersions = append(out.InputVersions, InputVersion{"calendar", v.ID, v.Version})
		}
		slots, e := s.FreeWorkInTx(ctx, tx, owner, date, start, end)
		if e != nil {
			return out, e
		}
		free = append(free, slots...)
	}
	for i, v := range in.Breaks {
		for _, other := range in.Breaks[i+1:] {
			if _, ok := intersection(v, other); ok {
				return out, invalid("breaks", "Перерывы не должны пересекаться")
			}
		}
		for _, d := range out.Days {
			for _, block := range d.Blocks {
				var start, end time.Time
				if block.AllDay {
					start, _ = time.ParseInLocation(time.DateOnly, block.Start, zone)
					end, _ = time.ParseInLocation(time.DateOnly, block.End, zone)
				} else {
					start, _ = time.Parse(time.RFC3339, block.Start)
					end, _ = time.Parse(time.RFC3339, block.End)
				}
				if _, ok := intersection(v, Interval{start, end}); ok {
					return out, invalid("breaks", "Новый перерыв пересекает фиксированное событие")
				}
			}
		}
	}
	free = subtract(free, in.Breaks)
	out.Suggestion = SuggestSlots(out.Tasks, free, in.BetweenMinutes)
	sort.Slice(out.InputVersions, func(i, j int) bool {
		a, b := out.InputVersions[i], out.InputVersions[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Version < b.Version
	})
	raw, e := json.Marshal(struct {
		Input    ScheduleInput
		Timezone string
		Tasks    []ScheduleTask
		Days     []ScheduleDay
		Versions []InputVersion
	}{in, out.Timezone, out.Tasks, out.Days, out.InputVersions})
	if e != nil {
		return out, e
	}
	hash := sha256.Sum256(raw)
	out.Hash = hex.EncodeToString(hash[:])
	return out, nil
}
func (s *Service) schedulePreview(ctx context.Context, tx pgx.Tx, owner string, in ScheduleInput) (SchedulePreview, error) {
	out, e := s.scheduleSnapshot(ctx, tx, owner, in, s.clock.Now())
	if e != nil {
		return out, e
	}
	raw, e := json.Marshal(out)
	if e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO smart_schedule_previews(owner_id,snapshot,expires_at,created_at) VALUES($1,$2,$3,$4) RETURNING id::text`, owner, raw, out.ExpiresAt, out.CreatedAt).Scan(&out.ID)
	return out, e
}
func (s *Service) applySchedule(r *http.Request, tx pgx.Tx, owner string, in ScheduleApply) (ScheduleResult, error) {
	ctx := r.Context()
	out := ScheduleResult{OperationID: in.OperationID, PreviewID: in.PreviewID, Blocks: []calendar.Event{}, Plans: []Plan{}}
	if !uuid(in.OperationID) || !uuid(in.PreviewID) || !in.Confirm {
		return out, privateapi.Invalid("Просмотрите предложение и подтвердите сохранение")
	}
	request, e := json.Marshal(in)
	if e != nil {
		return out, e
	}
	var saved []byte
	var same bool
	e = tx.QueryRow(ctx, `SELECT request=$3::jsonb,result FROM smart_schedule_applies WHERE owner_id=$1 AND operation_id=$2`, owner, in.OperationID, request).Scan(&same, &saved)
	if e == nil {
		if !same {
			return out, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Ключ использован для другого предложения")
		}
		e = json.Unmarshal(saved, &out)
		return out, e
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	var raw []byte
	var expires time.Time
	e = tx.QueryRow(ctx, `SELECT snapshot,expires_at FROM smart_schedule_previews WHERE owner_id=$1 AND id=$2`, owner, in.PreviewID).Scan(&raw, &expires)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Предложение не найдено")
	}
	if e != nil {
		return out, e
	}
	var consumed bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM smart_schedule_applies WHERE owner_id=$1 AND preview_id=$2)`, owner, in.PreviewID).Scan(&consumed); e != nil {
		return out, e
	}
	if consumed {
		return out, privateapi.Problem(409, "PREVIEW_USED", "Предложение уже применено")
	}
	if !s.clock.Now().Before(expires) {
		return out, privateapi.Problem(409, "PREVIEW_EXPIRED", "Срок предложения истёк. Подготовьте новое")
	}
	var preview SchedulePreview
	if e = json.Unmarshal(raw, &preview); e != nil {
		return out, e
	}
	fresh, e := s.scheduleSnapshot(ctx, tx, owner, preview.Input, preview.CreatedAt)
	if e != nil {
		return out, e
	}
	if fresh.Hash != preview.Hash {
		return out, privateapi.Problem(409, "PREVIEW_STALE", "Задачи, планы или занятость изменились. Подготовьте новое предложение")
	}
	if len(preview.Placements) == 0 {
		return out, privateapi.Problem(409, "NO_PLACEMENTS", "Нет подходящих блоков для сохранения")
	}
	now := s.clock.Now()
	for _, p := range preview.Placements {
		if p.Start.Before(now) {
			return out, privateapi.Problem(409, "PREVIEW_STALE", "Начало блока уже прошло. Подготовьте новое предложение")
		}
	}
	zone, e := time.LoadLocation(preview.Timezone)
	if e != nil {
		return out, e
	}
	versions := map[string]int64{}
	plans := map[string][]string{}
	for _, p := range preview.Placements {
		date := p.Start.In(zone).Format(time.DateOnly)
		if _, ok := versions[p.TaskID]; !ok {
			q, e := s.quests.ScheduleInTx(ctx, tx, owner, p.TaskID, p.TaskVersion, date, "Подтверждено предложение расписания")
			if e != nil {
				return out, e
			}
			versions[p.TaskID] = q.Version
		}
		ev, e := s.calendar.CreateInTx(ctx, tx, owner, calendar.Input{Title: p.Title, QuestID: &p.TaskID, Start: p.Start.Format(time.RFC3339), End: p.End.Format(time.RFC3339), Timezone: preview.Timezone})
		if e != nil {
			return out, e
		}
		out.Blocks = append(out.Blocks, ev)
		if p.BreakEnd.After(p.End) {
			ev, e = s.calendar.CreateInTx(ctx, tx, owner, calendar.Input{Title: "Перерыв", Start: p.End.Format(time.RFC3339), End: p.BreakEnd.Format(time.RFC3339), Timezone: preview.Timezone})
			if e != nil {
				return out, e
			}
			out.Blocks = append(out.Blocks, ev)
		}
		plans[date] = append(plans[date], p.TaskID)
	}
	// Explicit breaks are saved with the proposal so future scheduling also sees them.
	for _, b := range preview.Input.Breaks {
		ev, e := s.calendar.CreateInTx(ctx, tx, owner, calendar.Input{Title: "Перерыв", Start: b.Start.Format(time.RFC3339), End: b.End.Format(time.RFC3339), Timezone: preview.Timezone})
		if e != nil {
			return out, e
		}
		out.Blocks = append(out.Blocks, ev)
	}
	for _, d := range preview.Days {
		ids := plans[d.Date]
		if len(ids) == 0 {
			continue
		}
		input := d.Input
		seen := map[string]bool{}
		for _, id := range input.QuestIDs {
			seen[id] = true
		}
		for _, id := range ids {
			if !seen[id] {
				input.QuestIDs = append(input.QuestIDs, id)
				seen[id] = true
			}
		}
		p, e := s.MorningInTx(r, tx, owner, d.Date, input, d.PlanVersion, "Подтверждено предложение расписания")
		if e != nil {
			return out, e
		}
		out.Plans = append(out.Plans, p)
	}
	out.AppliedAt = now
	saved, e = json.Marshal(out)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO smart_schedule_applies(owner_id,operation_id,preview_id,request,result,created_at) VALUES($1,$2,$3,$4,$5,$6)`, owner, in.OperationID, in.PreviewID, request, saved, now)
	if e != nil {
		return out, e
	}
	e = jobs.OwnerChanged(ctx, tx, owner, s.clock)
	return out, e
}
func (s *Service) mountSchedule(r chi.Router) {
	api := privateapi.New(s.pool, s.clock)
	r.Post("/api/v1/plans/schedule/preview", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(api, w, r, func(in ScheduleInput) (any, error) {
			return api.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.schedulePreview(ctx, tx, owner, in)
			})
		})
	})
	r.Post("/api/v1/plans/schedule/apply", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(api, w, r, func(in ScheduleApply) (any, error) {
			return api.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.applySchedule(r, tx, owner, in)
			})
		})
	})
}
