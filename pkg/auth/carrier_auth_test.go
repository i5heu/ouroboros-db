package auth

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

type verifyFixture struct { // A
	carrier            *CarrierAuth
	clock              *fakeClock
	caAC               *keys.AsyncCrypt
	caPub              *keys.PublicKey
	adminAnchorAC      *keys.AsyncCrypt
	adminAnchorPub     *keys.PublicKey
	nodeAC             *keys.AsyncCrypt
	nodePub            *keys.PublicKey
	tlsCertPubKeyHash  [32]byte
	tlsExporterBinding [32]byte
	cert               *NodeCert
	caSig              []byte
	x509FP             [32]byte
	proof              *DelegationProof
	delegationSig      []byte
}

func newVerifyFixture( // A
	t *testing.T,
	caScope TrustScope,
	certRole TrustScope,
) verifyFixture {
	t.Helper()
	now := time.Now().UTC()
	clk := &fakeClock{now: now}
	carrier := NewCarrierAuth(CarrierAuthConfig{
		Clock:        clk,
		NonceTTL:     5 * time.Minute,
		FreshnessTTL: 30 * time.Minute,
		MaxDelegTTL:  5 * time.Minute,
	})

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	tlsCertPubKeyHash := testNonce(t)
	tlsExporterBinding := testNonce(t)

	var adminAnchorAC *keys.AsyncCrypt
	var adminAnchorPub *keys.PublicKey

	switch caScope {
	case ScopeAdmin:
		if err := carrier.AddAdminPubKey(caPub); err != nil {
			t.Fatalf("AddAdminPubKey: %v", err)
		}
	case ScopeUser:
		adminAnchorAC = generateKeys(t)
		adminAnchorPub = pubKeyPtr(t, adminAnchorAC)
		if err := carrier.AddAdminPubKey(adminAnchorPub); err != nil {
			t.Fatalf("AddAdminPubKey for anchor: %v", err)
		}
		anchorSig, adminHash := buildAnchoredUserCA(
			t,
			adminAnchorAC,
			caPub,
		)
		if err := carrier.AddUserPubKey(
			caPub,
			anchorSig,
			adminHash,
		); err != nil {
			t.Fatalf("AddUserPubKey: %v", err)
		}
	default:
		t.Fatalf("invalid caScope: %v", caScope)
	}

	cert := buildTestCert(t, caPub, nodePub, certRole)
	caSig := signCert(t, caAC, cert)
	x509FP := testNonce(t)
	proof := buildTestDelegation(
		t,
		tlsCertPubKeyHash,
		tlsExporterBinding,
		cert,
		x509FP,
	)
	delegationSig := signDelegation(t, nodeAC, proof)

	carrier.RefreshRevocationState(cert.IssuerCAHash())

	return verifyFixture{
		carrier:            carrier,
		clock:              clk,
		caAC:               caAC,
		caPub:              caPub,
		adminAnchorAC:      adminAnchorAC,
		adminAnchorPub:     adminAnchorPub,
		nodeAC:             nodeAC,
		nodePub:            nodePub,
		tlsCertPubKeyHash:  tlsCertPubKeyHash,
		tlsExporterBinding: tlsExporterBinding,
		cert:               cert,
		caSig:              caSig,
		x509FP:             x509FP,
		proof:              proof,
		delegationSig:      delegationSig,
	}
}

func buildParams( // A
	t *testing.T,
	f verifyFixture,
) VerifyPeerCertParams {
	t.Helper()
	return buildVerifyParams(
		t,
		f.cert,
		f.caSig,
		f.proof,
		f.delegationSig,
		f.tlsCertPubKeyHash,
		f.tlsExporterBinding,
		f.x509FP,
	)
}

func rebuildProof( // A
	t *testing.T,
	f verifyFixture,
	notBefore time.Time,
	notAfter time.Time,
	nonce [32]byte,
	tlsCertPubKeyHash [32]byte,
	tlsExporterBinding [32]byte,
	x509 [32]byte,
	nodeHash CaHash,
) (*DelegationProof, []byte) {
	t.Helper()
	proof, err := NewDelegationProof(DelegationProofParams{
		TLSCertPubKeyHash:  tlsCertPubKeyHash,
		TLSExporterBinding: tlsExporterBinding,
		X509Fingerprint:    x509,
		NodeCertHash:       nodeHash,
		NotBefore:          notBefore,
		NotAfter:           notAfter,
		HandshakeNonce:     nonce,
	})
	if err != nil {
		t.Fatalf("NewDelegationProof: %v", err)
	}
	sig := signDelegation(t, f.nodeAC, proof)
	return proof, sig
}

func TestNewCarrierAuth(t *testing.T) { // A
	ca := newTestCarrierAuth()
	if ca == nil {
		t.Fatal("expected non-nil CarrierAuth")
	}
}

func TestVerifyPeerCertHappyPath(t *testing.T) { // A
	tests := []struct {
		name     string
		caScope  TrustScope
		certRole TrustScope
		want     TrustScope
	}{
		{name: "admin", caScope: ScopeAdmin, certRole: ScopeAdmin, want: ScopeAdmin},
		{name: "user", caScope: ScopeUser, certRole: ScopeUser, want: ScopeUser},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newVerifyFixture(t, tc.caScope, tc.certRole)
			params := buildParams(t, f)

			nodeID, scope, err := f.carrier.VerifyPeerCert(params)
			if err != nil {
				t.Fatalf("VerifyPeerCert: %v", err)
			}
			if scope != tc.want {
				t.Fatalf("scope = %v, want %v", scope, tc.want)
			}
			expected, _ := f.nodePub.NodeID()
			if nodeID != expected {
				t.Fatalf("nodeID = %v, want %v", nodeID, expected)
			}
		})
	}
}

func TestVerifyPeerCertStep1UnknownCA(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	f.carrier = newTestCarrierAuth()
	_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyPeerCertStep2Failures(t *testing.T) { // A
	t.Run("expired cert", testStep2ExpiredCert)
	t.Run("not yet valid", testStep2NotYetValid)
	t.Run("revoked node", testStep2RevokedNode)
	t.Run("revoked CA", testStep2RevokedCA)
	t.Run("stale revocation state", testStep2StaleRevocation)
}

func testStep2ExpiredCert(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	cert, err := NewNodeCert(NodeCertParams{
		NodePubKey:   f.nodePub,
		IssuerCAHash: f.cert.IssuerCAHash(),
		ValidFrom:    f.clock.Now().Add(-2 * time.Hour),
		ValidUntil:   f.clock.Now().Add(-time.Hour),
		Serial:       testSerial(t),
		RoleClaims:   ScopeAdmin,
		CertNonce:    testNonce(t),
	})
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	f.cert = cert
	f.caSig = signCert(t, f.caAC, cert)
	_, _, err = f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testStep2NotYetValid(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	cert, err := NewNodeCert(NodeCertParams{
		NodePubKey:   f.nodePub,
		IssuerCAHash: f.cert.IssuerCAHash(),
		ValidFrom:    f.clock.Now().Add(time.Hour),
		ValidUntil:   f.clock.Now().Add(2 * time.Hour),
		Serial:       testSerial(t),
		RoleClaims:   ScopeAdmin,
		CertNonce:    testNonce(t),
	})
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	f.cert = cert
	f.caSig = signCert(t, f.caAC, cert)
	_, _, err = f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testStep2RevokedNode(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	nodeID, _ := f.nodePub.NodeID()
	_ = f.carrier.RevokeNode(nodeID)
	_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testStep2RevokedCA(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	_ = f.carrier.RevokeAdminCA(f.cert.IssuerCAHash())
	_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testStep2StaleRevocation(t *testing.T) { // A
	clk := &fakeClock{now: time.Now().UTC()}
	ca := NewCarrierAuth(CarrierAuthConfig{
		Clock:        clk,
		FreshnessTTL: time.Minute,
	})
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	if err := ca.AddAdminPubKey(caPub); err != nil {
		t.Fatalf("AddAdminPubKey: %v", err)
	}
	cert := buildTestCert(t, caPub, nodePub, ScopeAdmin)
	ca.RefreshRevocationState(cert.IssuerCAHash())
	clk.now = clk.now.Add(2 * time.Minute)

	x509 := testNonce(t)
	tlsCertPubKeyHash := testNonce(t)
	tlsExporterBinding := testNonce(t)
	proof := buildTestDelegation(
		t,
		tlsCertPubKeyHash,
		tlsExporterBinding,
		cert,
		x509,
	)
	params := buildVerifyParams(
		t,
		cert,
		signCert(t, caAC, cert),
		proof,
		signDelegation(t, nodeAC, proof),
		tlsCertPubKeyHash,
		tlsExporterBinding,
		x509,
	)
	_, _, err := ca.VerifyPeerCert(params)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyPeerCertStep3BadCASignature(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	params := buildParams(t, f)
	params.CASignature = []byte("bad")
	_, _, err := f.carrier.VerifyPeerCert(params)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyPeerCertStep4Failures(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)

	t.Run("bad delegation signature", func(t *testing.T) {
		params := buildParams(t, f)
		params.DelegationSig = []byte("bad")
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("session mismatch", func(t *testing.T) {
		params := buildParams(t, f)
		params.TLSCertPubKeyHash = testNonce(t)
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("x509 mismatch", func(t *testing.T) {
		params := buildParams(t, f)
		params.TLSX509Fingerprint = testNonce(t)
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cert hash mismatch", func(t *testing.T) {
		now := f.clock.Now()
		proof, sig := rebuildProof(
			t,
			f,
			now.Add(-time.Minute),
			now.Add(time.Minute),
			testNonce(t),
			f.tlsCertPubKeyHash,
			f.tlsExporterBinding,
			f.x509FP,
			testNonce(t),
		)
		params := buildParams(t, f)
		params.DelegationProof = proof
		params.DelegationSig = sig
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerifyPeerCertStep5Failures(t *testing.T) { // A
	t.Run("delegation too long", func(t *testing.T) {
		f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
		certHash, _ := f.cert.Hash()
		proof, sig := rebuildProof(
			t,
			f,
			f.clock.Now().Add(-time.Minute),
			f.clock.Now().Add(10*time.Minute),
			testNonce(t),
			f.tlsCertPubKeyHash,
			f.tlsExporterBinding,
			f.x509FP,
			certHash,
		)
		params := buildParams(t, f)
		params.DelegationProof = proof
		params.DelegationSig = sig
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("delegation expired", func(t *testing.T) {
		f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
		certHash, _ := f.cert.Hash()
		proof, sig := rebuildProof(
			t,
			f,
			f.clock.Now().Add(-2*time.Minute),
			f.clock.Now().Add(-time.Minute),
			testNonce(t),
			f.tlsCertPubKeyHash,
			f.tlsExporterBinding,
			f.x509FP,
			certHash,
		)
		params := buildParams(t, f)
		params.DelegationProof = proof
		params.DelegationSig = sig
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("replayed nonce", func(t *testing.T) {
		f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
		params := buildParams(t, f)
		if _, _, err := f.carrier.VerifyPeerCert(params); err != nil {
			t.Fatalf("first verify: %v", err)
		}
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty transcript", func(t *testing.T) {
		f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
		params := buildParams(t, f)
		params.TLSTranscriptHash = nil
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerifyPeerCertStep6Failures(t *testing.T) { // A
	t.Run("invalid role claim", func(t *testing.T) {
		f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
		cert := &NodeCert{
			certVersion:  f.cert.CertVersion(),
			nodePubKey:   f.nodePub,
			issuerCAHash: f.cert.IssuerCAHash(),
			validFrom:    f.clock.Now().Add(-time.Hour),
			validUntil:   f.clock.Now().Add(time.Hour),
			serial:       testSerial(t),
			roleClaims:   TrustScope(99),
			certNonce:    testNonce(t),
		}
		f.cert = cert
		f.caSig = signCert(t, f.caAC, cert)
		certHash, _ := cert.Hash()
		proof, sig := rebuildProof(
			t,
			f,
			f.clock.Now().Add(-time.Minute),
			f.clock.Now().Add(time.Minute),
			testNonce(t),
			f.tlsCertPubKeyHash,
			f.tlsExporterBinding,
			f.x509FP,
			certHash,
		)
		f.proof = proof
		f.delegationSig = sig
		_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("role mismatch with CA", func(t *testing.T) {
		f := newVerifyFixture(t, ScopeAdmin, ScopeUser)
		_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerifyPeerCertNilGuards(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	params := buildParams(t, f)

	tests := []struct {
		name    string
		carrier *CarrierAuth
		params  VerifyPeerCertParams
		wantErr string
	}{
		{
			name:    "nil carrier",
			carrier: nil,
			params:  params,
			wantErr: "carrier auth must not be nil",
		},
		{
			name:    "nil peer cert",
			carrier: f.carrier,
			params: func() VerifyPeerCertParams {
				cp := params
				cp.PeerCert = nil
				return cp
			}(),
			wantErr: "peer cert must not be nil",
		},
		{
			name:    "zero TLS cert hash",
			carrier: f.carrier,
			params: func() VerifyPeerCertParams {
				cp := params
				cp.TLSCertPubKeyHash = [32]byte{}
				return cp
			}(),
			wantErr: "TLS cert pubkey hash must not be zero",
		},
		{
			name:    "zero TLS exporter",
			carrier: f.carrier,
			params: func() VerifyPeerCertParams {
				cp := params
				cp.TLSExporterBinding = [32]byte{}
				return cp
			}(),
			wantErr: "TLS exporter binding must not be zero",
		},
		{
			name:    "malformed cert",
			carrier: f.carrier,
			params: func() VerifyPeerCertParams {
				cp := params
				cp.PeerCert = &NodeCert{issuerCAHash: f.cert.IssuerCAHash()}
				return cp
			}(),
			wantErr: "peer cert node public key must not be nil",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tc.carrier.VerifyPeerCert(tc.params)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestConcurrentVerifyPeerCert(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
			_, _, err := local.carrier.VerifyPeerCert(buildParams(t, local))
			if err != nil {
				t.Errorf("concurrent verify: %v", err)
			}
		}()
	}
	wg.Wait()
	_ = f
}

func TestCarrierAuthKeyManagement(t *testing.T) { // A
	ca := newTestCarrierAuth()
	adminAC := generateKeys(t)
	adminPub := pubKeyPtr(t, adminAC)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)

	mustErr(t, ca.AddAdminPubKey(nil), "nil admin pubkey")
	mustNoErr(t, ca.AddAdminPubKey(adminPub), "AddAdminPubKey")

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)
	mustErr(t, ca.AddUserPubKey(nil, anchorSig, adminHash), "nil user pubkey")
	mustErr(t, ca.AddUserPubKey(userPub, nil, adminHash), "nil anchor sig")
	mustErr(t, ca.AddUserPubKey(userPub, anchorSig, CaHash{}), "zero admin hash")
	mustNoErr(t, ca.AddUserPubKey(userPub, anchorSig, adminHash), "AddUserPubKey")

	userHash, _ := computeCAHash(userPub)
	mustNoErr(t, ca.RemoveAdminPubKey(adminHash), "RemoveAdminPubKey")
	mustNoErr(t, ca.RemoveUserPubKey(userHash), "RemoveUserPubKey")
	mustNoErr(t, ca.RevokeAdminCA(adminHash), "RevokeAdminCA")
	mustNoErr(t, ca.RevokeUserCA(userHash), "RevokeUserCA")
	mustErr(t, ca.AddAdminPubKey(adminPub), "revoked admin CA")
	mustErr(t, ca.AddUserPubKey(userPub, anchorSig, adminHash), "revoked user CA")
}

func mustNoErr( // A
	t *testing.T,
	err error,
	label string,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

func mustErr( // A
	t *testing.T,
	err error,
	label string,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error: %s", label)
	}
}

func TestCrossContextAdmissionSigAsDelegation(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	params := buildParams(t, f)
	// Swap DelegationSig with CASignature (Admission context)
	params.DelegationSig = f.caSig
	_, _, err := f.carrier.VerifyPeerCert(params)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Step 4 must reject cross-context signature: %v", err)
	}
}

func TestCrossContextDelegationSigAsAdmission(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	params := buildParams(t, f)
	// Swap CASignature with DelegationSig (Delegation context)
	params.CASignature = f.delegationSig
	_, _, err := f.carrier.VerifyPeerCert(params)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Step 3 must reject cross-context signature: %v", err)
	}
}

func TestScopeUserCertSignedByAdminCA(t *testing.T) { // A
	// NodeCert claims ScopeUser, but signed by AdminCA.
	f := newVerifyFixture(t, ScopeAdmin, ScopeUser)
	_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Step 6 must reject role mismatch: %v", err)
	}
}

func TestDelegationTTLBoundary(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	certHash, _ := f.cert.Hash()

	t.Run("exactly max TTL", func(t *testing.T) {
		proof, sig := rebuildProof(
			t,
			f,
			f.clock.Now(),
			f.clock.Now().Add(5*time.Minute),
			testNonce(t),
			f.tlsCertPubKeyHash,
			f.tlsExporterBinding,
			f.x509FP,
			certHash,
		)
		params := buildParams(t, f)
		params.DelegationProof = proof
		params.DelegationSig = sig
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err != nil {
			t.Fatalf("exactly max TTL should pass: %v", err)
		}
	})

	t.Run("exceeds max TTL by 1ns", func(t *testing.T) {
		proof, sig := rebuildProof(
			t,
			f,
			f.clock.Now(),
			f.clock.Now().Add(5*time.Minute+time.Nanosecond),
			testNonce(t),
			f.tlsCertPubKeyHash,
			f.tlsExporterBinding,
			f.x509FP,
			certHash,
		)
		params := buildParams(t, f)
		params.DelegationProof = proof
		params.DelegationSig = sig
		_, _, err := f.carrier.VerifyPeerCert(params)
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("exceeding max TTL must fail: %v", err)
		}
	})
}

func TestDelegationNotBeforeInFuture(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	certHash, _ := f.cert.Hash()
	proof, sig := rebuildProof(
		t,
		f,
		f.clock.Now().Add(time.Minute), // NotBefore in future
		f.clock.Now().Add(2*time.Minute),
		testNonce(t),
		f.tlsCertPubKeyHash,
		f.tlsExporterBinding,
		f.x509FP,
		certHash,
	)
	params := buildParams(t, f)
	params.DelegationProof = proof
	params.DelegationSig = sig
	_, _, err := f.carrier.VerifyPeerCert(params)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("not yet valid delegation must fail: %v", err)
	}
}

func TestCertValidityBoundaryExactNow(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)

	t.Run("ValidFrom equals now", func(t *testing.T) {
		cert, _ := NewNodeCert(NodeCertParams{
			NodePubKey:   f.nodePub,
			IssuerCAHash: f.cert.IssuerCAHash(),
			ValidFrom:    f.clock.Now(),
			ValidUntil:   f.clock.Now().Add(time.Hour),
			Serial:       testSerial(t),
			RoleClaims:   ScopeAdmin,
			CertNonce:    testNonce(t),
		})
		f.cert = cert
		f.caSig = signCert(t, f.caAC, cert)
		certHash, _ := cert.Hash()
		f.proof, f.delegationSig = rebuildProof(
			t, f, f.clock.now.Add(-time.Minute), f.clock.now.Add(time.Minute),
			testNonce(t), f.tlsCertPubKeyHash, f.tlsExporterBinding, f.x509FP, certHash,
		)
		_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
		if err != nil {
			t.Fatalf("ValidFrom = now should pass: %v", err)
		}
	})

	t.Run("ValidUntil equals now", func(t *testing.T) {
		cert, _ := NewNodeCert(NodeCertParams{
			NodePubKey:   f.nodePub,
			IssuerCAHash: f.cert.IssuerCAHash(),
			ValidFrom:    f.clock.Now().Add(-time.Hour),
			ValidUntil:   f.clock.Now(),
			Serial:       testSerial(t),
			RoleClaims:   ScopeAdmin,
			CertNonce:    testNonce(t),
		})
		f.cert = cert
		f.caSig = signCert(t, f.caAC, cert)
		certHash, _ := cert.Hash()
		f.proof, f.delegationSig = rebuildProof(
			t, f, f.clock.now.Add(-time.Minute), f.clock.now.Add(time.Minute),
			testNonce(t), f.tlsCertPubKeyHash, f.tlsExporterBinding, f.x509FP, certHash,
		)
		_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
		if err != nil {
			t.Fatalf("ValidUntil = now should pass (After is strict): %v", err)
		}
	})

	t.Run("ValidUntil equals now minus 1ns", func(t *testing.T) {
		cert, _ := NewNodeCert(NodeCertParams{
			NodePubKey:   f.nodePub,
			IssuerCAHash: f.cert.IssuerCAHash(),
			ValidFrom:    f.clock.Now().Add(-time.Hour),
			ValidUntil:   f.clock.Now().Add(-time.Nanosecond),
			Serial:       testSerial(t),
			RoleClaims:   ScopeAdmin,
			CertNonce:    testNonce(t),
		})
		f.cert = cert
		f.caSig = signCert(t, f.caAC, cert)
		certHash, _ := cert.Hash()
		f.proof, f.delegationSig = rebuildProof(
			t, f, f.clock.now.Add(-time.Minute), f.clock.now.Add(time.Minute),
			testNonce(t), f.tlsCertPubKeyHash, f.tlsExporterBinding, f.x509FP, certHash,
		)
		_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
		if err == nil || !strings.Contains(err.Error(), "authentication failed") {
			t.Fatalf("ValidUntil = now-1ns must fail: %v", err)
		}
	})
}

func TestRevocationFreshnessBoundary(t *testing.T) { // A
	clk := &fakeClock{now: time.Now().UTC()}
	caAuth := NewCarrierAuth(CarrierAuthConfig{
		Clock:        clk,
		FreshnessTTL: time.Minute,
	})
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	_ = caAuth.AddAdminPubKey(caPub)
	caHash, _ := computeCAHash(caPub)

	t.Run("exactly at TTL boundary", func(t *testing.T) {
		caAuth.RefreshRevocationState(caHash)
		clk.now = clk.now.Add(time.Minute)
		// checkRevocationFreshness uses Now().After(lastRefresh + TTL)
		// So exactly at TTL boundary it should pass.
		err := caAuth.checkRevocationFreshness(caHash)
		if err != nil {
			t.Fatalf("exactly at TTL boundary should pass: %v", err)
		}
	})

	t.Run("1ns past TTL boundary", func(t *testing.T) {
		caAuth.RefreshRevocationState(caHash)
		clk.now = clk.now.Add(time.Minute + time.Nanosecond)
		err := caAuth.checkRevocationFreshness(caHash)
		if err == nil {
			t.Fatal("1ns past TTL boundary must fail")
		}
	})
}

func TestReAddRevokedAdminCA(t *testing.T) { // A
	caAuth := newTestCarrierAuth()
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	caHash, _ := computeCAHash(caPub)

	_ = caAuth.RevokeAdminCA(caHash)
	err := caAuth.AddAdminPubKey(caPub)
	if err == nil || !strings.Contains(err.Error(), "is revoked") {
		t.Fatalf("re-adding revoked Admin CA must fail: %v", err)
	}
}

func TestReAddRevokedUserCA(t *testing.T) { // A
	caAuth := newTestCarrierAuth()
	adminAC := generateKeys(t)
	adminPub := pubKeyPtr(t, adminAC)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)
	userHash, _ := computeCAHash(userPub)

	mustNoErr(t, caAuth.AddAdminPubKey(adminPub), "AddAdminPubKey")
	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	_ = caAuth.RevokeUserCA(userHash)
	err := caAuth.AddUserPubKey(userPub, anchorSig, adminHash)
	if err == nil || !strings.Contains(err.Error(), "is revoked") {
		t.Fatalf("re-adding revoked User CA must fail: %v", err)
	}
}

func TestRevokeNodeThenVerify(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	nodeID, _ := f.nodePub.NodeID()
	_ = f.carrier.RevokeNode(nodeID)

	_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("revoked node must fail verification: %v", err)
	}
}

func TestRemoveAdminCAThenVerify(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	caHash := f.cert.IssuerCAHash()

	_ = f.carrier.RemoveAdminPubKey(caHash)
	_, _, err := f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("removed CA must fail verification: %v", err)
	}

	_ = f.carrier.AddAdminPubKey(f.caPub)
	f.carrier.RefreshRevocationState(caHash)
	_, _, err = f.carrier.VerifyPeerCert(buildParams(t, f))
	if err != nil {
		t.Fatalf("re-added CA should succeed verification: %v", err)
	}
}

func TestConcurrentRevocationDuringVerify(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, _ = f.carrier.VerifyPeerCert(buildParams(t, f))
		}()
		go func(idx int) {
			defer wg.Done()
			nodeID := keys.NodeID{byte(idx + 10)}
			_ = f.carrier.RevokeNode(nodeID)
		}(i)
	}
	wg.Wait()
}

func TestNonceReplayWithinTTL(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	params := buildParams(t, f)

	_, _, err := f.carrier.VerifyPeerCert(params)
	if err != nil {
		t.Fatalf("first verify failed: %v", err)
	}

	_, _, err = f.carrier.VerifyPeerCert(params)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("replay within TTL must fail: %v", err)
	}
}

func TestNonceReuseAfterTTLExpiry(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeAdmin, ScopeAdmin)
	params := buildParams(t, f)

	_, _, err := f.carrier.VerifyPeerCert(params)
	if err != nil {
		t.Fatalf("first verify failed: %v", err)
	}

	// Advance clock past NonceTTL (5m in fixture)
	f.clock.now = f.clock.now.Add(6 * time.Minute)

	// Rebuild proof with SAME nonce but valid window for NEW 'now'
	certHash, _ := f.cert.Hash()
	proof, sig := rebuildProof(
		t,
		f,
		f.clock.now.Add(-time.Minute),
		f.clock.now.Add(time.Minute),
		f.proof.HandshakeNonce(), // SAME NONCE
		f.tlsCertPubKeyHash,
		f.tlsExporterBinding,
		f.x509FP,
		certHash,
	)
	params.DelegationProof = proof
	params.DelegationSig = sig

	_, _, err = f.carrier.VerifyPeerCert(params)
	if err != nil {
		t.Fatalf("nonce reuse after TTL expiry should succeed: %v", err)
	}
}

func TestAddUserPubKey_UnknownAdminCA(t *testing.T) { // A
	ca := newTestCarrierAuth()
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)
	adminAC := generateKeys(t)

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	err := ca.AddUserPubKey(userPub, anchorSig, adminHash)
	if err == nil {
		t.Fatal("expected error for unknown admin CA")
	}
	if !strings.Contains(err.Error(), "not found in trust store") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddUserPubKey_InvalidAnchorSig(t *testing.T) { // A
	ca := newTestCarrierAuth()
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)
	adminAC := generateKeys(t)
	adminPub := pubKeyPtr(t, adminAC)

	mustNoErr(t, ca.AddAdminPubKey(adminPub), "AddAdminPubKey")

	adminHash, _ := computeCAHash(adminPub)
	badSig := []byte("invalid-signature")

	err := ca.AddUserPubKey(userPub, badSig, adminHash)
	if err == nil {
		t.Fatal("expected error for invalid anchor signature")
	}
	if !strings.Contains(err.Error(), "anchor signature verification failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddUserPubKey_RevokedAnchorAdmin(t *testing.T) { // A
	ca := newTestCarrierAuth()
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)
	adminAC := generateKeys(t)
	adminPub := pubKeyPtr(t, adminAC)

	mustNoErr(t, ca.AddAdminPubKey(adminPub), "AddAdminPubKey")

	anchorSig, adminHash := buildAnchoredUserCA(t, adminAC, userPub)

	mustNoErr(t, ca.RevokeAdminCA(adminHash), "RevokeAdminCA")

	err := ca.AddUserPubKey(userPub, anchorSig, adminHash)
	if err == nil {
		t.Fatal("expected error for revoked anchor admin")
	}
	if !strings.Contains(err.Error(), "is revoked") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyPeerCert_UserCA_AnchorAdminRevoked(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeUser, ScopeUser)

	adminAnchorHash, err := computeCAHash(f.adminAnchorPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	mustNoErr(
		t,
		f.carrier.RevokeAdminCA(adminAnchorHash),
		"RevokeAdminCA",
	)

	_, _, err = f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth failure with revoked anchor admin: %v", err)
	}
}

func TestVerifyPeerCert_UserCA_AnchorAdminRemoved(t *testing.T) { // A
	f := newVerifyFixture(t, ScopeUser, ScopeUser)

	adminAnchorHash, err := computeCAHash(f.adminAnchorPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	_ = f.carrier.RemoveAdminPubKey(adminAnchorHash)

	_, _, err = f.carrier.VerifyPeerCert(buildParams(t, f))
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth failure with removed anchor admin: %v", err)
	}
}
