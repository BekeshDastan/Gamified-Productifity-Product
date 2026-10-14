package character

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type AchievementStatus struct {
	Definition
	Current   int64      `json:"current"`
	Available bool       `json:"available"`
	Active    bool       `json:"active"`
	Revoked   bool       `json:"revoked"`
	AwardedAt *time.Time `json:"awarded_at"`
}

func (s *Service) evaluateAchievements(ctx context.Context, tx pgx.Tx, owner string) (bool, error) {
	f, _, _, e := s.facts(ctx, tx, owner)
	if e != nil {
		return false, e
	}
	changed := false
	for _, d := range Definitions() {
		b, e := json.Marshal(d)
		if e != nil {
			return false, e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO achievement_definitions(id,version,snapshot) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", d.ID, d.Version, b); e != nil {
			return false, e
		}
		if !f.Available[d.Metric] {
			continue
		}
		var active bool
		e = tx.QueryRow(ctx, "SELECT active FROM user_achievements WHERE owner_id=$1 AND definition_id=$2 AND definition_version=$3", owner, d.ID, d.Version).Scan(&active)
		exists := e == nil
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return false, e
		}
		met := d.Met(f)
		if (!exists && !met) || (exists && active == met) {
			continue
		}
		evidence, e := json.Marshal(map[string]any{"metric": d.Metric, "current": f.Values[d.Metric], "threshold": d.Threshold, "source_url": d.SourceURL, "version": d.Version})
		if e != nil {
			return false, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO user_achievements(owner_id,definition_id,definition_version,active,awarded_at,revoked_at,evidence) VALUES($1,$2,$3,$4,$5,CASE WHEN $4 THEN NULL ELSE $5::timestamptz END,$6) ON CONFLICT(owner_id,definition_id,definition_version) DO UPDATE SET active=EXCLUDED.active,revoked_at=EXCLUDED.revoked_at,evidence=EXCLUDED.evidence`, owner, d.ID, d.Version, met, s.clock.Now(), evidence)
		if e != nil {
			return false, e
		}
		var revision int64
		e = tx.QueryRow(ctx, "INSERT INTO achievement_changes(owner_id,definition_id,definition_version,active,evidence,created_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id", owner, d.ID, d.Version, met, evidence, s.clock.Now()).Scan(&revision)
		if e != nil {
			return false, e
		}
		changed = true
		if s.achievementObserver != nil {
			if e = s.achievementObserver(ctx, tx, owner, d.ID+"/"+d.Version, revision, met); e != nil {
				return false, e
			}
		}
	}
	return changed, nil
}
func (s *Service) achievementStatuses(ctx context.Context, tx pgx.Tx, owner string) ([]AchievementStatus, error) {
	f, _, _, e := s.facts(ctx, tx, owner, false)
	out := []AchievementStatus{}
	if e != nil {
		return out, e
	}
	for _, d := range Definitions() {
		a := AchievementStatus{Definition: d, Current: f.Values[d.Metric], Available: f.Available[d.Metric]}
		var active bool
		var awarded time.Time
		e = tx.QueryRow(ctx, "SELECT active,awarded_at FROM user_achievements WHERE owner_id=$1 AND definition_id=$2 AND definition_version=$3", owner, d.ID, d.Version).Scan(&active, &awarded)
		if e == nil {
			a.Active = active
			a.Revoked = !active
			a.AwardedAt = &awarded
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return out, e
		}
		out = append(out, a)
	}
	return out, nil
}
func (s *Service) mountAchievements(r chi.Router) {
	r.Get("/api/v1/achievements", func(w http.ResponseWriter, r *http.Request) {
		owner, e := currentOwner(r.Context())
		out := []AchievementStatus{}
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error {
				if e := s.EvaluateInTx(r.Context(), tx, owner); e != nil {
					return e
				}
				var e error
				out, e = s.achievementStatuses(r.Context(), tx, owner)
				return e
			})
		}
		s.reply(w, r, out, e)
	})
}
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{"character.changed": func(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
		var payload map[string]json.RawMessage
		if job.OwnerID == nil || !ValidOwner(*job.OwnerID) || json.Unmarshal(job.Payload, &payload) != nil || payload == nil || len(payload) != 0 {
			return jobs.ErrPermanent
		}
		if e := s.progress.LockOwner(ctx, tx, *job.OwnerID); e != nil {
			return e
		}
		return s.EvaluateInTx(ctx, tx, *job.OwnerID)
	}}
}
