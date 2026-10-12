package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStructuredLogsRedactNestedCredentials(t *testing.T) {
	var out bytes.Buffer
	logger := NewLogger(&out, "info")
	logger.Debug("hidden")
	logger.Info("ready", "request_id", "public-id", "Authorization", "do-not-leak",
		slog.Group("settings", "database_url", "do-not-leak", "refresh_token", "do-not-leak"))
	if strings.Contains(out.String(), "do-not-leak") || strings.Contains(out.String(), "hidden") {
		t.Fatal("log leaked credentials or ignored level")
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if string(record["request_id"]) != `"public-id"` {
		t.Fatal("request correlation missing")
	}
}

func TestCorrelationAndPrivacy(t *testing.T) {
	var logs bytes.Buffer
	router := NewRouter(NewLogger(&logs, "info"), FixedClock{Time: time.Unix(0, 0)})
	router.Get("/test/{id}", func(w http.ResponseWriter, r *http.Request) {
		if RequestMeta(r.Context()).RequestID != "client-id" {
			t.Error("request metadata missing")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	r := httptest.NewRequest("GET", "/test/private-name?token=private-token", nil)
	r.Header.Set("X-Request-ID", "client-id")
	r.Header.Set("X-Correlation-ID", strings.Repeat("bad", 100))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Header().Get("X-Request-ID") != "client-id" || !validID.MatchString(w.Header().Get("X-Correlation-ID")) {
		t.Fatal("unsafe correlation")
	}
	if strings.Contains(logs.String(), "private-") {
		t.Fatal("request URL leaked")
	}
	if !strings.Contains(logs.String(), `/test/{id}`) {
		t.Fatal("route template missing")
	}
}

func TestShutdownDrainsInFlightRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.WriteHeader(204) })}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, server, listener) }()
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		r, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = r.Body.Close()
			if r.StatusCode != 204 {
				err = context.Canceled
			}
		}
		response <- err
	}()
	<-entered
	cancel()
	select {
	case <-done:
		t.Fatal("shutdown discarded active request")
	default:
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
