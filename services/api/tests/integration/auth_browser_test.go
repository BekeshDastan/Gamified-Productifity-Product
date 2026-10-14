//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
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
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/navigator"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/blob"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/rewards"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
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
	t.Setenv("FEATURE_MYSTERY_REWARDS", "true")
	pool, ctx := databaseWithTimeout(t, 10*time.Minute)
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
	limits.Registrations = 40
	limits.MailRequests = 20
	// Acceptance compresses many lifecycle actions into seconds. Production's
	// default 120/min quota is covered separately by HTTP integration tests.
	limits.UserRequests = 600
	if err := sessions.Mount(router, limits); err != nil {
		t.Fatal(err)
	}
	domainClock := testkit.NewAdvancingClock(time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), 60)
	profile.NewService(pool, domainClock).Mount(router, sessions.Protect)
	goalService := goals.NewService(pool, domainClock)
	goalService.Mount(router, sessions.Protect)
	files, err := blob.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	rewardService := progression.NewService(pool, domainClock)
	rewardService.Mount(router, sessions.Protect)
	characterService := character.NewService(pool, domainClock, rewardService)
	rewardService.WithProjectionObserver(characterService.EvaluateInTx)
	characterService.Mount(router, sessions.Protect)
	rewards.NewService(pool, domainClock, rewardService, characterService).Mount(router, sessions.Protect)

	questService := quests.NewService(pool, domainClock).WithObserver(goalService.QuestChanged).WithProgression(rewardService).WithFiles(files, blob.UnavailableScanner{})
	questService.Mount(router, sessions.Protect)
	calendarService := calendar.NewService(pool, domainClock, questService)
	calendarService.Mount(router, sessions.Protect)
	habitService := habits.NewService(pool, domainClock).WithProgression(rewardService)
	habitService.Mount(router, sessions.Protect)
	challengeService := challenges.NewService(pool, domainClock).WithSchedulingClock(clock).WithProgression(rewardService).WithQuests(questService)
	challengeService.Mount(router, sessions.Protect)
	planService := planning.NewService(pool, domainClock, questService, calendarService)
	seasons.NewService(pool, domainClock).WithSchedulingClock(clock).WithChallenges(challengeService).WithRecovery(questService, goalService).WithPlanning(planService, calendarService).WithHabits(habitService).Mount(router, sessions.Protect)
	planService.Mount(router, sessions.Protect)
	analyticsService := analytics.NewService(pool, domainClock).WithRecords(characterService)
	analyticsService.Mount(router, sessions.Protect)
	notificationService := notifications.NewService(pool, domainClock).WithHabits(habitService).WithSchedulingClock(clock)
	characterService.WithAchievementObserver(notificationService.AchievementChanged)
	analyticsService.WithReportObserver(notificationService.ReportCreated)

	if err := notificationService.ConfigureDelivery("http://127.0.0.1:3100", key, nil); err != nil {
		t.Fatal(err)
	}
	notificationService.Mount(router, sessions.Protect)
	focus.NewService(pool, domainClock, rewardService).WithResultObserver(notificationService.FocusChanged).Mount(router, sessions.Protect)
	navigator.NewService(pool, domainClock, planService, goalService, habitService, analyticsService).WithQuests(questService).Mount(router, sessions.Protect)
	journal.NewService(pool, domainClock).WithPlanning(planService).WithQuests(questService).Mount(router, sessions.Protect)
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
	if err := notificationService.ConfigureDelivery("http://127.0.0.1:3100", key, mailer); err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		handlers := sessions.MailHandlers(mailer)
		for kind, handler := range notificationService.Handlers() {
			handlers[kind] = handler
		}
		for kind, handler := range analyticsService.Handlers() {
			handlers[kind] = handler
		}
		for kind, handler := range seasons.NewService(pool, domainClock).WithChallenges(challengeService).Handlers() {
			handlers[kind] = handler
		}
		for kind, handler := range challenges.NewService(pool, domainClock).WithProgression(rewardService).Handlers() {
			handlers[kind] = handler
		}
		for kind, handler := range characterService.Handlers() {
			handlers[kind] = handler
		}
		for kind, handler := range rewardService.Handlers() {
			handlers[kind] = handler
		}
		jobs.Run(workerCtx, jobs.Store{Pool: pool, Clock: clock}, handlers, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	deadline := time.Now().Add(30 * time.Second)
	for {
		var pending int
		e := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM outbox_events WHERE kind IN('character.changed','progression.reward_changed','challenges.changed','analytics.changed','notifications.changed','notifications.reminders_continue') AND dispatched_at IS NULL)+(SELECT count(*) FROM jobs WHERE kind IN('character.changed','progression.reward_changed','challenges.changed','analytics.changed','notifications.changed','notifications.reminders_continue') AND state IN('pending','running','retry_wait'))").Scan(&pending)
		if e != nil {
			t.Fatal(e)
		}
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("derived-profile events did not finish", pending)
		}
		select {
		case <-ctx.Done():
			t.Fatal("browser fixture cancelled")
		case <-time.After(100 * time.Millisecond):
		}
	}
	var dead int
	if e := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind IN('character.changed','progression.reward_changed','challenges.changed','analytics.changed','notifications.changed','notifications.reminders_continue','challenges.close','seasons.close','notifications.reminder') AND state='dead'").Scan(&dead); e != nil || dead != 0 {
		t.Fatal("derived-profile outbox handler failure", dead, e)
	}
	for _, secret := range []string{`"password":`, `"cookie":`, `"authorization":`, "browser test password phrase", "browser password phrase", "new browser password phrase", "ascend_access=", "ascend_refresh=", "#token=", "JOURNAL_PRIVATE_SENTINEL_C120", "CHALLENGE_PRIVATE_SENTINEL_C128"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("sensitive authentication data in API logs")
		}
	}
}
