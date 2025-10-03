/*
!! Currently the database is in a very early stage of development and should not be used in production environments. !!
*/
package ouroboros

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	crypt "github.com/i5heu/ouroboros-crypt"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

// OuroborosDB is the main database handle. It owns the crypt layer, the KV
// store, and the lifecycle of background components.
type OuroborosDB struct {
	log    *slog.Logger
	config Config

	crypt *crypt.Crypt
	kv    *ouroboroskv.KV

	started   atomic.Bool
	startOnce sync.Once
	closeOnce sync.Once
	mu        sync.Mutex
}

// Config configures the database instance. Only Paths[0] is used at the
// moment; future versions may use multiple paths for sharding or tiering.
type Config struct {
	// Paths contains data directories. Currently only Paths[0] is used.
	Paths []string
	// MinimumFreeGB is a free-space threshold for on-disk operations.
	MinimumFreeGB uint
	// Logger is an optional structured logger. If nil, a stderr logger is used.
	Logger *slog.Logger
}

// defaultLogger returns a logger that writes text logs to stderr at Info level.
// Applications can inject their own slog.Logger for JSON, different levels, etc.
func defaultLogger() *slog.Logger {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(h)
}

// New constructs a database handle. New does not perform heavy I/O or start
// background goroutines. Call Start to initialize subsystems.
func New(conf Config) (*OuroborosDB, error) {
	if len(conf.Paths) == 0 {
		return nil, fmt.Errorf("at least one path must be provided in config")
	}
	if conf.Logger == nil {
		conf.Logger = defaultLogger()
	}
	return &OuroborosDB{
		log:    conf.Logger,
		config: conf,
	}, nil
}

// Start initializes the crypt layer and KV store and marks the database as
// ready. Start is safe to call multiple times; only the first call has effect.
func (ou *OuroborosDB) Start(ctx context.Context) error {
	var startErr error
	ou.startOnce.Do(func() {
		dataRoot := ou.config.Paths[0]
		if err := os.MkdirAll(dataRoot, 0o700); err != nil {
			startErr = fmt.Errorf("mkdir %s: %w", dataRoot, err)
			return
		}

		// Initialize ouroboros-crypt from Paths[0]/ouroboros.key.
		cryptKeyPath := filepath.Join(dataRoot, "ouroboros.key")
		c, err := crypt.NewFromFile(cryptKeyPath)
		if err != nil {
			startErr = fmt.Errorf("init crypt: %w", err)
			return
		}

		// Initialize KV under Paths[0]/kv.
		kvConfig := &ouroboroskv.Config{
			Paths:            []string{filepath.Join(dataRoot, "kv")},
			MinimumFreeSpace: int(ou.config.MinimumFreeGB),
			Logger:           ou.log, // assumes ouroboros-kv also uses slog.Logger
		}
		kv, err := ouroboroskv.Init(c, kvConfig)
		if err != nil {
			startErr = fmt.Errorf("init kv: %w", err)
			return
		}

		ou.mu.Lock()
		ou.crypt = c
		ou.kv = kv
		ou.mu.Unlock()

		ou.started.Store(true)
		ou.log.Info("OuroborosDB started", "path", dataRoot)
	})
	return startErr
}

// Run starts the database, then blocks until ctx is canceled, and finally
// performs a bounded graceful shutdown. It is a convenience for services.
func (ou *OuroborosDB) Run(ctx context.Context) error {
	if err := ou.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ou.Close(shutdownCtx)
}

// Close terminates background components and releases resources. Close is
// idempotent and safe to call multiple times.
func (ou *OuroborosDB) Close(ctx context.Context) error {
	var closeErr error
	ou.closeOnce.Do(func() {
		ou.mu.Lock()
		defer ou.mu.Unlock()

		if ou.kv != nil {
			if err := ou.kv.Close(); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("close kv: %w", err))
			}
			ou.kv = nil
		}
		// Add crypt teardown here if the crypt package provides one.

		ou.log.Info("OuroborosDB closed")
	})
	return closeErr
}

// CloseWithoutContext closes the database using a background context.
// Prefer Close(ctx) to enforce an application-specific shutdown deadline.
func (ou *OuroborosDB) CloseWithoutContext() error {
	return ou.Close(context.Background())
}
