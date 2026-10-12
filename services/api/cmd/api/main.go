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

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/config"
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
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           platform.NewRouter(logger, platform.RealClock{}),
		ReadHeaderTimeout: 5 * time.Second,
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
