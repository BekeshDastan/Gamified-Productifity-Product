package quests

import (
	"bytes"
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/blob"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxFile = 10 << 20

var uploadGate = make(chan struct{}, 2)

type Attachment struct {
	ID        string       `json:"id"`
	QuestID   string       `json:"quest_id"`
	Name      string       `json:"name"`
	MIME      string       `json:"mime"`
	Size      int64        `json:"size"`
	Verdict   blob.Verdict `json:"verdict"`
	Version   int64        `json:"version"`
	CreatedAt time.Time    `json:"created_at"`
	key       string
	checksum  string
}

const attachmentColumns = `id::text,quest_id::text,name,mime,size,verdict,version,created_at,storage_key,checksum`

func scanAttachment(row pgx.Row) (Attachment, error) {
	var a Attachment
	e := row.Scan(&a.ID, &a.QuestID, &a.Name, &a.MIME, &a.Size, &a.Verdict, &a.Version, &a.CreatedAt, &a.key, &a.checksum)
	return a, e
}
func attachment(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id, key string) (Attachment, error) {
	if !uuid(key) {
		return Attachment{}, problem(404, "NOT_FOUND", "Файл не найден")
	}
	a, e := scanAttachment(tx.QueryRow(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE owner_id=$1 AND id=$2 AND deleted_at IS NULL`, id, key))
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Файл не найден")
	}
	return a, e
}
func validateFile(name string, data []byte) (string, error) {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 180 || name != path.Base(name) || strings.ContainsAny(name, `\/:`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", invalid("file", "Недопустимое имя файла")
	}
	if len(data) < 1 || len(data) > maxFile {
		return "", invalid("file", "Файл: от 1 байта до 10 МБ")
	}
	kind := http.DetectContentType(data)
	ext := strings.ToLower(path.Ext(name))
	expected := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".pdf": "application/pdf", ".txt": "text/plain; charset=utf-8"}[ext]
	if expected == "" || kind != expected {
		return "", invalid("file", "Разрешены PNG, JPEG, WebP, PDF и UTF-8 TXT; расширение должно совпадать с содержимым")
	}
	if strings.HasPrefix(kind, "image/") {
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 16_000_000 {
			return "", invalid("file", "Изображение повреждено или больше 16 млн пикселей")
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return "", invalid("file", "Изображение повреждено")
		}
	}
	if ext == ".txt" && (!utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0) {
		return "", invalid("file", "Текст должен быть в UTF-8")
	}
	return kind, nil
}
func (s *Service) upload(r *http.Request) (Attachment, error) {
	id, e := owner(r.Context())
	if e != nil {
		return Attachment{}, e
	}
	if s.files == nil || s.scanner == nil {
		return Attachment{}, problem(503, "UPLOAD_UNAVAILABLE", "Загрузка файлов временно недоступна")
	}
	select {
	case uploadGate <- struct{}{}:
		defer func() { <-uploadGate }()
	default:
		return Attachment{}, problem(429, "UPLOAD_BUSY", "Дождитесь окончания текущей загрузки")
	}
	reader, e := r.MultipartReader()
	if e != nil {
		return Attachment{}, invalid("file", "Ожидается файл")
	}
	fields := map[string]string{}
	var data []byte
	name := ""
	for count := 0; ; count++ {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil || count >= 3 {
			return Attachment{}, invalid("file", "Некорректная загрузка")
		}
		switch part.FormName() {
		case "quest_id", "version":
			if _, exists := fields[part.FormName()]; exists {
				return Attachment{}, invalid("file", "Повтор поля")
			}
			value, e := io.ReadAll(io.LimitReader(part, 101))
			if e != nil || len(value) > 100 {
				return Attachment{}, invalid("file", "Некорректное поле")
			}
			fields[part.FormName()] = string(value)
		case "file":
			if name != "" {
				return Attachment{}, invalid("file", "Один файл за запрос")
			}
			// Inspect the original multipart filename: FileName() alone strips paths.
			_, params, e := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if e != nil {
				return Attachment{}, invalid("file", "Некорректное имя")
			}
			name = params["filename"]
			data, e = io.ReadAll(io.LimitReader(part, maxFile+1))
			if e != nil {
				return Attachment{}, invalid("file", "Загрузка прервана")
			}
		default:
			return Attachment{}, invalid("file", "Неизвестное поле")
		}
		_ = part.Close()
	}
	key := fields["quest_id"]
	if !uuid(key) {
		return Attachment{}, invalid("quest_id", "Выберите задачу")
	}
	// Version is a multipart field to avoid exposing mutable metadata in URLs.
	r.Header.Set("If-Match", fields["version"])
	kind, e := validateFile(name, data)
	if e != nil {
		return Attachment{}, e
	}
	var a Attachment
	var object blob.Object
	committed := false
	defer func() {
		if !committed && object.Key != "" {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
			defer cancel()
			_ = s.files.Delete(ctx, object.Key)
		}
	}()
	e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		if e := s.lockOwner(r.Context(), tx, id); e != nil {
			return e
		}
		q, e := get(r.Context(), tx, id, key, true)
		if e != nil {
			return e
		}
		if p := platform.MatchVersion(r, q.Version); p != nil {
			return p
		}
		if q.ArchivedAt != nil {
			return problem(409, "ARCHIVED", "Сначала восстановите задачу")
		}
		var files int
		var used int64
		e = tx.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE quest_id=$2),COALESCE(sum(size),0) FROM attachments WHERE owner_id=$1 AND deleted_at IS NULL`, id, key).Scan(&files, &used)
		if e != nil {
			return e
		}
		if files >= 5 || used+int64(len(data)) > 250<<20 {
			return invalid("file", "Лимит: 5 файлов на задачу и 250 МБ на аккаунт")
		}
		object, e = s.files.Put(r.Context(), bytes.NewReader(data), maxFile)
		if e != nil {
			return e
		}
		a, e = scanAttachment(tx.QueryRow(r.Context(), `INSERT INTO attachments(owner_id,quest_id,storage_key,name,mime,size,checksum,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+attachmentColumns, id, key, object.Key, name, kind, object.Size, object.SHA256, s.clock.Now().UTC()))
		if e != nil {
			return e
		}
		updated, e := scan(tx.QueryRow(r.Context(), `UPDATE quests SET version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, key, s.clock.Now().UTC()))
		if e == nil {
			e = s.event(r.Context(), tx, id, "attachment_added", name, &q, &updated)
		}
		return e
	})
	if e != nil {
		return Attachment{}, e
	}
	committed = true
	result, err := s.scanFile(r.Context(), id, a)
	var conflict *platform.APIError
	if errors.As(err, &conflict) && conflict.Code == "VERSION_CONFLICT" {
		return attachment(r.Context(), s.pool, id, a.ID)
	}
	return result, err
}
func (s *Service) scanFile(ctx context.Context, id string, a Attachment) (Attachment, error) {
	if s.scanner == nil {
		return a, nil
	}
	verdict, e := s.scanner.Scan(ctx, blob.Object{Key: a.key, Size: a.Size, SHA256: a.checksum})
	if e != nil || verdict == blob.Pending {
		return a, nil
	}
	if verdict != blob.Clean && verdict != blob.Rejected {
		return a, nil
	}
	result, err := scanAttachment(s.pool.QueryRow(ctx, `UPDATE attachments SET verdict=$3,version=version+1 WHERE owner_id=$1 AND id=$2 AND version=$4 AND deleted_at IS NULL RETURNING `+attachmentColumns, id, a.ID, verdict, a.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		fresh, e := attachment(ctx, s.pool, id, a.ID)
		if e != nil {
			return a, e
		}
		return a, &platform.APIError{Status: 409, Code: "VERSION_CONFLICT", Message: "Файл уже изменён", CurrentVersion: &fresh.Version}
	}
	return result, err
}
func (s *Service) mountAttachments(r chi.Router) {
	r.Post("/api/v1/attachments", func(w http.ResponseWriter, r *http.Request) { a, e := s.upload(r); s.reply(w, r, a, e) })
	r.Get("/api/v1/quests/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		key := chi.URLParam(r, "id")
		if _, e = get(r.Context(), s.pool, id, key, false); e != nil {
			s.reply(w, r, nil, e)
			return
		}
		rows, e := s.pool.Query(r.Context(), `SELECT `+attachmentColumns+` FROM attachments WHERE owner_id=$1 AND quest_id=$2 AND deleted_at IS NULL ORDER BY created_at,id`, id, key)
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		defer rows.Close()
		out := []Attachment{}
		for rows.Next() {
			a, e := scanAttachment(rows)
			if e != nil {
				s.reply(w, r, nil, e)
				return
			}
			out = append(out, a)
		}
		s.reply(w, r, out, rows.Err())
	})
	r.Post("/api/v1/attachments/{id}/scan", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in struct{}) (any, error) {
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			a, e := attachment(r.Context(), s.pool, id, chi.URLParam(r, "id"))
			if e != nil {
				return nil, e
			}
			if p := platform.MatchVersion(r, a.Version); p != nil {
				return nil, p
			}
			if a.Verdict != blob.Pending {
				return a, nil
			}
			return s.scanFile(r.Context(), id, a)
		}, s)
	})
	r.Get("/api/v1/attachments/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		a, e := attachment(r.Context(), s.pool, id, chi.URLParam(r, "id"))
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		if a.Verdict != blob.Clean {
			s.reply(w, r, nil, problem(409, "FILE_QUARANTINED", "Файл ожидает проверки или отклонён"))
			return
		}
		if s.files == nil {
			s.reply(w, r, nil, problem(503, "UPLOAD_UNAVAILABLE", "Файл временно недоступен"))
			return
		}
		reader, e := s.files.Get(r.Context(), a.key)
		if e != nil {
			s.reply(w, r, nil, problem(404, "NOT_FOUND", "Файл не найден"))
			return
		}
		defer reader.Close()
		w.Header().Set("Content-Type", a.MIME)
		w.Header().Set("Content-Length", strconv.FormatInt(a.Size, 10))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Name}))
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		_, _ = io.Copy(w, io.LimitReader(reader, a.Size))
	})
	r.Delete("/api/v1/attachments/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in struct{}) (any, error) {
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			var a Attachment
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				if e := s.lockOwner(r.Context(), tx, id); e != nil {
					return e
				}
				var e error
				a, e = attachment(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return e
				}
				if p := platform.MatchVersion(r, a.Version); p != nil {
					return p
				}
				q, e := get(r.Context(), tx, id, a.QuestID, true)
				if e != nil {
					return e
				}
				if _, e = tx.Exec(r.Context(), `UPDATE attachments SET deleted_at=$3,version=version+1 WHERE owner_id=$1 AND id=$2`, id, a.ID, s.clock.Now().UTC()); e != nil {
					return e
				}
				updated, e := scan(tx.QueryRow(r.Context(), `UPDATE quests SET version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, q.ID, s.clock.Now().UTC()))
				if e == nil {
					e = s.event(r.Context(), tx, id, "attachment_removed", a.Name, &q, &updated)
				}
				return e
			})
			if e == nil && s.files != nil {
				_ = s.files.Delete(r.Context(), a.key)
			}
			return struct{}{}, e
		}, s)
	})
}
