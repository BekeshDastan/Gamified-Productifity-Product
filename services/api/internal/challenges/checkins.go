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

type CheckinInput struct {
	OperationID string `json:"operation_id"`
	Unit        string `json:"unit"`
	Amount      int64  `json:"amount"`
	Note        string `json:"note"`
}
type CheckinCorrection struct {
	Amount int64  `json:"amount"`
	Note   string `json:"note"`
}
type Checkin struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participant_id"`
	Date          string    `json:"date"`
	Unit          string    `json:"unit"`
	InitialAmount int64     `json:"initial_amount"`
	Amount        int64     `json:"amount"`
	Note          string    `json:"note"`
	Version       int64     `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Confirmation  string    `json:"confirmation"`
}

const checkinColumns = "id::text,participant_id::text,local_date::text,unit,initial_amount,amount,note,version,created_at,updated_at"

func scanCheckin(row pgx.Row) (Checkin, error) {
	v := Checkin{Confirmation: "self_report"}
	e := row.Scan(&v.ID, &v.ParticipantID, &v.Date, &v.Unit, &v.InitialAmount, &v.Amount, &v.Note, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = privateapi.Problem(404, "NOT_FOUND", "Отметка не найдена")
	}
	return v, e
}
func (s *Service) getCheckin(ctx context.Context, tx pgx.Tx, owner, runID, id string) (Checkin, error) {
	if !privateapi.UUID(id) || !privateapi.UUID(runID) {
		return Checkin{}, privateapi.Problem(404, "NOT_FOUND", "Отметка не найдена")
	}
	return scanCheckin(tx.QueryRow(ctx, "SELECT "+checkinColumns+" FROM challenge_checkins WHERE owner_id=$1 AND id=$2 AND participant_id IN(SELECT id FROM challenge_participants WHERE owner_id=$1 AND run_owner_id=$1 AND run_id=$3)", owner, id, runID))
}
func validNote(note string) bool {
	return utf8.RuneCountInString(note) <= 2000 && !strings.ContainsRune(note, 0)
}
func (s *Service) checkinEvent(ctx context.Context, tx pgx.Tx, owner string, v Checkin) error {
	body, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO challenge_checkin_events(owner_id,checkin_id,version,snapshot,created_at) VALUES($1,$2,$3,$4,$5)", owner, v.ID, v.Version, body, s.Clock.Now())
	return e
}
func (s *Service) createCheckin(ctx context.Context, tx pgx.Tx, owner, runID string, in CheckinInput) (Checkin, error) {
	run, e := s.getRun(ctx, tx, owner, runID)
	if e != nil {
		return Checkin{}, e
	}
	if !privateapi.UUID(in.OperationID) {
		return Checkin{}, privateapi.Invalid("Нужен UUID операции")
	}
	if strings.ContainsRune(in.Note, 0) {
		return Checkin{}, privateapi.Invalid("Нулевой символ в заметке недопустим")
	}
	body, e := json.Marshal(in)
	if e != nil {
		return Checkin{}, e
	}
	var same bool
	e = tx.QueryRow(ctx, "SELECT request=$3::jsonb FROM challenge_checkins WHERE owner_id=$1 AND id=$2", owner, in.OperationID, body).Scan(&same)
	if e == nil {
		if !same {
			return Checkin{}, privateapi.Problem(409, "IDEMPOTENCY_MISMATCH", "Отметка уже создана с другим содержимым")
		}
		return s.getCheckin(ctx, tx, owner, runID, in.OperationID)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Checkin{}, e
	}
	member, e := s.member(ctx, tx, owner, owner, runID)
	if e != nil {
		return Checkin{}, e
	}
	now := s.Clock.Now().UTC()
	if member == nil || member.State != "joined" || run.State != "open" || now.Before(run.StartsAt) || !now.Before(run.EndsAt) {
		return Checkin{}, privateapi.Problem(409, "NOT_PARTICIPATING", "Новая отметка доступна только во время активного участия")
	}
	if run.Rules.Metric != "manual" || in.Unit != run.Rules.Unit || in.Amount < 1 || in.Amount > run.Rules.DailyCap || !validNote(in.Note) {
		return Checkin{}, privateapi.Invalid("Проверьте единицу, положительное количество в дневном пределе и заметку до 2000 символов")
	}
	var n int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM challenge_checkins WHERE owner_id=$1 AND participant_id=$2", owner, member.ID).Scan(&n); e != nil {
		return Checkin{}, e
	}
	if n >= 10000 {
		return Checkin{}, privateapi.Problem(409, "CHECKIN_LIMIT", "Достигнут предел отметок этого запуска")
	}
	loc, e := time.LoadLocation(run.Rules.Timezone)
	if e != nil {
		return Checkin{}, e
	}
	v, e := scanCheckin(tx.QueryRow(ctx, "INSERT INTO challenge_checkins(owner_id,id,participant_id,request,local_date,unit,initial_amount,amount,note,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7,$8,$9,$9) RETURNING "+checkinColumns, owner, in.OperationID, member.ID, body, now.In(loc).Format("2006-01-02"), in.Unit, in.Amount, in.Note, now))
	if e != nil {
		return v, e
	}
	return v, s.checkinEvent(ctx, tx, owner, v)
}
func (s *Service) correctCheckin(r *http.Request, tx pgx.Tx, owner, runID, id string, in CheckinCorrection) (Checkin, error) {
	v, e := s.getCheckin(r.Context(), tx, owner, runID, id)
	if e != nil {
		return v, e
	}
	if p := platform.MatchVersion(r, v.Version); p != nil {
		return v, p
	}
	if in.Amount < 0 || in.Amount > v.InitialAmount || !validNote(in.Note) {
		return v, privateapi.Invalid("Исправление — от нуля до исходного количества; дополнительные действия оформляйте новой отметкой")
	}
	if in.Amount == v.Amount && in.Note == v.Note {
		return v, nil
	}
	v, e = scanCheckin(tx.QueryRow(r.Context(), "UPDATE challenge_checkins SET amount=$3,note=$4,version=version+1,updated_at=$5 WHERE owner_id=$1 AND id=$2 RETURNING "+checkinColumns, owner, id, in.Amount, in.Note, s.Clock.Now()))
	if e != nil {
		return v, e
	}
	return v, s.checkinEvent(r.Context(), tx, owner, v)
}

type CheckinPage struct {
	Items []Checkin `json:"items"`
	Next  string    `json:"next"`
}

func (s *Service) checkins(r *http.Request, tx pgx.Tx, owner, runID string) (CheckinPage, error) {
	out := CheckinPage{Items: []Checkin{}}
	if _, e := s.getRun(r.Context(), tx, owner, runID); e != nil {
		return out, e
	}
	m, e := s.member(r.Context(), tx, owner, owner, runID)
	if e != nil || m == nil {
		return out, e
	}
	var at *time.Time
	var key *string
	before := r.URL.Query().Get("before")
	if before != "" {
		v, e := s.getCheckin(r.Context(), tx, owner, runID, before)
		if e != nil {
			return out, e
		}
		at = &v.CreatedAt
		key = &v.ID
	}
	rows, e := tx.Query(r.Context(), "SELECT "+checkinColumns+" FROM challenge_checkins WHERE owner_id=$1 AND participant_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT 51", owner, m.ID, at, key)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanCheckin(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > 50 {
		out.Items = out.Items[:50]
		out.Next = out.Items[49].ID
	}
	return out, nil
}
func (s *Service) mountCheckins(r chi.Router) {
	r.Get("/api/v1/challenges/runs/{id}/check-ins", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.checkins(r, tx, owner, chi.URLParam(r, "id"))
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/challenges/runs/{id}/check-ins", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in CheckinInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.createCheckin(ctx, tx, owner, chi.URLParam(r, "id"), in)
			})
		})
	})
	r.Patch("/api/v1/challenges/runs/{id}/check-ins/{checkin}", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in CheckinCorrection) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.correctCheckin(r, tx, owner, chi.URLParam(r, "id"), chi.URLParam(r, "checkin"), in)
			})
		})
	})
	s.mountEvidence(r)
}
