package auth

import (
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// caBase holds the shared public key and hash fields
// common to AdminCAImpl and UserCAImpl.
type caBase struct { // A
	pubKey *keys.PublicKey
	hash   string
}

// PubKey returns the concatenated KEM+Sign bytes.
func (b *caBase) PubKey() []byte { // A
	out, err := marshalPubKeyBytes(b.pubKey)
	if err != nil {
		return nil
	}
	return out
}

// Hash returns hex(SHA-256(signPubKeyBytes)).
func (b *caBase) Hash() string { // A
	return b.hash
}

// VerifyNodeCert verifies a CA signature on a
// NodeCert and returns the derived NodeID.
func (b *caBase) VerifyNodeCert( // A
	cert NodeCertLike,
	caSignature []byte,
) (keys.NodeID, error) {
	pubKey := cert.NodePubKey()
	nodePubKeyBytes, err := marshalPubKeyBytes(&pubKey)
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"pubkey marshal failed: %w", err,
		)
	}
	c := canonicalCert{
		CertVersion:  cert.CertVersion(),
		NodePubKey:   nodePubKeyBytes,
		IssuerCAHash: cert.IssuerCAHash(),
		ValidFrom:    cert.ValidFrom(),
		ValidUntil:   cert.ValidUntil(),
		Serial:       cert.Serial(),
		CertNonce:    cert.CertNonce(),
	}
	canonical, err := cborEncMode.Marshal(c)
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"canonical encoding failed: %w", err,
		)
	}
	msg := DomainSeparate(CTXNodeAdmissionV1, canonical)
	if !b.pubKey.Verify(msg, caSignature) {
		return keys.NodeID{}, ErrInvalidCASignature
	}
	nid, err := pubKey.NodeID()
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"node ID derivation failed: %w", err,
		)
	}
	return nid, nil
}
