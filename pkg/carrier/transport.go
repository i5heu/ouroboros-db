package carrier

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// Transport defines the low-level network operations for the carrier.
// The default implementation uses QUIC for reliable, multiplexed communication.
type Transport interface {
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
type Listener interface {
	// Accept waits for and returns the next incoming connection.
	Accept(ctx context.Context) (Connection, error)

	// Addr returns the listener's network address.
	Addr() string

	// Close stops the listener.
	Close() error
}

// Connection represents a network connection to another node.
// Connections use QUIC streams for multiplexed, ordered, reliable delivery.
type Connection interface {
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
	RemoteNodeID() cluster.NodeID

	// RemotePublicKey returns the public key of the connected node.
	RemotePublicKey() *keys.PublicKey
}

// EncryptedMessage represents a message encrypted for a specific recipient.
type EncryptedMessage struct {
	// SenderID identifies the sender of the message.
	SenderID cluster.NodeID

	// Ciphertext is the encrypted payload.
	Ciphertext []byte

	// Nonce is the encryption nonce.
	Nonce []byte

	// EncapsulatedKey is the key encapsulated for the recipient.
	EncapsulatedKey []byte

	// Signature is the sender's signature over the encrypted data.
	Signature []byte
}
