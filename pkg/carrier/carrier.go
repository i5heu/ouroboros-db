// Package carrier defines interfaces for inter-node communication in OuroborosDB.
package carrier

import (
	"context"

	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// Carrier defines the interface for inter-node communication.
type Carrier interface {
	// GetNodes returns all known nodes in the cluster.
	GetNodes(ctx context.Context) ([]cluster.Node, error)

	// Broadcast sends a message to all nodes in the cluster.
	// Returns the nodes that successfully received the message and any error.
	Broadcast(ctx context.Context, message Message) (*BroadcastResult, error)

	// SendMessageToNode sends a message to a specific node.
	SendMessageToNode(ctx context.Context, nodeID cluster.NodeID, message Message) error

	// JoinCluster requests to join a cluster via the specified node.
	JoinCluster(ctx context.Context, clusterNode cluster.Node, cert cluster.NodeCert) error

	// LeaveCluster notifies the cluster that this node is leaving.
	LeaveCluster(ctx context.Context) error

	// Start begins listening for incoming connections on the local node's
	// address. It returns immediately; connections are handled in background
	// goroutines.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the carrier, closing all connections and
	// stopping the listener.
	Stop(ctx context.Context) error

	// RegisterHandler registers a handler for a specific message type.
	// Multiple handlers can be registered for the same type.
	RegisterHandler(msgType MessageType, handler MessageHandler)

	// BootstrapFromAddresses attempts to join a cluster by connecting to one
	// of the provided bootstrap addresses. It tries each address in order
	// until one succeeds. Returns an error if all addresses fail.
	BootstrapFromAddresses(ctx context.Context, addresses []string) error
}

// BootStrapper handles the initialization of nodes joining the cluster.
type BootStrapper interface {
	// BootstrapNode initializes a node for cluster participation.
	BootstrapNode(ctx context.Context, node cluster.Node) error
}
