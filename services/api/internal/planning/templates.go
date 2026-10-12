package planning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type RoutineAction struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Minutes  int    `json:"minutes"`
	Priority string `json:"priority"`
	Start    string `json:"start"`
	End      string `json:"end"`
}
type RoutineDay struct {
	Day              int             `json:"day"`
	AvailableMinutes int             `json:"available_minutes"`
	Windows          []Window        `json:"windows"`
	Rest             bool            `json:"rest"`
	Actions          []RoutineAction `json:"actions"`
}
type RoutineInput struct {
	Title  string       `json:"title"`
	Period string       `json:"period"`
	Days   []RoutineDay `json:"days"`
}
type Routine struct {
	RoutineInput
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type RoutineRequest struct {
	StartDate       string `json:"start_date"`
	PreviewHash     string `json:"preview_hash"`
	AcceptConflicts bool   `json:"accept_conflicts"`
	Reason          string `json:"reason"`
}
type PreviewDay struct {
	Date           string          `json:"date"`
	Timezone       string          `json:"timezone"`
	Items          []Item          `json:"items"`
	PlanVersion    int64           `json:"plan_version"`
	AlreadyApplied bool            `json:"already_applied"`
	Past           bool            `json:"past"`
	Confirmed      bool            `json:"confirmed"`
	Input          Input           `json:"input"`
	Actions        []RoutineAction `json:"actions"`
	Availability   Availability    `json:"availability"`
}
type RoutinePreview struct {
	Template  Routine      `json:"template"`
	StartDate string       `json:"start_date"`
	Days      []PreviewDay `json:"days"`
	Hash      string       `json:"hash"`
}

func builtins() []Routine {
	study := RoutineDay{Day: 1, AvailableMinutes: 180, Windows: []Window{{"09:00", "12:00"}}, Actions: []RoutineAction{{Key: "study", Title: "Учебная практика", Minutes: 60, Priority: "normal", Start: "09:00", End: "10:00"}}}
	work := RoutineDay{Day: 1, AvailableMinutes: 360, Windows: []Window{{"09:00", "12:00"}, {"13:00", "17:00"}}, Actions: []RoutineAction{{Key: "project", Title: "Главный рабочий шаг", Minutes: 90, Priority: "high", Start: "09:00", End: "10:30"}, {Key: "review", Title: "Подвести итоги работы", Minutes: 30, Priority: "normal", Start: "16:00", End: "16:30"}}}
	rest := RoutineDay{Day: 1, Rest: true, Windows: []Window{}, Actions: []RoutineAction{}}
	week := []RoutineDay{}
	for i := 1; i <= 7; i++ {
		d := study
		d.Day = i
		if i == 6 {
			d.AvailableMinutes = 60
			d.Actions = []RoutineAction{{Key: "personal", Title: "Личный проект", Minutes: 30, Priority: "normal", Start: "10:00", End: "10:30"}}
		}
		if i == 7 {
			d = rest
			d.Day = 7
		}
		week = append(week, d)
	}
	return []Routine{{RoutineInput: RoutineInput{"Учебный день", "day", []RoutineDay{study}}, ID: "study-day", Version: 1}, {RoutineInput: RoutineInput{"Рабочий день", "day", []RoutineDay{work}}, ID: "work-day", Version: 1}, {RoutineInput: RoutineInput{"Выходной", "day", []RoutineDay{rest}}, ID: "rest-day", Version: 1}, {RoutineInput: RoutineInput{"Идеальная неделя", "week", week}, ID: "ideal-week", Version: 1}}
}
func validateRoutine(in RoutineInput) (RoutineInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	if !utf8.ValidString(in.Title) || len([]rune(in.Title)) < 1 || len([]rune(in.Title)) > 200 || strings.ContainsRune(in.Title, 0) {
		return in, invalid("title", "Название шаблона: 1–200 символов")
	}
	if in.Period != "day" && in.Period != "week" {
		return in, invalid("period", "Выберите день или неделю")
	}
	if len(in.Days) < 1 || len(in.Days) > 7 || (in.Period == "day" && len(in.Days) != 1) {
		return in, invalid("days", "От 1 до 7 дней; для дневного шаблона один день")
	}
	seen := map[int]bool{}
	for i, d := range in.Days {
		if d.Day < 1 || d.Day > 7 || seen[d.Day] || (in.Period == "day" && d.Day != 1) {
			return in, invalid("days", "Дни недели от 1 до 7 без повторов")
		}
		seen[d.Day] = true
		v, e := Validate(Input{AvailableMinutes: d.AvailableMinutes, Windows: d.Windows, Rest: d.Rest})
		if e != nil {
			return in, e
		}
		in.Days[i].Windows = v.Windows
		if len(d.Actions) > 20 || (d.Rest && len(d.Actions) > 0) {
			return in, invalid("actions", "До 20 действий; день отдыха без обязательных действий")
		}
		keys := map[string]bool{}
		if d.Actions == nil {
			in.Days[i].Actions = []RoutineAction{}
		}
		for j, a := range d.Actions {
			if a.Key == "" || len(a.Key) > 64 || keys[a.Key] || strings.ContainsRune(a.Key, 0) {
				return in, invalid("actions", "Ключи действий не должны повторяться")
			}
			keys[a.Key] = true
			q, err := quests.Validate(quests.Input{Title: a.Title, EstimatedMinutes: a.Minutes, Priority: a.Priority, Difficulty: 1, Status: "todo", Required: true, Tags: []string{}})
			if err != nil {
				return in, invalid("actions", "Проверьте название, приоритет и длительность действия")
			}
			in.Days[i].Actions[j].Title = q.Title
			if (a.Start == "") != (a.End == "") {
				return in, invalid("actions", "Укажите обе границы блока или оставьте обе пустыми")
			}
			if a.Start != "" {
				start, e := minute(a.Start, false)
				if e != nil {
					return in, e
				}
				end, e := minute(a.End, true)
				if e != nil {
					return in, e
				}
				if end <= start {
					return in, invalid("actions", "Конец действия должен быть позже начала")
				}
			}
		}
	}
	return in, nil
}
func (s *Service) routine(ctx context.Context, tx pgx.Tx, id, key string) (Routine, error) {
	var v Routine
	var data []byte
	e := tx.QueryRow(ctx, `SELECT payload,version FROM routine_templates WHERE owner_id=$1 AND id=$2`, id, key).Scan(&data, &v.Version)
	v.ID = key
	if errors.Is(e, pgx.ErrNoRows) {
		for _, v := range builtins() {
			if v.ID == key {
				return v, nil
			}
		}
		return v, problem(404, "NOT_FOUND", "Шаблон не найден")
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(data, &v.RoutineInput)
	return v, e
}
func (s *Service) routineList(ctx context.Context, tx pgx.Tx, id string) ([]Routine, error) {
	out := builtins()
	rows, e := tx.Query(ctx, `SELECT id,payload,version FROM routine_templates WHERE owner_id=$1 ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Routine
		var data []byte
		if e = rows.Scan(&v.ID, &data, &v.Version); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(data, &v.RoutineInput); e != nil {
			return nil, e
		}
		replaced := false
		for i := range out {
			if out[i].ID == v.ID {
				out[i] = v
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}
func mergeRoutine(p Plan, d RoutineDay) Input {
	in := p.Input
	if p.ID == "" || (in.Rest && len(d.Actions) > 0) {
		in = emptyInput()
		in.AvailableMinutes = d.AvailableMinutes
		in.Windows = d.Windows
		in.Rest = d.Rest
	}
	return in
}
func (s *Service) previewRoutine(ctx context.Context, tx pgx.Tx, id, key, start string) (RoutinePreview, error) {
	out := RoutinePreview{StartDate: start, Days: []PreviewDay{}}
	d, e := day(start)
	if e != nil {
		return out, e
	}
	out.Template, e = s.routine(ctx, tx, id, key)
	if e != nil {
		return out, e
	}
	if out.Template.Period == "week" && d.Weekday() != time.Monday {
		return out, invalid("start_date", "Недельный шаблон начинается в понедельник")
	}
	for _, rd := range out.Template.Days {
		date := d
		if out.Template.Period == "week" {
			date = d.AddDate(0, 0, rd.Day-1)
		}
		p, e := s.get(ctx, tx, id, date.Format("2006-01-02"))
		if e != nil {
			return out, e
		}
		zone, e := time.LoadLocation(p.Timezone)
		if e != nil {
			return out, e
		}
		v := PreviewDay{Date: p.Date, Timezone: p.Timezone, Items: p.Items, PlanVersion: p.Version, Confirmed: p.ConfirmedAt != nil, Past: p.Date < s.clock.Now().In(zone).Format("2006-01-02"), Actions: rd.Actions, Input: mergeRoutine(p, rd)}
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM routine_applications WHERE owner_id=$1 AND template_id=$2 AND local_date=$3::text::date)`, id, key, p.Date).Scan(&v.AlreadyApplied); e != nil {
			return out, e
		}
		from, to, e := bounds(p.Date, p.Timezone)
		if e != nil {
			return out, e
		}
		blocks, e := s.calendar.PreviewTimedInTx(ctx, tx, id, date, date.AddDate(0, 0, 1), from, to)
		if e != nil {
			return out, e
		}
		if !v.AlreadyApplied && !v.Past {
			for _, a := range rd.Actions {
				h := sha256.Sum256([]byte(id + "/" + key + "/" + p.Date + "/" + a.Key))
				h[6] = (h[6] & 15) | 80
				h[8] = (h[8] & 63) | 128
				synthetic := fmt.Sprintf("%x-%x-%x-%x-%x", h[:4], h[4:6], h[6:8], h[8:10], h[10:16])
				v.Input.QuestIDs = append(append([]string{}, v.Input.QuestIDs...), synthetic)
				p.Items = append(p.Items, Item{ID: synthetic, Title: a.Title, EstimatedMinutes: a.Minutes})
				if a.Start != "" {
					start, e := wall(p.Date, p.Timezone, a.Start, false)
					if e != nil {
						return out, e
					}
					end, e := wall(p.Date, p.Timezone, a.End, true)
					if e != nil {
						return out, e
					}
					blocks = append(blocks, calendar.Event{ID: synthetic, Source: "block", Input: calendar.Input{Title: a.Title, QuestID: &synthetic, Start: start.UTC().Format(time.RFC3339), End: end.UTC().Format(time.RFC3339), Timezone: p.Timezone}})
				}
			}
		}
		p.Input = v.Input
		windows, e := windowsFor(p.Date, p.Timezone, p.Windows)
		if e != nil {
			return out, e
		}
		v.Availability, e = calculate(p, windows, blocks)
		if e != nil {
			return out, e
		}
		out.Days = append(out.Days, v)
	}
	encoded, e := json.Marshal(struct {
		Owner   string
		Preview RoutinePreview
	}{id, out})
	if e != nil {
		return out, e
	}
	hash := sha256.Sum256(encoded)
	out.Hash = hex.EncodeToString(hash[:])
	return out, nil
}
func (s *Service) applyRoutine(r *http.Request, tx pgx.Tx, id, key string, in RoutineRequest) (RoutinePreview, error) {
	preview, e := s.previewRoutine(r.Context(), tx, id, key, in.StartDate)
	if e != nil {
		return preview, e
	}
	if in.PreviewHash != preview.Hash {
		return preview, problem(409, "PREVIEW_STALE", "План или календарь изменились. Обновите предпросмотр")
	}
	for _, v := range preview.Days {
		if !v.AlreadyApplied && !v.Past && (len(v.Availability.Conflicts) > 0 || v.Availability.DeficitMinutes > 0) && !in.AcceptConflicts {
			return preview, invalid("accept_conflicts", "Подтвердите пересечения и дефицит времени или исправьте шаблон")
		}
	}
	data, e := json.Marshal(preview.Template.RoutineInput)
	if e != nil {
		return preview, e
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO routine_templates(owner_id,id,payload,version) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, key, data, preview.Template.Version); e != nil {
		return preview, e
	}
	for i, v := range preview.Days {
		if v.AlreadyApplied || v.Past {
			continue
		}
		p, e := s.get(r.Context(), tx, id, v.Date)
		if e != nil {
			return preview, e
		}
		before := p
		rd := preview.Template.Days[i]
		next := mergeRoutine(p, rd)
		for _, a := range rd.Actions {
			planned := p.Date
			q, e := s.quests.CreateInTx(r.Context(), tx, id, quests.Input{Title: a.Title, Priority: a.Priority, Difficulty: 1, Status: "todo", PlannedDate: &planned, EstimatedMinutes: a.Minutes, Required: true, Tags: []string{}})
			if e != nil {
				return preview, e
			}
			next.QuestIDs = append(append([]string{}, next.QuestIDs...), q.ID)
			if a.Start != "" {
				start, e := wall(p.Date, p.Timezone, a.Start, false)
				if e != nil {
					return preview, e
				}
				end, e := wall(p.Date, p.Timezone, a.End, true)
				if e != nil {
					return preview, e
				}
				if _, e = s.calendar.CreateInTx(r.Context(), tx, id, calendar.Input{Title: q.Title, QuestID: &q.ID, Start: start.UTC().Format(time.RFC3339), End: end.UTC().Format(time.RFC3339), Timezone: p.Timezone}); e != nil {
					return preview, e
				}
			}
		}
		valid, e := Validate(next)
		if e != nil {
			return preview, e
		}
		after, e := s.save(r.Context(), tx, id, p.Date, valid, p.Version, true)
		if e != nil {
			return preview, e
		}
		if e = s.journal(r.Context(), tx, id, before, after, "template", in.Reason); e != nil {
			return preview, e
		}
		if _, e = tx.Exec(r.Context(), `INSERT INTO routine_applications(owner_id,template_id,local_date,plan_id,template_version,created_at) VALUES($1,$2,$3::text::date,$4,$5,$6)`, id, key, p.Date, after.ID, preview.Template.Version, s.clock.Now().UTC()); e != nil {
			return preview, e
		}
	}
	return s.previewRoutine(r.Context(), tx, id, key, in.StartDate)
}
func (s *Service) mountRoutines(r chi.Router) {
	r.Get("/api/v1/templates/routines", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		var out []Routine
		if e == nil {
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error { var err error; out, err = s.routineList(r.Context(), tx, id); return err })
		}
		s.reply(w, r, out, e)
	})
	r.Post("/api/v1/templates/routines", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in RoutineInput) (any, error) {
			valid, e := validateRoutine(in)
			if e != nil {
				return nil, e
			}
			return s.mutate(r, valid, func(tx pgx.Tx, id string) (any, error) {
				var key string
				if e := tx.QueryRow(r.Context(), `SELECT gen_random_uuid()::text`).Scan(&key); e != nil {
					return nil, e
				}
				data, e := json.Marshal(valid)
				if e != nil {
					return nil, e
				}
				var count int
				if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM routine_templates WHERE owner_id=$1`, id).Scan(&count); e != nil {
					return nil, e
				}
				if count >= 100 {
					return nil, invalid("title", "До 100 собственных шаблонов")
				}
				if _, e = tx.Exec(r.Context(), `INSERT INTO routine_templates(owner_id,id,payload) VALUES($1,$2,$3)`, id, key, data); e != nil {
					return nil, e
				}
				return s.routine(r.Context(), tx, id, key)
			})
		})
	})
	r.Patch("/api/v1/templates/routines/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in RoutineInput) (any, error) {
			valid, e := validateRoutine(in)
			if e != nil {
				return nil, e
			}
			key := chi.URLParam(r, "id")
			return s.mutate(r, valid, func(tx pgx.Tx, id string) (any, error) {
				old, e := s.routine(r.Context(), tx, id, key)
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, old.Version); p != nil {
					return nil, p
				}
				data, e := json.Marshal(valid)
				if e != nil {
					return nil, e
				}
				if _, e = tx.Exec(r.Context(), `INSERT INTO routine_templates(owner_id,id,payload,version) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,id) DO UPDATE SET payload=excluded.payload,version=excluded.version`, id, key, data, old.Version+1); e != nil {
					return nil, e
				}
				return s.routine(r.Context(), tx, id, key)
			})
		})
	})
	r.Post("/api/v1/templates/routines/{id}/preview", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in struct {
			StartDate string `json:"start_date"`
		}) (any, error) {
			id, e := owner(r.Context())
			var out RoutinePreview
			if e == nil {
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					if e := s.lock(r.Context(), tx, id); e != nil {
						return e
					}
					var err error
					out, err = s.previewRoutine(r.Context(), tx, id, chi.URLParam(r, "id"), in.StartDate)
					return err
				})
			}
			return out, e
		})
	})
	r.Post("/api/v1/templates/routines/{id}/apply", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in RoutineRequest) (any, error) {
			if e := reason(in.Reason); e != nil {
				return nil, e
			}
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) { return s.applyRoutine(r, tx, id, chi.URLParam(r, "id"), in) })
		})
	})
}
