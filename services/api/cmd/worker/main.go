package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/config"
)

func main() {
	if _, err := config.Load(os.LookupEnv); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	slog.Info("worker starting", "mode", "bootstrap")
	<-ctx.Done()
	slog.Info("worker stopped")
}
