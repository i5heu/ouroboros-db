package auth

import (
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// generateKeys creates a fresh AsyncCrypt key pair for
// testing.
func generateKeys( // A
	t *testing.T,
) *keys.AsyncCrypt {
	t.Helper()
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("generate keys: %v", err)
	}
	return ac
}

// pubKeyPtr extracts the public key from an AsyncCrypt
// and returns a pointer.
func pubKeyPtr( // A
	t *testing.T,
	ac *keys.AsyncCrypt,
) *keys.PublicKey {
	t.Helper()
	pub := ac.GetPublicKey()
	return &pub
}

// signCert signs a NodeCert with the given CA key pair
// using the domain-separated signing payload.
func signCert( // A
	t *testing.T,
	ca *keys.AsyncCrypt,
	cert *NodeCert,
) []byte {
	t.Helper()
	payload, err := signingPayload(cert)
	if err != nil {
		t.Fatalf("signing payload: %v", err)
	}
	sig, err := ca.Sign(payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return sig
}

// buildTestCert creates a NodeCert issued by the given
// CA for the given node key pair.
func buildTestCert( // A
	t *testing.T,
	caPub *keys.PublicKey,
	nodePub *keys.PublicKey,
) *NodeCert {
	t.Helper()
	caHash, err := computeCAHash(caPub)
	if err != nil {
		t.Fatalf("compute CA hash: %v", err)
	}
	cert, err := NewNodeCert(nodePub, caHash)
	if err != nil {
		t.Fatalf("new node cert: %v", err)
	}
	return cert
}
