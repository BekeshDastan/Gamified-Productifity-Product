//go:build integration

package integration

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
)

func TestHTTPReadinessUsesRealPool(t *testing.T) {
	t.Parallel()
	pool, _ := database(t)
	server := httptest.NewServer(platform.NewRouter(platform.NewLogger(io.Discard, "info"), platform.RealClock{}, pool.Ping))
	defer server.Close()
	client := testkit.NewClient(t, server.URL)
	if client.JSON(t, "GET", "/health/ready", nil, nil).StatusCode != 200 {
		t.Fatal("available DB is not ready")
	}
	pool.Close()
	if client.JSON(t, "GET", "/health/ready", nil, nil).StatusCode != 503 {
		t.Fatal("closed pool is reported ready")
	}
	if client.JSON(t, "GET", "/health/live", nil, nil).StatusCode != 200 {
		t.Fatal("DB failure changed liveness")
	}
}
