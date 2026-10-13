package focus

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxActiveSeconds int64 = 4 * 60 * 60
const MaxElapsedSeconds int64 = 8 * 60 * 60

type Service struct {
	pool     *pgxpool.Pool
	clock    platform.Clock
	progress *progression.Service
}

func NewService(p *pgxpool.Pool, c platform.Clock, r *progression.Service) *Service {
	return &Service{p, c, r}
}
func problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func validID(id string) bool { var v pgtype.UUID; return v.Scan(id) == nil && v.Valid }
func owner(ctx context.Context) (string, error) {
	v, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return v.UserID, nil
}
func (s *Service) now() time.Time { return s.clock.Now().UTC().Truncate(time.Microsecond) }

type StartInput struct {
	OperationID  string  `json:"operation_id"`
	Mode         string  `json:"mode"`
	Title        string  `json:"title"`
	QuestID      *string `json:"quest_id"`
	GoalID       *string `json:"goal_id"`
	AreaID       *string `json:"area_id"`
	WorkMinutes  int     `json:"work_minutes"`
	BreakMinutes int     `json:"break_minutes"`
	SoundEnabled bool    `json:"sound_enabled"`
}
type Segment struct {
	ID        int64      `json:"id"`
	Kind      string     `json:"kind"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Seconds   int64      `json:"seconds"`
}
type Session struct {
	ID                    string     `json:"id"`
	State                 string     `json:"state"`
	Mode                  string     `json:"mode"`
	Phase                 string     `json:"phase"`
	Title                 string     `json:"title"`
	QuestID               *string    `json:"quest_id"`
	GoalID                *string    `json:"goal_id"`
	AreaID                *string    `json:"area_id"`
	Timezone              string     `json:"timezone"`
	WorkMinutes           int        `json:"work_minutes"`
	BreakMinutes          int        `json:"break_minutes"`
	SoundEnabled          bool       `json:"sound_enabled"`
	Cycle                 int64      `json:"cycle"`
	Version               int64      `json:"version"`
	StartedAt             time.Time  `json:"started_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	EndedAt               *time.Time `json:"ended_at"`
	PhaseDeadline         *time.Time `json:"phase_deadline"`
	PhaseRemainingSeconds *int64     `json:"phase_remaining_seconds"`
	Result                string     `json:"result"`
	Comment               string     `json:"comment"`
	ActiveSeconds         int64      `json:"active_seconds"`
	BreakSeconds          int64      `json:"break_seconds"`
	Segments              []Segment  `json:"segments"`
}
type Current struct {
	ServerNow time.Time `json:"server_now"`
	Session   *Session  `json:"session"`
}

const columns = "id::text,state,mode,phase,title,quest_id::text,goal_id::text,area_id::text,timezone,work_minutes,break_minutes,sound_enabled,cycle,version,started_at,updated_at,ended_at,phase_deadline,result,comment,phase_remaining_seconds"

func (s *Service) load(ctx context.Context, tx pgx.Tx, owner, id string) (Session, error) {
	var f Session
	f.Segments = []Segment{}
	if !validID(id) {
		return f, problem(404, "NOT_FOUND", "Сессия не найдена")
	}
	e := tx.QueryRow(ctx, "SELECT "+columns+" FROM focus_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE", owner, id).Scan(&f.ID, &f.State, &f.Mode, &f.Phase, &f.Title, &f.QuestID, &f.GoalID, &f.AreaID, &f.Timezone, &f.WorkMinutes, &f.BreakMinutes, &f.SoundEnabled, &f.Cycle, &f.Version, &f.StartedAt, &f.UpdatedAt, &f.EndedAt, &f.PhaseDeadline, &f.Result, &f.Comment, &f.PhaseRemainingSeconds)
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Сессия не найдена")
	}
	if e != nil {
		return f, e
	}
	rows, e := tx.Query(ctx, "SELECT id,kind,started_at,ended_at FROM focus_segments WHERE owner_id=$1 AND session_id=$2 ORDER BY id", owner, id)
	if e != nil {
		return f, e
	}
	defer rows.Close()
	for rows.Next() {
		var seg Segment
		if e = rows.Scan(&seg.ID, &seg.Kind, &seg.StartedAt, &seg.EndedAt); e != nil {
			return f, e
		}
		end := s.now()
		if seg.EndedAt != nil {
			end = *seg.EndedAt
		}
		seg.Seconds = max(0, int64(end.Sub(seg.StartedAt)/time.Second))
		if seg.Kind == "work" {
			f.ActiveSeconds += seg.Seconds
		} else {
			f.BreakSeconds += seg.Seconds
		}
		f.Segments = append(f.Segments, seg)
	}
	return f, rows.Err()
}
func (s *Service) event(ctx context.Context, tx pgx.Tx, owner, kind string, f Session) error {
	b, e := json.Marshal(f)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO focus_events(owner_id,session_id,kind,snapshot,created_at) VALUES($1,$2,$3,$4,$5)", owner, f.ID, kind, b, s.now())
	return e
}
func (s *Service) current(ctx context.Context, tx pgx.Tx, owner string) (Current, error) {
	out := Current{ServerNow: s.now()}
	var id string
	e := tx.QueryRow(ctx, "SELECT id::text FROM focus_sessions WHERE owner_id=$1 AND state IN('running','paused','awaiting_confirmation')", owner).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	f, e := s.get(ctx, tx, owner, id)
	out.Session = &f
	return out, e
}
func validateStart(in StartInput) (StartInput, error) {
	if in.Mode == "" {
		in.Mode = "free"
	}
	if in.WorkMinutes == 0 {
		in.WorkMinutes = 25
	}
	if in.BreakMinutes == 0 {
		in.BreakMinutes = 5
	}
	in.Title = strings.TrimSpace(in.Title)
	if !validID(in.OperationID) || in.Mode != "free" || in.WorkMinutes < 5 || in.WorkMinutes > 120 || in.BreakMinutes < 1 || in.BreakMinutes > 60 || utf8.RuneCountInString(in.Title) > 200 || strings.ContainsRune(in.Title, 0) {
		return in, problem(422, "VALIDATION_ERROR", "Проверьте режим и длительность сессии")
	}
	for _, id := range []*string{in.QuestID, in.GoalID, in.AreaID} {
		if id != nil && !validID(*id) {
			return in, problem(422, "VALIDATION_ERROR", "Проверьте связь с задачей, целью и направлением")
		}
	}
	return in, nil
}
func (s *Service) start(ctx context.Context, tx pgx.Tx, owner string, input StartInput) (Session, error) {
	in, e := validateStart(input)
	if e != nil {
		return Session{}, e
	}
	request, e := json.Marshal(in)
	if e != nil {
		return Session{}, e
	}
	var matches bool
	e = tx.QueryRow(ctx, "SELECT request=$3::jsonb FROM focus_sessions WHERE owner_id=$1 AND id=$2", owner, in.OperationID, request).Scan(&matches)
	if e == nil {
		if !matches {
			return Session{}, problem(409, "IDEMPOTENCY_MISMATCH", "ID сессии использован для другого старта")
		}
		return s.get(ctx, tx, owner, in.OperationID)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Session{}, e
	}
	cur, e := s.current(ctx, tx, owner)
	if e != nil {
		return Session{}, e
	}
	if cur.Session != nil {
		return Session{}, problem(409, "FOCUS_ALREADY_ACTIVE", "Уже есть незавершённая сессия. Откройте её")
	}
	title, area := in.Title, in.AreaID
	if in.QuestID != nil {
		var qArea *string
		e = tx.QueryRow(ctx, "SELECT payload->>'title',area_id::text FROM quests WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL AND status NOT IN('completed','failed','skipped')", owner, *in.QuestID).Scan(&title, &qArea)
		if errors.Is(e, pgx.ErrNoRows) {
			return Session{}, problem(404, "NOT_FOUND", "Открытая задача не найдена")
		}
		if e != nil {
			return Session{}, e
		}
		if area == nil {
			area = qArea
		}
	}
	if in.GoalID != nil {
		var gTitle string
		var gArea *string
		e = tx.QueryRow(ctx, "SELECT payload->>'title',area_id::text FROM goals WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL AND state NOT IN('achieved','cancelled')", owner, *in.GoalID).Scan(&gTitle, &gArea)
		if errors.Is(e, pgx.ErrNoRows) {
			return Session{}, problem(404, "NOT_FOUND", "Открытая цель не найдена")
		}
		if e != nil {
			return Session{}, e
		}
		if title == "" {
			title = gTitle
		}
		if area == nil {
			area = gArea
		}
	}
	if area != nil {
		var exists bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM life_areas WHERE owner_id=$1 AND id=$2)", owner, *area).Scan(&exists); e != nil {
			return Session{}, e
		}
		if !exists {
			return Session{}, problem(404, "NOT_FOUND", "Направление не найдено")
		}
	}
	if title == "" {
		title = "Свободная сессия"
	}
	var zone string
	if e = tx.QueryRow(ctx, "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&zone); e != nil {
		return Session{}, e
	}
	now := s.now()
	_, e = tx.Exec(ctx, `INSERT INTO focus_sessions(owner_id,id,request,state,mode,phase,title,quest_id,goal_id,area_id,timezone,work_minutes,break_minutes,sound_enabled,started_at,updated_at) VALUES($1,$2,$3,'running',$4,'work',$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)`, owner, in.OperationID, request, in.Mode, title, in.QuestID, in.GoalID, area, zone, in.WorkMinutes, in.BreakMinutes, in.SoundEnabled, now)
	if e != nil {
		return Session{}, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO focus_segments(owner_id,session_id,kind,started_at) VALUES($1,$2,'work',$3)", owner, in.OperationID, now); e != nil {
		return Session{}, e
	}
	f, e := s.get(ctx, tx, owner, in.OperationID)
	if e == nil {
		e = s.event(ctx, tx, owner, "started", f)
	}
	return f, e
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, out any, e error) {
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) {
			p = problem(500, "INTERNAL_ERROR", "Не удалось сохранить фокус-сессию")
		}
		platform.WriteError(w, r, p)
		return
	}
	platform.WriteData(w, r, s.clock, 200, out)
}
func (s *Service) mutate(r *http.Request, input any, fn func(context.Context, pgx.Tx, string) (any, error)) (any, error) {
	id, e := owner(r.Context())
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(struct {
		Input   any
		Version string
	}{input, r.Header.Get("If-Match")})
	if e != nil {
		return nil, e
	}
	result, e := (storage.IdempotencyStore{DB: s.pool, Now: s.clock.Now}).Execute(r.Context(), storage.Scope{UserID: id, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, b, func(tx pgx.Tx) (storage.Result, error) {
		if e := s.progress.LockOwner(r.Context(), tx, id); e != nil {
			return storage.Result{}, e
		}
		out, e := fn(r.Context(), tx, id)
		if e != nil {
			return storage.Result{}, e
		}
		if e = jobs.OwnerChanged(r.Context(), tx, id, s.clock); e != nil {
			return storage.Result{}, e
		}
		body, e := json.Marshal(out)
		return storage.Result{Status: 200, Body: body}, e
	})
	switch {
	case errors.Is(e, storage.ErrInvalidScope):
		e = problem(422, "VALIDATION_ERROR", "Нужен ключ запроса")
	case errors.Is(e, storage.ErrIdempotencyMismatch):
		e = problem(409, "IDEMPOTENCY_MISMATCH", "Ключ использован для другого запроса")
	case errors.Is(e, storage.ErrOperationInProgress):
		e = problem(409, "OPERATION_IN_PROGRESS", "Запрос выполняется")
	}
	return json.RawMessage(result.Body), e
}
func decode[T any](s *Service, w http.ResponseWriter, r *http.Request, fn func(T) (any, error)) {
	in, p := platform.DecodeJSON[T](r)
	if p != nil {
		platform.WriteError(w, r, p)
		return
	}
	out, e := fn(in)
	s.reply(w, r, out, e)
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountActions(r)
		r.Get("/api/v1/focus/current", func(w http.ResponseWriter, r *http.Request) {
			id, e := owner(r.Context())
			var out Current
			if e == nil {
				e = s.progress.WithinOwner(r.Context(), id, func(tx pgx.Tx) error { var e error; out, e = s.current(r.Context(), tx, id); return e })
			}
			s.reply(w, r, out, e)
		})
		r.Post("/api/v1/focus/start", func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in StartInput) (any, error) {
				return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, id string) (any, error) { return s.start(ctx, tx, id, in) })
			})
		})
	})
}
