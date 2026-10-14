package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type ReportSnapshot struct {
	ReportInput
	Range
	PreviousRange    Range              `json:"previous_range"`
	Version          string             `json:"version"`
	ScoreVersion     string             `json:"score_version"`
	SourceRevision   int64              `json:"source_revision"`
	ComputedAt       time.Time          `json:"computed_at"`
	ObservedFrom     time.Time          `json:"observed_from"`
	CurrentObserved  bool               `json:"current_observed"`
	PreviousObserved bool               `json:"previous_observed"`
	Comparisons      []Comparison       `json:"comparisons"`
	Obligations      ReportObligations  `json:"obligations"`
	FrequentReason   *ReportReason      `json:"frequent_reason"`
	NextStep         string             `json:"next_step"`
	Text             string             `json:"text"`
	Days             []Day              `json:"days"`
	PreviousDays     []Day              `json:"previous_days"`
	Balance          BalanceView        `json:"balance"`
	Records          []character.Record `json:"records"`
}
type ReportPreview struct {
	ID        string         `json:"id"`
	ExpiresAt time.Time      `json:"expires_at"`
	Snapshot  ReportSnapshot `json:"snapshot"`
}
type Report struct {
	ID        string         `json:"id"`
	Revision  int64          `json:"revision"`
	Automatic bool           `json:"automatic"`
	CreatedAt time.Time      `json:"created_at"`
	Snapshot  ReportSnapshot `json:"snapshot"`
}
type SaveReportInput struct {
	OperationID string `json:"operation_id"`
	PreviewID   string `json:"preview_id"`
	Confirm     bool   `json:"confirm"`
}
type ReportPage struct {
	Items []ReportBrief `json:"items"`
	Next  string        `json:"next"`
}
type ReportBrief struct {
	Range
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Timezone  string    `json:"timezone"`
	Revision  int64     `json:"revision"`
	Automatic bool      `json:"automatic"`
	CreatedAt time.Time `json:"created_at"`
	Text      string    `json:"text"`
}

func (s *Service) reportSnapshot(ctx context.Context, tx pgx.Tx, owner string, in ReportInput) (ReportSnapshot, error) {
	out := ReportSnapshot{ReportInput: in, Version: ReportVersion, ScoreVersion: ScoreVersion, Records: []character.Record{}}
	current, previous, e := reportRanges(in)
	if e != nil {
		return out, e
	}
	out.Range = current
	out.PreviousRange = previous
	var timezone string
	if e = tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC') FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&timezone); e != nil {
		return out, e
	}
	if in.Timezone != timezone {
		return out, privateapi.Problem(409, "REPORT_CONTEXT_CHANGED", "Часовой пояс изменился. Обновите условия отчёта")
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return out, e
	}
	end, _ := time.ParseInLocation(time.DateOnly, current.To, zone)
	now := s.Clock.Now()
	if now.Before(end) {
		return out, privateapi.Problem(409, "PERIOD_NOT_CLOSED", "Период ещё не завершён")
	}
	state, e := s.state(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	out.SourceRevision = state.Revision
	out.ComputedAt = now
	if e = tx.QueryRow(ctx, `SELECT created_at FROM users WHERE id=$1`, owner).Scan(&out.ObservedFrom); e != nil {
		return out, e
	}
	start, _ := time.ParseInLocation(time.DateOnly, current.From, zone)
	prevStart, _ := time.ParseInLocation(time.DateOnly, previous.From, zone)
	out.CurrentObserved = !start.Before(out.ObservedFrom)
	out.PreviousObserved = !prevStart.Before(out.ObservedFrom)
	out.Days, e = s.collect(ctx, tx, owner, current, timezone)
	if e != nil {
		return out, e
	}
	out.PreviousDays, e = s.collect(ctx, tx, owner, previous, timezone)
	if e != nil {
		return out, e
	}
	for _, d := range out.Days {
		if !d.HabitsComplete {
			return out, privateapi.Problem(409, "PROJECTION_PENDING", "История привычек ещё пересчитывается. Повторите предпросмотр")
		}
		if !d.Scores.Closed && (d.HabitObligations > 0 || (d.Plan != nil && d.Plan.Confirmed)) {
			return out, privateapi.Problem(409, "PERIOD_NOT_CLOSED", "Обязательства в исходном часовом поясе ещё не завершены")
		}
	}
	out.Comparisons = reportComparisons(out.Days, out.PreviousDays, out.CurrentObserved, out.PreviousObserved)
	out.Obligations = reportObligations(out.Days)
	out.FrequentReason, e = reportReason(ctx, tx, owner, current, zone)
	if e != nil {
		return out, e
	}
	out.NextStep = "Выберите приоритет следующего периода."
	out.Text = reportText(in.Kind, current, previous, out.Comparisons, out.CurrentObserved, out.PreviousObserved)
	out.Text += reportPlanText(out.Obligations, out.FrequentReason) + " " + out.NextStep
	at := out.ComputedAt
	view := View{Range: current, Timezone: timezone, ComputedAt: &at, SourceRevision: state.Revision, ProjectedRevision: state.Revision, Days: out.Days}
	out.Balance, e = s.balanceFrom(ctx, tx, owner, view)
	if e != nil {
		return out, e
	}
	if s.records != nil {
		out.Records, e = s.records.RecordsInTx(ctx, tx, owner)
	}
	return out, e
}
func (s *Service) previewReport(ctx context.Context, tx pgx.Tx, owner string, in ReportInput) (ReportPreview, error) {
	v, e := s.reportSnapshot(ctx, tx, owner, in)
	if e != nil {
		return ReportPreview{}, e
	}
	out := ReportPreview{Snapshot: v, ExpiresAt: s.Clock.Now().Add(10 * time.Minute)}
	b, e := json.Marshal(v)
	if e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO report_previews(owner_id,snapshot,expires_at,created_at) VALUES($1,$2,$3,$4) RETURNING id::text`, owner, b, out.ExpiresAt, s.Clock.Now()).Scan(&out.ID)
	return out, e
}
func scanReport(row pgx.Row) (Report, error) {
	var out Report
	var b []byte
	e := row.Scan(&out.ID, &out.Revision, &out.Automatic, &out.CreatedAt, &b)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Отчёт не найден")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(b, &out.Snapshot)
	return out, e
}

const reportColumns = "id::text,revision,automatic,created_at,snapshot"

func (s *Service) getReport(ctx context.Context, tx pgx.Tx, owner, id string) (Report, error) {
	if !privateapi.UUID(id) {
		return Report{}, privateapi.Problem(404, "NOT_FOUND", "Отчёт не найден")
	}
	return scanReport(tx.QueryRow(ctx, `SELECT `+reportColumns+` FROM report_snapshots WHERE owner_id=$1 AND id=$2`, owner, id))
}
func (s *Service) saveReport(ctx context.Context, tx pgx.Tx, owner string, in SaveReportInput) (Report, error) {
	out := Report{}
	if !privateapi.UUID(in.OperationID) || !privateapi.UUID(in.PreviewID) {
		return out, privateapi.Invalid("Проверьте идентификаторы отчёта")
	}
	request, e := json.Marshal(in)
	if e != nil {
		return out, e
	}
	var same bool
	e = tx.QueryRow(ctx, `SELECT request=$3::jsonb FROM report_snapshots WHERE owner_id=$1 AND id=$2`, owner, in.OperationID, request).Scan(&same)
	if e == nil {
		if !same {
			return out, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Этот отчёт уже сохранён с другим запросом")
		}
		return s.getReport(ctx, tx, owner, in.OperationID)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if !in.Confirm {
		return out, privateapi.Invalid("Подтвердите сохранение снимка")
	}
	var snapshot []byte
	var expires time.Time
	e = tx.QueryRow(ctx, `SELECT snapshot,expires_at FROM report_previews WHERE owner_id=$1 AND id=$2`, owner, in.PreviewID).Scan(&snapshot, &expires)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, privateapi.Problem(404, "NOT_FOUND", "Предпросмотр не найден")
	}
	if e != nil {
		return out, e
	}
	var v ReportSnapshot
	if e = json.Unmarshal(snapshot, &v); e != nil {
		return out, e
	}
	if !s.Clock.Now().Before(expires) {
		return out, privateapi.Problem(409, "PREVIEW_EXPIRED", "Предпросмотр устарел. Создайте новый")
	}
	state, e := s.state(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	var timezone string
	if e = tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC') FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&timezone); e != nil {
		return out, e
	}
	if state.Revision != v.SourceRevision || timezone != v.Timezone {
		return out, privateapi.Problem(409, "SOURCE_CHANGED", "Записи или часовой пояс изменились. Обновите предпросмотр")
	}
	var used bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM report_snapshots WHERE owner_id=$1 AND preview_id=$2)`, owner, in.PreviewID).Scan(&used); e != nil {
		return out, e
	}
	if used {
		return out, privateapi.Problem(409, "PREVIEW_USED", "Этот снимок уже сохранён. Откройте историю отчётов")
	}
	e = tx.QueryRow(ctx, `INSERT INTO report_snapshots(owner_id,id,preview_id,request,kind,starts_on,ends_before,timezone,revision,automatic,snapshot,created_at) SELECT $1,$2,$3,$4,$5,$6::text::date,$7::text::date,$8,COALESCE(max(revision),0)+1,false,$9,$10 FROM report_snapshots WHERE owner_id=$1 AND kind=$5 AND starts_on=$6::text::date AND timezone=$8 RETURNING `+reportColumns, owner, in.OperationID, in.PreviewID, request, v.Kind, v.From, v.To, v.Timezone, snapshot, s.Clock.Now()).Scan(&out.ID, &out.Revision, &out.Automatic, &out.CreatedAt, &snapshot)
	if e != nil {
		return out, e
	}
	out.Snapshot = v
	if s.reportObserver != nil {
		if e = s.reportObserver(ctx, tx, owner, out.ID, out.Revision); e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *Service) listReports(r *http.Request, tx pgx.Tx, owner string) (ReportPage, error) {
	out := ReportPage{Items: []ReportBrief{}}
	var at *time.Time
	var id *string
	if before := r.URL.Query().Get("before"); before != "" {
		v, e := s.getReport(r.Context(), tx, owner, before)
		if e != nil {
			return out, e
		}
		at = &v.CreatedAt
		id = &v.ID
	}
	rows, e := tx.Query(r.Context(), `SELECT id::text,revision,automatic,created_at,kind,starts_on::text,ends_before::text,timezone,snapshot->>'text' FROM report_snapshots WHERE owner_id=$1 AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3::uuid)) ORDER BY created_at DESC,id DESC LIMIT 26`, owner, at, id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v ReportBrief
		if e := rows.Scan(&v.ID, &v.Revision, &v.Automatic, &v.CreatedAt, &v.Kind, &v.From, &v.To, &v.Timezone, &v.Text); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		out.Next = out.Items[24].ID
	}
	return out, rows.Err()
}
func (s *Service) mountReports(r chi.Router) {
	r.Post("/api/v1/analytics/reports/preview", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in ReportInput) (any, error) {
			return s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.previewReport(ctx, tx, owner, in)
			})
		})
	})
	r.Post("/api/v1/analytics/reports", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in SaveReportInput) (any, error) {
			return s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.saveReport(ctx, tx, owner, in)
			})
		})
	})
	r.Get("/api/v1/analytics/reports", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.listReports(r, tx, owner) })
		s.Reply(w, r, out, e)
	})
	r.Get("/api/v1/analytics/reports/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.getReport(ctx, tx, owner, chi.URLParam(r, "id"))
		})
		s.Reply(w, r, out, e)
	})
}
