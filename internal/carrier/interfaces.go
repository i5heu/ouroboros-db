package carrier

import (
	"context"
	"log/slog"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// MessageHandler is a callback function for handling incoming messages.
// It receives the sender's node ID, the message, and should return a response
// message (or nil if no response is needed) and any error.
type MessageHandler func(
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) // A

// Carrier defines the interface for inter-node communication.
type Carrier interface { // A
	// GetNodes returns all known nodes in the cluster.
	GetNodes(ctx context.Context) ([]Node, error)

	// LocalNode returns the local node's identity.
	LocalNode() Node

	// Broadcast sends a message to all nodes in the cluster.
	// Returns the nodes that successfully received the message and any error.
	Broadcast(ctx context.Context, message Message) (*BroadcastResult, error)

	// SendMessageToNode sends a message to a specific node.
	SendMessageToNode(ctx context.Context, nodeID NodeID, message Message) error

	// JoinCluster requests to join a cluster via the specified node.
	JoinCluster(ctx context.Context, clusterNode Node, cert NodeCert) error

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

	// SetLogger updates the logger used by the carrier.
	// This is useful for resolving circular dependencies (e.g. log broadcasting).
	SetLogger(logger *slog.Logger)
}

// BootStrapper handles the initialization of nodes joining the cluster.
type BootStrapper interface { // A
	// BootstrapNode initializes a node for cluster participation.
	BootstrapNode(ctx context.Context, node Node) error
}

// Transport defines the low-level network operations for the carrier.
// The default implementation uses QUIC for reliable, multiplexed communication.
type Transport interface { // A
	// Connect establishes an encrypted connection to a node.
	// For QUIC transport, this initiates a TLS 1.3 handshake.
	Connect(ctx context.Context, address string) (Connection, error)
	// Listen starts accepting incoming connections on the specified address.
	// For QUIC transport, address should be in the format "host:port".
	// Returns a Listener that can be used to accept connections.
	Listen(ctx context.Context, address string) (Listener, error)
	// Close closes the transport and releases any resources.
	Close() error

	// SetLogger updates the logger used by the transport.
	SetLogger(logger *slog.Logger)
}

// Listener accepts incoming connections from remote nodes.
type Listener interface { // A
	// Accept waits for and returns the next incoming connection.
	Accept(ctx context.Context) (Connection, error)
	// Addr returns the listener's network address.
	Addr() string
	// Close stops the listener.
	Close() error
}

// Connection represents a network connection to another node.
// Connections use QUIC streams for multiplexed, ordered, reliable delivery.
type Connection interface { // A
	// Send transmits a message over the connection.
	// Messages are encrypted using the recipient's public key.
	Send(ctx context.Context, msg Message) error
	// SendEncrypted transmits a pre-encrypted message over the connection.
	SendEncrypted(ctx context.Context, enc *EncryptedMessage) error
	// Receive waits for and returns the next message.
	Receive(ctx context.Context) (Message, error)
	// ReceiveEncrypted waits for and returns the next encrypted message.
	ReceiveEncrypted(ctx context.Context) (*EncryptedMessage, error)
	// Close terminates the connection.
	Close() error
	// RemoteNodeID returns the ID of the connected node.
	RemoteNodeID() NodeID
	// RemotePublicKey returns the public key of the connected node.
	RemotePublicKey() *keys.PublicKey
}
