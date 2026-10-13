package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	owner := flag.String("owner", "", "UUID владельца")
	all := flag.Bool("all", false, "Пересчитать всех владельцев пакетами до 500")
	flag.Parse()
	if (*owner == "" && !*all) || (*owner != "" && *all) || (*owner != "" && !character.ValidOwner(*owner)) {
		slog.Error("specify exactly one valid --owner or --all")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, e := storage.Open(ctx, os.Getenv("DATABASE_URL"))
	if e != nil {
		slog.Error("database configuration unavailable")
		os.Exit(1)
	}
	defer pool.Close()
	clock := platform.RealClock{}
	svc := character.NewService(pool, clock, progression.NewService(pool, clock))
	count := 0
	if *all {
		count, e = svc.RecomputeAll(ctx)
	} else {
		_, e = svc.RecomputeOwner(ctx, *owner)
		if e == nil {
			count = 1
		}
	}
	if e != nil {
		slog.Error("character reconciliation failed")
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(map[string]int{"recomputed": count}) != nil {
		os.Exit(1)
	}
}
