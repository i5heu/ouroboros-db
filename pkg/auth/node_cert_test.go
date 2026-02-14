package auth

import "testing"

func TestNewNodeCert(t *testing.T) { // A
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()

	cert, err := NewNodeCert(
		pub, "issuer-hash",
		100, 200,
		[]byte("ser"), nil, []byte("nonce"),
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
		pub, "h", 0, 0, nil, nil, nil,
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
		pub, "h", 0, 0, nil, nil, nil,
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
		pub, "h", 0, 0, nil, nil, nil,
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
