// Package stream provides transport-level abstractions for carrier
// communication.
//
// This package contains the Transport, Listener, and Connection interfaces
// along with their QUIC implementation and connection pooling logic.
package stream

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// NodeID is an alias for cluster.NodeID used throughout the stream package.
type NodeID = cluster.NodeID

// Message represents a message exchanged between nodes.
type Message struct { // A
	// Type identifies what kind of message this is.
	Type uint8
	// Payload is the message data, format depends on Type.
	Payload []byte
}

// EncryptedMessage represents a message encrypted for a specific recipient.
type EncryptedMessage struct { // A
	// SenderID identifies the sender of the message.
	SenderID NodeID
	// Ciphertext is the encrypted payload.
	Ciphertext []byte
	// Nonce is the encryption nonce.
	Nonce []byte
	// EncapsulatedKey is the ML-KEM encapsulated key.
	EncapsulatedKey []byte
	// Signature is the sender's signature over the encrypted data.
	Signature []byte
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
	// Close shuts down the transport and all active connections.
	Close() error
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

// Closeable extends Connection with a method to check if closed.
type Closeable interface { // A
	Connection
	// IsClosed returns whether the connection has been closed.
	IsClosed() bool
}
