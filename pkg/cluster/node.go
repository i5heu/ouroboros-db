// Package cluster defines interfaces and types for cluster management in OuroborosDB.
package cluster

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// NodeID is a unique identifier for a node in the cluster.
// It is derived from the hash of the node's public key.
type NodeID string

// NodeCert represents the certificate used to authenticate a node.
// It contains the node's public key and a signature for verification.
type NodeCert struct {
	// PubKeyHash is the hash of the node's public key.
	PubKeyHash hash.Hash

	// PublicKey is the node's public key for encryption and verification.
	PublicKey keys.PublicKey

	// Signature is a self-signature over the public key (for integrity).
	Signature []byte
}

// Node represents a node in the OuroborosDB cluster.
type Node struct {
	// NodeID is the unique identifier for this node (derived from public key).
	NodeID NodeID

	// Addresses are the network addresses where this node can be reached.
	// For QUIC transport, these should be in the format "host:port".
	Addresses []string

	// Cert is the node's certificate for authentication.
	Cert NodeCert
}

// NodeIdentity defines the interface for a node's cryptographic identity.
// Each node has its own key pair for encryption and signing.
type NodeIdentity interface {
	// GetPublicKey returns the node's public key.
	GetPublicKey() *keys.PublicKey

	// Sign signs data using the node's private key.
	Sign(data []byte) ([]byte, error)

	// Verify verifies a signature using the node's public key.
	Verify(data, signature []byte) bool

	// EncryptFor encrypts data for a specific recipient using their public key.
	EncryptFor(data []byte, recipientPub *keys.PublicKey) ([]byte, error)

	// Decrypt decrypts data that was encrypted for this node.
	Decrypt(encryptedData []byte) ([]byte, error)

	// SaveToFile persists the node identity to a key file.
	SaveToFile(filepath string) error
}
