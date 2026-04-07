package ca

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
	caBase
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
	cached, err := marshalPubKeyBytes(pub)
	if err != nil {
		return nil, err
	}
	return &AdminCAImpl{
		caBase: caBase{
			pubKey:      pub,
			pubKeyBytes: cached,
			hash:        h,
		},
	}, nil
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

// marshalPubKeyBytes returns concatenated KEM+Sign
// public key bytes from a keys.PublicKey.
func marshalPubKeyBytes( // A
	pub *keys.PublicKey,
) ([]byte, error) {
	kem, err := pub.MarshalBinaryKEM()
	if err != nil {
		return nil, err
	}
	sign, err := pub.MarshalBinarySign()
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(kem)+len(sign))
	copy(out, kem)
	copy(out[len(kem):], sign)
	return out, nil
}

// MarshalPubKeyBytes serializes a composite public
// key into the KEM || Sign binary concatenation
// used on the wire and in canonical encoding.
func MarshalPubKeyBytes( // A
	pub *keys.PublicKey,
) ([]byte, error) {
	return marshalPubKeyBytes(pub)
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
