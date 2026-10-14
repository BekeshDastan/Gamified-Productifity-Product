package main

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/config"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/rewards"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
)

func main() {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := platform.NewLogger(os.Stdout, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("invalid database configuration")
		os.Exit(1)
	}
	defer pool.Close()
	authConfig, err := auth.LoadSessionConfig(cfg.Origin, cfg.Environment, os.LookupEnv)
	if err != nil {
		logger.Error("invalid auth configuration", "error", err)
		os.Exit(1)
	}
	sessions, err := auth.NewSessions(ctx, auth.NewService(pool, auth.NewPasswordHasher(), platform.RealClock{}), authConfig)
	if err != nil {
		logger.Error("invalid session configuration")
		os.Exit(1)
	}
	router := platform.NewRouter(logger, platform.RealClock{}, pool.Ping)
	limits, err := auth.LoadLimits(os.LookupEnv)
	if err != nil {
		logger.Error("invalid auth limits")
		os.Exit(1)
	}
	if err := sessions.Mount(router, limits); err != nil {
		logger.Error("invalid auth limits")
		os.Exit(1)
	}
	profile.NewService(pool, platform.RealClock{}).Mount(router, sessions.Protect)
	goalService := goals.NewService(pool, platform.RealClock{})
	goalService.Mount(router, sessions.Protect)
	directory := os.Getenv("BLOB_PRIVATE_DIR")
	if directory == "" {
		directory = ".data/blobs"
	}
	files, err := blob.NewLocal(directory)
	if err != nil {
		logger.Error("private blob directory unavailable")
		os.Exit(1)
	}
	defer files.Close()
	scanner, err := blob.ConfigureScanner(files, os.Getenv("UPLOAD_SCAN_MODE"), os.Getenv("CLAMAV_ADDRESS"))
	if err != nil {
		logger.Error("invalid scanner configuration")
		os.Exit(1)
	}
	rewardService := progression.NewService(pool, platform.RealClock{})
	rewardService.Mount(router, sessions.Protect)
	characterService := character.NewService(pool, platform.RealClock{}, rewardService)
	rewardService.WithProjectionObserver(characterService.EvaluateInTx)
	characterService.Mount(router, sessions.Protect)
	rewards.NewService(pool, platform.RealClock{}, rewardService, characterService).Mount(router, sessions.Protect)
	focus.NewService(pool, platform.RealClock{}, rewardService).Mount(router, sessions.Protect)
	questService := quests.NewService(pool, platform.RealClock{}).WithObserver(goalService.QuestChanged).WithProgression(rewardService).WithFiles(files, scanner)
	questService.Mount(router, sessions.Protect)
	calendarService := calendar.NewService(pool, platform.RealClock{}, questService)
	calendarService.Mount(router, sessions.Protect)
	habitService := habits.NewService(pool, platform.RealClock{}).WithProgression(rewardService)
	habitService.Mount(router, sessions.Protect)
	challengeService := challenges.NewService(pool, platform.RealClock{}).WithProgression(rewardService).WithQuests(questService)
	challengeService.Mount(router, sessions.Protect)
	planService := planning.NewService(pool, platform.RealClock{}, questService, calendarService)
	seasons.NewService(pool, platform.RealClock{}).WithChallenges(challengeService).WithRecovery(questService, goalService).WithPlanning(planService, calendarService).WithHabits(habitService).Mount(router, sessions.Protect)
	planService.Mount(router, sessions.Protect)
	analyticsService := analytics.NewService(pool, platform.RealClock{}).WithRecords(characterService)
	analyticsService.Mount(router, sessions.Protect)
	notificationService := notifications.NewService(pool, platform.RealClock{}).WithHabits(habitService)
	notificationService.Mount(router, sessions.Protect)
	navigator.NewService(pool, platform.RealClock{}, planService, goalService, habitService, analyticsService).WithQuests(questService).Mount(router, sessions.Protect)
	journal.NewService(pool, platform.RealClock{}).WithPlanning(planService).WithQuests(questService).Mount(router, sessions.Protect)
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		logger.Error("api listener unavailable")
		os.Exit(1)
	}
	logger.Info("api starting", "address", server.Addr, "environment", cfg.Environment)
	if err := platform.Serve(ctx, server, listener); err != nil {
		logger.Error("api stopped unexpectedly")
		os.Exit(1)
	}
	logger.Info("api stopped")
}
