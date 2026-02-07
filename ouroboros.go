/*
!! Currently the database is in a very early stage of development !!
!! It must not be used in production environments !!
*/
package ouroboros

import (
	"fmt"
	"log/slog"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/node"
)

const (
	CurrentDbVersion = "v0.1.1-alpha-3"
)

type OuroborosDB struct {
	log    *slog.Logger
	config Config
	node   *node.Node
}

func New(conf Config) (*OuroborosDB, error) { // AC
	if len(conf.Paths) == 0 {
		return nil, fmt.Errorf("at least one path must be provided in config")
	}
	if conf.Logger == nil {
		conf.Logger = defaultLogger()
	}

	n, err := node.New(conf.Paths[0])
	if err != nil {
		return nil, fmt.Errorf(
			"init node: %w",
			err,
		)
	}

	return &OuroborosDB{
		log:    conf.Logger,
		config: conf,
		node:   n,
	}, nil
}

// NodeID returns the cryptographic identity of this
// database node.
func (db *OuroborosDB) NodeID() keys.NodeID { // H
	return db.node.ID()
}

func GetVersion() string {
	return CurrentDbVersion
}
