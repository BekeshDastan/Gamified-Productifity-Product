package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/config"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
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
	logger.Info("worker starting")
	jobs.Run(ctx, jobs.Store{Pool: pool, Clock: platform.RealClock{}}, map[string]jobs.Handler{}, logger)
	logger.Info("worker stopped")
}
