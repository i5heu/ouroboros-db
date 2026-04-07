package auth

import (
	"bytes"
	"testing"
)

func TestNewNodeCert(t *testing.T) { // A
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()

	cert, err := NewNodeCert(
		pub, "issuer-hash",
		100, 200,
		[]byte("ser"), []byte("nonce"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	if cert.IssuerCAHash() != "issuer-hash" {
		t.Error("wrong issuer hash")
	}
	if cert.ValidFrom() != 100 {
		t.Error("wrong ValidFrom")
	}
	if cert.ValidUntil() != 200 {
		t.Error("wrong ValidUntil")
	}
	if string(cert.Serial()) != "ser" {
		t.Error("wrong serial")
	}
	if string(cert.CertNonce()) != "nonce" {
		t.Error("wrong nonce")
	}
	if cert.NodeID().IsZero() {
		t.Error("NodeID should not be zero")
	}
}

func TestNodeCertPubKeyRoundTrip(t *testing.T) { // A
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()

	cert, err := NewNodeCert(
		pub, "h", 0, 0, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	returned := cert.NodePubKey()
	origID, err := pub.NodeID()
	if err != nil {
		t.Fatal(err)
	}
	retID, err := returned.NodeID()
	if err != nil {
		t.Fatal(err)
	}
	if origID != retID {
		t.Error("NodeIDs differ after round-trip")
	}
}

func TestNodeCertNodeIDConsistency(t *testing.T) { // A
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()

	cert, err := NewNodeCert(
		pub, "h", 0, 0, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := pub.NodeID()
	if err != nil {
		t.Fatal(err)
	}
	if cert.NodeID() != expected {
		t.Error("cert NodeID != pubKey NodeID")
	}
}

func TestNodeCertImplementsInterface( // A
	t *testing.T,
) {
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()
	cert, err := NewNodeCert(
		pub, "h", 0, 0, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Verify structural compatibility with
	// NodeCertLike.
	var _ NodeCertLike = cert

	// Also check it has NodeID() method matching
	// the interfaces.NodeCert contract.
	nid := cert.NodeID()
	if nid.IsZero() {
		t.Error("should have valid NodeID")
	}
}

func TestNodeCertDefensiveCopiesByteFields( // A
	t *testing.T,
) {
	ac := mustKeyPair(t)
	serial := []byte("ser")
	certNonce := []byte("nonce")

	cert, err := NewNodeCert(
		ac.GetPublicKey(),
		"issuer-hash",
		100,
		200,
		serial,
		certNonce,
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	serial[0] = 'X'
	certNonce[0] = 'Y'
	if string(cert.Serial()) != "ser" {
		t.Fatalf("serial mutated after constructor copy: %q", cert.Serial())
	}
	if string(cert.CertNonce()) != "nonce" {
		t.Fatalf(
			"cert nonce mutated after constructor copy: %q",
			cert.CertNonce(),
		)
	}

	returnedSerial := cert.Serial()
	returnedNonce := cert.CertNonce()
	returnedSerial[0] = 'Z'
	returnedNonce[0] = 'W'

	if !bytes.Equal(cert.Serial(), []byte("ser")) {
		t.Fatalf("serial mutated through getter: %q", cert.Serial())
	}
	if !bytes.Equal(cert.CertNonce(), []byte("nonce")) {
		t.Fatalf(
			"cert nonce mutated through getter: %q",
			cert.CertNonce(),
		)
	}
}
