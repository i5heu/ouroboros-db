package auth

import (
	"errors"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// AdminCA represents an admin certificate authority for
// cluster-level node admission. It is verification-only;
// signing happens off-device.
type AdminCA struct { // H
	pubKey *keys.PublicKey
	hash   CaHash
}

// NewAdminCA creates an AdminCA from the admin public
// key.
func NewAdminCA( // AP
	pubKey *keys.PublicKey,
) (*AdminCA, error) {
	if pubKey == nil {
		return nil, errors.New(
			"admin CA public key must not be nil",
		)
	}
	h, err := computeCAHash(pubKey)
	if err != nil {
		return nil, err
	}
	return &AdminCA{
		pubKey: pubKey,
		hash:   h,
	}, nil
}

// PubKey returns the admin CA public key.
func (ca *AdminCA) PubKey() *keys.PublicKey { // A
	return ca.pubKey
}

// Hash returns the SHA-256 identifier of this admin CA.
func (ca *AdminCA) Hash() CaHash { // A
	return ca.hash
}

// VerifyNodeCert verifies a node certificate was signed
// by this admin CA and returns the derived NodeID.
func (ca *AdminCA) VerifyNodeCert( // AP
	cert *NodeCert,
	caSignature []byte,
) (keys.NodeID, error) {
	if ca == nil {
		return keys.NodeID{}, errors.New(
			"admin CA must not be nil",
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
