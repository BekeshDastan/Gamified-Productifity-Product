package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type WorkInterval struct {
	Day   int    `json:"day"`
	Start string `json:"start"`
	End   string `json:"end"`
}
type Preferences struct {
	Privacy       Privacy        `json:"privacy"`
	Timezone      string         `json:"timezone"`
	Language      string         `json:"language"`
	WeekStart     int            `json:"week_start"`
	TimeFormat    string         `json:"time_format"`
	WorkIntervals []WorkInterval `json:"work_intervals"`
	RestDays      []int          `json:"rest_days"`
}
type Privacy struct {
	ProfileVisibility string `json:"profile_visibility"`
	ShareActivity     bool   `json:"share_activity"`
	ShareAchievements bool   `json:"share_achievements"`
	Discoverable      bool   `json:"discoverable"`
}

func DefaultPrivacy() Privacy { return Privacy{ProfileVisibility: "private"} }

type Settings struct {
	Preferences
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var timePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func ValidatePreferences(p Preferences) error {
	if p.Privacy.ProfileVisibility != "private" && p.Privacy.ProfileVisibility != "friends" {
		return invalid("privacy.profile_visibility", "Выберите закрытый профиль или будущий доступ друзьям")
	}
	if len(p.Timezone) > 100 || p.Timezone == "Local" || strings.Contains(p.Timezone, "..") || (p.Timezone != "UTC" && !strings.Contains(p.Timezone, "/")) {
		return invalid("timezone", "Выберите IANA-часовой пояс")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return invalid("timezone", "Неизвестный часовой пояс")
	}
	if p.Language != "ru" && p.Language != "en" {
		return invalid("language", "Выберите язык дат")
	}
	if p.WeekStart != 1 && p.WeekStart != 7 {
		return invalid("week_start", "Выберите понедельник или воскресенье")
	}
	if p.TimeFormat != "24h" && p.TimeFormat != "12h" {
		return invalid("time_format", "Выберите формат времени")
	}
	if p.WorkIntervals == nil || len(p.WorkIntervals) > 28 {
		return invalid("work_intervals", "До 28 рабочих интервалов")
	}
	if p.RestDays == nil || len(p.RestDays) > 7 {
		return invalid("rest_days", "Укажите дни отдыха")
	}
	rest := map[int]bool{}
	for _, day := range p.RestDays {
		if day < 1 || day > 7 || rest[day] {
			return invalid("rest_days", "Дни от 1 до 7 без повторов")
		}
		rest[day] = true
	}
	sorted := append([]WorkInterval{}, p.WorkIntervals...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Day != sorted[j].Day {
			return sorted[i].Day < sorted[j].Day
		}
		return sorted[i].Start < sorted[j].Start
	})
	for i, item := range sorted {
		if item.Day < 1 || item.Day > 7 || rest[item.Day] || !timePattern.MatchString(item.Start) || !timePattern.MatchString(item.End) || item.Start >= item.End {
			return invalid("work_intervals", "Интервал должен заканчиваться позже начала в тот же рабочий день")
		}
		if i > 0 && sorted[i-1].Day == item.Day && sorted[i-1].End > item.Start {
			return invalid("work_intervals", "Рабочие интервалы не должны пересекаться")
		}
	}
	return nil
}

const settingsColumns = `preferences,version,created_at,updated_at`

func scanSettings(row pgx.Row) (Settings, error) {
	var value Settings
	var data []byte
	if err := row.Scan(&data, &value.Version, &value.CreatedAt, &value.UpdatedAt); err != nil {
		return value, err
	}
	err := json.Unmarshal(data, &value.Preferences)
	return value, err
}
func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	id, err := owner(ctx)
	if err != nil {
		return Settings{}, err
	}
	return scanSettings(s.pool.QueryRow(ctx, `SELECT `+settingsColumns+` FROM user_settings WHERE owner_id=$1`, id))
}
func (s *Service) UpdateSettings(r *http.Request, input Preferences) (Settings, error) {
	id, err := owner(r.Context())
	if err != nil {
		return Settings{}, err
	}
	if err := ValidatePreferences(input); err != nil {
		return Settings{}, err
	}
	var result Settings
	err = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		current, err := scanSettings(tx.QueryRow(r.Context(), `SELECT `+settingsColumns+` FROM user_settings WHERE owner_id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if problem := platform.MatchVersion(r, current.Version); problem != nil {
			return problem
		}
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		result, err = scanSettings(tx.QueryRow(r.Context(), `UPDATE user_settings SET preferences=$2,version=version+1,updated_at=$3 WHERE owner_id=$1 RETURNING `+settingsColumns, id, data, s.clock.Now().UTC()))
		if err != nil {
			return err
		}
		return audit(r.Context(), tx, id, "settings.updated", "user_settings", id)
	})
	return result, err
}
func (s *Service) mountSettings(r chi.Router) {
	r.Get("/api/v1/me/settings", func(w http.ResponseWriter, r *http.Request) {
		value, err := s.GetSettings(r.Context())
		s.reply(w, r, value, err)
	})
	r.Patch("/api/v1/me/settings", func(w http.ResponseWriter, r *http.Request) {
		input, problem := platform.DecodeJSON[Preferences](r)
		if problem != nil {
			platform.WriteError(w, r, problem)
			return
		}
		value, err := s.UpdateSettings(r, input)
		s.reply(w, r, value, err)
	})
}
