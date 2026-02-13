package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// DelegationProof binds a TLS session to a NodeCert
// identity. It proves the holder of the NodeCert
// private key authorized this specific TLS session.
type DelegationProof struct { // A
	sessionPubKey   *keys.PublicKey
	x509Fingerprint [32]byte
	nodeCertHash    CaHash
	notBefore       time.Time
	notAfter        time.Time
	handshakeNonce  [32]byte
}

// DelegationProofParams holds parameters for creating
// a new DelegationProof.
type DelegationProofParams struct { // A
	SessionPubKey   *keys.PublicKey
	X509Fingerprint [32]byte
	NodeCertHash    CaHash
	NotBefore       time.Time
	NotAfter        time.Time
	HandshakeNonce  [32]byte
}

// NewDelegationProof creates a DelegationProof from the
// given params after validation.
func NewDelegationProof( // A
	params DelegationProofParams,
) (*DelegationProof, error) {
	if params.SessionPubKey == nil {
		return nil, errors.New(
			"session public key must not be nil",
		)
	}
	if !params.NotAfter.After(params.NotBefore) {
		return nil, errors.New(
			"notAfter must be after notBefore",
		)
	}
	if params.X509Fingerprint == [32]byte{} {
		return nil, errors.New(
			"x509 fingerprint must not be zero",
		)
	}
	if params.NodeCertHash.IsZero() {
		return nil, errors.New(
			"node cert hash must not be zero",
		)
	}
	if params.HandshakeNonce == [32]byte{} {
		return nil, errors.New(
			"handshake nonce must not be zero",
		)
	}
	return &DelegationProof{
		sessionPubKey:   params.SessionPubKey,
		x509Fingerprint: params.X509Fingerprint,
		nodeCertHash:    params.NodeCertHash,
		notBefore:       params.NotBefore.UTC(),
		notAfter:        params.NotAfter.UTC(),
		handshakeNonce:  params.HandshakeNonce,
	}, nil
}

// SessionPubKey returns the TLS session public key.
func (d *DelegationProof) SessionPubKey() *keys.PublicKey { // A
	return d.sessionPubKey
}

// X509Fingerprint returns the SHA-256 of the presented
// X.509 DER certificate.
func (d *DelegationProof) X509Fingerprint() [32]byte { // A
	return d.x509Fingerprint
}

// NodeCertHash returns the SHA-256 of the canonical
// NodeCert this proof binds to.
func (d *DelegationProof) NodeCertHash() CaHash { // A
	return d.nodeCertHash
}

// NotBefore returns the delegation validity start.
func (d *DelegationProof) NotBefore() time.Time { // A
	return d.notBefore
}

// NotAfter returns the delegation validity end.
func (d *DelegationProof) NotAfter() time.Time { // A
	return d.notAfter
}

// HandshakeNonce returns the single-use nonce.
func (d *DelegationProof) HandshakeNonce() [32]byte { // A
	return d.handshakeNonce
}

// canonicalSerializeDelegation encodes a DelegationProof
// into a deterministic binary representation:
// len(SessionKEM)(4) || SessionKEM ||
// len(SessionSign)(4) || SessionSign ||
// X509Fingerprint(32) || NodeCertHash(32) ||
// NotBefore(8, BE) || NotAfter(8, BE) ||
// HandshakeNonce(32).
func canonicalSerializeDelegation( // A
	proof *DelegationProof,
) ([]byte, error) {
	kemBytes, err := proof.sessionPubKey.MarshalBinaryKEM()
	if err != nil {
		return nil, fmt.Errorf(
			"marshal session KEM key: %w", err,
		)
	}

	signBytes, err := proof.sessionPubKey.MarshalBinarySign()
	if err != nil {
		return nil, fmt.Errorf(
			"marshal session sign key: %w", err,
		)
	}

	if len(kemBytes) > math.MaxUint32 {
		return nil, fmt.Errorf(
			"session KEM key too large: %d bytes",
			len(kemBytes),
		)
	}
	if len(signBytes) > math.MaxUint32 {
		return nil, fmt.Errorf(
			"session sign key too large: %d bytes",
			len(signBytes),
		)
	}

	size := 4 + len(kemBytes) +
		4 + len(signBytes) +
		32 + 32 + 8 + 8 + 32
	buf := make([]byte, 0, size)

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(
		lenBuf[:], uint32(len(kemBytes)), //#nosec G115
	)
	buf = append(buf, lenBuf[:]...)
	buf = append(buf, kemBytes...)

	binary.BigEndian.PutUint32(
		lenBuf[:], uint32(len(signBytes)), //#nosec G115
	)
	buf = append(buf, lenBuf[:]...)
	buf = append(buf, signBytes...)

	buf = append(buf, proof.x509Fingerprint[:]...)
	buf = append(buf, proof.nodeCertHash[:]...)

	var timeBuf [8]byte
	binary.BigEndian.PutUint64(
		timeBuf[:],
		uint64(proof.notBefore.Unix()),
	)
	buf = append(buf, timeBuf[:]...)

	binary.BigEndian.PutUint64(
		timeBuf[:],
		uint64(proof.notAfter.Unix()),
	)
	buf = append(buf, timeBuf[:]...)

	buf = append(buf, proof.handshakeNonce[:]...)

	return buf, nil
}
