package cluster

// Cluster represents a collection of nodes in OuroborosDB.
type Cluster interface {
	// GetNodes returns all nodes currently in the cluster.
	GetNodes() []Node

	// AddNode adds a node to the cluster.
	AddNode(node Node) error

	// RemoveNode removes a node from the cluster.
	RemoveNode(nodeID NodeID) error

	// GetNode returns a specific node by its ID.
	GetNode(nodeID NodeID) (Node, error)
}
