package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"log/slog"

	"github.com/i5heu/ouroboros-crypt/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
	api "github.com/i5heu/ouroboros-db/api"
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

		ac, err := keys.NewAsyncCrypt()
		if err != nil {
			panic(err)
		}

		err = ac.SaveToFile(cfg.Paths[0] + "/ouroboros.key")
		if err != nil {
			panic(err)
		}

		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := ouroboros.New(cfg)
	if err != nil {
		slog.Error("failed to construct DB", "error", err)
		os.Exit(1)
	}

	if err := db.Start(ctx); err != nil {
		slog.Error("failed to start DB", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:    ":8083",
		Handler: api.New(db, api.WithLogger(logger)),
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting API server", "addr", "http://localhost"+httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				serverErr <- err
				return
			}
		}
		serverErr <- nil
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil {
			slog.Error("API server encountered an error", "error", err)
			stop()
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("API server shutdown error", "error", err)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			slog.Error("API server encountered an error", "error", err)
		}
	default:
	}

	if err := db.Close(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("graceful shutdown error", "error", err)
	}
}
