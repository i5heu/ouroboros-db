package auth

import (
	"strings"
	"testing"
)

func TestNewUserCA(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	user, err := NewUserCA(pub)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}
	if user == nil {
		t.Fatal("expected non-nil UserCA")
	}
}

func TestNewUserCANilPubKey(t *testing.T) { // A
	_, err := NewUserCA(nil)
	if err == nil {
		t.Fatal("expected error for nil pubkey")
	}
}

func TestUserCAPubKey(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	user, err := NewUserCA(pub)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	if !user.PubKey().Equal(pub) {
		t.Error("PubKey() does not match input")
	}
}

func TestUserCAHash(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	user, err := NewUserCA(pub)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	expected, err := computeCAHash(pub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	if !user.Hash().Equal(expected) {
		t.Error("Hash() does not match expected")
	}
}

func TestUserCAHashNotZero(t *testing.T) { // A
	ac := generateKeys(t)
	user, err := NewUserCA(pubKeyPtr(t, ac))
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}
	if user.Hash().IsZero() {
		t.Error("user CA hash should not be zero")
	}
}

func TestUserCAVerifyNodeCert(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	user, err := NewUserCA(caPub)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	nodeID, err := user.VerifyNodeCert(cert, sig)
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

func TestUserCAVerifyNodeCertBadSig( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	user, _ := NewUserCA(caPub)
	cert := buildTestCert(t, caPub, nodePub)

	_, err := user.VerifyNodeCert(
		cert, []byte("bad"),
	)
	if err == nil {
		t.Fatal(
			"expected error for bad signature",
		)
	}
}

func TestUserCAVerifyNodeCertWrongCA( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	otherAC := generateKeys(t)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	user, _ := NewUserCA(pubKeyPtr(t, otherAC))
	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, err := user.VerifyNodeCert(cert, sig)
	if err == nil {
		t.Fatal(
			"expected error verifying with " +
				"wrong CA",
		)
	}
}

func TestUserCAVerifyNodeCertIssuerMismatch( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	otherAC := generateKeys(t)
	otherPub := pubKeyPtr(t, otherAC)

	user, err := NewUserCA(caPub)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
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

	_, err = user.VerifyNodeCert(cert, sig)
	if err == nil {
		t.Fatal("expected issuer mismatch error")
	}
	if !strings.Contains(err.Error(), "issuer CA hash") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUserCAVerifyNodeCertNilCert( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	user, err := NewUserCA(pubKeyPtr(t, caAC))
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	_, err = user.VerifyNodeCert(nil, []byte("sig"))
	if err == nil {
		t.Fatal("expected error for nil cert")
	}
}
