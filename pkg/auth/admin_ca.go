package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// KEMPublicKeySize is the ML-KEM1024 public key size
// in bytes, used to split composite key material.
const KEMPublicKeySize = 1568 // A

// AdminCAImpl implements interfaces.AdminCA using
// ML-DSA-87 signature verification.
type AdminCAImpl struct { // A
	pubKey *keys.PublicKey
	hash   string
}

// NewAdminCA constructs an AdminCAImpl from
// concatenated KEM+Sign public key bytes.
func NewAdminCA( // A
	pubKeyBytes []byte,
) (*AdminCAImpl, error) {
	pub, err := splitAndParsePubKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	h, err := caHash(pub)
	if err != nil {
		return nil, err
	}
	return &AdminCAImpl{pubKey: pub, hash: h}, nil
}

// PubKey returns the concatenated KEM+Sign bytes.
func (a *AdminCAImpl) PubKey() []byte { // A
	kem, err := a.pubKey.MarshalBinaryKEM()
	if err != nil {
		return nil
	}
	sign, err := a.pubKey.MarshalBinarySign()
	if err != nil {
		return nil
	}
	out := make([]byte, len(kem)+len(sign))
	copy(out, kem)
	copy(out[len(kem):], sign)
	return out
}

// Hash returns hex(SHA-256(signPubKeyBytes)).
func (a *AdminCAImpl) Hash() string { // A
	return a.hash
}

// VerifyNodeCert verifies a CA signature on a
// NodeCert and returns the derived NodeID.
func (a *AdminCAImpl) VerifyNodeCert( // A
	cert NodeCertLike,
	caSignature []byte,
) (keys.NodeID, error) {
	canonical, err := CanonicalNodeCert(cert)
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"canonical encoding failed: %w", err,
		)
	}
	msg := DomainSeparate(CTXNodeAdmissionV1, canonical)
	if !a.pubKey.Verify(msg, caSignature) {
		return keys.NodeID{}, ErrInvalidCASignature
	}
	pubKey := cert.NodePubKey()
	nid, err := pubKey.NodeID()
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"node ID derivation failed: %w", err,
		)
	}
	return nid, nil
}

// splitAndParsePubKey splits concatenated KEM+Sign
// bytes and reconstructs a keys.PublicKey.
func splitAndParsePubKey( // A
	pubKeyBytes []byte,
) (*keys.PublicKey, error) {
	if len(pubKeyBytes) <= KEMPublicKeySize {
		return nil, fmt.Errorf(
			"public key too short: %d bytes",
			len(pubKeyBytes),
		)
	}
	kemBytes := pubKeyBytes[:KEMPublicKeySize]
	signBytes := pubKeyBytes[KEMPublicKeySize:]
	return keys.NewPublicKeyFromBinary(kemBytes, signBytes)
}

// caHash computes hex(SHA-256(signPubKeyBytes)).
func caHash(pub *keys.PublicKey) (string, error) { // A
	signBytes, err := pub.MarshalBinarySign()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(signBytes)
	return hex.EncodeToString(h[:]), nil
}
