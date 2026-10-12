//go:build integration

package integration

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
)

func TestConcurrentRegistrationCreatesOneUserAndAudit(t *testing.T) {
	pool, ctx := database(t)
	passwords := auth.NewPasswordHasher()
	service := auth.NewService(pool, passwords, platform.RealClock{})
	const password = "correct horse battery staple"
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			account, err := service.Register(ctx, auth.RegisterInput{Email: " Concurrent@EXAMPLE.test ", Password: password})
			if err == nil && (account.Role != "user" || account.EmailVerified || account.Email != "concurrent@example.test") {
				err = errors.New("untrusted role/verification")
			}
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var problem *platform.APIError
		if !errors.As(err, &problem) || problem.Code != "EMAIL_TAKEN" {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("created %d accounts", successes)
	}
	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users`).Scan(&hash); err != nil || hash == password || strings.Contains(hash, password) {
		t.Fatal("password stored unsafely", err)
	}
	ok, err := passwords.Verify(ctx, password, hash)
	if err != nil || !ok {
		t.Fatal("stored hash does not verify", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='auth.registered'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("registration audit not atomic", err)
	}
}
func TestRegistrationValidationDoesNotWrite(t *testing.T) {
	t.Parallel()
	pool, ctx := database(t)
	service := auth.NewService(pool, auth.NewPasswordHasher(), platform.RealClock{})
	for _, input := range []auth.RegisterInput{{Email: "invalid", Password: "valid password phrase"}, {Email: "user@example.test", Password: "short"}} {
		if _, err := service.Register(ctx, input); err == nil {
			t.Fatal("invalid registration accepted")
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid input wrote user", err)
	}
}

func TestRegistrationRollsBackIfAuditCannotCommit(t *testing.T) {
	t.Parallel()
	pool, ctx := database(t)
	if _, err := pool.Exec(ctx, `ALTER TABLE audit_events ADD CONSTRAINT test_reject_registration CHECK(action <> 'auth.registered')`); err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(pool, auth.NewPasswordHasher(), platform.RealClock{})
	if _, err := service.Register(ctx, auth.RegisterInput{Email: "rollback@example.test", Password: "valid password phrase"}); err == nil {
		t.Fatal("audit failure accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("account survived failed audit", err)
	}
}
