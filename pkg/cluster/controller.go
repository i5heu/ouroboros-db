package cluster

import (
	"context"
)

// ClusterController manages the cluster operations and coordinates
// between various subsystems.
//
// ClusterController is the central coordination point for a node's
// participation in the cluster. It manages the lifecycle of cluster-related
// components and coordinates their interactions.
//
// # Managed Components
//
// The ClusterController coordinates:
//
//   - Carrier: Inter-node communication (QUIC transport)
//   - DistributedIndex: Hash-to-node mappings
//   - DataReBalancer: Data distribution during topology changes
//   - ClusterMonitor: Node health monitoring
//   - BackupManager: Scheduled backups
//
// # Lifecycle
//
//	┌─────────┐   Start()   ┌─────────┐   Stop()   ┌─────────┐
//	│ Created │ ─────────► │ Running │ ─────────► │ Stopped │
//	└─────────┘             └─────────┘            └─────────┘
//
// Start() initializes all components and joins the cluster.
// Stop() gracefully leaves the cluster and shuts down components.
//
// # Startup Sequence
//
//  1. Initialize Carrier and start listening
//  2. Bootstrap from seed nodes (if configured)
//  3. Sync cluster membership
//  4. Initialize DistributedIndex
//  5. Start health monitoring
//  6. Signal ready for operations
//
// # Shutdown Sequence
//
//  1. Stop accepting new operations
//  2. Announce departure to cluster (NodeLeaveNotification)
//  3. Wait for in-flight operations to complete
//  4. Stop health monitoring
//  5. Close Carrier connections
//  6. Persist any pending state
//
// # Thread Safety
//
// Implementations must be safe for concurrent use. Start and Stop
// should be called from a single goroutine, but GetCluster and
// GetLocalNode may be called from any goroutine.
type ClusterController interface {
	// Start initializes and starts the cluster controller.
	//
	// This begins the node's participation in the cluster:
	//   - Starts the Carrier listener
	//   - Connects to seed nodes (if any)
	//   - Syncs cluster membership
	//   - Starts background services (monitoring, rebalancing)
	//
	// Parameters:
	//   - ctx: Context for cancellation; if cancelled, Start returns early
	//
	// Returns:
	//   - Error if initialization fails
	//
	// After Start returns successfully, the node is fully operational
	// and can handle cluster operations.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the cluster controller.
	//
	// This cleanly removes the node from the cluster:
	//   - Announces departure to peers
	//   - Stops accepting new operations
	//   - Waits for in-flight operations (with timeout)
	//   - Closes all connections
	//   - Persists state
	//
	// Parameters:
	//   - ctx: Context for timeout; if deadline exceeded, force shutdown
	//
	// Returns:
	//   - Error if shutdown encounters problems (logged but not fatal)
	//
	// After Stop returns, the node is no longer part of the cluster.
	Stop(ctx context.Context) error

	// GetCluster returns the cluster managed by this controller.
	//
	// Returns:
	//   - The Cluster interface for querying membership
	//
	// This can be called at any time, even before Start or after Stop,
	// but the cluster will only be populated while the controller is running.
	GetCluster() Cluster

	// GetLocalNode returns the local node's information.
	//
	// Returns:
	//   - This node's Node struct (NodeID, Addresses, Cert)
	//
	// This is useful for:
	//   - Identifying this node in logs
	//   - Providing node info to peers
	//   - Checking local addresses
	GetLocalNode() Node
}
