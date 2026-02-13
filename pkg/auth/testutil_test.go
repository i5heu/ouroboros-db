package auth

import (
	"crypto/rand"
	"testing"
	"time"

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

// testSerial returns a non-zero serial for tests.
func testSerial(t *testing.T) [16]byte { // A
	t.Helper()
	var s [16]byte
	if _, err := rand.Read(s[:]); err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	return s
}

// testNonce returns a non-zero nonce for tests.
func testNonce(t *testing.T) [32]byte { // A
	t.Helper()
	var n [32]byte
	if _, err := rand.Read(n[:]); err != nil {
		t.Fatalf("generate nonce: %v", err)
	}
	return n
}

// buildTestCert creates a NodeCert issued by the given
// CA for the given node key pair with default test
// validity.
func buildTestCert( // A
	t *testing.T,
	caPub *keys.PublicKey,
	nodePub *keys.PublicKey,
	role ...TrustScope,
) *NodeCert {
	t.Helper()
	caHash, err := computeCAHash(caPub)
	if err != nil {
		t.Fatalf("compute CA hash: %v", err)
	}
	certRole := ScopeUser
	if len(role) > 0 {
		certRole = role[0]
	}
	now := time.Now()
	cert, err := NewNodeCert(NodeCertParams{
		NodePubKey:   nodePub,
		IssuerCAHash: caHash,
		ValidFrom:    now.Add(-time.Hour),
		ValidUntil:   now.Add(time.Hour),
		Serial:       testSerial(t),
		RoleClaims:   certRole,
		CertNonce:    testNonce(t),
	})
	if err != nil {
		t.Fatalf("new node cert: %v", err)
	}
	return cert
}

// buildTestDelegation creates a DelegationProof for
// tests. It binds the given session key and cert hash.
func buildTestDelegation( // A
	t *testing.T,
	sessionPub *keys.PublicKey,
	cert *NodeCert,
	x509FP [32]byte,
) *DelegationProof {
	t.Helper()
	certHash, err := cert.Hash()
	if err != nil {
		t.Fatalf("cert hash: %v", err)
	}
	now := time.Now()
	proof, err := NewDelegationProof(DelegationProofParams{
		SessionPubKey:   sessionPub,
		X509Fingerprint: x509FP,
		NodeCertHash:    certHash,
		NotBefore:       now.Add(-time.Minute),
		NotAfter:        now.Add(time.Minute),
		HandshakeNonce:  testNonce(t),
	})
	if err != nil {
		t.Fatalf("new delegation proof: %v", err)
	}
	return proof
}

// signDelegation signs a DelegationProof with the
// given node key pair.
func signDelegation( // A
	t *testing.T,
	nodeAC *keys.AsyncCrypt,
	proof *DelegationProof,
) []byte {
	t.Helper()
	payload, err := delegationSigningPayload(proof)
	if err != nil {
		t.Fatalf("delegation payload: %v", err)
	}
	sig, err := nodeAC.Sign(payload)
	if err != nil {
		t.Fatalf("sign delegation: %v", err)
	}
	return sig
}

// buildVerifyParams constructs a full happy-path
// VerifyPeerCertParams.
func buildVerifyParams( // A
	t *testing.T,
	cert *NodeCert,
	caSig []byte,
	proof *DelegationProof,
	delegSig []byte,
	sessionPub *keys.PublicKey,
	x509FP [32]byte,
) VerifyPeerCertParams {
	t.Helper()
	return VerifyPeerCertParams{
		PeerCert:           cert,
		CASignature:        caSig,
		DelegationProof:    proof,
		DelegationSig:      delegSig,
		TLSSessionPubKey:   sessionPub,
		TLSX509Fingerprint: x509FP,
		TLSTranscriptHash:  []byte("test-transcript"),
	}
}

// fakeClock is a test clock with a controllable Now().
type fakeClock struct { // A
	now time.Time
}

func (c *fakeClock) Now() time.Time { // A
	return c.now
}

func newTestCarrierAuth() *CarrierAuth { // A
	return NewCarrierAuth(CarrierAuthConfig{})
}
