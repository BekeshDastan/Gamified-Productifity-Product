package journal

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Run struct {
	ID        string          `json:"id"`
	Date      string          `json:"date"`
	Kind      string          `json:"kind"`
	Version   int64           `json:"version"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type Morning struct {
	Run  *Run          `json:"run"`
	Plan planning.Plan `json:"plan"`
}
type MorningInput struct {
	TemplateVersion int64          `json:"template_version"`
	Checked         []string       `json:"checked"`
	Input           planning.Input `json:"input"`
	PlanVersion     int64          `json:"plan_version"`
	Reason          string         `json:"reason"`
}

func (s *Service) WithPlanning(p *planning.Service) *Service { s.plans = p; return s }
func (s *Service) run(ctx context.Context, tx pgx.Tx, owner, date, kind string) (*Run, error) {
	if e := privateapi.Day(date); e != nil {
		return nil, e
	}
	var out Run
	e := tx.QueryRow(ctx, "SELECT id::text,local_date::text,kind,version,payload,created_at,updated_at FROM routine_runs WHERE owner_id=$1 AND local_date=$2::text::date AND kind=$3", owner, date, kind).Scan(&out.ID, &out.Date, &out.Kind, &out.Version, &out.Payload, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &out, e
}
func (s *Service) saveRun(ctx context.Context, tx pgx.Tx, owner, date, kind string, payload any) (*Run, error) {
	body, e := json.Marshal(payload)
	if e != nil {
		return nil, e
	}
	now := s.Clock.Now().UTC()
	_, e = tx.Exec(ctx, `INSERT INTO routine_runs(owner_id,local_date,kind,payload,created_at,updated_at) VALUES($1,$2::text::date,$3,$4,$5,$5) ON CONFLICT(owner_id,local_date,kind) DO UPDATE SET payload=excluded.payload,version=routine_runs.version+1,updated_at=excluded.updated_at`, owner, date, kind, body, now)
	if e != nil {
		return nil, e
	}
	out, e := s.run(ctx, tx, owner, date, kind)
	if e != nil {
		return out, e
	}
	snapshot, e := json.Marshal(out)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, "INSERT INTO routine_run_events(owner_id,run_id,version,snapshot,created_at) VALUES($1,$2,$3,$4,$5)", owner, out.ID, out.Version, snapshot, now)
	return out, e
}
func (s *Service) morning(r *http.Request, tx pgx.Tx, owner, date string, in *MorningInput) (Morning, error) {
	if s.plans == nil {
		return Morning{}, privateapi.Problem(503, "UNAVAILABLE", "Планирование недоступно")
	}
	run, e := s.run(r.Context(), tx, owner, date, "morning")
	if e != nil {
		return Morning{}, e
	}
	var plan planning.Plan
	if in != nil {
		template, e := s.checkTemplate(r.Context(), tx, owner, "morning", in.TemplateVersion, in.Checked)
		if e != nil {
			return Morning{}, e
		}
		var version int64
		if run != nil {
			version = run.Version
		}
		if p := privateapi.MatchVersion(r, version); p != nil {
			return Morning{}, p
		}
		plan, e = s.plans.MorningInTx(r, tx, owner, date, in.Input, in.PlanVersion, in.Reason)
		if e != nil {
			return Morning{}, e
		}
		run, e = s.saveRun(r.Context(), tx, owner, date, "morning", struct {
			Template    RoutineTemplate `json:"template"`
			Input       MorningInput    `json:"input"`
			PlanID      string          `json:"plan_id"`
			PlanVersion int64           `json:"plan_version"`
		}{template, *in, plan.ID, plan.Version})
		if e != nil {
			return Morning{}, e
		}
	} else {
		plan, e = s.plans.ViewInTx(r.Context(), tx, owner, date)
	}
	return Morning{run, plan}, e
}
func (s *Service) mountMorning(r chi.Router) {
	r.Get("/api/v1/routine-runs/morning/{date}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.morning(r, tx, owner, chi.URLParam(r, "date"), nil)
		})
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/routine-runs/morning/{date}", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in MorningInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.morning(r, tx, owner, chi.URLParam(r, "date"), &in)
			})
		})
	})
}
