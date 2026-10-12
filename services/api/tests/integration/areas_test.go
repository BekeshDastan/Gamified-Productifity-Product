//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
)

func readLabel(t *testing.T, response authResponse) profile.Label {
	t.Helper()
	if response.status != 200 {
		t.Fatal("label operation failed", response.status)
	}
	var body struct {
		Data profile.Label `json:"data"`
	}
	if err := json.Unmarshal(response.body, &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}
func TestAreasConcurrentLimitArchiveAndTagIsolation(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "areas@example.test")
	other, _ := f.signedIn(t, "areas-other@example.test")
	input := profile.LabelInput{Name: "Study", Color: "#6478c8", Icon: "book"}
	first := readLabel(t, f.privateRequest(t, client, "POST", "/areas", "", input))
	// Existing future links can reference owner+id and survive archival.
	if _, err := f.pool.Exec(f.ctx, `CREATE TABLE area_test_links(owner_id uuid,area_id uuid,FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id))`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO area_test_links VALUES($1,$2)`, account.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	statuses := make(chan int, 16)
	for i := 0; i < 16; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			request := input
			request.Name = fmt.Sprintf("Area %02d", i)
			// The helper refreshes CSRF using read-only requests; the cookie jar is concurrency-safe.
			result := f.privateRequest(t, client, http.MethodPost, "/areas", "", request)
			statuses <- result.status
		}(i)
	}
	wait.Wait()
	close(statuses)
	success, rejected := 0, 0
	for code := range statuses {
		if code == 200 {
			success++
		} else if code == 422 {
			rejected++
		} else {
			t.Fatal("unexpected concurrent result", code)
		}
	}
	if success != 11 || rejected != 5 {
		t.Fatal("active cap race", success, rejected)
	}
	if got := f.privateRequest(t, other, "PATCH", "/areas/"+first.ID, `"1"`, input); got.status != 404 {
		t.Fatal("foreign area accepted")
	}
	archived := readLabel(t, f.privateRequest(t, client, "DELETE", "/areas/"+first.ID, `"1"`, struct{}{}))
	if !archived.Archived || archived.ID != first.ID {
		t.Fatal("archive changed identity")
	}
	var links int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM area_test_links`).Scan(&links); err != nil || links != 1 {
		t.Fatal("archive broke links", err)
	}
	input.Name = "New area"
	readLabel(t, f.privateRequest(t, client, "POST", "/areas", "", input))
	input = first.LabelInput
	if got := f.privateRequest(t, client, "PATCH", "/areas/"+first.ID, `"2"`, input); got.status != 422 {
		t.Fatal("unarchive bypassed cap")
	}
	input.Name = "Focus"
	input.Icon = "tag"
	tag := readLabel(t, f.privateRequest(t, client, "POST", "/tags", "", input))
	if got := f.privateRequest(t, other, "DELETE", "/tags/"+tag.ID, `"1"`, struct{}{}); got.status != 404 {
		t.Fatal("foreign tag accepted")
	}
	readLabel(t, f.privateRequest(t, client, "DELETE", "/tags/"+tag.ID, `"1"`, struct{}{}))
}
