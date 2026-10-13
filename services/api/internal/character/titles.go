package character

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
)

type Title struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Requirement string `json:"requirement"`
	Earned      bool   `json:"earned"`
}
type titleBand struct {
	level int64
	name  string
}

var titleBands = []titleBand{{1, "Новичок"}, {5, "Ученик"}, {10, "Последователь"}, {20, "Воин"}, {35, "Мастер"}, {50, "Легенда"}}

func levelTitle(level int64) string {
	result := titleBands[0].name
	for _, band := range titleBands {
		if level >= band.level {
			result = band.name
		}
	}
	return result
}
func (s *Service) titles(ctx context.Context, tx pgx.Tx, owner string, level int64) ([]Title, error) {
	out := []Title{{"level:auto", levelTitle(level) + " · автоматически", "Титул текущего уровня", true}}
	for _, band := range titleBands {
		out = append(out, Title{fmtTitleID(band.level), band.name, "Уровень " + fmtNumber(band.level), level >= band.level})
	}
	for _, special := range []struct{ id, name, requirement string }{{"task-1", "Первый шаг", "Достижение «Первый шаг»"}, {"streak-7", "Хранитель ритма", "Достижение «Устойчивость»"}, {"comeback-1", "Возвращающийся", "Достижение «Возвращение»"}} {
		active, e := s.achievementActive(ctx, tx, owner, special.id)
		if e != nil {
			return out, e
		}
		out = append(out, Title{"achievement:" + special.id, special.name, special.requirement, active})
	}
	var victory bool
	if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM cosmetic_permanent_grants WHERE owner_id=$1 AND source_kind='boss' AND active)", owner).Scan(&victory); e != nil {
		return out, e
	}
	out = append(out, Title{"boss:victory", "Победитель босса", "Подтверждённая победа над боссом цели", victory})
	return out, nil
}
func entitledTitle(id string, titles []Title) bool {
	for _, t := range titles {
		if t.ID == id {
			return t.Earned
		}
	}
	return false
}
func (s *Service) mountTitles(r chi.Router) {
	r.Patch("/api/v1/character/title", func(w http.ResponseWriter, r *http.Request) {
		in, p := platform.DecodeJSON[struct {
			TitleID string `json:"title_id"`
		}](r)
		if p != nil {
			platform.WriteError(w, r, p)
			return
		}
		out, e := s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string, p Profile) error {
			if !entitledTitle(in.TitleID, p.Titles) {
				return apiError(409, "TITLE_LOCKED", "Этот титул ещё не получен")
			}
			_, e := tx.Exec(ctx, "UPDATE character_profiles SET title_id=$2 WHERE owner_id=$1", owner, in.TitleID)
			return e
		})
		s.reply(w, r, out, e)
	})
}

func fmtTitleID(n int64) string { return "level:" + fmtNumber(n) }
func fmtNumber(n int64) string  { return strconv.FormatInt(n, 10) }
