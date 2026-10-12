package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/config"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
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
	quests.NewService(pool, platform.RealClock{}).Mount(router, sessions.Protect)
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
