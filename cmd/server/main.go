package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"log/slog"

	"github.com/i5heu/ouroboros-crypt/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
)

func main() {

	cfg := ouroboros.Config{
		Paths:         []string{"./data"},
		MinimumFreeGB: 1,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	slog.SetDefault(logger)

	create := flag.Bool("create", false, "create a new keypair and exit")
	flag.Parse()

	if *create {
		slog.Info("creating new keypair...")

		// Generate a new key pair
		ac, err := keys.NewAsyncCrypt()
		if err != nil {
			panic(err)
		}

		// Save keys to a file
		err = ac.SaveToFile(cfg.Paths[0] + "/ouroboros.key")
		if err != nil {
			panic(err)
		}

		return
	}

	// Create a context that is canceled on Ctrl+C or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Construct the DB (no heavy side effects here).
	db, err := ouroboros.New(cfg)
	if err != nil {
		slog.Error("failed to construct DB", "error", err)
		os.Exit(1)
	}

	// Start the DB and its subsystems.
	if err := db.Start(ctx); err != nil {
		slog.Error("failed to start DB", "error", err)
		os.Exit(1)
	}

	// Block until a shutdown signal is received.
	<-ctx.Done()

	// Attempt a graceful shutdown with a timeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.Close(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("graceful shutdown error", "error", err)
	}
}
