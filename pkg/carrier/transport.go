package carrier

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// Transport defines the low-level network operations for the carrier.
//
// Transport is the abstraction layer for network communication. The default
// implementation uses QUIC, which provides:
//
//   - Reliable, ordered delivery (like TCP)
//   - Multiplexed streams over a single connection
//   - Built-in TLS 1.3 encryption
//   - Connection migration (handles IP changes)
//   - 0-RTT connection resumption
//
// # Connection Lifecycle
//
//	Connect() ──► [Connection] ──► Send/Receive ──► Close()
//	              ▲
//	Listen() ──► Accept() ──┘
//
// # Thread Safety
//
// Transport methods must be safe for concurrent use.
// Multiple goroutines may Connect, Listen, and Close simultaneously.
type Transport interface {
	// Connect establishes an encrypted connection to a node.
	//
	// This initiates the connection handshake:
	//  1. DNS resolution (if hostname provided)
	//  2. QUIC connection establishment
	//  3. TLS 1.3 handshake
	//  4. Node identity exchange and verification
	//
	// Parameters:
	//   - address: Network address in "host:port" format
	//
	// Returns:
	//   - Connection ready for Send/Receive
	//   - Error if connection fails
	Connect(ctx context.Context, address string) (Connection, error)

	// Listen starts accepting incoming connections on the specified address.
	//
	// Parameters:
	//   - address: Local address to bind ("host:port" or ":port")
	//
	// Returns:
	//   - Listener for accepting connections
	//   - Error if binding fails
	Listen(ctx context.Context, address string) (Listener, error)

	// Close shuts down the transport and all active connections.
	//
	// This cleanly terminates all communication:
	//   - Close all active connections
	//   - Stop all listeners
	//   - Release network resources
	//
	// Returns:
	//   - Error if shutdown encounters problems
	Close() error
}

// Listener accepts incoming connections from remote nodes.
//
// Listener is returned by Transport.Listen() and is used to accept
// incoming connections in a server-like pattern.
type Listener interface {
	// Accept waits for and returns the next incoming connection.
	//
	// This blocks until:
	//   - A new connection is established
	//   - The context is cancelled
	//   - The listener is closed
	//
	// Returns:
	//   - The new Connection
	//   - Error if accept fails or listener is closed
	Accept(ctx context.Context) (Connection, error)

	// Addr returns the listener's network address.
	//
	// Returns the actual bound address, which may differ from the
	// requested address (e.g., if port 0 was specified).
	Addr() string

	// Close stops the listener.
	//
	// After Close, Accept will return an error.
	// Existing connections are NOT affected.
	Close() error
}

// Connection represents a network connection to another node.
//
// Connection provides bidirectional, encrypted communication with a peer.
// It uses QUIC streams for multiplexed, ordered, reliable delivery.
//
// # Message Format
//
// Messages can be sent in two ways:
//   - Send/Receive: Carrier handles encryption/decryption
//   - SendEncrypted/ReceiveEncrypted: Pre-encrypted messages
//
// # Thread Safety
//
// Send and Receive may be called concurrently from different goroutines.
// However, only one Send and one Receive should be active at a time
// (multiplexing is handled at the QUIC level).
type Connection interface {
	// Send transmits a message over the connection.
	//
	// The message is encrypted for the recipient before sending.
	// This blocks until the message is sent or an error occurs.
	//
	// Parameters:
	//   - msg: The message to send
	//
	// Returns:
	//   - Error if sending fails
	Send(ctx context.Context, msg Message) error

	// SendEncrypted transmits a pre-encrypted message over the connection.
	//
	// Use this when you need control over encryption (e.g., for
	// message signing or custom encryption schemes).
	//
	// Parameters:
	//   - enc: The pre-encrypted message
	//
	// Returns:
	//   - Error if sending fails
	SendEncrypted(ctx context.Context, enc *EncryptedMessage) error

	// Receive waits for and returns the next message.
	//
	// The message is decrypted automatically before returning.
	// This blocks until a message is received or an error occurs.
	//
	// Returns:
	//   - The decrypted message
	//   - Error if receiving or decryption fails
	Receive(ctx context.Context) (Message, error)

	// ReceiveEncrypted waits for and returns the next encrypted message.
	//
	// Use this when you need to handle decryption manually or
	// verify signatures before decryption.
	//
	// Returns:
	//   - The encrypted message (not decrypted)
	//   - Error if receiving fails
	ReceiveEncrypted(ctx context.Context) (*EncryptedMessage, error)

	// Close terminates the connection.
	//
	// This gracefully closes the QUIC connection:
	//   - Send connection close frame
	//   - Wait for acknowledgment (with timeout)
	//   - Release resources
	//
	// Returns:
	//   - Error if close encounters problems
	Close() error

	// RemoteNodeID returns the ID of the connected node.
	//
	// This is verified during the TLS handshake and is guaranteed
	// to match the remote node's certificate.
	RemoteNodeID() cluster.NodeID

	// RemotePublicKey returns the public key of the connected node.
	//
	// This key is used for encrypting messages to this node and
	// verifying signatures from this node.
	RemotePublicKey() *keys.PublicKey
}

// EncryptedMessage represents a message encrypted for a specific recipient.
//
// EncryptedMessage contains all information needed for the recipient to:
//  1. Verify the sender's identity (via SenderID and Signature)
//  2. Decrypt the message (via EncapsulatedKey and Nonce)
//  3. Verify message integrity (AES-GCM authentication tag in Ciphertext)
//
// # Encryption Scheme
//
// The encryption uses a hybrid approach:
//   - ML-KEM: Encapsulate a symmetric key for the recipient
//   - AES-256-GCM: Encrypt the actual message payload
//   - ML-DSA: Sign the encrypted data for authentication
//
// # Wire Format
//
// The EncryptedMessage is serialized for transmission. The exact format
// is implementation-defined but typically includes length prefixes for
// variable-length fields.
type EncryptedMessage struct {
	// SenderID identifies the sender of the message.
	// Used by the recipient to look up the sender's public key
	// for signature verification.
	SenderID cluster.NodeID

	// Ciphertext is the AES-256-GCM encrypted payload.
	// Includes the authentication tag for integrity verification.
	Ciphertext []byte

	// Nonce is the AES-GCM nonce (12 bytes).
	// Must be unique for each message encrypted with the same key.
	Nonce []byte

	// EncapsulatedKey is the symmetric key encapsulated using ML-KEM
	// for the recipient's public key. Only the recipient can recover
	// the symmetric key to decrypt Ciphertext.
	EncapsulatedKey []byte

	// Signature is the sender's ML-DSA signature over the encrypted data.
	// Covers: Ciphertext || Nonce || EncapsulatedKey
	// Verifies authenticity and integrity of the encrypted message.
	Signature []byte
}
