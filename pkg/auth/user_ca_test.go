package auth

import (
	"strings"
	"testing"
	"time"
)

func TestNewUserCA(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	user, err := NewUserCA(userPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}
	if user == nil {
		t.Fatal("expected non-nil UserCA")
	}
}

func TestNewUserCANilPubKey(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	_, err := NewUserCA(nil, anchorSig, adminHash)
	if err == nil {
		t.Fatal("expected error for nil pubkey")
	}
}

func TestNewUserCA_RequiresAnchorSig(t *testing.T) { // A
	adminAC := generateKeys(t)
	adminPub := pubKeyPtr(t, adminAC)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	adminHash, err := computeCAHash(adminPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	_, err = NewUserCA(userPub, nil, adminHash)
	if err == nil {
		t.Fatal("expected error for nil anchor sig")
	}

	_, err = NewUserCA(userPub, []byte{}, adminHash)
	if err == nil {
		t.Fatal("expected error for empty anchor sig")
	}
}

func TestNewUserCA_RequiresAnchorAdminHash(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, _ := buildAnchoredUserCA(t, adminAC, userPub)

	_, err := NewUserCA(userPub, anchorSig, CaHash{})
	if err == nil {
		t.Fatal("expected error for zero admin hash")
	}
}

func TestUserCAPubKey(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	user, err := NewUserCA(userPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	if !user.PubKey().Equal(userPub) {
		t.Error("PubKey() does not match input")
	}
}

func TestUserCAHash(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	user, err := NewUserCA(userPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	expected, err := computeCAHash(userPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	if !user.Hash().Equal(expected) {
		t.Error("Hash() does not match expected")
	}
}

func TestUserCAHashNotZero(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	user, err := NewUserCA(userPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}
	if user.Hash().IsZero() {
		t.Error("user CA hash should not be zero")
	}
}

func TestUserCAAnchorSig(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	user, err := NewUserCA(userPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	if !equalByteSlices(user.AnchorSig(), anchorSig) {
		t.Error("AnchorSig() does not match input")
	}
}

func TestUserCAAnchorAdminHash(t *testing.T) { // A
	adminAC := generateKeys(t)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	user, err := NewUserCA(userPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	if !user.AnchorAdminHash().Equal(adminHash) {
		t.Error("AnchorAdminHash() does not match input")
	}
}

func equalByteSlices(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUserCAVerifyNodeCert(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, caAC, caPub)

	user, err := NewUserCA(caPub, anchorSig, adminHash)
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

	anchorSig, adminHash := buildAnchoredUserCA(t, caAC, caPub)

	user, _ := NewUserCA(caPub, anchorSig, adminHash)
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
	otherPub := pubKeyPtr(t, otherAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, otherAC, otherPub)

	user, _ := NewUserCA(otherPub, anchorSig, adminHash)
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

	anchorSig, adminHash := buildAnchoredUserCA(t, caAC, caPub)

	user, err := NewUserCA(caPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	wrongIssuerHash, err := computeCAHash(otherPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	now := time.Now()
	cert, err := NewNodeCert(NodeCertParams{
		NodePubKey:   nodePub,
		IssuerCAHash: wrongIssuerHash,
		ValidFrom:    now.Add(-time.Hour),
		ValidUntil:   now.Add(time.Hour),
		Serial:       testSerial(t),
		RoleClaims:   ScopeUser,
		CertNonce:    testNonce(t),
	})
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
	caPub := pubKeyPtr(t, caAC)

	anchorSig, adminHash := buildAnchoredUserCA(t, caAC, caPub)

	user, err := NewUserCA(caPub, anchorSig, adminHash)
	if err != nil {
		t.Fatalf("NewUserCA: %v", err)
	}

	_, err = user.VerifyNodeCert(nil, []byte("sig"))
	if err == nil {
		t.Fatal("expected error for nil cert")
	}
}
