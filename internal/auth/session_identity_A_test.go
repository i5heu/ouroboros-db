package auth

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

func TestNewSessionIdentity(t *testing.T) { // A
	ttl := 5 * time.Minute
	si, err := NewSessionIdentity(ttl)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}

	// TLS certificate must have exactly one leaf.
	if len(si.TLSCertificate.Certificate) != 1 {
		t.Fatalf(
			"expected 1 cert, got %d",
			len(si.TLSCertificate.Certificate),
		)
	}

	// DER bytes must match embedded cert.
	if !bytes.Equal(
		si.X509DER,
		si.TLSCertificate.Certificate[0],
	) {
		t.Fatal("X509DER does not match TLS cert")
	}

	// Parse and validate the X.509 cert.
	cert, err := x509.ParseCertificate(si.X509DER)
	if err != nil {
		t.Fatalf("parse X.509: %v", err)
	}
	if cert.Subject.CommonName != "ouroboros-session" {
		t.Fatalf(
			"unexpected CN: %s",
			cert.Subject.CommonName,
		)
	}

	// SPKI hash must match.
	wantSPKI := sha256.Sum256(
		cert.RawSubjectPublicKeyInfo,
	)
	if si.CertPubKeyHash != wantSPKI {
		t.Fatal("CertPubKeyHash mismatch")
	}

	// X.509 fingerprint must match.
	wantFP := sha256.Sum256(si.X509DER)
	if si.X509Fingerprint != wantFP {
		t.Fatal("X509Fingerprint mismatch")
	}

	// Cert should not be expired yet.
	now := time.Now()
	if now.Before(cert.NotBefore) {
		t.Fatal("cert not yet valid")
	}
	if now.After(cert.NotAfter) {
		t.Fatal("cert already expired")
	}
}

func TestSignDelegation(t *testing.T) { // A
	// Generate node identity.
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("NewAsyncCrypt: %v", err)
	}

	// Create a session.
	si, err := NewSessionIdentity(5 * time.Minute)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}

	// Build a dummy cert for the bundle.
	pub := ac.GetPublicKey()
	cert, err := NewNodeCert(
		pub,
		"ca-hash-test",
		time.Now().Unix()-60,
		time.Now().Unix()+3600,
		[]byte("serial-1"),
		[]byte("nonce-1"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	certs := []NodeCertLike{cert}

	// Fake exporter function that handles both
	// TranscriptBindingLabel and ExporterLabel.
	fakeExporter := func(
		label string,
		ctx []byte,
		length int,
	) ([]byte, error) {
		out := make([]byte, length)
		switch label {
		case TranscriptBindingLabel:
			for i := range out {
				out[i] = byte(i)
			}
		case ExporterLabel:
			for i := range out {
				out[i] = 0xAB
			}
		default:
			t.Fatalf(
				"unexpected label: %s", label,
			)
		}
		return out, nil
	}

	proof, sig, err := SignDelegation(
		ac, certs, si, fakeExporter,
	)
	if err != nil {
		t.Fatalf("SignDelegation: %v", err)
	}

	// Proof fields must be populated.
	if len(proof.TLSCertPubKeyHash()) != 32 {
		t.Fatal("TLSCertPubKeyHash wrong size")
	}
	if len(proof.TLSExporterBinding()) != 32 {
		t.Fatal("TLSExporterBinding wrong size")
	}
	if len(proof.TLSTranscriptHash()) != 32 {
		t.Fatal("TLSTranscriptHash wrong size")
	}
	if len(proof.X509Fingerprint()) != 32 {
		t.Fatal("X509Fingerprint wrong size")
	}
	if len(proof.NodeCertBundleHash()) != 32 {
		t.Fatal("NodeCertBundleHash wrong size")
	}
	if proof.NotAfter()-proof.NotBefore() != MaxDelegationTTL {
		t.Fatal("TTL not MaxDelegationTTL")
	}

	// CertPubKeyHash must match session.
	if !bytes.Equal(
		proof.TLSCertPubKeyHash(),
		si.CertPubKeyHash[:],
	) {
		t.Fatal("CertPubKeyHash mismatch")
	}

	// X509Fingerprint must match session.
	if !bytes.Equal(
		proof.X509Fingerprint(),
		si.X509Fingerprint[:],
	) {
		t.Fatal("X509Fingerprint mismatch")
	}

	// Verify signature.
	canon, err := CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatalf("canonical proof: %v", err)
	}
	msg := DomainSeparate(CTXNodeDelegationV1, canon)
	if !pub.Verify(msg, sig) {
		t.Fatal(
			"delegation signature failed verification",
		)
	}
}

func TestSignDelegationBundleHash(t *testing.T) { // A
	// Ensure the bundle hash in the proof matches
	// an independent computation.
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("NewAsyncCrypt: %v", err)
	}
	si, err := NewSessionIdentity(5 * time.Minute)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}
	pub := ac.GetPublicKey()
	cert, err := NewNodeCert(
		pub, "ca-hash",
		time.Now().Unix()-60,
		time.Now().Unix()+3600,
		[]byte("serial"), []byte("nonce"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	certs := []NodeCertLike{cert}

	proof, _, err := SignDelegation(
		ac, certs, si,
		func(
			_ string, _ []byte, l int,
		) ([]byte, error) {
			return make([]byte, l), nil
		},
	)
	if err != nil {
		t.Fatalf("SignDelegation: %v", err)
	}

	// Compute expected bundle hash.
	bundleBytes, err := CanonicalNodeCertBundle(certs)
	if err != nil {
		t.Fatalf("CanonicalNodeCertBundle: %v", err)
	}
	want := sha256.Sum256(bundleBytes)
	if !bytes.Equal(
		proof.NodeCertBundleHash(), want[:],
	) {
		t.Fatal("bundle hash mismatch")
	}
}
