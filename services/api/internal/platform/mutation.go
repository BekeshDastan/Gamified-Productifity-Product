package platform

import (
	"context"
	"net/http"
	"strconv"
)

type identityKey struct{}
type Identity struct{ UserID, SessionID, Role string }

// WithIdentity is called only after server-side session verification.
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}
func CurrentIdentity(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok && identity.UserID != "" && identity.SessionID != ""
}
func RequireOwner(ctx context.Context, ownerID string) *APIError {
	identity, ok := CurrentIdentity(ctx)
	if !ok {
		return problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	if identity.UserID != ownerID {
		return problem(404, "NOT_FOUND", "Не найдено")
	}
	return nil
}
func MatchVersion(r *http.Request, current int64) *APIError {
	raw := r.Header.Get("If-Match")
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return problem(422, "VALIDATION_ERROR", "Укажите If-Match с версией")
	}
	value := raw[1 : len(raw)-1]
	if value[0] == '0' {
		return problem(422, "VALIDATION_ERROR", "Некорректная версия")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return problem(422, "VALIDATION_ERROR", "Некорректная версия")
		}
	}
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version < 1 {
		return problem(422, "VALIDATION_ERROR", "Некорректная версия")
	}
	if version != current {
		return &APIError{Status: 409, Code: "VERSION_CONFLICT", Message: "Запись уже изменена. Обновите данные", CurrentVersion: &current}
	}
	return nil
}
