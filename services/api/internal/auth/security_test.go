package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
)

func TestPreauthExpiryOriginContentTypeAndCookieBinding(t *testing.T) {
	clock := testkit.NewClock(time.Now())
	service := NewService(nil, NewPasswordHasher(), clock)
	sessions, err := NewSessions(context.Background(), service, SessionConfig{Origin: "http://localhost:3000", Environment: "test", InsecureLocalCookies: true, SigningKey: bytes.Repeat([]byte{8}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	csrfRequest := httptest.NewRequest("GET", "http://localhost:3000/api/v1/auth/csrf", nil)
	response := httptest.NewRecorder()
	sessions.csrf(response, csrfRequest)
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if json.Unmarshal(response.Body.Bytes(), &envelope) != nil {
		t.Fatal("invalid csrf envelope")
	}
	cookie := response.Result().Cookies()[0]
	for _, scenario := range []string{"valid", "no-origin", "foreign-origin", "no-csrf", "bad-csrf", "no-cookie", "bad-type", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			request := httptest.NewRequest("POST", "http://localhost:3000/api/v1/auth/register", strings.NewReader(`{}`))
			request.Header.Set("Origin", "http://localhost:3000")
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", envelope.Data.Token)
			if scenario != "no-cookie" {
				request.AddCookie(cookie)
			}
			switch scenario {
			case "no-origin":
				request.Header.Del("Origin")
			case "foreign-origin":
				request.Header.Set("Origin", "https://foreign.example")
			case "no-csrf":
				request.Header.Del("X-CSRF-Token")
			case "bad-csrf":
				request.Header.Set("X-CSRF-Token", strings.Repeat("a", 43))
			case "bad-type":
				request.Header.Set("Content-Type", "text/plain")
			case "expired":
				clock.Advance(30 * time.Minute)
			}
			called := false
			recorder := httptest.NewRecorder()
			sessions.browserMutation(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(recorder, request)
			if called != (scenario == "valid") {
				t.Fatal("unsafe browser boundary", scenario)
			}
		})
	}
}

func TestAuthConfigurationDoesNotDiscloseSecretsOrTrustForwardedIP(t *testing.T) {
	lookup := func(values map[string]string) func(string) (string, bool) {
		return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
	}
	if _, err := LoadSessionConfig("https://ascend.example", "production", lookup(map[string]string{"AUTH_SIGNING_KEY": "private-invalid-key"})); err == nil || strings.Contains(err.Error(), "private-invalid-key") {
		t.Fatal("invalid key accepted/disclosed")
	}
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32))
	config, err := LoadSessionConfig("https://ascend.example", "production", lookup(map[string]string{"AUTH_SIGNING_KEY": key}))
	if err != nil || config.InsecureLocalCookies {
		t.Fatal("secure defaults", err)
	}
	for _, values := range []map[string]string{{"AUTH_LOGIN_FAILURES": "0"}, {"AUTH_USER_REQUESTS": "10001"}, {"AUTH_MAIL_REQUESTS": "invalid"}, {"AUTH_LOGIN_FAILURES": "60", "AUTH_IP_FAILURES": "50"}} {
		if _, err := LoadLimits(lookup(values)); err == nil {
			t.Fatal("invalid limit accepted")
		}
	}
	request := httptest.NewRequest("POST", "http://localhost:3000/api/v1/auth/login", nil)
	request.RemoteAddr = "127.0.0.1:4567"
	request.Header.Set("X-Forwarded-For", "203.0.113.1")
	request.Header.Set("Forwarded", "for=203.0.113.2")
	if remoteIP(request) != "127.0.0.1" {
		t.Fatal("untrusted proxy header bypasses limits")
	}
}
