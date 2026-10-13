package main

import (
	"context"
	"encoding/json"
	"flag"
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
	all := flag.Bool("all", false, "Проверить все существующие проекции")
	dry := flag.Bool("dry-run", false, "Только сравнить, без исправления")
	flag.Parse()
	if (*owner == "" && !*all) || (*owner != "" && *all) {
		slog.Error("specify exactly one of --owner or --all")
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
	svc := progression.NewService(pool, platform.RealClock{})
	checked, mismatches, rebuilt := 0, 0, 0
	run := func(id string) error {
		var r progression.Reconciliation
		var e error
		if *dry {
			r, e = svc.Check(ctx, id)
		} else {
			r, e = svc.Rebuild(ctx, id)
		}
		if e != nil {
			return e
		}
		checked++
		if r.Changed {
			mismatches++
			if !*dry {
				rebuilt++
			}
		}
		return nil
	}
	if *all {
		cursor := ""
		for {
			ids, e := svc.OwnerBatch(ctx, cursor)
			if e != nil {
				slog.Error("owner batch unavailable")
				os.Exit(1)
			}
			if len(ids) == 0 {
				break
			}
			for _, id := range ids {
				if e = run(id); e != nil {
					slog.Error("progression reconciliation failed")
					os.Exit(1)
				}
			}
			cursor = ids[len(ids)-1]
		}
	} else if e = run(*owner); e != nil {
		slog.Error("progression reconciliation failed")
		os.Exit(1)
	}
	if e = json.NewEncoder(os.Stdout).Encode(map[string]int{"checked": checked, "mismatches": mismatches, "rebuilt": rebuilt}); e != nil {
		os.Exit(1)
	}
}
