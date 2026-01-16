package cluster

// Cluster represents a collection of nodes in OuroborosDB.
//
// The Cluster interface provides access to the current cluster membership.
// It maintains the authoritative view of which nodes are part of the cluster
// and their current state.
//
// # Membership Management
//
// Cluster membership changes through:
//   - AddNode: A new node joins (after authentication via Carrier)
//   - RemoveNode: A node leaves (graceful departure or eviction)
//
// # Consistency
//
// In a distributed system, different nodes may have temporarily different
// views of cluster membership. The Carrier propagates membership changes
// to eventually achieve consistency across all nodes.
//
// # Thread Safety
//
// Implementations must be safe for concurrent use. Multiple goroutines may
// query and modify membership simultaneously.
type Cluster interface {
	// GetNodes returns all nodes currently in the cluster.
	//
	// Returns:
	//   - Slice of all known cluster nodes
	//
	// The returned slice is a snapshot; subsequent changes to the cluster
	// will not be reflected. The order is not guaranteed.
	GetNodes() []Node

	// AddNode adds a node to the cluster.
	//
	// This should be called after the node has been authenticated
	// (e.g., after receiving a valid NodeJoinRequest via Carrier).
	//
	// Parameters:
	//   - node: The node to add
	//
	// Returns:
	//   - Error if the node cannot be added (e.g., duplicate NodeID)
	//
	// Adding a node that already exists updates its information.
	AddNode(node Node) error

	// RemoveNode removes a node from the cluster.
	//
	// This should be called when a node gracefully leaves or is evicted
	// due to prolonged unavailability.
	//
	// Parameters:
	//   - nodeID: The ID of the node to remove
	//
	// Returns:
	//   - Error if removal fails
	//
	// Removing a non-existent node is a no-op (no error).
	RemoveNode(nodeID NodeID) error

	// GetNode returns a specific node by its ID.
	//
	// Parameters:
	//   - nodeID: The ID of the node to retrieve
	//
	// Returns:
	//   - The node if found
	//   - Error if the node is not in the cluster
	GetNode(nodeID NodeID) (Node, error)
}
