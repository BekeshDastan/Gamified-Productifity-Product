package auth

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrationDTORejectsTrustedFields(t *testing.T) {
	for _, trusted := range []string{"role", "is_admin", "owner_id", "email_verified"} {
		request := httptest.NewRequest("POST", "/auth/register", strings.NewReader(`{"email":"user@example.test","password":"valid password phrase","`+trusted+`":"admin"}`))
		request.Header.Set("Content-Type", "application/json")
		if _, invalid := platform.DecodeJSON[RegisterInput](request); invalid == nil || invalid.Status != 422 {
			t.Fatal("trusted field accepted", trusted)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("x", 129), string([]byte{0xff, 0xfe})} {
		if ValidatePassword(password) == nil {
			t.Fatal("invalid password accepted")
		}
	}
	for _, password := range []string{"valid pass phrase", strings.Repeat("я", 128), strings.Repeat("🙂", 128)} {
		if ValidatePassword(password) != nil {
			t.Fatal("valid Unicode/spaces rejected")
		}
	}
}
func TestHashSaltVerificationAndCancellation(t *testing.T) {
	h := NewPasswordHasher()
	ctx := context.Background()
	password := "  надёжная фраза 🙂 "
	first, err := h.Hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || strings.Contains(first, password) || !strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatal("unsafe hash")
	}
	for _, attempt := range []struct {
		password string
		want     bool
	}{{password, true}, {strings.TrimSpace(password), false}, {"different password", false}} {
		ok, err := h.Verify(ctx, attempt.password, first)
		if err != nil || ok != attempt.want {
			t.Fatal("verification mismatch", err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := h.Hash(cancelled, password); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled work started")
	}
}
func TestHostileStoredHashRejectedBeforeAllocation(t *testing.T) {
	h := NewPasswordHasher()
	for _, hash := range []string{"plaintext", "$argon2id$v=19$m=4294967295,t=3,p=2$AA$AA", "$argon2id$v=19$m=65536,t=0,p=0$AA$AA", "$argon2id$v=19$m=65536,t=3,p=2suffix$AA$AA", strings.Repeat("x", 513)} {
		if _, err := h.Verify(context.Background(), "password", hash); !errors.Is(err, ErrInvalidHash) {
			t.Fatal("hostile hash accepted")
		}
	}
}
func TestEmailNormalization(t *testing.T) {
	email, err := NormalizeEmail("  Person+test@EXAMPLE.test ")
	if err != nil || email != "person+test@example.test" {
		t.Fatal("normalization failed")
	}
	for _, value := range []string{"not email", "Person <test@example.test>", "test@example.test\r\nBcc: other@example.test", "имя@example.test", "a..b@example.test"} {
		if _, err := NormalizeEmail(value); err == nil {
			t.Fatal("unsafe email accepted")
		}
	}
}
