package delegation

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"testing"
	"time"
)

func TestNewSessionIdentity(t *testing.T) { // A
	ttl := 5 * time.Minute
	si, err := NewSessionIdentity(ttl)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}

	if len(si.TLSCertificate.Certificate) != 1 {
		t.Fatalf(
			"expected 1 cert, got %d",
			len(si.TLSCertificate.Certificate),
		)
	}

	if !bytes.Equal(
		si.X509DER,
		si.TLSCertificate.Certificate[0],
	) {
		t.Fatal("X509DER does not match TLS cert")
	}

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

	wantSPKI := sha256.Sum256(
		cert.RawSubjectPublicKeyInfo,
	)
	if si.CertPubKeyHash != wantSPKI {
		t.Fatal("CertPubKeyHash mismatch")
	}

	wantFP := sha256.Sum256(si.X509DER)
	if si.X509Fingerprint != wantFP {
		t.Fatal("X509Fingerprint mismatch")
	}

	now := time.Now()
	if now.Before(cert.NotBefore) {
		t.Fatal("cert not yet valid")
	}
	if now.After(cert.NotAfter) {
		t.Fatal("cert already expired")
	}
}
