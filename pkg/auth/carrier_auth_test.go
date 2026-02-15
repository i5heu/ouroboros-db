package auth

import (
	"crypto/sha256"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger { // A
	return slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelError,
		}),
	)
}

func deriveTestExporter(ctx []byte) []byte { // A
	payload := append([]byte(ExporterLabel), ctx...)
	sum := sha256.Sum256(payload)
	return sum[:]
}

// fullScenario builds a complete verification
// scenario: admin CA, node cert, delegation proof,
// and all signatures.
type fullScenario struct { // A
	ca         *carrierAuth
	adminCA    *AdminCAImpl
	adminAC    *keys.AsyncCrypt
	nodeAC     *keys.AsyncCrypt
	cert       *NodeCertImpl
	caSig      []byte
	proof      *DelegationProofImpl
	delSig     []byte
	certHash   []byte
	exporter   []byte
	x509FP     []byte
	transcript []byte
}

func buildScenario(t *testing.T) *fullScenario { // A
	t.Helper()
	ca := NewCarrierAuth(testLogger())

	// Create and add AdminCA.
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatal(err)
	}
	adminPub := adminAC.GetPublicKey()
	kem, _ := adminPub.MarshalBinaryKEM()
	sign, _ := adminPub.MarshalBinarySign()
	adminBytes := append(kem, sign...)
	if err := ca.AddAdminPubKey(adminBytes); err != nil {
		t.Fatal(err)
	}
	adminCA, _ := NewAdminCA(adminBytes)

	// Create node keypair and cert.
	nodeAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatal(err)
	}
	nodePub := nodeAC.GetPublicKey()
	now := time.Now().Unix()
	cert, err := NewNodeCert(
		nodePub, adminCA.Hash(),
		now-10, now+600,
		[]byte("serial-1"), []byte("nonce-1"),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Sign cert with AdminCA.
	canonical, _ := CanonicalNodeCert(cert)
	msg := DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, err := adminAC.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	// Build delegation proof.
	certHash := []byte("tls-cert-pub-key-hash")
	x509FP := []byte("x509-fingerprint")
	transcript := []byte("transcript-hash")

	certs := []NodeCertLike{cert}
	bundleBytes, _ := CanonicalNodeCertBundle(certs)
	bundleHash := sha256.Sum256(bundleBytes)

	proof := NewDelegationProof(
		certHash,
		nil, // exporter set after
		transcript,
		x509FP,
		bundleHash[:],
		now-5, now+MaxDelegationTTL-10,
	)

	// Compute exporter from proof-without-exporter.
	exporterCtx, _ := CanonicalDelegationProofForExporter(
		proof,
	)
	exporter := deriveTestExporter(exporterCtx)

	// Set exporter in proof.
	proof = NewDelegationProof(
		certHash,
		exporter,
		transcript,
		x509FP,
		bundleHash[:],
		now-5, now+MaxDelegationTTL-10,
	)

	// Sign delegation proof with node key.
	delCanonical, _ := CanonicalDelegationProof(proof)
	delMsg := DomainSeparate(
		CTXNodeDelegationV1, delCanonical,
	)
	delSig, err := nodeAC.Sign(delMsg)
	if err != nil {
		t.Fatal(err)
	}

	return &fullScenario{
		ca:         ca,
		adminCA:    adminCA,
		adminAC:    adminAC,
		nodeAC:     nodeAC,
		cert:       cert,
		caSig:      caSig,
		proof:      proof,
		delSig:     delSig,
		certHash:   certHash,
		exporter:   exporter,
		x509FP:     x509FP,
		transcript: transcript,
	}
}

func TestVerifyPeerCertSuccess(t *testing.T) { // A
	s := buildScenario(t)

	ctx, err := s.ca.VerifyPeerCert(
		[]NodeCertLike{s.cert},
		[][]byte{s.caSig},
		s.proof,
		s.delSig,
		s.certHash,
		s.exporter,
		s.x509FP,
		s.transcript,
	)
	if err != nil {
		t.Fatalf("VerifyPeerCert: %v", err)
	}
	if ctx.EffectiveScope != ScopeAdmin {
		t.Errorf(
			"scope = %v, want ScopeAdmin",
			ctx.EffectiveScope,
		)
	}
	if !ctx.HasValidAdminCert {
		t.Error("should have valid admin cert")
	}
	nodePub := s.nodeAC.GetPublicKey()
	expectedNID, _ := nodePub.NodeID()
	if ctx.NodeID != expectedNID {
		t.Error("NodeID mismatch")
	}
}

func TestVerifyPeerCertNoCerts(t *testing.T) { // A
	ca := NewCarrierAuth(testLogger())
	_, err := ca.VerifyPeerCert(
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	if err != ErrNoCerts {
		t.Errorf("got %v, want ErrNoCerts", err)
	}
}

func TestVerifyPeerCertUnknownIssuer( // A
	t *testing.T,
) {
	ca := NewCarrierAuth(testLogger())
	ac := mustKeyPair(t)
	cert := mustNodeCert(
		t, ac.GetPublicKey(), "unknown-hash",
	)
	_, err := ca.VerifyPeerCert(
		[]NodeCertLike{cert},
		[][]byte{[]byte("sig")},
		NewDelegationProof(
			nil, nil, nil, nil, nil, 0, 0,
		),
		nil, nil, nil, nil, nil,
	)
	if err != ErrNoValidCerts {
		t.Errorf(
			"got %v, want ErrNoValidCerts", err,
		)
	}
}

func TestVerifyPeerCertExpiredCert( // A
	t *testing.T,
) {
	s := buildScenario(t)

	// Create an expired cert.
	expired, _ := NewNodeCert(
		s.nodeAC.GetPublicKey(), s.adminCA.Hash(),
		1000, 2000,
		[]byte("ser"), []byte("non"),
	)
	canonical, _ := CanonicalNodeCert(expired)
	msg := DomainSeparate(CTXNodeAdmissionV1, canonical)
	sig, _ := s.adminAC.Sign(msg)

	_, err := s.ca.VerifyPeerCert(
		[]NodeCertLike{expired},
		[][]byte{sig},
		s.proof,
		s.delSig,
		s.certHash,
		s.exporter,
		s.x509FP,
		s.transcript,
	)
	if err != ErrNoValidCerts {
		t.Errorf(
			"got %v, want ErrNoValidCerts", err,
		)
	}
}

func TestVerifyPeerCertBadDelegationSig( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := s.ca.VerifyPeerCert(
		[]NodeCertLike{s.cert},
		[][]byte{s.caSig},
		s.proof,
		[]byte("bad-delegation-sig"),
		s.certHash,
		s.exporter,
		s.x509FP,
		s.transcript,
	)
	if err != ErrInvalidDelegationSig {
		t.Errorf(
			"got %v, want ErrInvalidDelegationSig", err,
		)
	}
}

func TestVerifyPeerCertTLSBindingMismatch( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := s.ca.VerifyPeerCert(
		[]NodeCertLike{s.cert},
		[][]byte{s.caSig},
		s.proof,
		s.delSig,
		[]byte("wrong-cert-hash"),
		s.exporter,
		s.x509FP,
		s.transcript,
	)
	if err != ErrTLSBindingMismatch {
		t.Errorf(
			"got %v, want ErrTLSBindingMismatch", err,
		)
	}
}

func TestVerifyPeerCertBundleHashMismatch( // A
	t *testing.T,
) {
	s := buildScenario(t)

	// Tamper with the bundle hash in the proof.
	tampered := NewDelegationProof(
		s.proof.TLSCertPubKeyHash(),
		s.proof.TLSExporterBinding(),
		s.proof.TLSTranscriptHash(),
		s.proof.X509Fingerprint(),
		[]byte("wrong-bundle-hash"),
		s.proof.NotBefore(),
		s.proof.NotAfter(),
	)

	// Re-sign with tampered proof.
	canon, _ := CanonicalDelegationProof(tampered)
	msg := DomainSeparate(CTXNodeDelegationV1, canon)
	sig, _ := s.nodeAC.Sign(msg)

	_, err := s.ca.VerifyPeerCert(
		[]NodeCertLike{s.cert},
		[][]byte{s.caSig},
		tampered,
		sig,
		s.certHash,
		s.exporter,
		s.x509FP,
		s.transcript,
	)
	if err != ErrBundleHashMismatch {
		t.Errorf(
			"got %v, want ErrBundleHashMismatch", err,
		)
	}
}

func TestAddAdminPubKeyDuplicate(t *testing.T) { // A
	ca := NewCarrierAuth(testLogger())
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()
	kem, _ := pub.MarshalBinaryKEM()
	sign, _ := pub.MarshalBinarySign()
	combined := append(kem, sign...)

	if err := ca.AddAdminPubKey(combined); err != nil {
		t.Fatal(err)
	}
	if err := ca.AddAdminPubKey(combined); err != ErrCAAlreadyExists {
		t.Errorf(
			"got %v, want ErrCAAlreadyExists", err,
		)
	}
}

func TestRemoveAdminPubKey(t *testing.T) { // A
	ca := NewCarrierAuth(testLogger())
	ac := mustKeyPair(t)
	pub := ac.GetPublicKey()
	kem, _ := pub.MarshalBinaryKEM()
	sign, _ := pub.MarshalBinarySign()
	combined := append(kem, sign...)

	if err := ca.AddAdminPubKey(combined); err != nil {
		t.Fatal(err)
	}

	admin, _ := NewAdminCA(combined)
	err := ca.RemoveAdminPubKey(admin.Hash())
	if err != nil {
		t.Fatal(err)
	}

	err = ca.RemoveAdminPubKey(admin.Hash())
	if err != ErrCANotFound {
		t.Errorf("got %v, want ErrCANotFound", err)
	}
}

func TestRevokeUserCA(t *testing.T) { // A
	s := buildScenario(t)

	// Create and add a UserCA.
	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	signBytes, _ := userPub.MarshalBinarySign()
	anchorMsg := DomainSeparate(CTXUserCAAnchorV1, signBytes)
	anchorSig, _ := s.adminAC.Sign(anchorMsg)
	kem, _ := userPub.MarshalBinaryKEM()
	sign, _ := userPub.MarshalBinarySign()
	combined := append(kem, sign...)

	err := s.ca.AddUserPubKey(
		combined, anchorSig, s.adminCA.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}

	user, _ := newUserCA(
		combined, anchorSig, s.adminCA.Hash(),
	)
	err = s.ca.RevokeUserCA(user.Hash())
	if err != nil {
		t.Fatal(err)
	}

	// Cannot re-add after revocation.
	err = s.ca.AddUserPubKey(
		combined, anchorSig, s.adminCA.Hash(),
	)
	if err != ErrCARevoked {
		t.Errorf("got %v, want ErrCARevoked", err)
	}
}

func TestRevokeNode(t *testing.T) { // A
	s := buildScenario(t)

	nodePub := s.nodeAC.GetPublicKey()
	nid, _ := nodePub.NodeID()
	err := s.ca.RevokeNode(nid)
	if err != nil {
		t.Fatal(err)
	}

	// Verification should fail with revoked node.
	_, err = s.ca.VerifyPeerCert(
		[]NodeCertLike{s.cert},
		[][]byte{s.caSig},
		s.proof,
		s.delSig,
		s.certHash,
		s.exporter,
		s.x509FP,
		s.transcript,
	)
	if err != ErrNoValidCerts {
		t.Errorf(
			"got %v, want ErrNoValidCerts", err,
		)
	}
}

func TestAddUserPubKeyAnchorVerification( // A
	t *testing.T,
) {
	s := buildScenario(t)

	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	kem, _ := userPub.MarshalBinaryKEM()
	sign, _ := userPub.MarshalBinarySign()
	combined := append(kem, sign...)

	// Bad anchor sig.
	err := s.ca.AddUserPubKey(
		combined, []byte("bad-sig"), s.adminCA.Hash(),
	)
	if err != ErrInvalidAnchorSig {
		t.Errorf(
			"got %v, want ErrInvalidAnchorSig", err,
		)
	}
}

func TestAddUserPubKeyBadAnchorAdmin( // A
	t *testing.T,
) {
	ca := NewCarrierAuth(testLogger())
	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	kem, _ := userPub.MarshalBinaryKEM()
	sign, _ := userPub.MarshalBinarySign()
	combined := append(kem, sign...)

	err := ca.AddUserPubKey(
		combined, []byte("sig"), "nonexistent",
	)
	if err != ErrAnchorAdminNotFound {
		t.Errorf(
			"got %v, want ErrAnchorAdminNotFound",
			err,
		)
	}
}

func TestUserScopedVerification(t *testing.T) { // A
	ca := NewCarrierAuth(testLogger())

	// Create AdminCA for anchoring only.
	adminAC, _ := keys.NewAsyncCrypt()
	adminPub := adminAC.GetPublicKey()
	kem, _ := adminPub.MarshalBinaryKEM()
	sign, _ := adminPub.MarshalBinarySign()
	adminBytes := append(kem, sign...)
	if err := ca.AddAdminPubKey(adminBytes); err != nil {
		t.Fatal(err)
	}
	adminCA, _ := NewAdminCA(adminBytes)

	// Create UserCA.
	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	userSign, _ := userPub.MarshalBinarySign()
	anchorMsg := DomainSeparate(CTXUserCAAnchorV1, userSign)
	anchorSig, _ := adminAC.Sign(anchorMsg)
	userKEM, _ := userPub.MarshalBinaryKEM()
	userBytes := append(userKEM, userSign...)
	err := ca.AddUserPubKey(
		userBytes, anchorSig, adminCA.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}
	userCA, _ := newUserCA(
		userBytes, anchorSig, adminCA.Hash(),
	)

	// Create node cert signed by UserCA.
	nodeAC, _ := keys.NewAsyncCrypt()
	nodePub := nodeAC.GetPublicKey()
	now := time.Now().Unix()
	cert, _ := NewNodeCert(
		nodePub, userCA.Hash(),
		now-10, now+600,
		[]byte("s1"), []byte("n1"),
	)
	canonical, _ := CanonicalNodeCert(cert)
	msg := DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, _ := userAC.Sign(msg)

	// Build delegation.
	certHash := []byte("cert-hash")
	x509FP := []byte("x509-fp")
	transcript := []byte("transcript")
	certs := []NodeCertLike{cert}
	bundleBytes, _ := CanonicalNodeCertBundle(certs)
	bh := sha256.Sum256(bundleBytes)

	proof := NewDelegationProof(
		certHash, nil, transcript, x509FP,
		bh[:], now-5, now+MaxDelegationTTL-10,
	)
	expCtx, _ := CanonicalDelegationProofForExporter(
		proof,
	)
	exporter := deriveTestExporter(expCtx)
	proof = NewDelegationProof(
		certHash, exporter, transcript, x509FP,
		bh[:], now-5, now+MaxDelegationTTL-10,
	)
	delCanon, _ := CanonicalDelegationProof(proof)
	delMsg := DomainSeparate(
		CTXNodeDelegationV1, delCanon,
	)
	delSig, _ := nodeAC.Sign(delMsg)

	ctx, err := ca.VerifyPeerCert(
		certs,
		[][]byte{caSig},
		proof, delSig,
		certHash, exporter, x509FP, transcript,
	)
	if err != nil {
		t.Fatalf("VerifyPeerCert: %v", err)
	}
	if ctx.EffectiveScope != ScopeUser {
		t.Errorf(
			"scope = %v, want ScopeUser",
			ctx.EffectiveScope,
		)
	}
	if ctx.HasValidAdminCert {
		t.Error("should not have admin cert")
	}
	if len(ctx.AllowedUserCAOwners) != 1 {
		t.Fatalf(
			"want 1 owner, got %d",
			len(ctx.AllowedUserCAOwners),
		)
	}
	if ctx.AllowedUserCAOwners[0] != userCA.Hash() {
		t.Error("wrong UserCA owner hash")
	}
}

func TestDelegationTTLTooLong(t *testing.T) { // A
	s := buildScenario(t)

	now := time.Now().Unix()
	longProof := NewDelegationProof(
		s.proof.TLSCertPubKeyHash(),
		s.proof.TLSExporterBinding(),
		s.proof.TLSTranscriptHash(),
		s.proof.X509Fingerprint(),
		s.proof.NodeCertBundleHash(),
		now-10, now+MaxDelegationTTL+100,
	)

	canon, _ := CanonicalDelegationProof(longProof)
	msg := DomainSeparate(CTXNodeDelegationV1, canon)
	sig, _ := s.nodeAC.Sign(msg)

	_, err := s.ca.VerifyPeerCert(
		[]NodeCertLike{s.cert},
		[][]byte{s.caSig},
		longProof, sig,
		s.certHash, s.exporter,
		s.x509FP, s.transcript,
	)
	if err != ErrDelegationTooLong {
		t.Errorf(
			"got %v, want ErrDelegationTooLong", err,
		)
	}
}

func TestVerifyPeerCertSignatureCountMismatch( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := s.ca.VerifyPeerCert(
		[]NodeCertLike{s.cert},
		nil,
		s.proof,
		s.delSig,
		s.certHash,
		s.exporter,
		s.x509FP,
		s.transcript,
	)
	if err != ErrSignatureCountMismatch {
		t.Errorf(
			"got %v, want ErrSignatureCountMismatch",
			err,
		)
	}
}

func TestUserCAInvalidWhenAnchorAdminRemoved( // A
	t *testing.T,
) {
	ca := NewCarrierAuth(testLogger())

	adminAC := mustKeyPair(t)
	adminPub := adminAC.GetPublicKey()
	adminKEM, _ := adminPub.MarshalBinaryKEM()
	adminSign, _ := adminPub.MarshalBinarySign()
	adminBytes := append(adminKEM, adminSign...)
	if err := ca.AddAdminPubKey(adminBytes); err != nil {
		t.Fatal(err)
	}
	adminCA, _ := NewAdminCA(adminBytes)

	userAC := mustKeyPair(t)
	userPub := userAC.GetPublicKey()
	userSign, _ := userPub.MarshalBinarySign()
	anchorMsg := DomainSeparate(CTXUserCAAnchorV1, userSign)
	anchorSig, _ := adminAC.Sign(anchorMsg)
	userKEM, _ := userPub.MarshalBinaryKEM()
	userBytes := append(userKEM, userSign...)
	if err := ca.AddUserPubKey(
		userBytes,
		anchorSig,
		adminCA.Hash(),
	); err != nil {
		t.Fatal(err)
	}
	userCA, _ := newUserCA(
		userBytes,
		anchorSig,
		adminCA.Hash(),
	)

	if err := ca.RemoveAdminPubKey(adminCA.Hash()); err != nil {
		t.Fatal(err)
	}

	nodeAC := mustKeyPair(t)
	nodePub := nodeAC.GetPublicKey()
	now := time.Now().Unix()
	cert, _ := NewNodeCert(
		nodePub,
		userCA.Hash(),
		now-10,
		now+600,
		[]byte("s1"),
		[]byte("n1"),
	)
	canonical, _ := CanonicalNodeCert(cert)
	certMsg := DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, _ := userAC.Sign(certMsg)

	bundleBytes, _ := CanonicalNodeCertBundle(
		[]NodeCertLike{cert},
	)
	bundleHash := sha256.Sum256(bundleBytes)
	proof := NewDelegationProof(
		[]byte("cert-hash"),
		nil,
		[]byte("transcript"),
		[]byte("x509-fp"),
		bundleHash[:],
		now-5,
		now+MaxDelegationTTL-10,
	)
	expCtx, _ := CanonicalDelegationProofForExporter(
		proof,
	)
	exporter := deriveTestExporter(expCtx)
	proof = NewDelegationProof(
		[]byte("cert-hash"),
		exporter,
		[]byte("transcript"),
		[]byte("x509-fp"),
		bundleHash[:],
		now-5,
		now+MaxDelegationTTL-10,
	)
	proofCanon, _ := CanonicalDelegationProof(proof)
	proofMsg := DomainSeparate(
		CTXNodeDelegationV1,
		proofCanon,
	)
	delSig, _ := nodeAC.Sign(proofMsg)

	_, err := ca.VerifyPeerCert(
		[]NodeCertLike{cert},
		[][]byte{caSig},
		proof,
		delSig,
		[]byte("cert-hash"),
		exporter,
		[]byte("x509-fp"),
		[]byte("transcript"),
	)
	if err != ErrNoValidCerts {
		t.Errorf("got %v, want ErrNoValidCerts", err)
	}
}
