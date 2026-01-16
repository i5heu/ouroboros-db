/*
!! Currently the database is in a very early stage of development !!
!! It must not be used in production environments !!
*/
package ouroboros

import (
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
)

const (
	CurrentDbVersion = "v0.1.1-alpha-3"
)

type OuroborosDB struct {
	log    *slog.Logger
	config Config
	db     *badger.DB
}

func New(conf Config) (*OuroborosDB, error) { // A
	if len(conf.Paths) == 0 {
		return nil, fmt.Errorf("at least one path must be provided in config")
	}
	if conf.Logger == nil {
		conf.Logger = defaultLogger()
	}

	opts := badger.DefaultOptions(conf.Paths[0])
	// Suppress Badger's default logging to stdout/stderr unless we want it.
	// We can adapt slog if needed, but for now let's keep it quiet or default.
	// badger.DefaultOptions defaults to logging to stderr.
	// If we want to use the provided logger, we would need an adapter.
	// For this step, let's just use defaults but maybe lower verbosity if possible?
	// Badger doesn't have a simple level knob in Options.Logger, it takes an interface.
	// Let's leave default logging for now.

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger db: %w", err)
	}

	return &OuroborosDB{
		log:    conf.Logger,
		config: conf,
		db:     db,
	}, nil
}

// Close closes the database and releases resources.
func (db *OuroborosDB) Close() error {
	if db.db != nil {
		return db.db.Close()
	}
	return nil
}
