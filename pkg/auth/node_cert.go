package auth

import (
	"errors"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// NodeCert holds the node identity certificate payload.
// It carries the node public key and identifies the
// issuer CA by hash.
type NodeCert struct { // A
	nodePubKey   *keys.PublicKey
	issuerCAHash CaHash
}

// NewNodeCert creates a NodeCert from a node public key
// and the hash of the issuing CA.
func NewNodeCert( // A
	nodePubKey *keys.PublicKey,
	issuerCAHash CaHash,
) (*NodeCert, error) {
	if nodePubKey == nil {
		return nil, errors.New(
			"node public key must not be nil",
		)
	}
	return &NodeCert{
		nodePubKey:   nodePubKey,
		issuerCAHash: issuerCAHash,
	}, nil
}

// NodePubKey returns the node's composite public key.
func (c *NodeCert) NodePubKey() *keys.PublicKey { // A
	return c.nodePubKey
}

// IssuerCAHash returns the SHA-256 hash identifying the
// CA that issued this certificate.
func (c *NodeCert) IssuerCAHash() CaHash { // A
	return c.issuerCAHash
}

// NodeID derives the node identifier (SHA-256) from the
// node public key material.
func (c *NodeCert) NodeID() (keys.NodeID, error) { // AP
	return c.nodePubKey.NodeID()
}
