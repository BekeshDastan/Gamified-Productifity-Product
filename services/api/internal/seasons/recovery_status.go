package seasons

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func (s *Service) WithHabits(h *habits.Service) *Service { s.habits = h; return s }

type RecoveryHabit struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	URL           string          `json:"url"`
	Stats         habits.Stats    `json:"stats"`
	Periods       []habits.Period `json:"periods"`
	CurrentAmount *float64        `json:"current_amount"`
	FullTarget    *float64        `json:"full_target"`
	Minimum       *float64        `json:"minimum"`
	MinimumMet    *bool           `json:"minimum_met"`
}
type ComebackProgress struct {
	OfferID string            `json:"offer_id"`
	GoalID  *string           `json:"goal_id"`
	RunID   string            `json:"run_id"`
	URL     string            `json:"url"`
	Member  challenges.Member `json:"member"`
}
type RecoveryStatus struct {
	ComputedAt time.Time          `json:"computed_at"`
	Complete   bool               `json:"complete"`
	Habits     []RecoveryHabit    `json:"habits"`
	Comebacks  []ComebackProgress `json:"comebacks"`
}

func (s *Service) recoveryStatus(ctx context.Context, tx pgx.Tx, owner string) (RecoveryStatus, error) {
	out := RecoveryStatus{ComputedAt: s.Clock.Now(), Habits: []RecoveryHabit{}, Comebacks: []ComebackProgress{}}
	evidence, complete, e := s.habits.EvidenceInTx(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	out.Complete = complete
	for _, h := range evidence {
		v := RecoveryHabit{ID: h.ID, Title: h.Title, URL: "/app/habits/" + h.ID, Stats: h.Stats, Periods: h.Periods}
		for _, p := range h.Periods {
			if !p.Excluded && !s.Clock.Now().Before(p.Start) && s.Clock.Now().Before(p.End) {
				var minimum float64
				if e = tx.QueryRow(ctx, `SELECT (payload->>'minimum')::float8 FROM habits WHERE owner_id=$1 AND id=$2`, owner, h.ID).Scan(&minimum); e != nil {
					return out, e
				}
				amount, target := p.Amount, p.Target
				met := amount >= minimum
				v.CurrentAmount = &amount
				v.FullTarget = &target
				v.Minimum = &minimum
				v.MinimumMet = &met
			}
		}
		out.Habits = append(out.Habits, v)
	}
	rows, e := tx.Query(ctx, `SELECT id::text,result FROM recovery_offers WHERE owner_id=$1 AND state='accepted' ORDER BY decided_at DESC,id DESC LIMIT 25`, owner)
	if e != nil {
		return out, e
	}
	offers := []struct {
		id     string
		result RecoveryResult
	}{}
	for rows.Next() {
		var id string
		var b []byte
		if e = rows.Scan(&id, &b); e != nil {
			rows.Close()
			return out, e
		}
		var result RecoveryResult
		if e = json.Unmarshal(b, &result); e != nil {
			rows.Close()
			return out, e
		}
		offers = append(offers, struct {
			id     string
			result RecoveryResult
		}{id, result})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, v := range offers {
		if v.result.RunID == nil {
			continue
		}
		m, e := s.challenges.ProgressInTx(ctx, tx, owner, *v.result.RunID)
		if e != nil {
			return out, e
		}
		out.Comebacks = append(out.Comebacks, ComebackProgress{v.id, v.result.GoalID, *v.result.RunID, "/app/challenges?run_id=" + *v.result.RunID, m})
	}
	return out, nil
}
func (s *Service) mountRecoveryStatus(r chi.Router) {
	if s.habits == nil {
		return
	}
	r.Get("/api/v1/recovery/status", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.recoveryStatus(ctx, tx, owner)
		})
		s.Reply(w, r, out, e)
	})
}
