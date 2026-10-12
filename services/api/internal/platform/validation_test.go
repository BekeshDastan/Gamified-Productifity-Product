package platform

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONErrorBoundaries(t *testing.T) {
	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"title":"Верно"}`, 0}, {`{"title":`, 400}, {`{} {}`, 400},
		{`{"title":2}`, 422}, {`{"title":"ok","role":"admin"}`, 422},
		{`{"title":"ok","owner_id":"other"}`, 422}, {`null`, 422}, {`[]`, 422},
		{`{"title":"` + strings.Repeat("x", MaxBodyBytes) + `"}`, 413},
	} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
		r.Header.Set("Content-Type", "application/json")
		_, err := DecodeJSON[struct {
			Title string `json:"title"`
		}](r)
		if tt.status == 0 {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil || err.Status != tt.status {
			t.Fatalf("want status %d got %v", tt.status, err)
		}
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	if _, err := DecodeJSON[struct{}](r); err == nil || err.Status != 415 {
		t.Fatal("missing content type accepted")
	}
}
func TestUnicodeAndPagination(t *testing.T) {
	if ValidateText("title", strings.Repeat("я", 200), 1, 200) != nil {
		t.Fatal("Unicode counted as bytes")
	}
	if ValidateText("title", strings.Repeat("я", 201), 1, 200) == nil {
		t.Fatal("overlong text accepted")
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=abc", "?limit=1&limit=2", "?cursor=!!"} {
		if _, err := ParsePage(httptest.NewRequest("GET", "/"+query, nil)); err == nil {
			t.Fatalf("invalid page accepted: %s", query)
		}
	}
	p, err := ParsePage(httptest.NewRequest("GET", "/", nil))
	if err != nil || p.Limit != 20 {
		t.Fatal("invalid default page")
	}
}
