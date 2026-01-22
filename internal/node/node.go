// Package node provides node identity and management for OuroborosDB.
package node

import (
	"fmt"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/encrypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// DefaultNodeIdentity implements the NodeIdentity interface.
type DefaultNodeIdentity struct {
	crypt      *crypt.Crypt
	publicKey  keys.PublicKey
	privateKey keys.PrivateKey
}

// NewNodeIdentity creates a new cryptographic identity for a node.
func NewNodeIdentity() (*DefaultNodeIdentity, error) {
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()
	return &DefaultNodeIdentity{
		crypt:      c,
		publicKey:  pub,
		privateKey: priv,
	}, nil
}

// NewNodeIdentityFromFile loads a node identity from a key file.
func NewNodeIdentityFromFile(filepath string) (*DefaultNodeIdentity, error) {
	c, err := crypt.NewFromFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to load node identity: %w", err)
	}
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()
	return &DefaultNodeIdentity{
		crypt:      c,
		publicKey:  pub,
		privateKey: priv,
	}, nil
}

// GetPublicKey returns the node's public key.
func (ni *DefaultNodeIdentity) GetPublicKey() *keys.PublicKey {
	return &ni.publicKey
}

// GetCrypt returns the underlying Crypt instance for low-level operations.
func (ni *DefaultNodeIdentity) GetCrypt() *crypt.Crypt {
	return ni.crypt
}

// Sign signs data using the node's private key.
func (ni *DefaultNodeIdentity) Sign(data []byte) ([]byte, error) {
	return ni.crypt.Keys.Sign(data)
}

// Verify verifies a signature using the node's public key.
func (ni *DefaultNodeIdentity) Verify(data, signature []byte) bool {
	return ni.crypt.Keys.Verify(data, signature)
}

// EncryptFor encrypts data for a specific recipient using their public key.
func (ni *DefaultNodeIdentity) EncryptFor(
	data []byte,
	recipientPub *keys.PublicKey,
) ([]byte, error) {
	result, err := encrypt.Encrypt(data, recipientPub)
	if err != nil {
		return nil, err
	}
	// Return combined encrypted data (simplified - actual implementation may need
	// more structure)
	return result.Ciphertext, nil
}

// Decrypt decrypts data that was encrypted for this node.
func (ni *DefaultNodeIdentity) Decrypt(encryptedData []byte) ([]byte, error) {
	// Note: This is a simplified interface. The actual encrypted message
	// needs to include nonce and encapsulated key. The full implementation
	// should use the EncryptResult structure.
	return nil, fmt.Errorf(
		"decrypt requires EncryptResult structure - use GetCrypt() for full decryption",
	)
}

// SaveToFile persists the node identity to a key file.
func (ni *DefaultNodeIdentity) SaveToFile(filepath string) error {
	return ni.crypt.Keys.SaveToFile(filepath)
}

// NodeIDFromPublicKey derives a NodeID from a public key.
func NodeIDFromPublicKey(pub *keys.PublicKey) (cluster.NodeID, error) {
	kemB64, err := pub.ToBase64KEM()
	if err != nil {
		return "", fmt.Errorf("failed to encode public key: %w", err)
	}
	h := hash.HashBytes([]byte(kemB64))
	return cluster.NodeID(h.String()[:32]), nil // Use first 32 chars of hex hash
}

// Ensure DefaultNodeIdentity implements the NodeIdentity interface.
var _ cluster.NodeIdentity = (*DefaultNodeIdentity)(nil)
