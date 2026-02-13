package auth

import (
	"crypto/sha256"
	"errors"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// NodeCert holds the node identity certificate payload.
// It carries the node public key, issuer CA reference,
// validity window, serial number, role claims, and an
// anti-replay nonce.
type NodeCert struct { // A
	certVersion  uint8
	nodePubKey   *keys.PublicKey
	issuerCAHash CaHash
	validFrom    time.Time
	validUntil   time.Time
	serial       [16]byte
	roleClaims   TrustScope
	certNonce    [32]byte
}

// NodeCertParams holds all parameters needed to create
// a new NodeCert.
type NodeCertParams struct { // A
	NodePubKey   *keys.PublicKey
	IssuerCAHash CaHash
	ValidFrom    time.Time
	ValidUntil   time.Time
	Serial       [16]byte
	RoleClaims   TrustScope
	CertNonce    [32]byte
}

// NewNodeCert creates a NodeCert from the given params
// after validating all fields.
func NewNodeCert( // A
	params NodeCertParams,
) (*NodeCert, error) {
	if params.NodePubKey == nil {
		return nil, errors.New(
			"node public key must not be nil",
		)
	}
	if !params.ValidUntil.After(params.ValidFrom) {
		return nil, errors.New(
			"validUntil must be after validFrom",
		)
	}
	if params.Serial == [16]byte{} {
		return nil, errors.New(
			"serial must not be zero",
		)
	}
	if params.CertNonce == [32]byte{} {
		return nil, errors.New(
			"cert nonce must not be zero",
		)
	}
	if err := validateRole(params.RoleClaims); err != nil {
		return nil, err
	}
	return &NodeCert{
		certVersion:  currentCertVersion,
		nodePubKey:   params.NodePubKey,
		issuerCAHash: params.IssuerCAHash,
		validFrom:    params.ValidFrom.UTC(),
		validUntil:   params.ValidUntil.UTC(),
		serial:       params.Serial,
		roleClaims:   params.RoleClaims,
		certNonce:    params.CertNonce,
	}, nil
}

// validateRole checks that the role is exactly
// ScopeAdmin or ScopeUser.
func validateRole(role TrustScope) error { // A
	if role != ScopeAdmin && role != ScopeUser {
		return errors.New(
			"role must be ScopeAdmin or ScopeUser",
		)
	}
	return nil
}

// CertVersion returns the certificate format version.
func (c *NodeCert) CertVersion() uint8 { // A
	return c.certVersion
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

// ValidFrom returns the start of the validity window.
func (c *NodeCert) ValidFrom() time.Time { // A
	return c.validFrom
}

// ValidUntil returns the end of the validity window.
func (c *NodeCert) ValidUntil() time.Time { // A
	return c.validUntil
}

// Serial returns the unique certificate serial number.
func (c *NodeCert) Serial() [16]byte { // A
	return c.serial
}

// RoleClaims returns the trust scope claimed by this
// certificate.
func (c *NodeCert) RoleClaims() TrustScope { // A
	return c.roleClaims
}

// CertNonce returns the anti-replay nonce.
func (c *NodeCert) CertNonce() [32]byte { // A
	return c.certNonce
}

// NodeID derives the node identifier (SHA-256) from the
// node public key material.
func (c *NodeCert) NodeID() (keys.NodeID, error) { // AP
	return c.nodePubKey.NodeID()
}

// Hash returns a SHA-256 hash of the canonical
// serialization of this certificate, used for binding
// in DelegationProof.NodeCertHash.
func (c *NodeCert) Hash() (CaHash, error) { // A
	canon, err := canonicalSerialize(c)
	if err != nil {
		return CaHash{}, err
	}
	return CaHash(sha256.Sum256(canon)), nil
}
