package auth

import "github.com/i5heu/ouroboros-crypt/pkg/keys"

func cloneBytes(src []byte) []byte { // A
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

// NodeCertImpl is a concrete implementation of the
// interfaces.NodeCert interface.
type NodeCertImpl struct { // A
	certVersion  uint16
	nodePubKey   keys.PublicKey
	issuerCAHash string
	validFrom    int64
	validUntil   int64
	serial       []byte
	certNonce    []byte
	nodeID       keys.NodeID
}

// NewNodeCert constructs a NodeCertImpl and derives
// the NodeID from the public key at creation time.
func NewNodeCert( // A
	pubKey keys.PublicKey,
	issuerCAHash string,
	validFrom int64,
	validUntil int64,
	serial []byte,
	certNonce []byte,
) (*NodeCertImpl, error) {
	nid, err := pubKey.NodeID()
	if err != nil {
		return nil, err
	}
	return &NodeCertImpl{
		certVersion:  DefaultCertVersion,
		nodePubKey:   pubKey,
		issuerCAHash: issuerCAHash,
		validFrom:    validFrom,
		validUntil:   validUntil,
		serial:       cloneBytes(serial),
		certNonce:    cloneBytes(certNonce),
		nodeID:       nid,
	}, nil
}

// CertVersion returns the NodeCert payload version.
func (n *NodeCertImpl) CertVersion() uint16 { // A
	return n.certVersion
}

// NodePubKey returns the node's composite public key.
func (n *NodeCertImpl) NodePubKey() keys.PublicKey { // A
	return n.nodePubKey
}

// IssuerCAHash returns the issuing CA's hash.
func (n *NodeCertImpl) IssuerCAHash() string { // A
	return n.issuerCAHash
}

// ValidFrom returns the cert validity start (unix).
func (n *NodeCertImpl) ValidFrom() int64 { // A
	return n.validFrom
}

// ValidUntil returns the cert validity end (unix).
func (n *NodeCertImpl) ValidUntil() int64 { // A
	return n.validUntil
}

// Serial returns the certificate serial number.
func (n *NodeCertImpl) Serial() []byte { // A
	return cloneBytes(n.serial)
}

// CertNonce returns the certificate nonce.
func (n *NodeCertImpl) CertNonce() []byte { // A
	return cloneBytes(n.certNonce)
}

// NodeID returns the derived node identifier.
func (n *NodeCertImpl) NodeID() keys.NodeID { // A
	return n.nodeID
}
