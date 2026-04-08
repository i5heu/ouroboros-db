/*
!! Currently the database is in a very early stage of development !!
!! It must not be used in production environments !!
*/
package ouroboros

import (
	"fmt"
	"log/slog"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/entities/node"
)

const (
	CurrentDbVersion = "v0.1.1-alpha-3"
)

type OuroborosDB struct {
	log    *slog.Logger
	config Config
	node   *node.Node
}

func New(conf *Config) (*OuroborosDB, error) { // AC
	primaryPath, err := conf.PrimaryPath()
	if err != nil {
		return nil, err
	}
	if conf.Logger == nil {
		conf.Logger = defaultLogger()
	}
	conf.Storage.Paths = conf.EffectivePaths()
	conf.Storage.MinimumFreeGB = conf.EffectiveMinimumFreeGB()

	n, err := node.New(primaryPath)
	if err != nil {
		return nil, fmt.Errorf(
			"init node: %w",
			err,
		)
	}

	return &OuroborosDB{
		log:    conf.Logger,
		config: *conf,
		node:   n,
	}, nil
}

// NodeID returns the cryptographic identity of this
// database node.
func (db *OuroborosDB) NodeID() keys.NodeID { // H
	return db.node.ID()
}

func GetVersion() string { // A
	return CurrentDbVersion
}
