package cluster

import (
	"context"
)

// ClusterController manages the cluster operations and coordinates
// between various subsystems like Carrier, Index, and DataReBalancer.
type ClusterController interface {
	// Start initializes and starts the cluster controller.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the cluster controller.
	Stop(ctx context.Context) error

	// GetCluster returns the cluster managed by this controller.
	GetCluster() Cluster

	// GetLocalNode returns the local node's information.
	GetLocalNode() Node
}
