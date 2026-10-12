package profile

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Starters struct {
	Goal  string `json:"goal"`
	Task  string `json:"task"`
	Habit string `json:"habit"`
}
type OnboardingInput struct {
	Step     int      `json:"step"`
	Starters Starters `json:"starters"`
}
type Onboarding struct {
	OnboardingInput
	CompletedAt *time.Time `json:"completed_at"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

const onboardingSelect = `SELECT o.step,o.completed_at,o.version,o.created_at,o.updated_at,
 COALESCE((SELECT title FROM onboarding_starters WHERE owner_id=o.owner_id AND kind='goal'),''),
 COALESCE((SELECT title FROM onboarding_starters WHERE owner_id=o.owner_id AND kind='task'),''),
 COALESCE((SELECT title FROM onboarding_starters WHERE owner_id=o.owner_id AND kind='habit'),'') FROM onboarding o WHERE owner_id=$1`

func scanOnboarding(row pgx.Row) (Onboarding, error) {
	var value Onboarding
	err := row.Scan(&value.Step, &value.CompletedAt, &value.Version, &value.CreatedAt, &value.UpdatedAt, &value.Starters.Goal, &value.Starters.Task, &value.Starters.Habit)
	return value, err
}
func (s *Service) GetOnboarding(ctx context.Context) (Onboarding, error) {
	id, err := owner(ctx)
	if err != nil {
		return Onboarding{}, err
	}
	return scanOnboarding(s.pool.QueryRow(ctx, onboardingSelect, id))
}
func (s *Service) SaveOnboarding(r *http.Request, input OnboardingInput, complete bool) (Onboarding, error) {
	id, err := owner(r.Context())
	if err != nil {
		return Onboarding{}, err
	}
	if !complete {
		if input.Step < 0 || input.Step > 6 {
			return Onboarding{}, invalid("step", "Неизвестный шаг")
		}
		for kind, title := range map[string]*string{"goal": &input.Starters.Goal, "task": &input.Starters.Task, "habit": &input.Starters.Habit} {
			*title = strings.TrimSpace(*title)
			if !utf8.ValidString(*title) || utf8.RuneCountInString(*title) > 200 || strings.IndexFunc(*title, unicode.IsControl) >= 0 {
				return Onboarding{}, invalid("starters."+kind, "До 200 символов без управляющих символов")
			}
		}
	}
	var result Onboarding
	err = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		current, err := scanOnboarding(tx.QueryRow(r.Context(), onboardingSelect+` FOR UPDATE OF o`, id))
		if err != nil {
			return err
		}
		if complete && current.CompletedAt != nil {
			result = current
			return nil
		}
		if problem := platform.MatchVersion(r, current.Version); problem != nil {
			return problem
		}
		now := s.clock.Now().UTC()
		if complete {
			if _, err := tx.Exec(r.Context(), `UPDATE onboarding SET completed_at=$2,step=6,version=version+1,updated_at=$2 WHERE owner_id=$1`, id, now); err != nil {
				return err
			}
		} else {
			for kind, title := range map[string]string{"goal": input.Starters.Goal, "task": input.Starters.Task, "habit": input.Starters.Habit} {
				if title == "" {
					if _, err := tx.Exec(r.Context(), `DELETE FROM onboarding_starters WHERE owner_id=$1 AND kind=$2 AND promoted_entity_id IS NULL`, id, kind); err != nil {
						return err
					}
					continue
				}
				if _, err := tx.Exec(r.Context(), `INSERT INTO onboarding_starters(owner_id,kind,title,created_at,updated_at) VALUES($1,$2,$3,$4,$4) ON CONFLICT(owner_id,kind) DO UPDATE SET title=excluded.title,updated_at=excluded.updated_at,version=onboarding_starters.version+1 WHERE onboarding_starters.promoted_entity_id IS NULL AND onboarding_starters.title<>excluded.title`, id, kind, title, now); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(r.Context(), `UPDATE onboarding SET step=$2,version=version+1,updated_at=$3 WHERE owner_id=$1`, id, input.Step, now); err != nil {
				return err
			}
		}
		if err := audit(r.Context(), tx, id, "onboarding.saved", "onboarding", id); err != nil {
			return err
		}
		result, err = scanOnboarding(tx.QueryRow(r.Context(), onboardingSelect, id))
		return err
	})
	return result, err
}
func (s *Service) mountOnboarding(r chi.Router) {
	r.Get("/api/v1/me/onboarding", func(w http.ResponseWriter, r *http.Request) {
		value, err := s.GetOnboarding(r.Context())
		s.reply(w, r, value, err)
	})
	r.Patch("/api/v1/me/onboarding", func(w http.ResponseWriter, r *http.Request) {
		input, problem := platform.DecodeJSON[OnboardingInput](r)
		if problem != nil {
			platform.WriteError(w, r, problem)
			return
		}
		value, err := s.SaveOnboarding(r, input, false)
		s.reply(w, r, value, err)
	})
	r.Post("/api/v1/me/onboarding/complete", func(w http.ResponseWriter, r *http.Request) {
		_, problem := platform.DecodeJSON[struct{}](r)
		if problem != nil {
			platform.WriteError(w, r, problem)
			return
		}
		value, err := s.SaveOnboarding(r, OnboardingInput{}, true)
		s.reply(w, r, value, err)
	})
}
