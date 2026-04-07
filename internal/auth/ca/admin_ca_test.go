package ca

import (
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	canonicalpkg "github.com/i5heu/ouroboros-db/internal/auth/canonical"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
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
) *certpkg.NodeCertImpl {
	t.Helper()
	cert, err := certpkg.NewNodeCert(
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

func mustAdminCA( // A
	t *testing.T,
) (*AdminCAImpl, *keys.AsyncCrypt) {
	t.Helper()
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatal(err)
	}
	pub := ac.GetPublicKey()
	kem, err := pub.MarshalBinaryKEM()
	if err != nil {
		t.Fatal(err)
	}
	sign, err := pub.MarshalBinarySign()
	if err != nil {
		t.Fatal(err)
	}
	combined := append(kem, sign...)
	admin, err := NewAdminCA(combined)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	return admin, ac
}

func TestNewAdminCA(t *testing.T) { // A
	admin, _ := mustAdminCA(t)

	if admin.Hash() == "" {
		t.Error("hash should not be empty")
	}
	if len(admin.PubKey()) == 0 {
		t.Error("PubKey should not be empty")
	}
}

func TestAdminCAHashDeterministic(t *testing.T) { // A
	admin, ac := mustAdminCA(t)

	pub := ac.GetPublicKey()
	kem, _ := pub.MarshalBinaryKEM()
	sign, _ := pub.MarshalBinarySign()
	combined := append(kem, sign...)
	admin2, err := NewAdminCA(combined)
	if err != nil {
		t.Fatal(err)
	}
	if admin.Hash() != admin2.Hash() {
		t.Error("hashes should match for same key")
	}
}

func TestAdminCAVerifyNodeCert(t *testing.T) { // A
	admin, caAC := mustAdminCA(t)

	// Generate a node keypair.
	nodeAC := mustKeyPair(t)
	nodePub := nodeAC.GetPublicKey()

	cert := mustNodeCert(t, nodePub, admin.Hash())

	// Sign the cert with the CA key.
	canonical, err := canonicalpkg.CanonicalNodeCert(cert)
	if err != nil {
		t.Fatal(err)
	}
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	sig, err := caAC.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	nid, err := admin.VerifyNodeCert(cert, sig)
	if err != nil {
		t.Fatalf("VerifyNodeCert: %v", err)
	}

	expectedNID, _ := nodePub.NodeID()
	if nid != expectedNID {
		t.Error("returned NodeID mismatch")
	}
}

func TestAdminCAVerifyNodeCertBadSig( // A
	t *testing.T,
) {
	admin, _ := mustAdminCA(t)

	nodeAC := mustKeyPair(t)
	cert := mustNodeCert(
		t, nodeAC.GetPublicKey(), admin.Hash(),
	)

	_, err := admin.VerifyNodeCert(
		cert, []byte("bad-sig"),
	)
	if err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestNewAdminCATooShort(t *testing.T) { // A
	_, err := NewAdminCA([]byte("short"))
	if err == nil {
		t.Fatal("expected error for short key")
	}
}
