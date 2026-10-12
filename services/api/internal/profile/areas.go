package profile

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type LabelInput struct {
	Name     string `json:"name"`
	Color    string `json:"color"`
	Icon     string `json:"icon"`
	Archived bool   `json:"archived"`
}
type Label struct {
	LabelInput
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func AreaCatalog() []LabelInput {
	return []LabelInput{
		{Name: "Учёба", Color: "#6478c8", Icon: "book"}, {Name: "Карьера", Color: "#c78748", Icon: "briefcase"},
		{Name: "Программирование", Color: "#588f85", Icon: "code"}, {Name: "Языки", Color: "#9a70b8", Icon: "languages"},
		{Name: "Активность", Color: "#bd6c72", Icon: "activity"}, {Name: "Личные проекты", Color: "#598ab3", Icon: "rocket"},
		{Name: "Отношения", Color: "#b56d96", Icon: "heart"}, {Name: "Отдых", Color: "#6f915c", Icon: "sprout"},
	}
}

var colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func validateLabel(input LabelInput) (LabelInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Color = strings.ToLower(input.Color)
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 60 || strings.IndexFunc(input.Name, unicode.IsControl) >= 0 {
		return input, invalid("name", "Название: от 1 до 60 символов")
	}
	if !colorPattern.MatchString(input.Color) {
		return input, invalid("color", "Выберите цвет")
	}
	switch input.Icon {
	case "book", "briefcase", "code", "languages", "activity", "rocket", "heart", "sprout", "tag":
	default:
		return input, invalid("icon", "Выберите значок из каталога")
	}
	return input, nil
}

// Only server-owned table names enter SQL; URL parameters never become identifiers.
func labelTable(kind string) string {
	if kind == "areas" {
		return "life_areas"
	}
	if kind == "tags" {
		return "tags"
	}
	panic("invalid label kind")
}

const labelColumns = `name,color,icon,archived,id::text,version,created_at,updated_at`

func scanLabel(row pgx.Row) (Label, error) {
	var value Label
	err := row.Scan(&value.Name, &value.Color, &value.Icon, &value.Archived, &value.ID, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}
func (s *Service) ListLabels(ctx context.Context, kind string) ([]Label, error) {
	id, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+labelColumns+` FROM `+labelTable(kind)+` WHERE owner_id=$1 ORDER BY archived,name,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Label{}
	for rows.Next() {
		item, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func labelMissing() *platform.APIError {
	return &platform.APIError{Status: 404, Code: "NOT_FOUND", Message: "Запись не найдена"}
}
func (s *Service) SaveLabel(r *http.Request, kind, labelID string, input LabelInput, archiveOnly bool) (Label, error) {
	id, err := owner(r.Context())
	if err != nil {
		return Label{}, err
	}
	if labelID != "" {
		var uuid pgtype.UUID
		if uuid.Scan(labelID) != nil || !uuid.Valid {
			return Label{}, labelMissing()
		}
	}
	if !archiveOnly {
		input, err = validateLabel(input)
		if err != nil {
			return Label{}, err
		}
	}
	table := labelTable(kind)
	var result Label
	err = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		// Serialize count/check/write for a single owner, including unarchive.
		if _, err := tx.Exec(r.Context(), `SELECT owner_id FROM profiles WHERE owner_id=$1 FOR UPDATE`, id); err != nil {
			return err
		}
		if labelID != "" {
			current, err := scanLabel(tx.QueryRow(r.Context(), `SELECT `+labelColumns+` FROM `+table+` WHERE owner_id=$1 AND id=$2 FOR UPDATE`, id, labelID))
			if errors.Is(err, pgx.ErrNoRows) {
				return labelMissing()
			}
			if err != nil {
				return err
			}
			if problem := platform.MatchVersion(r, current.Version); problem != nil {
				return problem
			}
			if archiveOnly {
				input = current.LabelInput
				input.Archived = true
			}
		}
		if kind == "areas" && !input.Archived {
			var count int
			if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM life_areas WHERE owner_id=$1 AND NOT archived AND ($2='' OR id::text<>$2)`, id, labelID).Scan(&count); err != nil {
				return err
			}
			if count >= 12 {
				return invalid("name", "Можно иметь до 12 активных направлений. Сначала перенесите одно в архив.")
			}
		}
		now := s.clock.Now().UTC()
		if labelID == "" {
			result, err = scanLabel(tx.QueryRow(r.Context(), `INSERT INTO `+table+`(owner_id,name,color,icon,archived,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$6) RETURNING `+labelColumns, id, input.Name, input.Color, input.Icon, input.Archived, now))
		} else {
			result, err = scanLabel(tx.QueryRow(r.Context(), `UPDATE `+table+` SET name=$3,color=$4,icon=$5,archived=$6,version=version+1,updated_at=$7 WHERE owner_id=$1 AND id=$2 RETURNING `+labelColumns, id, labelID, input.Name, input.Color, input.Icon, input.Archived, now))
		}
		if err != nil {
			var db *pgconn.PgError
			if errors.As(err, &db) && db.Code == "23505" {
				return invalid("name", "Такое название уже есть, в том числе в архиве")
			}
			return err
		}
		return audit(r.Context(), tx, id, kind+".saved", table, result.ID)
	})
	return result, err
}
func (s *Service) mountAreas(r chi.Router) {
	r.Get("/api/v1/catalog/areas", func(w http.ResponseWriter, r *http.Request) { s.reply(w, r, AreaCatalog(), nil) })
	for _, kind := range []string{"areas", "tags"} {
		path := "/api/v1/" + kind
		r.Get(path, func(w http.ResponseWriter, r *http.Request) {
			value, err := s.ListLabels(r.Context(), kind)
			s.reply(w, r, value, err)
		})
		save := func(w http.ResponseWriter, r *http.Request) {
			input, problem := platform.DecodeJSON[LabelInput](r)
			if problem != nil {
				platform.WriteError(w, r, problem)
				return
			}
			value, err := s.SaveLabel(r, kind, chi.URLParam(r, "id"), input, false)
			s.reply(w, r, value, err)
		}
		r.Post(path, save)
		r.Patch(path+"/{id}", save)
		r.Delete(path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			_, problem := platform.DecodeJSON[struct{}](r)
			if problem != nil {
				platform.WriteError(w, r, problem)
				return
			}
			value, err := s.SaveLabel(r, kind, chi.URLParam(r, "id"), LabelInput{}, true)
			s.reply(w, r, value, err)
		})
	}
}
