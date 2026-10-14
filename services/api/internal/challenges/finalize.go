package challenges

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type CloseInput struct {
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
}
type Finalization struct {
	Run    Run                 `json:"run"`
	Member *Member             `json:"member"`
	Reward *progression.Reward `json:"reward"`
}
type FinalSnapshot struct {
	Kind      string    `json:"kind"`
	Result    string    `json:"result"`
	At        time.Time `json:"at"`
	Reason    string    `json:"reason"`
	RulesHash string    `json:"rules_hash"`
	Progress  *Progress `json:"progress"`
}

func (s *Service) WithSchedulingClock(c platform.Clock) *Service { s.schedulingClock = c; return s }
func (s *Service) scheduleClose(ctx context.Context, tx pgx.Tx, owner string, run Run) error {
	body, e := json.Marshal(struct {
		RunID string `json:"run_id"`
	}{run.ID})
	if e != nil {
		return e
	}
	now := s.Clock.Now()
	due := run.EndsAt
	if s.schedulingClock != nil {
		now = s.schedulingClock.Now()
		due = now.Add(run.EndsAt.Sub(s.Clock.Now()))
	}
	_, e = tx.Exec(ctx, "INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'challenges.close',$2,$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING", owner, body, "challenge-close/"+owner+"/"+run.ID, due, now)
	return e
}
func (s *Service) syncClosedReward(ctx context.Context, tx pgx.Tx, owner string, run Run, m Member, p Progress) (Member, error) {
	if m.State != "finished" || m.FinalSnapshot == nil {
		return m, nil
	}
	var original FinalSnapshot
	if e := json.Unmarshal(m.FinalSnapshot, &original); e != nil {
		return m, e
	}
	if original.Kind != "result" || original.Result == "left" {
		return m, nil
	}
	if run.Rules.EconomyVersion != progression.Version || run.Rules.RewardXP != progression.BaseXP("challenge", 0) {
		return m, privateapi.Problem(409, "ECONOMY_VERSION_UNAVAILABLE", "Версия награды сейчас недоступна")
	}
	result := "not_achieved"
	if p.TargetMet {
		result = "achieved"
	}
	if m.Result == nil || *m.Result != result {
		if _, e := tx.Exec(ctx, "UPDATE challenge_participants SET result=$3,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2", owner, m.ID, result, s.Clock.Now()); e != nil {
			return m, e
		}
		fresh, e := s.member(ctx, tx, owner, owner, run.ID)
		if e != nil {
			return m, e
		}
		m = *fresh
		if e = s.memberEvent(ctx, tx, owner, "result_corrected", m); e != nil {
			return m, e
		}
	}
	reward, e := s.progress.Sync(ctx, tx, owner, progression.Source{Kind: "challenge", Key: m.ID, AreaID: run.Rules.AreaID, Complete: p.TargetMet, Eligible: true, AllowFirst: true})
	m.Reward = reward
	return m, e
}
func (s *Service) finalize(ctx context.Context, tx pgx.Tx, owner, id string, early bool, reason string) (Finalization, error) {
	out := Finalization{}
	run, e := s.getRun(ctx, tx, owner, id)
	if e != nil {
		return out, e
	}
	m, e := s.member(ctx, tx, owner, owner, id)
	if e != nil {
		return out, e
	}
	out.Run = run
	out.Member = m
	if run.State == "closed" {
		if m != nil {
			v, e := s.rebuild(ctx, tx, owner, run, *m)
			out.Member = &v
			out.Reward = v.Reward
			return out, e
		}
		return out, nil
	}
	now := s.Clock.Now().UTC()
	expired := !now.Before(run.EndsAt)
	var p *Progress
	if m != nil {
		fresh, e := s.rebuild(ctx, tx, owner, run, *m)
		if e != nil {
			return out, e
		}
		m = &fresh
		var v Progress
		if e = json.Unmarshal(m.Progress, &v); e != nil {
			return out, e
		}
		p = &v
	}
	if !expired && (!early || m == nil || m.State != "joined" || p == nil || !p.TargetMet) {
		return out, privateapi.Problem(409, "RESULT_NOT_READY", "Досрочное закрытие доступно после достижения цели; иначе дождитесь конца срока")
	}
	cutoff := now
	if expired {
		cutoff = run.EndsAt
	}
	result := "not_achieved"
	if m != nil && m.State == "left" {
		result = "left"
	} else if p != nil && p.TargetMet {
		result = "achieved"
	}
	snapshot := FinalSnapshot{Kind: "result", Result: result, At: cutoff, Reason: reason, RulesHash: run.RulesHash, Progress: p}
	body, e := json.Marshal(snapshot)
	if e != nil {
		return out, e
	}
	if m != nil {
		// Retain previously credited identities, including a correction currently at
		// zero. Closing freezes scope; later work cannot enlarge this result.
		if _, e = tx.Exec(ctx, "INSERT INTO challenge_final_sources(owner_id,participant_id,source_kind,source_key,title,source_url,amount,source_time,source_start,source_version,active,captured_at) SELECT owner_id,participant_id,source_kind,source_key,title,source_url,amount,source_time,source_start,source_version,active,$3 FROM challenge_source_credits WHERE owner_id=$1 AND participant_id=$2 ON CONFLICT DO NOTHING", owner, m.ID, cutoff); e != nil {
			return out, e
		}
		if _, e = tx.Exec(ctx, "UPDATE challenge_membership_windows SET left_at=GREATEST(joined_at,$3) WHERE owner_id=$1 AND participant_id=$2 AND left_at IS NULL", owner, m.ID, cutoff); e != nil {
			return out, e
		}
		if _, e = tx.Exec(ctx, "UPDATE challenge_participants SET state='finished',result=$3,closed_at=$4,updated_at=$5,version=version+1,final_snapshot=$6 WHERE owner_id=$1 AND id=$2", owner, m.ID, result, cutoff, now, body); e != nil {
			return out, e
		}
		m, e = s.member(ctx, tx, owner, owner, id)
		if e != nil {
			return out, e
		}
		if e = s.memberEvent(ctx, tx, owner, "finished", *m); e != nil {
			return out, e
		}
	}
	run, e = scanRun(tx.QueryRow(ctx, "UPDATE challenge_runs SET state='closed',version=version+1,finalized_at=$3,final_snapshot=$4 WHERE owner_id=$1 AND id=$2 RETURNING "+runColumns, owner, id, now, body))
	if e != nil {
		return out, e
	}
	out.Run = run
	out.Member = m
	if m != nil && p != nil {
		v, e := s.syncClosedReward(ctx, tx, owner, run, *m, *p)
		out.Member = &v
		out.Reward = v.Reward
		return out, e
	}
	return out, nil
}
func (s *Service) expired(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	if job.OwnerID == nil || s.progress == nil {
		return jobs.ErrPermanent
	}
	var in struct {
		RunID string `json:"run_id"`
	}
	if e := json.Unmarshal(job.Payload, &in); e != nil || !privateapi.UUID(in.RunID) {
		return jobs.ErrPermanent
	}
	if e := s.progress.LockOwner(ctx, tx, *job.OwnerID); e != nil {
		return e
	}
	_, e := s.finalize(ctx, tx, *job.OwnerID, in.RunID, false, "")
	return e
}
func (s *Service) mountFinalization(r chi.Router) {
	r.Post("/api/v1/challenges/runs/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in CloseInput) (any, error) {
			if !in.Confirm || utf8.RuneCountInString(in.Reason) > 1000 || strings.ContainsRune(in.Reason, 0) {
				return nil, privateapi.Invalid("Подтвердите закрытие; причина до 1000 символов")
			}
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				run, e := s.getRun(ctx, tx, owner, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if run.State != "closed" {
					if p := platform.MatchVersion(r, run.Version); p != nil {
						return nil, p
					}
				}
				return s.finalize(ctx, tx, owner, run.ID, true, in.Reason)
			})
		})
	})
	r.Get("/api/v1/challenges/runs/{id}/final-sources", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			run, e := s.getRun(ctx, tx, owner, chi.URLParam(r, "id"))
			if e != nil {
				return nil, e
			}
			m, e := s.member(ctx, tx, owner, owner, run.ID)
			if e != nil {
				return nil, e
			}
			if m == nil || m.ClosedAt == nil {
				return nil, privateapi.Problem(404, "NOT_FOUND", "Итог ещё не сохранён")
			}
			return s.sourcePageTable(r, tx, owner, run, *m, "challenge_final_sources")
		})
		s.Reply(w, r, out, e)
	})
}
