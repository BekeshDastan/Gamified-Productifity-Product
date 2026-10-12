package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage/sqlgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type Account struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}
type Service struct {
	pool      *pgxpool.Pool
	passwords *PasswordHasher
	clock     platform.Clock
}

func NewService(pool *pgxpool.Pool, passwords *PasswordHasher, clock platform.Clock) *Service {
	return &Service{pool: pool, passwords: passwords, clock: clock}
}

func NormalizeEmail(value string) (string, *platform.APIError) {
	email := strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(email)
	valid := err == nil && address.Name == "" && address.Address == email && len(email) <= 254 && len(email) >= 3 && !strings.ContainsAny(email, "\r\n\x00")
	for _, character := range email {
		if character > 127 {
			valid = false
		}
	}
	if !valid {
		return "", &platform.APIError{Status: 422, Code: "VALIDATION_ERROR", Message: "Проверьте поля формы", Fields: map[string]string{"email": "Введите корректный email без имени отправителя"}}
	}
	return email, nil
}

// Register is deliberately not exposed as a public route before C027 adds pre-auth CSRF and limits.
func (s *Service) Register(ctx context.Context, input RegisterInput) (Account, error) {
	email, invalid := NormalizeEmail(input.Email)
	if invalid != nil {
		return Account{}, invalid
	}
	hash, err := s.passwords.Hash(ctx, input.Password)
	if err != nil {
		return Account{}, err
	}
	var account Account
	err = storage.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		queries := sqlgen.New(tx)
		user, err := queries.RegisterUser(ctx, sqlgen.RegisterUserParams{Email: email, PasswordHash: hash, CreatedAt: pgtype.Timestamptz{Time: s.clock.Now().UTC(), Valid: true}})
		if err != nil {
			var databaseError *pgconn.PgError
			if errors.As(err, &databaseError) && databaseError.Code == "23505" && databaseError.ConstraintName == "users_email_key" {
				return &platform.APIError{Status: 409, Code: "EMAIL_TAKEN", Message: "Этот адрес уже зарегистрирован", Fields: map[string]string{"email": "Используйте вход или восстановление пароля"}}
			}
			return errors.New("registration storage failed")
		}
		var id pgtype.UUID
		if err := id.Scan(user.ID); err != nil {
			return errors.New("registration identity invalid")
		}
		if _, err := queries.RecordAuditEvent(ctx, sqlgen.RecordAuditEventParams{ActorID: id, Action: "auth.registered", EntityType: "user", EntityID: id, Metadata: []byte(`{}`)}); err != nil {
			return errors.New("registration audit failed")
		}
		account = Account{ID: user.ID, Email: user.Email, Role: user.Role, EmailVerified: user.EmailVerifiedAt.Valid, CreatedAt: user.CreatedAt.Time.UTC()}
		return nil
	})
	if err != nil {
		return Account{}, err
	}
	return account, nil
}
