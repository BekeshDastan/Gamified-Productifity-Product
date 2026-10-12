package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/config"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/migrations"
)

func main() {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := migrations.Up(ctx, cfg.DatabaseURL); err != nil {
		slog.Error("migration failed; inspect database and migration version")
		os.Exit(1)
	}
	slog.Info("migrations applied", "environment", cfg.Environment)
}
