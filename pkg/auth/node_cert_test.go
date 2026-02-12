package auth

import (
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

func TestNewNodeCert(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(pub, caHash)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
}

func TestNewNodeCertNilPubKey(t *testing.T) { // A
	caHash := HashBytes([]byte("test-ca"))
	_, err := NewNodeCert(nil, caHash)
	if err == nil {
		t.Fatal("expected error for nil pubkey")
	}
}

func TestNodeCertNodePubKey(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(pub, caHash)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	got := cert.NodePubKey()
	if !got.Equal(pub) {
		t.Error(
			"NodePubKey() does not match input",
		)
	}
}

func TestNodeCertIssuerCAHash(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(pub, caHash)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	if !cert.IssuerCAHash().Equal(caHash) {
		t.Error(
			"IssuerCAHash() does not match input",
		)
	}
}

func TestNodeCertNodeID(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(pub, caHash)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	nodeID, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID: %v", err)
	}

	expected, err := pub.NodeID()
	if err != nil {
		t.Fatalf("pub.NodeID: %v", err)
	}

	if nodeID != expected {
		t.Errorf(
			"NodeID = %v, want %v",
			nodeID, expected,
		)
	}
}

func TestNodeCertNodeIDStable(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(pub, caHash)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	id1, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID call 1: %v", err)
	}

	id2, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID call 2: %v", err)
	}

	if id1 != id2 {
		t.Error("NodeID not stable across calls")
	}
}

func TestNodeCertDifferentKeysProduceDifferentIDs( // A
	t *testing.T,
) {
	ac1 := generateKeys(t)
	ac2 := generateKeys(t)
	caHash := HashBytes([]byte("test-ca"))

	cert1, _ := NewNodeCert(
		pubKeyPtr(t, ac1), caHash,
	)
	cert2, _ := NewNodeCert(
		pubKeyPtr(t, ac2), caHash,
	)

	id1, _ := cert1.NodeID()
	id2, _ := cert2.NodeID()

	if id1 == id2 {
		t.Error(
			"different keys should produce " +
				"different NodeIDs",
		)
	}
}

func TestNodeCertNodeIDIndependentOfCA( // A
	t *testing.T,
) {
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	caHash1 := HashBytes([]byte("ca-1"))
	caHash2 := HashBytes([]byte("ca-2"))

	cert1, _ := NewNodeCert(nodePub, caHash1)
	cert2, _ := NewNodeCert(nodePub, caHash2)

	id1, _ := cert1.NodeID()
	id2, _ := cert2.NodeID()

	if id1 != id2 {
		t.Error(
			"NodeID should be independent of CA",
		)
	}
}

func TestNodeCertNodeIDNotZero(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	cert, _ := NewNodeCert(
		pub, HashBytes([]byte("ca")),
	)

	id, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID: %v", err)
	}

	if id == (keys.NodeID{}) {
		t.Error("NodeID should not be zero")
	}
}
