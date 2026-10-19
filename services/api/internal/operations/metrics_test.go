package operations

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRejectsPublicAccess(t *testing.T) {
	if _, err := Handler(nil, "short", ""); err == nil {
		t.Fatal("short credential accepted")
	}
	handler, err := Handler(nil, strings.Repeat("a", 43), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", "Bearer wrong", "Bearer " + strings.Repeat("b", 43)} {
		r := httptest.NewRequest("GET", "/metrics", nil)
		r.Header.Set("Authorization", header)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 401 || w.Body.Len() != 0 {
			t.Fatal("unauthorized metrics disclosed")
		}
	}
}
