package challenges

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Member struct {
	ID            string          `json:"id"`
	RunID         string          `json:"run_id"`
	RulesHash     string          `json:"rules_hash"`
	State         string          `json:"state"`
	Version       int64           `json:"version"`
	FirstJoinedAt time.Time       `json:"first_joined_at"`
	LeftAt        *time.Time      `json:"left_at"`
	LeaveReason   string          `json:"leave_reason"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Progress      json.RawMessage `json:"progress"`
	ComputedAt    *time.Time      `json:"computed_at"`
	Result        *string         `json:"result"`
	FinalSnapshot json.RawMessage `json:"final_snapshot"`
	ClosedAt      *time.Time      `json:"closed_at"`
}
type Membership struct {
	Member *Member `json:"member"`
}
type JoinInput struct {
	RulesHash         string `json:"rules_hash"`
	AcceptRules       bool   `json:"accept_rules"`
	MembershipVersion int64  `json:"membership_version"`
}
type LeaveInput struct {
	Reason string `json:"reason"`
}

const memberColumns = "id::text,run_id::text,rules_hash,state,version,first_joined_at,left_at,leave_reason,updated_at,progress,computed_at,result,final_snapshot,closed_at"

func scanMember(row pgx.Row) (Member, error) {
	var out Member
	e := row.Scan(&out.ID, &out.RunID, &out.RulesHash, &out.State, &out.Version, &out.FirstJoinedAt, &out.LeftAt, &out.LeaveReason, &out.UpdatedAt, &out.Progress, &out.ComputedAt, &out.Result, &out.FinalSnapshot, &out.ClosedAt)
	return out, e
}
func (s *Service) member(ctx context.Context, tx pgx.Tx, owner, runOwner, runID string) (*Member, error) {
	out, e := scanMember(tx.QueryRow(ctx, "SELECT "+memberColumns+" FROM challenge_participants WHERE owner_id=$1 AND run_owner_id=$2 AND run_id=$3", owner, runOwner, runID))
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &out, e
}
func (s *Service) memberEvent(ctx context.Context, tx pgx.Tx, owner, kind string, member Member) error {
	body, e := json.Marshal(member)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO challenge_participant_events(owner_id,participant_id,version,kind,snapshot,created_at) VALUES($1,$2,$3,$4,$5,$6)", owner, member.ID, member.Version, kind, body, s.Clock.Now().UTC())
	return e
}
func (s *Service) join(ctx context.Context, tx pgx.Tx, owner, id string, in JoinInput) (Member, error) {
	run, e := s.getRun(ctx, tx, owner, id)
	if e != nil {
		return Member{}, e
	}
	if !in.AcceptRules || in.RulesHash != run.RulesHash || in.MembershipVersion < 0 {
		return Member{}, privateapi.Invalid("Перед вступлением подтвердите показанные правила")
	}
	now := s.Clock.Now().UTC().Truncate(time.Microsecond)
	if run.State != "open" || !now.Before(run.EndsAt) {
		return Member{}, privateapi.Problem(409, "RUN_ENDED", "Срок испытания окончен")
	}
	old, e := s.member(ctx, tx, owner, owner, id)
	if e != nil {
		return Member{}, e
	}
	if old != nil && old.State == "joined" {
		return *old, nil
	}
	if old != nil && (old.State == "finished" || !run.Rules.Rejoin) {
		return *old, privateapi.Problem(409, "REJOIN_DISABLED", "Повторное вступление не разрешено правилами")
	}
	current := int64(0)
	if old != nil {
		current = old.Version
	}
	if in.MembershipVersion != current {
		return Member{}, privateapi.Problem(409, "VERSION_CONFLICT", "Участие изменилось. Обновите его перед вступлением")
	}
	sequence := 1
	var member Member
	if old == nil {
		member, e = scanMember(tx.QueryRow(ctx, "INSERT INTO challenge_participants(owner_id,run_owner_id,run_id,rules_hash,state,first_joined_at,updated_at) VALUES($1,$1,$2,$3,'joined',$4,$4) RETURNING "+memberColumns, owner, id, run.RulesHash, now))
	} else {
		if e = tx.QueryRow(ctx, "SELECT COALESCE(max(sequence),0)+1 FROM challenge_membership_windows WHERE owner_id=$1 AND participant_id=$2", owner, old.ID).Scan(&sequence); e != nil {
			return Member{}, e
		}
		if sequence > 100 {
			return Member{}, privateapi.Problem(409, "REJOIN_LIMIT", "Достигнут предел повторных вступлений")
		}
		if now.Before(old.FirstJoinedAt) {
			now = old.FirstJoinedAt
		}
		member, e = scanMember(tx.QueryRow(ctx, "UPDATE challenge_participants SET state='joined',left_at=NULL,leave_reason='',version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING "+memberColumns, owner, old.ID, now))
	}
	if e != nil {
		return member, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO challenge_membership_windows(owner_id,participant_id,sequence,joined_at) VALUES($1,$2,$3,$4)", owner, member.ID, sequence, now); e != nil {
		return member, e
	}
	e = s.memberEvent(ctx, tx, owner, "joined", member)
	return member, e
}
func (s *Service) leave(r *http.Request, tx pgx.Tx, owner, id string, in LeaveInput) (Member, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > 1000 || strings.ContainsRune(in.Reason, 0) {
		return Member{}, privateapi.Invalid("Причина до 1000 символов, необязательно")
	}
	run, e := s.getRun(r.Context(), tx, owner, id)
	if e != nil {
		return Member{}, e
	}
	member, e := s.member(r.Context(), tx, owner, owner, id)
	if e != nil {
		return Member{}, e
	}
	if member == nil {
		return Member{}, privateapi.Problem(404, "NOT_FOUND", "Участие не найдено")
	}
	if member.State == "left" {
		return *member, nil
	}
	if member.State != "joined" || run.State != "open" || !s.Clock.Now().Before(run.EndsAt) {
		return *member, privateapi.Problem(409, "RUN_ENDED", "Сначала получите итог завершённого периода")
	}
	if p := platform.MatchVersion(r, member.Version); p != nil {
		return *member, p
	}
	now := s.Clock.Now().UTC().Truncate(time.Microsecond)
	var joined time.Time
	if e = tx.QueryRow(r.Context(), "SELECT joined_at FROM challenge_membership_windows WHERE owner_id=$1 AND participant_id=$2 AND left_at IS NULL", owner, member.ID).Scan(&joined); e != nil {
		return *member, e
	}
	if now.Before(joined) {
		now = joined
	}
	if _, e = tx.Exec(r.Context(), "UPDATE challenge_membership_windows SET left_at=$3 WHERE owner_id=$1 AND participant_id=$2 AND left_at IS NULL", owner, member.ID, now); e != nil {
		return *member, e
	}
	saved, e := scanMember(tx.QueryRow(r.Context(), "UPDATE challenge_participants SET state='left',left_at=$3,leave_reason=$4,version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING "+memberColumns, owner, member.ID, now, in.Reason))
	if e != nil {
		return saved, e
	}
	e = s.memberEvent(r.Context(), tx, owner, "left", saved)
	return saved, e
}
func (s *Service) cancelRun(r *http.Request, tx pgx.Tx, owner, id string, in LeaveInput) (Run, error) {
	run, e := s.getRun(r.Context(), tx, owner, id)
	if e != nil {
		return run, e
	}
	if run.State == "closed" {
		return run, nil
	}
	if p := platform.MatchVersion(r, run.Version); p != nil {
		return run, p
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > 1000 || strings.ContainsRune(in.Reason, 0) {
		return run, privateapi.Invalid("Причина до 1000 символов")
	}
	var active int
	if e = tx.QueryRow(r.Context(), "SELECT count(*) FROM challenge_participants WHERE run_owner_id=$1 AND run_id=$2 AND state='joined'", owner, id).Scan(&active); e != nil {
		return run, e
	}
	if active > 0 {
		return run, privateapi.Problem(409, "MEMBERS_ACTIVE", "Сначала выйдите из испытания")
	}
	now := s.Clock.Now().UTC().Truncate(time.Microsecond)
	snapshot, e := json.Marshal(struct {
		Kind   string    `json:"kind"`
		Reason string    `json:"reason"`
		At     time.Time `json:"at"`
	}{"cancelled", in.Reason, now})
	if e != nil {
		return run, e
	}
	saved, e := scanRun(tx.QueryRow(r.Context(), "UPDATE challenge_runs SET state='closed',version=version+1,finalized_at=$3,final_snapshot=$4 WHERE owner_id=$1 AND id=$2 RETURNING "+runColumns, owner, id, now, snapshot))
	if e != nil {
		return saved, e
	}
	member, e := s.member(r.Context(), tx, owner, owner, id)
	if e != nil {
		return saved, e
	}
	if member != nil && member.State == "left" {
		member.State = "finished"
		member.Version++
		result := "left"
		member.Result = &result
		member.ClosedAt = &now
		member.UpdatedAt = now
		body, e := json.Marshal(member)
		if e != nil {
			return saved, e
		}
		_, e = tx.Exec(r.Context(), "UPDATE challenge_participants SET state='finished',version=$3,result='left',closed_at=$4,updated_at=$4,final_snapshot=$5 WHERE owner_id=$1 AND id=$2", owner, member.ID, member.Version, now, body)
		if e != nil {
			return saved, e
		}
		member.FinalSnapshot = body
		if e = s.memberEvent(r.Context(), tx, owner, "cancelled", *member); e != nil {
			return saved, e
		}
	}
	return saved, nil
}
func (s *Service) mountMembers(r chi.Router) {
	r.Get("/api/v1/challenges/runs/{id}/membership", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			id := chi.URLParam(r, "id")
			if _, e := s.getRun(ctx, tx, owner, id); e != nil {
				return nil, e
			}
			member, e := s.member(ctx, tx, owner, owner, id)
			return Membership{member}, e
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/challenges/runs/{id}/join", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in JoinInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.join(ctx, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
	r.Post("/api/v1/challenges/runs/{id}/leave", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in LeaveInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.leave(r, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
	r.Post("/api/v1/challenges/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in LeaveInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.cancelRun(r, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
}
