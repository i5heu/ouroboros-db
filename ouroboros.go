/*
!! Currently the database is in a very early stage of development and should not be used in production environments. !!
*/
package ouroboros

import (
	"fmt"
	"log/slog"

	crypt "github.com/i5heu/ouroboros-crypt"
)

const (
	CurrentDbVersion = "2.0.0-alpha-1"
)

type OuroborosDB struct {
	log    *slog.Logger
	config Config
	crypt  *crypt.Crypt
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
