// Package node provides node identity and cryptographic types for the carrier.
//
// This package contains the internal implementation of node identity,
// including key management, signing, and encryption operations.
package node

import (
	"errors"
	"fmt"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/encrypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// NodeID is an alias for cluster.NodeID.
type NodeID = cluster.NodeID

// NodeCert is an alias for cluster.NodeCert.
type NodeCert = cluster.NodeCert

// Node is an alias for cluster.Node.
type Node = cluster.Node

// Identity holds the cryptographic identity for a node.
// Each node has its own key pair for encryption and signing.
type Identity struct { // A
	// Crypt is the cryptographic context containing the node's keys.
	Crypt *crypt.Crypt
	// PublicKey is the node's public key (can be shared with others).
	PublicKey keys.PublicKey
	// PrivateKey is the node's private key (must be kept secret).
	PrivateKey keys.PrivateKey
}

// NewIdentity creates a new cryptographic identity for a node.
func NewIdentity() (*Identity, error) { // A
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()
	return &Identity{
		Crypt:      c,
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// NewIdentityFromFile loads a node identity from a key file.
func NewIdentityFromFile(filepath string) (*Identity, error) { // A
	c, err := crypt.NewFromFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to load node identity: %w", err)
	}
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()
	return &Identity{
		Crypt:      c,
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// SaveToFile persists the node identity to a key file.
func (ni *Identity) SaveToFile(filepath string) error { // A
	return ni.Crypt.Keys.SaveToFile(filepath)
}

// IDFromPublicKey derives a NodeID from a public key hash.
func IDFromPublicKey(pub *keys.PublicKey) (NodeID, error) { // A
	kemB64, err := pub.ToBase64KEM()
	if err != nil {
		return "", fmt.Errorf("failed to encode public key: %w", err)
	}
	h := hash.HashBytes([]byte(kemB64))
	return NodeID(h.String()[:32]), nil // Use first 32 chars of hex hash
}

// Sign signs data using the node's private key.
func (ni *Identity) Sign(data []byte) ([]byte, error) { // A
	return ni.Crypt.Keys.Sign(data)
}

// Verify verifies a signature using the node's public key.
func (ni *Identity) Verify(data, signature []byte) bool { // A
	return ni.Crypt.Keys.Verify(data, signature)
}

// EncryptFor encrypts data for a specific recipient using their public key.
func (ni *Identity) EncryptFor(
	data []byte,
	recipientPub *keys.PublicKey,
) (*encrypt.EncryptResult, error) { // A
	return encrypt.Encrypt(data, recipientPub)
}

// Decrypt decrypts data that was encrypted for this node.
func (ni *Identity) Decrypt(enc *encrypt.EncryptResult) ([]byte, error) { // A
	return encrypt.Decrypt(enc, &ni.PrivateKey)
}

// GetPublicKey returns a pointer to the node's public key.
func (ni *Identity) GetPublicKey() *keys.PublicKey { // A
	return &ni.PublicKey
}

// Validate checks if the Node has valid configuration.
func Validate(n Node) error { // A
	if n.NodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if len(n.Addresses) == 0 {
		return errors.New("node must have at least one address")
	}
	return nil
}
