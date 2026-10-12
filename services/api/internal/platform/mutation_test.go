package platform

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestOwnershipAndVersions(t *testing.T) {
	if err := RequireOwner(context.Background(), "owner-a"); err == nil || err.Status != 401 {
		t.Fatal("anonymous accepted")
	}
	ctx := WithIdentity(context.Background(), Identity{UserID: "owner-a", SessionID: "session"})
	if err := RequireOwner(ctx, "owner-b"); err == nil || err.Status != 404 {
		t.Fatal("foreign owner not hidden")
	}
	if err := RequireOwner(ctx, "owner-a"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "1", `W/"1"`, `"0"`, `"01"`, `"-1"`, `"+1"`, `"9223372036854775808"`} {
		r := httptest.NewRequest("PATCH", "/", nil)
		r.Header.Set("If-Match", raw)
		if err := MatchVersion(r, 1); err == nil || err.Status != 422 {
			t.Fatalf("invalid version accepted: %s", raw)
		}
	}
	r := httptest.NewRequest("PATCH", "/", nil)
	r.Header.Set("If-Match", `"1"`)
	if err := MatchVersion(r, 2); err == nil || err.Status != 409 || *err.CurrentVersion != 2 {
		t.Fatal("stale version accepted")
	}
	r.Header.Set("If-Match", `"2"`)
	if err := MatchVersion(r, 2); err != nil {
		t.Fatal(err)
	}
}
