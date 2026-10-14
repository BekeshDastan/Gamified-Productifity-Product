package analytics

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

const ProjectionVersion = "analytics-v1"

type Service struct{ *privateapi.Service }

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{privateapi.New(pool, clock)}
}

type Range struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Source struct {
	Date    string    `json:"date"`
	Kind    string    `json:"kind"`
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	At      time.Time `json:"at"`
	Value   float64   `json:"value"`
	Unit    string    `json:"unit"`
	AreaID  *string   `json:"area_id"`
	Version int64     `json:"version"`
}
type Day struct {
	TimeMetrics
	Scores Scores `json:"scores"`
	Outcomes
	Date         string   `json:"date"`
	Completed    int      `json:"completed"`
	FocusSeconds int64    `json:"focus_seconds"`
	XPDelta      int64    `json:"xp_delta"`
	CoinsDelta   int64    `json:"coins_delta"`
	Sources      []Source `json:"sources"`
}
type Week struct {
	TimeTotals
	OutcomeTotals
	Start        string `json:"start"`
	End          string `json:"end"`
	Days         int    `json:"days"`
	Completed    int    `json:"completed"`
	FocusSeconds int64  `json:"focus_seconds"`
	XPDelta      int64  `json:"xp_delta"`
	CoinsDelta   int64  `json:"coins_delta"`
}
type View struct {
	Definitions []Definition `json:"definitions"`
	Range
	Timezone          string     `json:"timezone"`
	ProjectionVersion string     `json:"projection_version"`
	SourceRevision    int64      `json:"source_revision"`
	ProjectedRevision int64      `json:"projected_revision"`
	ComputedAt        *time.Time `json:"computed_at"`
	Stale             bool       `json:"stale"`
	Days              []Day      `json:"days"`
	Weeks             []Week     `json:"weeks"`
}
type projectionState struct {
	Revision, Projected int64
	Timezone            string
	From, To            *string
	At                  *time.Time
}

func validateRange(in Range) (time.Time, time.Time, error) {
	return validateRangeLimit(in, 366)
}
func validateRangeLimit(in Range, limit int) (time.Time, time.Time, error) {
	a, e := time.Parse(time.DateOnly, in.From)
	if e != nil || privateapi.Day(in.From) != nil {
		return a, a, privateapi.Invalid("Укажите начало диапазона")
	}
	b, e := time.Parse(time.DateOnly, in.To)
	if e != nil || privateapi.Day(in.To) != nil || !b.After(a) || b.Sub(a) > time.Duration(limit)*24*time.Hour {
		return a, b, privateapi.Invalid("Диапазон: от 1 до 366 дней; конечная дата не включается")
	}
	return a, b, nil
}
func (s *Service) state(ctx context.Context, tx pgx.Tx, owner string) (projectionState, error) {
	v := projectionState{}
	if _, e := tx.Exec(ctx, `INSERT INTO analytics_projection_state(owner_id) VALUES($1) ON CONFLICT DO NOTHING`, owner); e != nil {
		return v, e
	}
	e := tx.QueryRow(ctx, `SELECT source_revision,projected_revision,timezone,requested_from::text,requested_to::text,computed_at FROM analytics_projection_state WHERE owner_id=$1`, owner).Scan(&v.Revision, &v.Projected, &v.Timezone, &v.From, &v.To, &v.At)
	return v, e
}
func (s *Service) read(ctx context.Context, tx pgx.Tx, owner string, in Range, rebuild bool, enqueue ...bool) (View, error) {
	if _, _, e := validateRange(in); e != nil {
		return View{}, e
	}
	state, e := s.state(ctx, tx, owner)
	if e != nil {
		return View{}, e
	}
	var timezone string
	if e = tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC') FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&timezone); e != nil {
		return View{}, e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return View{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE analytics_projection_state SET requested_from=$2::text::date,requested_to=$3::text::date WHERE owner_id=$1`, owner, in.From, in.To); e != nil {
		return View{}, e
	}
	out := View{Range: in, Definitions: definitions(), Timezone: timezone, ProjectionVersion: ProjectionVersion, SourceRevision: state.Revision, ProjectedRevision: state.Projected, ComputedAt: state.At, Days: []Day{}, Weeks: []Week{}}
	rows, e := tx.Query(ctx, `SELECT payload,source_revision,computed_at,projection_version FROM daily_metrics WHERE owner_id=$1 AND timezone=$4 AND local_date>=$2::text::date AND local_date<$3::text::date ORDER BY local_date`, owner, in.From, in.To, timezone)
	if e != nil {
		return out, e
	}
	out.ProjectedRevision = state.Revision
	versionMismatch := false
	for rows.Next() {
		var b []byte
		var rev int64
		var at time.Time
		var projectionVersion string
		if e = rows.Scan(&b, &rev, &at, &projectionVersion); e != nil {
			rows.Close()
			return out, e
		}
		var d Day
		if e = json.Unmarshal(b, &d); e != nil {
			rows.Close()
			return out, e
		}
		out.Days = append(out.Days, d)
		if !d.HabitsComplete {
			versionMismatch = true
		}
		versionMismatch = versionMismatch || projectionVersion != ProjectionVersion
		out.ProjectedRevision = min(out.ProjectedRevision, rev)
		if out.ComputedAt == nil || at.Before(*out.ComputedAt) {
			out.ComputedAt = &at
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	a, b, _ := validateRange(in)
	missing := len(out.Days) != int(b.Sub(a)/(24*time.Hour))
	out.Stale = versionMismatch || out.ProjectedRevision != state.Revision || out.ComputedAt == nil || out.ComputedAt.In(zone).Format(time.DateOnly) != s.Clock.Now().In(zone).Format(time.DateOnly)
	if rebuild || missing {
		return s.rebuild(ctx, tx, owner, in, timezone, state.Revision)
	}
	out.Weeks = aggregateWeeks(out.Days)
	if out.Stale && (len(enqueue) == 0 || enqueue[0]) {
		if _, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,owner_id,kind,payload,occurred_at) VALUES(gen_random_uuid(),$1,'analytics.changed','{}',$2)`, owner, s.Clock.Now()); e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *Service) rebuild(ctx context.Context, tx pgx.Tx, owner string, in Range, timezone string, revision int64) (View, error) {
	days, e := s.collect(ctx, tx, owner, in, timezone)
	if e != nil {
		return View{}, e
	}
	now := s.Clock.Now()
	out := View{Range: in, Definitions: definitions(), Timezone: timezone, ProjectionVersion: ProjectionVersion, SourceRevision: revision, ProjectedRevision: revision, ComputedAt: &now, Days: days, Weeks: aggregateWeeks(days)}
	for _, d := range days {
		out.Stale = out.Stale || !d.HabitsComplete
	}
	for _, d := range days {
		b, e := json.Marshal(d)
		if e != nil {
			return out, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO daily_metrics(owner_id,local_date,timezone,source_revision,projection_version,computed_at,payload) VALUES($1,$2::text::date,$3,$4,$5,$6,$7) ON CONFLICT(owner_id,local_date) DO UPDATE SET timezone=excluded.timezone,source_revision=excluded.source_revision,projection_version=excluded.projection_version,computed_at=excluded.computed_at,payload=excluded.payload`, owner, d.Date, timezone, revision, ProjectionVersion, now, b); e != nil {
			return out, e
		}
	}
	for _, w := range out.Weeks {
		b, e := json.Marshal(w)
		if e != nil {
			return out, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO weekly_metrics(owner_id,week_start,timezone,source_revision,projection_version,computed_at,payload) VALUES($1,$2::text::date,$3,$4,$5,$6,$7) ON CONFLICT(owner_id,week_start) DO UPDATE SET timezone=excluded.timezone,source_revision=excluded.source_revision,projection_version=excluded.projection_version,computed_at=excluded.computed_at,payload=excluded.payload`, owner, w.Start, timezone, revision, ProjectionVersion, now, b); e != nil {
			return out, e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE analytics_projection_state SET projected_revision=$2,timezone=$3,computed_at=$4 WHERE owner_id=$1`, owner, revision, timezone, now)
	if e == nil && out.Stale {
		_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,owner_id,kind,payload,occurred_at) VALUES(gen_random_uuid(),$1,'analytics.changed','{}',$2)`, owner, now)
	}
	return out, e
}
func aggregateWeeks(days []Day) []Week {
	out := []Week{}
	postponed := map[string]map[string]bool{}
	for _, d := range days {
		date, _ := time.Parse(time.DateOnly, d.Date)
		offset := (int(date.Weekday()) + 6) % 7
		start := date.AddDate(0, 0, -offset).Format(time.DateOnly)
		if len(out) == 0 || out[len(out)-1].Start != start {
			end := date.AddDate(0, 0, 7-offset).Format(time.DateOnly)
			out = append(out, Week{Start: start, End: end})
		}
		w := &out[len(out)-1]
		w.OutcomeTotals.add(d.OutcomeTotals)
		w.TimeTotals.add(d.TimeTotals)
		if postponed[start] == nil {
			postponed[start] = map[string]bool{}
		}
		for _, source := range d.Sources {
			if source.Kind == "postponement" {
				postponed[start][source.ID] = true
			}
		}
		w.Postponed = len(postponed[start])
		w.Days++
		w.Completed += d.Completed
		w.FocusSeconds += d.FocusSeconds
		w.XPDelta += d.XPDelta
		w.CoinsDelta += d.CoinsDelta
	}
	return out
}
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{"analytics.changed": func(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
		var body map[string]json.RawMessage
		if j.OwnerID == nil || !privateapi.UUID(*j.OwnerID) || json.Unmarshal(j.Payload, &body) != nil || body == nil || len(body) != 0 {
			return jobs.ErrPermanent
		}
		owner := *j.OwnerID
		var locked string
		if e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&locked); e != nil {
			return e
		}
		state, e := s.state(ctx, tx, owner)
		if e != nil {
			return e
		}
		if state.From == nil {
			return nil
		}
		in := Range{*state.From, *state.To}
		view, e := s.read(ctx, tx, owner, in, false, false)
		if e != nil {
			return e
		}
		if view.Stale {
			_, e = s.read(ctx, tx, owner, in, true, false)
		}
		return e
	}}
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountTime(r)
		r.Get("/api/v1/analytics", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.read(ctx, tx, owner, Range{r.URL.Query().Get("from"), r.URL.Query().Get("to")}, false)
			})
			s.Reply(w, r, out, e)
		})
		r.Post("/api/v1/analytics/rebuild", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in Range) (any, error) {
				return s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					return s.read(ctx, tx, owner, in, true)
				})
			})
		})
	})
}

// RebuildOwner refreshes a range without trusting existing counters.
func (s *Service) RebuildOwner(ctx context.Context, owner string, in Range) (View, error) {
	if !privateapi.UUID(owner) {
		return View{}, privateapi.Invalid("Некорректный владелец")
	}
	var out View
	e := storage.WithinTx(ctx, s.Pool, func(tx pgx.Tx) error {
		var id string
		if e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&id); e != nil {
			return e
		}
		var e error
		out, e = s.read(ctx, tx, owner, in, true)
		return e
	})
	return out, e
}
