//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
)

func TestOnboardingResumeAndConcurrentCompletionWithoutDuplicates(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "setup@example.test")
	other, _ := f.signedIn(t, "setup-other@example.test")
	input := profile.OnboardingInput{Step: 4, Starters: profile.Starters{Goal: "Learn Go", Task: "Read one page", Habit: "Walk"}}
	result := f.privateRequest(t, client, "PATCH", "/me/onboarding", `"1"`, input)
	if result.status != 200 {
		t.Fatal("setup save failed", result.status)
	}
	var firstID string
	if err := f.pool.QueryRow(f.ctx, `SELECT id::text FROM onboarding_starters WHERE owner_id=$1 AND kind='goal'`, account.ID).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	// A second device restores both current step and the saved titles.
	fresh := authClient(t)
	f.loginAs(t, fresh, account.Email)
	result = f.privateRequest(t, fresh, "GET", "/me/onboarding", "", nil)
	var restored struct {
		Data profile.Onboarding `json:"data"`
	}
	if json.Unmarshal(result.body, &restored) != nil || restored.Data.Step != 4 || restored.Data.Starters.Goal != "Learn Go" {
		t.Fatal("setup did not resume")
	}
	if result := f.privateRequest(t, client, "PATCH", "/me/onboarding", `"1"`, input); result.status != 409 {
		t.Fatal("stale setup overwrite accepted")
	}
	result = f.privateRequest(t, other, "GET", "/me/onboarding", "", nil)
	if bytes.Contains(result.body, []byte("Learn Go")) {
		t.Fatal("setup leaked")
	}
	if result := f.privateRequest(t, client, "PATCH", "/me/onboarding", `"2"`, input); result.status != 200 {
		t.Fatal("repeat setup failed")
	}
	var wait sync.WaitGroup
	statuses := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			statuses <- f.privateRequest(t, client, "POST", "/me/onboarding/complete", `"3"`, struct{}{}).status
		}()
	}
	wait.Wait()
	close(statuses)
	for code := range statuses {
		if code != 200 {
			t.Fatal("completion was not idempotent", code)
		}
	}
	var count int
	var currentID string
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM onboarding_starters WHERE owner_id=$1`, account.ID).Scan(&count); err != nil || count != 3 {
		t.Fatal("starter duplicates", count, err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT id::text FROM onboarding_starters WHERE owner_id=$1 AND kind='goal'`, account.ID).Scan(&currentID); err != nil || currentID != firstID {
		t.Fatal("starter identity changed")
	}
	var version int64
	if err := f.pool.QueryRow(f.ctx, `SELECT version FROM onboarding WHERE owner_id=$1`, account.ID).Scan(&version); err != nil || version != 4 {
		t.Fatal("completion replay mutated state")
	}
	// Reopen and skip: saved drafts remain; explicit empty title clears only that draft.
	input.Step = 0
	if result := f.privateRequest(t, client, "PATCH", "/me/onboarding", `"4"`, input); result.status != 200 {
		t.Fatal("reopen failed")
	}
	input.Starters.Task = ""
	if result := f.privateRequest(t, client, "PATCH", "/me/onboarding", `"5"`, input); result.status != 200 {
		t.Fatal("draft clear failed")
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM onboarding_starters WHERE owner_id=$1`, account.ID).Scan(&count); err != nil || count != 2 {
		t.Fatal("skip changed other drafts")
	}
}
