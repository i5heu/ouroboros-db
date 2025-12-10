/*
!! Currently the database is in a very early stage of development and should not be used in production environments. !!
*/
package ouroboros

import (
	"fmt"
	"log/slog"
	"os"

	crypt "github.com/i5heu/ouroboros-crypt"
)

type OuroborosDB struct {
	log    *slog.Logger
	config Config
	crypt  *crypt.Crypt
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
	// UiPort specifies the port for the built-in web UI. If 0, the UI is disabled.
	UiPort uint16
}

func defaultLogger() *slog.Logger { // A
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(h)
}

func New(conf Config) (*OuroborosDB, error) { // A
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
