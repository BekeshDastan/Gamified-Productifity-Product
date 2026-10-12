package platform

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthAndErrorEnvelopes(t *testing.T) {
	var logs bytes.Buffer
	router := NewRouter(NewLogger(&logs, "info"), FixedClock{Time: time.Date(2026, 10, 12, 4, 10, 0, 0, time.UTC)})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/health/live", nil))
	var success Response[struct {
		Status string `json:"status"`
	}]
	decoder := json.NewDecoder(w.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&success); err != nil {
		t.Fatal(err)
	}
	if success.Data.Status != "ok" || success.Meta.ServerTime != "2026-10-12T04:10:00Z" || success.Meta.RequestID == "" {
		t.Fatal("response contract mismatch")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/private", nil))
	var failure struct {
		Error APIError `json:"error"`
	}
	decoder = json.NewDecoder(w.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if w.Code != 404 || failure.Error.Code != "NOT_FOUND" || failure.Error.RequestID == "" {
		t.Fatal("error contract mismatch")
	}
}
