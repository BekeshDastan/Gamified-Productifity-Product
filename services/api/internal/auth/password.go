package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"golang.org/x/crypto/argon2"
)

const passwordMemory uint32 = 64 * 1024
const passwordIterations uint32 = 3
const passwordParallelism uint8 = 2

var ErrInvalidHash = errors.New("invalid stored password hash")

// PasswordHasher bounds concurrent memory-hard work; parameters are encoded in PHC format.
type PasswordHasher struct{ slots chan struct{} }

func NewPasswordHasher() *PasswordHasher { return &PasswordHasher{slots: make(chan struct{}, 2)} }

func ValidatePassword(password string) *platform.APIError {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || utf8.RuneCountInString(password) > 128 || len(password) > 512 {
		return &platform.APIError{Status: 422, Code: "VALIDATION_ERROR", Message: "Проверьте поля формы", Fields: map[string]string{"password": "Пароль: 12–128 символов, не более 512 байт"}}
	}
	return nil
}
func (h *PasswordHasher) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case h.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-h.slots
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *PasswordHasher) Hash(ctx context.Context, password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer func() { <-h.slots }()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemory, passwordParallelism, 32)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", passwordMemory, passwordIterations, passwordParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}
func (h *PasswordHasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	if !utf8.ValidString(password) || len(password) > 512 || utf8.RuneCountInString(password) > 128 {
		return false, nil
	}
	if len(encoded) > 512 {
		return false, ErrInvalidHash
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, ErrInvalidHash
	}
	var memory, iterations uint32
	var parallelism uint8
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || n != 3 || parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, parallelism) || memory < 19*1024 || memory > 128*1024 || iterations < 2 || iterations > 6 || parallelism < 1 || parallelism > 4 {
		return false, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, ErrInvalidHash
	}
	expected, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false, ErrInvalidHash
	}
	if err := h.acquire(ctx); err != nil {
		return false, err
	}
	defer func() { <-h.slots }()
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
