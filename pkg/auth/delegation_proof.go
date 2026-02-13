package auth

import (
	"encoding/binary"
	"errors"
	"time"
)

// DelegationProof binds a TLS session to a NodeCert
// identity. It proves the holder of the NodeCert
// private key authorized this specific TLS session.
type DelegationProof struct { // A
	tlsCertPubKeyHash [32]byte
	tlsExporterBinding [32]byte
	x509Fingerprint [32]byte
	nodeCertHash    CaHash
	notBefore       time.Time
	notAfter        time.Time
	handshakeNonce  [32]byte
}

// DelegationProofParams holds parameters for creating
// a new DelegationProof.
type DelegationProofParams struct { // A
	TLSCertPubKeyHash [32]byte
	TLSExporterBinding [32]byte
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
	if params.TLSCertPubKeyHash == [32]byte{} {
		return nil, errors.New(
			"TLS cert pubkey hash must not be zero",
		)
	}
	if params.TLSExporterBinding == [32]byte{} {
		return nil, errors.New(
			"TLS exporter binding must not be zero",
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
			tlsCertPubKeyHash: params.TLSCertPubKeyHash,
			tlsExporterBinding: params.TLSExporterBinding,
		x509Fingerprint: params.X509Fingerprint,
		nodeCertHash:    params.NodeCertHash,
		notBefore:       params.NotBefore.UTC(),
		notAfter:        params.NotAfter.UTC(),
		handshakeNonce:  params.HandshakeNonce,
	}, nil
}

	// TLSCertPubKeyHash returns SHA-256 of the TLS leaf
	// certificate SubjectPublicKeyInfo.
	func (d *DelegationProof) TLSCertPubKeyHash() [32]byte { // A
		return d.tlsCertPubKeyHash
	}

	// TLSExporterBinding returns exporter-derived bytes
	// bound to the active TLS transcript.
	func (d *DelegationProof) TLSExporterBinding() [32]byte { // A
		return d.tlsExporterBinding
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
// TLSCertPubKeyHash(32) ||
// TLSExporterBinding(32) ||
// X509Fingerprint(32) || NodeCertHash(32) ||
// NotBefore(8, BE) || NotAfter(8, BE) ||
// HandshakeNonce(32).
func canonicalSerializeDelegation( // A
	proof *DelegationProof,
) ([]byte, error) {
	size := 32 + 32 + 32 + 32 + 8 + 8 + 32
	buf := make([]byte, 0, size)

	buf = append(buf, proof.tlsCertPubKeyHash[:]...)
	buf = append(buf, proof.tlsExporterBinding[:]...)

	buf = append(buf, proof.x509Fingerprint[:]...)
	buf = append(buf, proof.nodeCertHash[:]...)

	notBeforeUnix := proof.notBefore.Unix()
	if notBeforeUnix < 0 {
		return nil, errors.New(
			"notBefore must be unix epoch or later",
		)
	}
	notAfterUnix := proof.notAfter.Unix()
	if notAfterUnix < 0 {
		return nil, errors.New(
			"notAfter must be unix epoch or later",
		)
	}

	var timeBuf [8]byte
	binary.BigEndian.PutUint64(
		timeBuf[:],
		uint64(notBeforeUnix),
	)
	buf = append(buf, timeBuf[:]...)

	binary.BigEndian.PutUint64(
		timeBuf[:],
		uint64(notAfterUnix),
	)
	buf = append(buf, timeBuf[:]...)

	buf = append(buf, proof.handshakeNonce[:]...)

	return buf, nil
}
