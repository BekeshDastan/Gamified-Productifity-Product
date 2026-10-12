package platform

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"unicode/utf8"
)

const MaxBodyBytes = 1 << 20

func problem(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

func DecodeJSON[T any](r *http.Request) (T, *APIError) {
	var dto T
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return dto, problem(415, "UNSUPPORTED_MEDIA_TYPE", "Используйте application/json")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	var tooLarge *http.MaxBytesError
	if len(body) > MaxBodyBytes || errors.As(err, &tooLarge) {
		return dto, problem(413, "BODY_TOO_LARGE", "Запрос слишком большой")
	}
	if err != nil || !utf8.Valid(body) || !json.Valid(body) {
		return dto, problem(400, "INVALID_JSON", "Некорректный JSON")
	}
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return dto, problem(422, "VALIDATION_ERROR", "Ожидается JSON-объект")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dto); err != nil {
		return dto, problem(422, "VALIDATION_ERROR", "Проверьте названия и типы полей")
	}
	return dto, nil
}

func ValidateText(field, value string, min, max int) *APIError {
	n := utf8.RuneCountInString(value)
	if !utf8.ValidString(value) || n < min || n > max {
		return &APIError{Status: 422, Code: "VALIDATION_ERROR", Message: "Проверьте поля формы", Fields: map[string]string{field: "Недопустимая длина текста"}}
	}
	return nil
}

type Page struct {
	Limit  int
	Cursor string
}

func ParsePage(r *http.Request) (Page, *APIError) {
	p := Page{Limit: 20, Cursor: r.URL.Query().Get("cursor")}
	if raw, ok := r.URL.Query()["limit"]; ok {
		if len(raw) != 1 {
			return Page{}, problem(422, "VALIDATION_ERROR", "Некорректный limit")
		}
		limit, err := strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > 100 {
			return Page{}, problem(422, "VALIDATION_ERROR", "limit должен быть от 1 до 100")
		}
		p.Limit = limit
	}
	if len(p.Cursor) > 1024 {
		return Page{}, problem(422, "VALIDATION_ERROR", "Некорректный cursor")
	}
	if p.Cursor != "" {
		if _, err := base64.RawURLEncoding.Strict().DecodeString(p.Cursor); err != nil {
			return Page{}, problem(422, "VALIDATION_ERROR", "Некорректный cursor")
		}
	}
	return p, nil
}
