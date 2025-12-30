package carrier

import (
	"errors"
	"fmt"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/encrypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// NodeID is a unique identifier for a node in the cluster.
// It is derived from the hash of the node's public key.
type NodeID string // A

// NodeIdentity holds the cryptographic identity for a node.
// Each node has its own key pair for encryption and signing.
type NodeIdentity struct { // A
	// Crypt is the cryptographic context containing the node's keys.
	Crypt *crypt.Crypt
	// PublicKey is the node's public key (can be shared with others).
	PublicKey keys.PublicKey
	// PrivateKey is the node's private key (must be kept secret).
	PrivateKey keys.PrivateKey
}

// NewNodeIdentity creates a new cryptographic identity for a node.
func NewNodeIdentity() (*NodeIdentity, error) { // A
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()
	return &NodeIdentity{
		Crypt:      c,
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// NewNodeIdentityFromFile loads a node identity from a key file.
func NewNodeIdentityFromFile(filepath string) (*NodeIdentity, error) { // A
	c, err := crypt.NewFromFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to load node identity: %w", err)
	}
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()
	return &NodeIdentity{
		Crypt:      c,
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// SaveToFile persists the node identity to a key file.
func (ni *NodeIdentity) SaveToFile(filepath string) error { // A
	return ni.Crypt.Keys.SaveToFile(filepath)
}

// NodeIDFromPublicKey derives a NodeID from a public key hash.
func NodeIDFromPublicKey(pub *keys.PublicKey) (NodeID, error) { // A
	kemB64, err := pub.ToBase64KEM()
	if err != nil {
		return "", fmt.Errorf("failed to encode public key: %w", err)
	}
	h := hash.HashBytes([]byte(kemB64))
	return NodeID(h.String()[:32]), nil // Use first 32 chars of hex hash
}

// Sign signs data using the node's private key.
func (ni *NodeIdentity) Sign(data []byte) ([]byte, error) { // A
	return ni.Crypt.Keys.Sign(data)
}

// Verify verifies a signature using the node's public key.
func (ni *NodeIdentity) Verify(data, signature []byte) bool { // A
	return ni.Crypt.Keys.Verify(data, signature)
}

// EncryptFor encrypts data for a specific recipient using their public key.
func (ni *NodeIdentity) EncryptFor(
	data []byte,
	recipientPub *keys.PublicKey,
) (*encrypt.EncryptResult, error) { // A
	return encrypt.Encrypt(data, recipientPub)
}

// Decrypt decrypts data that was encrypted for this node.
func (ni *NodeIdentity) Decrypt(
	enc *encrypt.EncryptResult,
) ([]byte, error) { // A
	return encrypt.Decrypt(enc, &ni.PrivateKey)
}

// NodeCert represents the certificate used to authenticate a node.
// It contains the node's public key and a signature for verification.
type NodeCert struct { // A
	// PubKeyHash is the hash of the node's public key.
	PubKeyHash hash.Hash
	// PublicKey is the node's public key for encryption and verification.
	PublicKey keys.PublicKey
	// Signature is a self-signature over the public key (for integrity).
	Signature []byte
}

// Node represents a node in the OuroborosDB cluster.
type Node struct { // A
	// NodeID is the unique identifier for this node (derived from public key).
	NodeID NodeID
	// Addresses are the network addresses where this node can be reached.
	// For QUIC transport, these should be in the format "host:port".
	Addresses []string
	// Cert is the node's certificate for authentication.
	Cert NodeCert
	// PublicKey is the node's public key for encrypting messages to this node.
	PublicKey *keys.PublicKey
}

// Validate checks if the Node has valid configuration.
func (n Node) Validate() error { // A
	if n.NodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if len(n.Addresses) == 0 {
		return errors.New("node must have at least one address")
	}
	return nil
}
