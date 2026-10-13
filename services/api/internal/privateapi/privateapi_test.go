package privateapi

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"net/http/httptest"
	"testing"
)

func TestSingletonVersionsDatesAndOwners(t *testing.T) {
	for _, test := range []struct {
		header  string
		current int64
		ok      bool
	}{{`"0"`, 0, true}, {`"1"`, 0, false}, {`"0"`, 1, false}, {`"1"`, 1, true}, {`"2"`, 1, false}, {"bad", 1, false}} {
		r := httptest.NewRequest("PUT", "/", nil)
		r.Header.Set("If-Match", test.header)
		if (MatchVersion(r, test.current) == nil) != test.ok {
			t.Fatal(test)
		}
	}
	for _, date := range []string{"bad", "2026-02-30", "0000-01-01", "9999-01-01"} {
		if Day(date) == nil {
			t.Fatal(date)
		}
	}
	if Day("2026-10-12") != nil || UUID("bad") {
		t.Fatal("validation")
	}
	r := httptest.NewRequest("GET", "/", nil)
	if _, e := Owner(r.Context()); e == nil {
		t.Fatal("anonymous")
	}
	ctx := platform.WithIdentity(r.Context(), platform.Identity{UserID: "owner", SessionID: "session"})
	if id, e := Owner(ctx); e != nil || id != "owner" {
		t.Fatal(id, e)
	}
}
