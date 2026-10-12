package testkit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIndependentOwnersClocksAndCookieJars(t *testing.T) {
	a, b := NewUser(), NewUser()
	if a.ID == b.ID || a.Email == b.Email {
		t.Fatal("owners share fixture identity")
	}
	clock := NewClock(time.Unix(0, 0))
	clock.Advance(time.Hour)
	if !clock.Now().Equal(time.Unix(3600, 0)) {
		t.Fatal("clock is not deterministic")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			http.SetCookie(w, &http.Cookie{Name: "test_session", Value: a.ID, HttpOnly: true, Path: "/"})
			return
		}
		if _, err := r.Cookie("test_session"); err != nil {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	first, second := NewClient(t, server.URL), NewClient(t, server.URL)
	first.JSON(t, "POST", "/login", nil, nil)
	if first.JSON(t, "GET", "/private", nil, nil).StatusCode != 204 {
		t.Fatal("first client lost cookie")
	}
	if second.JSON(t, "GET", "/private", nil, nil).StatusCode != 401 {
		t.Fatal("cookie jar leaked between owners")
	}
}
