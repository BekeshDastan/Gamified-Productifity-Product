package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type ChannelPolicy struct {
	Type  string `json:"type"`
	InApp bool   `json:"in_app"`
	Email bool   `json:"email"`
	Push  bool   `json:"push"`
}
type QuietWindow struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"`
	End     string `json:"end"`
}
type PreferencesInput struct {
	Timezone            string          `json:"timezone"`
	UseProfileTimezone  bool            `json:"use_profile_timezone"`
	Quiet               QuietWindow     `json:"quiet"`
	DailyCap            int             `json:"daily_cap"`
	DeadlineLeadMinutes int             `json:"deadline_lead_minutes"`
	HabitLeadMinutes    int             `json:"habit_lead_minutes"`
	Channels            []ChannelPolicy `json:"channels"`
}
type Preferences struct {
	PreferencesInput
	Version           int64      `json:"version"`
	EffectiveTimezone string     `json:"effective_timezone"`
	UpdatedAt         *time.Time `json:"updated_at"`
}

func DefaultPreferences() PreferencesInput {
	out := PreferencesInput{Timezone: "UTC", UseProfileTimezone: true, Quiet: QuietWindow{true, "22:00", "08:00"}, DailyCap: 10, DeadlineLeadMinutes: 60, HabitLeadMinutes: 30, Channels: []ChannelPolicy{}}
	for _, kind := range []string{"deadline", "habit", "achievement", "invite", "report", "focus_result"} {
		out.Channels = append(out.Channels, ChannelPolicy{Type: kind, InApp: true})
	}
	return out
}
func minute(value string) (int, error) {
	v, e := time.Parse("15:04", value)
	if e != nil || v.Format("15:04") != value {
		return 0, privateapi.Invalid("Время тихих часов: ЧЧ:ММ")
	}
	return v.Hour()*60 + v.Minute(), nil
}
func ValidatePreferences(in PreferencesInput) error {
	if in.Timezone == "" || in.Timezone == "Local" {
		return privateapi.Invalid("Выберите часовой пояс")
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return privateapi.Invalid("Неизвестный часовой пояс")
	}
	if _, e := minute(in.Quiet.Start); e != nil {
		return e
	}
	if _, e := minute(in.Quiet.End); e != nil {
		return e
	}
	if in.Quiet.Enabled && in.Quiet.Start == in.Quiet.End {
		return privateapi.Invalid("Начало и конец тихих часов должны различаться")
	}
	if in.DailyCap < 0 || in.DailyCap > 50 || in.DeadlineLeadMinutes < 0 || in.DeadlineLeadMinutes > 10080 || in.HabitLeadMinutes < 0 || in.HabitLeadMinutes > 1440 || len(in.Channels) != len(titles) {
		return privateapi.Invalid("Проверьте каналы, частоту и время напоминаний")
	}
	seen := map[string]bool{}
	for _, c := range in.Channels {
		if _, ok := titles[c.Type]; !ok || seen[c.Type] {
			return privateapi.Invalid("Укажите каждый тип уведомления один раз")
		}
		seen[c.Type] = true
	}
	return nil
}
func (p PreferencesInput) Channel(kind string) ChannelPolicy {
	for _, c := range p.Channels {
		if c.Type == kind {
			return c
		}
	}
	return ChannelPolicy{Type: kind}
}
func (s *Service) preferences(ctx context.Context, tx pgx.Tx, owner string) (Preferences, error) {
	out := Preferences{PreferencesInput: DefaultPreferences()}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT payload,version,updated_at FROM notification_preferences WHERE owner_id=$1`, owner).Scan(&raw, &out.Version, &out.UpdatedAt)
	if e == nil {
		if e = json.Unmarshal(raw, &out.PreferencesInput); e != nil {
			return out, e
		}
		if e = ValidatePreferences(out.PreferencesInput); e != nil {
			return out, e
		}
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	out.EffectiveTimezone = out.Timezone
	if out.UseProfileTimezone {
		if e = tx.QueryRow(ctx, `SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&out.EffectiveTimezone); e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *Service) savePreferences(r *http.Request, tx pgx.Tx, owner string, in PreferencesInput) (Preferences, error) {
	if e := ValidatePreferences(in); e != nil {
		return Preferences{}, e
	}
	old, e := s.preferences(r.Context(), tx, owner)
	if e != nil {
		return old, e
	}
	if e := privateapi.MatchVersion(r, old.Version); e != nil {
		return old, e
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return old, e
	}
	now := s.Clock.Now()
	_, e = tx.Exec(r.Context(), `INSERT INTO notification_preferences(owner_id,payload,version,updated_at) VALUES($1,$2,1,$3) ON CONFLICT(owner_id) DO UPDATE SET payload=excluded.payload,version=notification_preferences.version+1,updated_at=excluded.updated_at`, owner, raw, now)
	if e != nil {
		return old, e
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO notification_preference_events(owner_id,version,snapshot,created_at) VALUES($1,$2,$3,$4)`, owner, old.Version+1, raw, now)
	if e != nil {
		return old, e
	}
	return s.preferences(r.Context(), tx, owner)
}
func (s *Service) mountPreferences(r chi.Router) {
	r.Get("/api/v1/notifications/preferences", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.preferences(ctx, tx, owner) })
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/notifications/preferences", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in PreferencesInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.savePreferences(r, tx, owner, in)
			})
		})
	})
}

// QuietUntil returns the next allowed instant using wall dates, including folds
// and missing hours. urgent is accepted only for an explicitly selected deadline.
func QuietUntil(now time.Time, timezone string, window QuietWindow, urgent bool) (time.Time, error) {
	if !window.Enabled || urgent {
		return now, nil
	}
	start, e := minute(window.Start)
	if e != nil {
		return now, e
	}
	end, e := minute(window.End)
	if e != nil {
		return now, e
	}
	if start == end {
		return now, privateapi.Invalid("Тихие часы должны иметь разные границы")
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return now, e
	}
	local := now.In(zone)
	m := local.Hour()*60 + local.Minute()
	inside := m >= start && m < end
	if start > end {
		inside = m >= start || m < end
	}
	if !inside {
		return now, nil
	}
	target := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if start > end && m >= start {
		target = target.AddDate(0, 0, 1)
	}
	// UTC iteration deliberately picks the first valid end after now. During a
	// repeated hour an earlier occurrence may already have passed; during a gap
	// the first valid wall minute after the requested end is used.
	for candidate := target.Add(-16 * time.Hour); candidate.Before(target.Add(56 * time.Hour)); candidate = candidate.Add(time.Minute) {
		if candidate.Before(now) {
			continue
		}
		wall := candidate.In(zone)
		if wall.Year() == target.Year() && wall.Month() == target.Month() && wall.Day() == target.Day() && wall.Hour()*60+wall.Minute() >= end {
			return candidate, nil
		}
	}
	return now, privateapi.Invalid("Не удалось определить конец тихих часов")
}
func PolicyTime(now, expires time.Time, p Preferences, urgent bool, used int, oldest *time.Time) (time.Time, bool, error) {
	if p.DailyCap == 0 {
		return now, false, nil
	}
	allowed, e := QuietUntil(now, p.EffectiveTimezone, p.Quiet, urgent)
	if e != nil {
		return now, false, e
	}
	if used >= p.DailyCap {
		if oldest == nil {
			return now, false, nil
		}
		capEnd := oldest.Add(24 * time.Hour)
		if capEnd.After(allowed) {
			allowed = capEnd
		}
		allowed, e = QuietUntil(allowed, p.EffectiveTimezone, p.Quiet, urgent)
		if e != nil {
			return now, false, e
		}
	}
	if !expires.IsZero() && !allowed.Before(expires) {
		return allowed, false, nil
	}
	return allowed, true, nil
}
