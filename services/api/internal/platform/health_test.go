package platform

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadinessDependsOnDatabaseButLivenessDoesNot(t *testing.T) {
	for _, available := range []bool{true, false} {
		var logs bytes.Buffer
		router := NewRouter(NewLogger(&logs, "info"), FixedClock{Time: time.Unix(0, 0)}, func(context.Context) error {
			if available {
				return nil
			}
			return errors.New("database secret")
		})
		for _, path := range []string{"/health/live", "/health/ready"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			expected := 200
			if path == "/health/ready" && !available {
				expected = 503
			}
			if w.Code != expected {
				t.Fatalf("%s: %d, want %d", path, w.Code, expected)
			}
			if strings.Contains(w.Body.String(), "secret") || strings.Contains(logs.String(), "secret") {
				t.Fatal("DB details leaked")
			}
		}
	}
}

func TestPanicAndBodyLimit(t *testing.T) {
	var logs bytes.Buffer
	router := NewRouter(NewLogger(&logs, "info"), RealClock{})
	router.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("private panic details") })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/panic", nil))
	if w.Code != 500 || strings.Contains(w.Body.String()+logs.String(), "private panic") {
		t.Fatal("panic boundary failed")
	}
	router.Post("/body", func(w http.ResponseWriter, _ *http.Request) { t.Error("oversized request reached handler") })
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/body", strings.NewReader(strings.Repeat("x", (1<<20)+1))))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body limit status: %d", w.Code)
	}
}
