package auth

import (
	"strings"
	"testing"
)

func TestNewAdminCA(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	admin, err := NewAdminCA(pub)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	if admin == nil {
		t.Fatal("expected non-nil AdminCA")
	}
}

func TestNewAdminCANilPubKey(t *testing.T) { // A
	_, err := NewAdminCA(nil)
	if err == nil {
		t.Fatal("expected error for nil pubkey")
	}
}

func TestAdminCAPubKey(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	admin, err := NewAdminCA(pub)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	if !admin.PubKey().Equal(pub) {
		t.Error("PubKey() does not match input")
	}
}

func TestAdminCAHash(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	admin, err := NewAdminCA(pub)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	expected, err := computeCAHash(pub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	if !admin.Hash().Equal(expected) {
		t.Error("Hash() does not match expected")
	}
}

func TestAdminCAHashNotZero(t *testing.T) { // A
	ac := generateKeys(t)
	admin, err := NewAdminCA(pubKeyPtr(t, ac))
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	if admin.Hash().IsZero() {
		t.Error("admin CA hash should not be zero")
	}
}

func TestAdminCAVerifyNodeCert(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	admin, err := NewAdminCA(caPub)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	nodeID, err := admin.VerifyNodeCert(cert, sig)
	if err != nil {
		t.Fatalf("VerifyNodeCert: %v", err)
	}

	expected, _ := nodePub.NodeID()
	if nodeID != expected {
		t.Errorf(
			"nodeID = %v, want %v",
			nodeID, expected,
		)
	}
}

func TestAdminCAVerifyNodeCertBadSig( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	admin, _ := NewAdminCA(caPub)
	cert := buildTestCert(t, caPub, nodePub)

	_, err := admin.VerifyNodeCert(
		cert, []byte("bad"),
	)
	if err == nil {
		t.Fatal(
			"expected error for bad signature",
		)
	}
}

func TestAdminCAVerifyNodeCertWrongCA( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	otherAC := generateKeys(t)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	admin, _ := NewAdminCA(
		pubKeyPtr(t, otherAC),
	)
	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, err := admin.VerifyNodeCert(cert, sig)
	if err == nil {
		t.Fatal(
			"expected error verifying with " +
				"wrong CA",
		)
	}
}

func TestAdminCAVerifyNodeCertIssuerMismatch( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	otherAC := generateKeys(t)
	otherPub := pubKeyPtr(t, otherAC)

	admin, err := NewAdminCA(caPub)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	wrongIssuerHash, err := computeCAHash(otherPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	cert, err := NewNodeCert(nodePub, wrongIssuerHash)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	sig := signCert(t, caAC, cert)

	_, err = admin.VerifyNodeCert(cert, sig)
	if err == nil {
		t.Fatal("expected issuer mismatch error")
	}
	if !strings.Contains(err.Error(), "issuer CA hash") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdminCAVerifyNodeCertNilCert( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	admin, err := NewAdminCA(pubKeyPtr(t, caAC))
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	_, err = admin.VerifyNodeCert(nil, []byte("sig"))
	if err == nil {
		t.Fatal("expected error for nil cert")
	}
}
