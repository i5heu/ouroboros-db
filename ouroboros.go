/*
!! Currently the database is in a very early stage of development !!
!! It must not be used in production environments !!
*/
package ouroboros

import (
	"fmt"
	"log/slog"
)

const (
	CurrentDbVersion = "v0.1.0-alpha-2"
)

type OuroborosDB struct {
	log    *slog.Logger
	config Config
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
