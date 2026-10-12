package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	slog.Info("worker starting", "mode", "bootstrap")
	<-ctx.Done()
	slog.Info("worker stopped")
}
