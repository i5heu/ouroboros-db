// Package carrier defines interfaces for inter-node communication in
// OuroborosDB.
//
// The Carrier is the communication backbone of an OuroborosDB cluster. It
// provides reliable, encrypted message passing between nodes using QUIC as the
// underlying
// transport protocol.
//
// # Architecture
//
// The carrier stack consists of:
//
//	┌─────────────────────────────────────────────┐
//	│                  Carrier                    │
//	│    (high-level: broadcast, join/leave)      │
//	├─────────────────────────────────────────────┤
//	│                 Transport                   │
//	│      (QUIC connections and listeners)       │
//	├─────────────────────────────────────────────┤
//	│              Connection/Listener            │
//	│    (send/receive, encrypt/decrypt)          │
//	└─────────────────────────────────────────────┘
//
// # Security
//
// All inter-node communication is secured with:
//   - TLS 1.3 (via QUIC) for transport security
//   - ML-KEM for post-quantum key encapsulation
//   - ML-DSA for post-quantum digital signatures
//   - AES-256-GCM for symmetric encryption
//
// # Message Flow
//
// Outgoing messages:
//  1. Application calls SendMessageToNode or Broadcast
//  2. Carrier encrypts the message for the recipient(s)
//  3. Transport sends via QUIC connection
//  4. Recipient's handler is invoked
//
// Incoming messages:
//  1. Transport receives via QUIC connection
//  2. Carrier decrypts and verifies signature
//  3. Message dispatched to registered handler(s)
//  4. Handler may return a response message
//
// # Cluster Membership
//
// The Carrier manages cluster membership through:
//   - JoinCluster: Request to join an existing cluster
//   - LeaveCluster: Gracefully depart from the cluster
//   - BootstrapFromAddresses: Initial cluster discovery
//   - NewNodeAnnouncement: Propagate new members to all nodes
package carrier

import (
	"context"

	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// Carrier defines the interface for inter-node communication.
//
// Carrier is the primary interface for all cluster communication. It handles
// connection management, message routing, encryption, and cluster membership.
//
// # Thread Safety
//
// All methods are safe for concurrent use from multiple goroutines.
//
// # Message Handlers
//
// Register handlers before calling Start() to ensure no messages are missed.
// Handlers are called synchronously; long-running handlers should spawn
// goroutines to avoid blocking the receive loop.
type Carrier interface {
	// GetNodes returns all known nodes in the cluster.
	//
	// Returns:
	//   - Slice of all known cluster nodes
	//   - Error if the node list cannot be retrieved
	//
	// This returns nodes that have been discovered through:
	//   - Bootstrap addresses
	//   - Node announcements
	//   - Join requests
	GetNodes(ctx context.Context) ([]cluster.Node, error)

	// Broadcast sends a message to all nodes in the cluster.
	//
	// This sends the message to every known node in parallel. The operation
	// completes when all sends have finished (success or failure).
	//
	// Parameters:
	//   - message: The message to broadcast
	//
	// Returns:
	//   - BroadcastResult with success/failure for each node
	//   - Error only for catastrophic failures (e.g., no nodes available)
	//
	// Individual node failures are reported in BroadcastResult.FailedNodes,
	// not as the returned error.
	Broadcast(ctx context.Context, message Message) (*BroadcastResult, error)

	// SendMessageToNode sends a message to a specific node.
	//
	// The message is encrypted for the target node and sent over QUIC.
	// This blocks until the message is acknowledged or an error occurs.
	//
	// Parameters:
	//   - nodeID: The target node's ID
	//   - message: The message to send
	//
	// Returns:
	//   - Error if sending fails (node not found, connection failed, timeout)
	SendMessageToNode(
		ctx context.Context,
		nodeID cluster.NodeID,
		message Message,
	) error

	// JoinCluster requests to join a cluster via the specified node.
	//
	// This initiates the cluster join protocol:
	//  1. Connect to the bootstrap node
	//  2. Send NodeJoinRequest with our certificate
	//  3. Receive current cluster membership
	//  4. Announce ourselves to all nodes (NewNodeAnnouncement)
	//
	// Parameters:
	//   - clusterNode: A node already in the cluster
	//   - cert: This node's certificate for authentication
	//
	// Returns:
	//   - Error if join fails (authentication, network, rejected)
	JoinCluster(
		ctx context.Context,
		clusterNode cluster.Node,
		cert cluster.NodeCert,
	) error

	// LeaveCluster notifies the cluster that this node is leaving.
	//
	// This gracefully removes the node from the cluster:
	//  1. Broadcast NodeLeaveNotification to all nodes
	//  2. Wait for acknowledgments (with timeout)
	//  3. Close all connections
	//
	// Other nodes will remove this node from their membership and may
	// trigger data rebalancing.
	//
	// Returns:
	//   - Error if the leave notification cannot be sent
	LeaveCluster(ctx context.Context) error

	// Start begins listening for incoming connections.
	//
	// This starts the Carrier and makes it operational:
	//  1. Start the QUIC listener on configured address(es)
	//  2. Begin accepting incoming connections
	//  3. Spawn goroutines for connection handling
	//
	// Parameters:
	//   - ctx: Context for cancellation; if cancelled, Start returns early
	//
	// Returns:
	//   - Error if the listener cannot be started
	//
	// After Start returns successfully, the Carrier is ready for communication.
	// Call RegisterHandler before Start to ensure handlers are ready.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the carrier.
	//
	// This cleanly stops all carrier operations:
	//  1. Stop accepting new connections
	//  2. Close all existing connections
	//  3. Stop the listener
	//
	// Parameters:
	//   - ctx: Context for timeout; if deadline exceeded, force shutdown
	//
	// Returns:
	//   - Error if shutdown encounters problems
	Stop(ctx context.Context) error

	// RegisterHandler registers a handler for a specific message type.
	//
	// Handlers are called when messages of the specified type are received.
	// Multiple handlers can be registered for the same type; all will be called.
	//
	// Parameters:
	//   - msgType: The message type to handle
	//   - handler: The callback function
	//
	// Handlers should be registered before Start() to avoid missing messages.
	// Handlers are called synchronously; use goroutines for long operations.
	RegisterHandler(msgType MessageType, handler MessageHandler)

	// BootstrapFromAddresses attempts to join a cluster from seed addresses.
	//
	// This is the primary way to join an existing cluster:
	//  1. Try each address in order
	//  2. Connect and request node information
	//  3. Call JoinCluster with the discovered node
	//  4. Return on first success
	//
	// Parameters:
	//   - addresses: List of "host:port" addresses to try
	//
	// Returns:
	//   - Error if all addresses fail
	//
	// If all addresses fail, the node cannot join the cluster. It may
	// operate standalone or retry later.
	BootstrapFromAddresses(ctx context.Context, addresses []string) error
}

// BootStrapper handles the initialization of nodes joining the cluster.
//
// BootStrapper prepares a new node for cluster participation. This includes
// ensuring the node has proper identity, configuration, and initial state
// before it can communicate with other cluster members.
type BootStrapper interface {
	// BootstrapNode initializes a node for cluster participation.
	//
	// This prepares a node to join the cluster:
	//   - Verify node identity (keys, certificate)
	//   - Initialize local storage
	//   - Set up communication channels
	//
	// Parameters:
	//   - node: The node to bootstrap
	//
	// Returns:
	//   - Error if bootstrapping fails
	BootstrapNode(ctx context.Context, node cluster.Node) error
}
