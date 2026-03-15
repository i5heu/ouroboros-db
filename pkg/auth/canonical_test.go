package auth

import (
	"bytes"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

func mustKeyPair(t *testing.T) *keys.AsyncCrypt { // A
	t.Helper()
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	return ac
}

func mustNodeCert( // A
	t *testing.T,
	pub keys.PublicKey,
	issuer string,
) *NodeCertImpl {
	t.Helper()
	cert, err := NewNodeCert(
		pub, issuer,
		1000, 2000,
		[]byte("serial-1"),
		[]byte("nonce-1"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert failed: %v", err)
	}
	return cert
}

func TestCanonicalNodeCert(t *testing.T) { // A
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()
	cert := mustNodeCert(t, pub, "ca-hash-1")

	data, err := CanonicalNodeCert(cert)
	if err != nil {
		t.Fatalf("CanonicalNodeCert: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty canonical encoding")
	}

	// Deterministic: same cert produces same bytes.
	data2, err := CanonicalNodeCert(cert)
	if err != nil {
		t.Fatalf("second encoding: %v", err)
	}
	if string(data) != string(data2) {
		t.Fatal("non-deterministic encoding")
	}
}

func TestCanonicalNodeCertBundle(t *testing.T) { // A
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()

	cert1, err := NewNodeCert(
		pub, "ca-1", 1000, 2000,
		[]byte("serial-1"), []byte("nonce-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	cert2, err := NewNodeCert(
		pub, "ca-2", 1000, 2000,
		[]byte("serial-2"), []byte("nonce-2"),
	)
	if err != nil {
		t.Fatal(err)
	}

	certs := []NodeCertLike{cert1, cert2}
	data, err := CanonicalNodeCertBundle(certs)
	if err != nil {
		t.Fatalf("CanonicalNodeCertBundle: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty bundle encoding")
	}

	// Order independence: reversed input should
	// produce same output.
	reversed := []NodeCertLike{cert2, cert1}
	data2, err := CanonicalNodeCertBundle(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(data2) {
		t.Fatal("bundle not order-independent")
	}
}

func TestCanonicalNodeCertBundleTotalOrdering( // A
	t *testing.T,
) {
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()

	cert1, err := NewNodeCert(
		pub, "ca-1", 1000, 2000,
		[]byte("serial-1"), []byte("nonce-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	cert2, err := NewNodeCert(
		pub, "ca-2", 1000, 2000,
		[]byte("serial-1"), []byte("nonce-2"),
	)
	if err != nil {
		t.Fatal(err)
	}

	data1, err := CanonicalNodeCertBundle(
		[]NodeCertLike{cert1, cert2},
	)
	if err != nil {
		t.Fatal(err)
	}
	data2, err := CanonicalNodeCertBundle(
		[]NodeCertLike{cert2, cert1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatal("bundle ordering is not total")
	}
}

func TestCanonicalDelegationProof(t *testing.T) { // A
	proof := NewDelegationProof(
		[]byte("cert-hash"),
		[]byte("exporter"),
		[]byte("transcript"),
		[]byte("x509-fp"),
		[]byte("bundle-hash"),
		1000, 1300,
	)

	data, err := CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatalf("CanonicalDelegationProof: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty encoding")
	}

	data2, err := CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(data2) {
		t.Fatal("non-deterministic encoding")
	}
}

func TestCanonicalDelegationProofForExporter( // A
	t *testing.T,
) {
	proof := NewDelegationProof(
		[]byte("cert-hash"),
		[]byte("exporter"),
		[]byte("transcript"),
		[]byte("x509-fp"),
		[]byte("bundle-hash"),
		1000, 1300,
	)

	full, err := CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatal(err)
	}
	noExp, err := CanonicalDelegationProofForExporter(
		proof,
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(full) == string(noExp) {
		t.Fatal(
			"exporter variant should differ from full",
		)
	}
}

func TestDomainSeparate(t *testing.T) { // A
	data := []byte("payload")
	sep := DomainSeparate("CTX_", data)
	expected := append([]byte("CTX_"), data...)
	if string(sep) != string(expected) {
		t.Fatalf("got %q, want %q", sep, expected)
	}
}
