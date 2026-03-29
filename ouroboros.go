/*
!! Currently the database is in a very early stage of development !!
!! It must not be used in production environments !!
*/
package ouroboros

import (
	"fmt"
	"log/slog"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/cluster"
	"github.com/i5heu/ouroboros-db/internal/node"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

const (
	CurrentDbVersion = "v0.1.1-alpha-3"
)

type OuroborosDB struct {
	log     *slog.Logger
	config  Config
	node    *node.Node
	cluster interfaces.ClusterController
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
	clusterController, err := cluster.NewClusterController(
		n,
		conf.Logger,
		conf.ClusterListenAddress,
		conf.TrustedAdminPubKeys,
		conf.LocalNodeCerts,
		conf.LocalCASignatures,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"init cluster controller: %w",
			err,
		)
	}

	return &OuroborosDB{
		log:     conf.Logger,
		config:  conf,
		node:    n,
		cluster: clusterController,
	}, nil
}

// NodeID returns the cryptographic identity of this
// database node.
func (db *OuroborosDB) NodeID() keys.NodeID { // H
	return db.node.ID()
}

// ClusterController exposes the cluster controller for
// integration and demos.
func (db *OuroborosDB) ClusterController( // A
) interfaces.ClusterController {
	return db.cluster
}

// Crypt exposes the node cryptographic identity for
// integration and demos.
func (db *OuroborosDB) Crypt() *crypt.Crypt { // A
	return db.node.Crypt()
}

func GetVersion() string {
	return CurrentDbVersion
}
