//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/blob"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
)

type authLogs struct {
	sync.Mutex
	buffer bytes.Buffer
}

func (l *authLogs) Write(data []byte) (int, error) {
	l.Lock()
	defer l.Unlock()
	return l.buffer.Write(data)
}
func (l *authLogs) String() string { l.Lock(); defer l.Unlock(); return l.buffer.String() }

// Explicit opt-in runner: production browser -> Next proxy -> real Go router,
// isolated PostgreSQL schema -> worker -> real SMTP/Mailpit. No auth API mocks.
func TestBrowserAuth(t *testing.T) {
	if os.Getenv("RUN_BROWSER_AUTH") != "1" {
		t.Skip("run npm run test:auth-e2e for the real browser suite")
	}
	pool, ctx := databaseWithTimeout(t, 6*time.Minute)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	clock := platform.RealClock{}
	sessions, err := auth.NewSessions(ctx, auth.NewService(pool, auth.NewPasswordHasher(), clock), auth.SessionConfig{Origin: "http://127.0.0.1:3100", Environment: "test", InsecureLocalCookies: true, SigningKey: key})
	if err != nil {
		t.Fatal(err)
	}
	logs := &authLogs{}
	router := platform.NewRouter(slog.New(slog.NewJSONHandler(logs, nil)), clock, pool.Ping)
	// Separate quotas for this isolated acceptance fixture, preserving production
	// 5-failure account allowance; multiple test accounts share one browser host.
	limits := auth.DefaultLimits()
	limits.Registrations = 30
	limits.MailRequests = 20
	// Acceptance compresses many lifecycle actions into seconds. Production's
	// default 120/min quota is covered separately by HTTP integration tests.
	limits.UserRequests = 600
	if err := sessions.Mount(router, limits); err != nil {
		t.Fatal(err)
	}
	profile.NewService(pool, clock).Mount(router, sessions.Protect)
	goalService := goals.NewService(pool, clock)
	goalService.Mount(router, sessions.Protect)
	files, err := blob.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	questService := quests.NewService(pool, clock).WithObserver(goalService.QuestChanged).WithFiles(files, blob.UnavailableScanner{})
	questService.Mount(router, sessions.Protect)
	calendar.NewService(pool, clock, questService).Mount(router, sessions.Protect)
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Fatal("stop dev before browser acceptance; API test port unavailable")
	}
	server := httptest.NewUnstartedServer(router)
	server.Listener = listener
	server.Start()
	defer server.Close()
	mailer, err := mail.NewSMTP("test", os.Getenv("TEST_SMTP_ADDR"), "ascend@example.test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		jobs.Run(workerCtx, jobs.Store{Pool: pool, Clock: clock}, sessions.MailHandlers(mailer), slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	defer func() { cancel(); worker.Wait() }()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{filepath.Join(root, "node_modules", "@playwright", "test", "cli.js"), "test", "--config=playwright.auth.config.ts"}
	if pattern := os.Getenv("AUTH_E2E_GREP"); pattern != "" {
		args = append(args, "--grep", pattern)
	}
	command := exec.CommandContext(ctx, "node", args...)
	command.Dir = filepath.Join(root, "apps", "web")
	command.Env = os.Environ()
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		t.Fatal("real authentication browser suite failed", err)
	}
	for _, secret := range []string{`"password":`, `"cookie":`, `"authorization":`, "browser test password phrase", "new browser password phrase", "ascend_access=", "ascend_refresh=", "#token="} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("sensitive authentication data in API logs")
		}
	}
}
