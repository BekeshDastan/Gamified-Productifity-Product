//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
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
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/rewards"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authFixture struct {
	pool     *pgxpool.Pool
	ctx      context.Context
	clock    *testkit.Clock
	sessions *auth.Sessions
	service  *auth.Service
	server   *httptest.Server
}

func newAuthFixture(t *testing.T, limits auth.Limits, scanners ...blob.Scanner) authFixture {
	t.Helper()
	pool, ctx := database(t)
	clock := testkit.NewClock(time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC))
	service := auth.NewService(pool, auth.NewPasswordHasher(), clock)
	sessions, err := auth.NewSessions(ctx, service, auth.SessionConfig{Origin: "http://localhost:3000", Environment: "test", InsecureLocalCookies: true, SigningKey: bytes.Repeat([]byte{12}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	router := platform.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), clock, pool.Ping)
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
	t.Cleanup(func() { _ = files.Close() })
	var scanner blob.Scanner = blob.UnavailableScanner{}
	if len(scanners) > 0 {
		scanner = scanners[0]
	}
	rewardService := progression.NewService(pool, clock)
	rewardService.Mount(router, sessions.Protect)
	characterService := character.NewService(pool, clock, rewardService)
	rewardService.WithProjectionObserver(characterService.EvaluateInTx)
	characterService.Mount(router, sessions.Protect)
	rewards.NewService(pool, clock, rewardService, characterService).Mount(router, sessions.Protect)

	questService := quests.NewService(pool, clock).WithObserver(goalService.QuestChanged).WithProgression(rewardService).WithFiles(files, scanner)
	questService.Mount(router, sessions.Protect)
	calendarService := calendar.NewService(pool, clock, questService)
	calendarService.Mount(router, sessions.Protect)
	habitService := habits.NewService(pool, clock).WithProgression(rewardService)
	habitService.Mount(router, sessions.Protect)
	challengeService := challenges.NewService(pool, clock).WithProgression(rewardService).WithQuests(questService)
	challengeService.Mount(router, sessions.Protect)
	planService := planning.NewService(pool, clock, questService, calendarService)
	seasons.NewService(pool, clock).WithChallenges(challengeService).WithRecovery(questService, goalService).WithPlanning(planService, calendarService).WithHabits(habitService).Mount(router, sessions.Protect)
	planService.Mount(router, sessions.Protect)
	analyticsService := analytics.NewService(pool, clock).WithRecords(characterService)
	analyticsService.Mount(router, sessions.Protect)
	notificationService := notifications.NewService(pool, clock).WithHabits(habitService)
	characterService.WithAchievementObserver(notificationService.AchievementChanged)
	analyticsService.WithReportObserver(notificationService.ReportCreated)

	notificationService.ConfigurePush("test-public-key", nil)
	if err := notificationService.ConfigureDelivery("http://localhost:3000", bytes.Repeat([]byte{12}, 32), nil); err != nil {
		t.Fatal(err)
	}
	notificationService.Mount(router, sessions.Protect)
	focus.NewService(pool, clock, rewardService).WithResultObserver(notificationService.FocusChanged).Mount(router, sessions.Protect)
	navigator.NewService(pool, clock, planService, goalService, habitService, analyticsService).WithQuests(questService).Mount(router, sessions.Protect)
	journal.NewService(pool, clock).WithPlanning(planService).WithQuests(questService).Mount(router, sessions.Protect)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return authFixture{pool: pool, ctx: ctx, clock: clock, sessions: sessions, service: service, server: server}
}

func authClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

type authResponse struct {
	status int
	header http.Header
	body   []byte
}

func (f authFixture) request(t *testing.T, client *http.Client, method, path, csrf string, input any) authResponse {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequest(method, f.server.URL+"/api/v1/auth"+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return authResponse{status: response.StatusCode, header: response.Header, body: body}
}
func (f authFixture) csrf(t *testing.T, client *http.Client) string {
	t.Helper()
	response := f.request(t, client, "GET", "/csrf", "", nil)
	var result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if response.status != 200 || json.Unmarshal(response.body, &result) != nil || len(result.Data.Token) != 43 {
		t.Fatal("csrf unavailable", response.status)
	}
	return result.Data.Token
}

func TestHTTPPreauthOriginCSRFAndStrictDTO(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client := authClient(t)
	input := auth.RegisterInput{Email: "http@example.test", Password: "correct password phrase"}
	if response := f.request(t, client, "POST", "/register", "", input); response.status != 403 {
		t.Fatal("missing csrf accepted", response.status)
	}
	csrf := f.csrf(t, client)
	foreign := authClient(t)
	if response := f.request(t, foreign, "POST", "/register", csrf, input); response.status != 403 {
		t.Fatal("csrf unbound to cookie", response.status)
	}
	r, _ := http.NewRequest("POST", f.server.URL+"/api/v1/auth/register", bytes.NewBufferString(`{"email":"foreign@example.test","password":"correct password phrase"}`))
	r.Header.Set("Origin", "https://foreign.example")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("foreign origin accepted")
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected request wrote user", err)
	}
	if response := f.request(t, client, "POST", "/register", csrf, map[string]any{"email": input.Email, "password": input.Password, "role": "admin"}); response.status != 422 {
		t.Fatal("untrusted role accepted", response.status)
	}
	if response := f.request(t, client, "POST", "/register", csrf, input); response.status != 201 {
		t.Fatal("registration failed", response.status)
	}
	login := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: input.Email, Password: input.Password})
	if login.status != 200 || len(login.header.Values("Set-Cookie")) != 2 {
		t.Fatal("login failed", login.status)
	}
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 200 || !bytes.Contains(got.body, []byte(input.Email)) {
		t.Fatal("session unavailable", got.status)
	}
	if got := f.request(t, foreign, "GET", "/session", "", nil); got.status != 401 {
		t.Fatal("session leaked to another cookie jar", got.status)
	}
}

func TestHTTPLoginLimitIsAtomicAndRestoresAfterWindow(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client := authClient(t)
	csrf := f.csrf(t, client)
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "limited@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	statuses := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: account.Email, Password: "wrong password phrase"})
			statuses <- response.status
		}()
	}
	wait.Wait()
	close(statuses)
	denied, limited := 0, 0
	for status := range statuses {
		switch status {
		case 401:
			denied++
		case 429:
			limited++
		default:
			t.Fatal("unexpected limit result", status)
		}
	}
	if denied != 5 || limited != 5 {
		t.Fatal("parallel limit bypassed", denied, limited)
	}
	if got := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); got.status != 429 || got.header.Get("Retry-After") == "" {
		t.Fatal("missing retry limit", got.status)
	}
	f.clock.Advance(15 * time.Minute)
	if got := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); got.status != 200 {
		t.Fatal("window did not restore", got.status)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM auth_rate_limits WHERE attempts=0`).Scan(&count); err != nil || count != 2 {
		t.Fatal("successful login counted as failed attempt", err)
	}
}

func TestUserRequestQuotaRestoresAndDoesNotBlockLogout(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 2
	f := newAuthFixture(t, limits)
	client := authClient(t)
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "quota@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	csrf := f.csrf(t, client)
	if got := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); got.status != 200 {
		t.Fatal(got.status)
	}
	csrf = f.csrf(t, client)
	for i := 0; i < 2; i++ {
		if got := f.request(t, client, "GET", "/session", "", nil); got.status != 200 {
			t.Fatal("quota decremented twice", got.status)
		}
	}
	if got := f.request(t, client, "POST", "/refresh", csrf, struct{}{}); got.status != 429 {
		t.Fatal("refresh bypassed quota", got.status)
	}
	if got := f.request(t, client, "GET", "/sessions", "", nil); got.status != 429 || got.header.Get("Retry-After") == "" {
		t.Fatal("missing user quota", got.status)
	}
	f.clock.Advance(time.Minute)
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 200 {
		t.Fatal("quota window did not restore", got.status)
	}
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 200 {
		t.Fatal(got.status)
	}
	if got := f.request(t, client, "POST", "/logout", csrf, struct{}{}); got.status != 200 {
		t.Fatal("quota must not trap user in an account", got.status)
	}
}

func TestCSRFIsBoundToAccessSessionEvenForSameAccount(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "csrf-binding@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	client := authClient(t)
	recorder := httptest.NewRecorder()
	f.sessions.WriteCookies(recorder, second)
	address, _ := url.Parse(f.server.URL)
	client.Jar.SetCookies(address, recorder.Result().Cookies())
	csrf := f.csrf(t, client)
	request := httptest.NewRequest("POST", "http://localhost:3000/private-test", bytes.NewBufferString(`{}`))
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	request.AddCookie(&http.Cookie{Name: "ascend_access", Value: first.Access})
	request.AddCookie(&http.Cookie{Name: "ascend_refresh", Value: second.Refresh})
	called := false
	recorder = httptest.NewRecorder()
	f.sessions.Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(recorder, request)
	if recorder.Code != 403 || called {
		t.Fatal("CSRF from another sid authorized mutation", recorder.Code)
	}
}
