package auth

import (
	"errors"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// UserCA represents a user certificate authority for
// user-scoped node admission. It is verification-only;
// signing happens off-device.
type UserCA struct { // A
	pubKey *keys.PublicKey
	hash   CaHash
}

// NewUserCA creates a UserCA from the user public key.
func NewUserCA( // AP
	pubKey *keys.PublicKey,
) (*UserCA, error) {
	if pubKey == nil {
		return nil, errors.New(
			"user CA public key must not be nil",
		)
	}
	h, err := computeCAHash(pubKey)
	if err != nil {
		return nil, err
	}
	return &UserCA{
		pubKey: pubKey,
		hash:   h,
	}, nil
}

// PubKey returns the user CA public key.
func (ca *UserCA) PubKey() *keys.PublicKey { // A
	return ca.pubKey
}

// Hash returns the SHA-256 identifier of this user CA.
func (ca *UserCA) Hash() CaHash { // A
	return ca.hash
}

// VerifyNodeCert verifies a node certificate was signed
// by this user CA and returns the derived NodeID.
func (ca *UserCA) VerifyNodeCert( // A
	cert *NodeCert,
	caSignature []byte,
) (keys.NodeID, error) {
	if ca == nil {
		return keys.NodeID{}, errors.New(
			"user CA must not be nil",
		)
	}

	if cert == nil {
		return keys.NodeID{}, errors.New(
			"node cert must not be nil",
		)
	}

	if !cert.IssuerCAHash().Equal(ca.hash) {
		return keys.NodeID{}, fmt.Errorf(
			"issuer CA hash mismatch: cert=%s ca=%s",
			cert.IssuerCAHash().Hex(),
			ca.hash.Hex(),
		)
	}

	return verifyNodeCert(
		ca.pubKey, cert, caSignature,
	)
}
