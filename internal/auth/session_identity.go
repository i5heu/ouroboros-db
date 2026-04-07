package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"
)

// SessionIdentity holds the ephemeral session
// materials created during Phase 2 (Session
// Delegation). The TLS certificate uses Ed25519
// for signing because:
//
//   - The X.509 cert is ephemeral and self-signed.
//   - Real authentication is via the ML-DSA-87
//     signed DelegationProof binding this cert to
//     the node's persistent identity.
//   - PQ confidentiality comes from X25519Kyber768
//     key exchange in the TLS group negotiation.
//   - The cert only proves session-key ownership
//     during the TLS CertificateVerify step.
//
// Use NewSessionIdentity to create a fresh session.
// The returned SessionIdentity is consumed by:
//
//  1. The TLS stack (TLSCertificate field goes into
//     tls.Config.Certificates).
//  2. SignDelegation in sign_delegation.go which
//     uses CertPubKeyHash and X509Fingerprint to
//     bind the identity key to this session.
type SessionIdentity struct { // A
	// TLSCertificate is the tls.Certificate
	// containing the self-signed X.509 leaf and
	// Ed25519 private key. Set this into
	// tls.Config.Certificates.
	TLSCertificate tls.Certificate

	// X509DER is the raw DER encoding of the leaf
	// X.509 certificate. Needed for fingerprinting
	// and for TLS binding derivation.
	X509DER []byte

	// CertPubKeyHash is SHA-256 over the DER-encoded
	// SubjectPublicKeyInfo (SPKI) of the session
	// certificate. This matches
	// DelegationProof.TLSCertPubKeyHash.
	CertPubKeyHash [sha256.Size]byte

	// X509Fingerprint is SHA-256 over the full DER
	// X.509 certificate. This matches
	// DelegationProof.X509Fingerprint.
	X509Fingerprint [sha256.Size]byte
}

// NewSessionIdentity generates an ephemeral Ed25519
// key pair, creates a self-signed X.509 certificate
// with the given TTL, and derives the TLS binding
// hashes. The TTL should be short (e.g. 5 minutes)
// to limit the session certificate's exposure window.
//
// After calling this, pass the SessionIdentity to
// SignDelegation together with the node's ML-DSA-87
// private key and the TLS transcript/exporter data
// to complete Phase 3.
func NewSessionIdentity( // A
	ttl time.Duration,
) (*SessionIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	serialLimit := new(big.Int).Lsh(
		big.NewInt(1), 128,
	)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "ouroboros-session",
		},
		NotBefore:             now.Add(-30 * time.Second),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader, tmpl, tmpl, pub, priv,
	)
	if err != nil {
		return nil, err
	}

	parsed, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, err
	}

	spkiHash := sha256.Sum256(
		parsed.RawSubjectPublicKeyInfo,
	)
	certHash := sha256.Sum256(derBytes)

	tlsCert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
		Leaf:        parsed,
	}

	return &SessionIdentity{
		TLSCertificate:  tlsCert,
		X509DER:         derBytes,
		CertPubKeyHash:  spkiHash,
		X509Fingerprint: certHash,
	}, nil
}
