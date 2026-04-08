package auth

import (
	"crypto/sha256"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	canonicalpkg "github.com/i5heu/ouroboros-db/internal/auth/canonical"
	delegationpkg "github.com/i5heu/ouroboros-db/internal/auth/delegation"
)

const oversizedPeerCertBundleSize = 1025 // A

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

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger { // A
	return slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelError,
		}),
	)
}

func deriveTestExporter(ctx []byte) []byte { // A
	payload := append([]byte(delegationpkg.ExporterLabel), ctx...)
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
	proof      *delegationpkg.DelegationProofImpl
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
	canonical, _ := canonicalpkg.CanonicalNodeCert(cert)
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, err := adminAC.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	// Build delegation proof.
	certHashSum := sha256.Sum256(
		[]byte("tls-cert-pub-key-hash"),
	)
	x509FPSum := sha256.Sum256(
		[]byte("x509-fingerprint"),
	)
	transcriptSum := sha256.Sum256(
		[]byte("transcript-hash"),
	)
	certHash := certHashSum[:]
	x509FP := x509FPSum[:]
	transcript := transcriptSum[:]

	certs := []canonicalpkg.NodeCertLike{cert}
	bundleBytes, _ := canonicalpkg.CanonicalNodeCertBundle(certs)
	bundleHash := sha256.Sum256(bundleBytes)

	proof := delegationpkg.NewDelegationProof(
		certHash,
		nil, // exporter set after
		transcript,
		x509FP,
		bundleHash[:],
		now-5, now+delegationpkg.MaxDelegationTTL-10,
	)

	// Compute exporter from proof-without-exporter.
	exporterCtx, _ := canonicalpkg.CanonicalDelegationProofForExporter(
		proof,
	)
	exporter := deriveTestExporter(exporterCtx)

	// Set exporter in proof.
	proof = delegationpkg.NewDelegationProof(
		certHash,
		exporter,
		transcript,
		x509FP,
		bundleHash[:],
		now-5, now+delegationpkg.MaxDelegationTTL-10,
	)

	// Sign delegation proof with node key.
	delCanonical, _ := canonicalpkg.CanonicalDelegationProof(proof)
	delMsg := canonicalpkg.DomainSeparate(
		delegationpkg.CTXNodeDelegationV1, delCanonical,
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

func TestVerifyPeerCertRejectsInvalidBindingFieldLength( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    [][]byte{s.caSig},
		DelegationProof: s.proof,
		DelegationSig:   s.delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  []byte("short"),
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if err == nil {
		t.Fatal("expected invalid binding field length")
	}
	if err.Error() == ErrTLSBindingMismatch.Error() {
		t.Fatalf(
			"got mismatch error instead of invalid length: %v",
			err,
		)
	}
}

func TestVerifyPeerCertSuccess(t *testing.T) { // A
	s := buildScenario(t)

	ctx, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    [][]byte{s.caSig},
		DelegationProof: s.proof,
		DelegationSig:   s.delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  s.certHash,
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
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
	_, err := ca.VerifyPeerCert(PeerHandshake{
		Certs:           nil,
		CASignatures:    nil,
		DelegationProof: nil,
		DelegationSig:   nil,
		TLS: TLSBindings{
			CertPubKeyHash:  nil,
			ExporterBinding: nil,
			X509Fingerprint: nil,
			TranscriptHash:  nil,
		},
	})
	if !errors.Is(err, ErrNoCerts) {
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
	_, err := ca.VerifyPeerCert(PeerHandshake{
		Certs:        []canonicalpkg.NodeCertLike{cert},
		CASignatures: [][]byte{[]byte("sig")},
		DelegationProof: delegationpkg.NewDelegationProof(
			nil, nil, nil, nil, nil, 0, 0,
		),
		DelegationSig: []byte("placeholder"),
		TLS: TLSBindings{
			CertPubKeyHash:  nil,
			ExporterBinding: nil,
			X509Fingerprint: nil,
			TranscriptHash:  nil,
		},
	})
	if !errors.Is(err, ErrNoValidCerts) {
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
	canonical, _ := canonicalpkg.CanonicalNodeCert(expired)
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	sig, _ := s.adminAC.Sign(msg)

	_, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{expired},
		CASignatures:    [][]byte{sig},
		DelegationProof: s.proof,
		DelegationSig:   s.delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  s.certHash,
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if !errors.Is(err, ErrNoValidCerts) {
		t.Errorf(
			"got %v, want ErrNoValidCerts", err,
		)
	}
}

func TestVerifyPeerCertBadDelegationSig( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    [][]byte{s.caSig},
		DelegationProof: s.proof,
		DelegationSig:   []byte("bad-delegation-sig"),
		TLS: TLSBindings{
			CertPubKeyHash:  s.certHash,
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if !errors.Is(err, ErrInvalidDelegationSig) {
		t.Errorf(
			"got %v, want ErrInvalidDelegationSig", err,
		)
	}
}

func TestVerifyPeerCertTLSBindingMismatch( // A
	t *testing.T,
) {
	s := buildScenario(t)
	wrongCertHash := sha256.Sum256(
		[]byte("wrong-cert-hash"),
	)

	_, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    [][]byte{s.caSig},
		DelegationProof: s.proof,
		DelegationSig:   s.delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  wrongCertHash[:],
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if !errors.Is(err, ErrTLSBindingMismatch) {
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
	wrongBundleHash := sha256.Sum256(
		[]byte("wrong-bundle-hash"),
	)
	tampered := delegationpkg.NewDelegationProof(
		s.proof.TLSCertPubKeyHash(),
		s.proof.TLSExporterBinding(),
		s.proof.TLSTranscriptHash(),
		s.proof.X509Fingerprint(),
		wrongBundleHash[:],
		s.proof.NotBefore(),
		s.proof.NotAfter(),
	)

	// Re-sign with tampered proof.
	canon, _ := canonicalpkg.CanonicalDelegationProof(tampered)
	msg := canonicalpkg.DomainSeparate(delegationpkg.CTXNodeDelegationV1, canon)
	sig, _ := s.nodeAC.Sign(msg)

	_, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    [][]byte{s.caSig},
		DelegationProof: tampered,
		DelegationSig:   sig,
		TLS: TLSBindings{
			CertPubKeyHash:  s.certHash,
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if !errors.Is(err, ErrBundleHashMismatch) {
		t.Errorf(
			"got %v, want ErrBundleHashMismatch", err,
		)
	}
}

type switchingNodeCert struct { // A
	signedPubKey    keys.PublicKey
	presentedPubKey keys.PublicKey
	issuerCAHash    string
	validFrom       int64
	validUntil      int64
	serial          []byte
	certNonce       []byte
	nodeID          keys.NodeID
	nodePubKeyCalls int
}

func newSwitchingNodeCert( // A
	signedPubKey keys.PublicKey,
	presentedPubKey keys.PublicKey,
	issuerCAHash string,
	validFrom int64,
	validUntil int64,
	serial []byte,
	certNonce []byte,
) (*switchingNodeCert, error) {
	nodeID, err := presentedPubKey.NodeID()
	if err != nil {
		return nil, err
	}
	return &switchingNodeCert{
		signedPubKey:    signedPubKey,
		presentedPubKey: presentedPubKey,
		issuerCAHash:    issuerCAHash,
		validFrom:       validFrom,
		validUntil:      validUntil,
		serial:          append([]byte(nil), serial...),
		certNonce:       append([]byte(nil), certNonce...),
		nodeID:          nodeID,
	}, nil
}

func (c *switchingNodeCert) CertVersion() uint16 { // A
	return DefaultCertVersion
}

func (c *switchingNodeCert) NodePubKey() keys.PublicKey { // A
	c.nodePubKeyCalls++
	if c.nodePubKeyCalls <= 2 {
		return c.signedPubKey
	}
	return c.presentedPubKey
}

func (c *switchingNodeCert) IssuerCAHash() string { // A
	return c.issuerCAHash
}

func (c *switchingNodeCert) ValidFrom() int64 { // A
	return c.validFrom
}

func (c *switchingNodeCert) ValidUntil() int64 { // A
	return c.validUntil
}

func (c *switchingNodeCert) Serial() []byte { // A
	return append([]byte(nil), c.serial...)
}

func (c *switchingNodeCert) CertNonce() []byte { // A
	return append([]byte(nil), c.certNonce...)
}

func (c *switchingNodeCert) NodeID() keys.NodeID { // A
	return c.nodeID
}

func mustVerifyPeerCertWithoutPanic( // A
	t *testing.T,
	ca *carrierAuth,
	hs PeerHandshake,
) (AuthContext, error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("VerifyPeerCert panicked: %v", recovered)
		}
	}()
	return ca.VerifyPeerCert(hs)
}

func TestVerifyPeerCertNilDelegationProofRejected( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := mustVerifyPeerCertWithoutPanic(
		t,
		s.ca,
		PeerHandshake{
			Certs:           []canonicalpkg.NodeCertLike{s.cert},
			CASignatures:    [][]byte{s.caSig},
			DelegationProof: nil,
			DelegationSig:   s.delSig,
			TLS: TLSBindings{
				CertPubKeyHash:  s.certHash,
				ExporterBinding: s.exporter,
				X509Fingerprint: s.x509FP,
				TranscriptHash:  s.transcript,
			},
		},
	)
	if err == nil {
		t.Fatal("expected nil delegation proof to be rejected")
	}
}

func TestVerifyPeerCertNilBundleEntryRejected( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := mustVerifyPeerCertWithoutPanic(
		t,
		s.ca,
		PeerHandshake{
			Certs:           []canonicalpkg.NodeCertLike{nil},
			CASignatures:    [][]byte{s.caSig},
			DelegationProof: s.proof,
			DelegationSig:   s.delSig,
			TLS: TLSBindings{
				CertPubKeyHash:  s.certHash,
				ExporterBinding: s.exporter,
				X509Fingerprint: s.x509FP,
				TranscriptHash:  s.transcript,
			},
		},
	)
	if err == nil {
		t.Fatal("expected nil certificate entry to be rejected")
	}
}

func TestVerifyPeerCertRejectsStatefulCertView( // A
	t *testing.T,
) {
	s := buildScenario(t)
	attackerAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatal(err)
	}
	attackerPub := attackerAC.GetPublicKey()

	maliciousCert, err := newSwitchingNodeCert(
		s.nodeAC.GetPublicKey(),
		attackerPub,
		s.adminCA.Hash(),
		s.cert.ValidFrom(),
		s.cert.ValidUntil(),
		s.cert.Serial(),
		s.cert.CertNonce(),
	)
	if err != nil {
		t.Fatal(err)
	}

	signedView, err := NewNodeCert(
		s.nodeAC.GetPublicKey(),
		s.adminCA.Hash(),
		s.cert.ValidFrom(),
		s.cert.ValidUntil(),
		s.cert.Serial(),
		s.cert.CertNonce(),
	)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalpkg.CanonicalNodeCert(signedView)
	if err != nil {
		t.Fatal(err)
	}
	certMsg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, err := s.adminAC.Sign(certMsg)
	if err != nil {
		t.Fatal(err)
	}

	presentedView, err := NewNodeCert(
		attackerPub,
		s.adminCA.Hash(),
		s.cert.ValidFrom(),
		s.cert.ValidUntil(),
		s.cert.Serial(),
		s.cert.CertNonce(),
	)
	if err != nil {
		t.Fatal(err)
	}
	bundleBytes, err := canonicalpkg.CanonicalNodeCertBundle(
		[]canonicalpkg.NodeCertLike{presentedView},
	)
	if err != nil {
		t.Fatal(err)
	}
	bundleHash := sha256.Sum256(bundleBytes)

	proof := delegationpkg.NewDelegationProof(
		s.certHash,
		nil,
		s.transcript,
		s.x509FP,
		bundleHash[:],
		time.Now().Unix()-5,
		time.Now().Unix()+delegationpkg.MaxDelegationTTL-10,
	)
	exporterCtx, err := canonicalpkg.CanonicalDelegationProofForExporter(
		proof,
	)
	if err != nil {
		t.Fatal(err)
	}
	exporter := deriveTestExporter(exporterCtx)
	proof = delegationpkg.NewDelegationProof(
		s.certHash,
		exporter,
		s.transcript,
		s.x509FP,
		bundleHash[:],
		proof.NotBefore(),
		proof.NotAfter(),
	)
	proofCanonical, err := canonicalpkg.CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatal(err)
	}
	delegationMsg := canonicalpkg.DomainSeparate(
		delegationpkg.CTXNodeDelegationV1,
		proofCanonical,
	)
	delegationSig, err := attackerAC.Sign(delegationMsg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := mustVerifyPeerCertWithoutPanic(
		t,
		s.ca,
		PeerHandshake{
			Certs:           []canonicalpkg.NodeCertLike{maliciousCert},
			CASignatures:    [][]byte{caSig},
			DelegationProof: proof,
			DelegationSig:   delegationSig,
			TLS: TLSBindings{
				CertPubKeyHash:  s.certHash,
				ExporterBinding: exporter,
				X509Fingerprint: s.x509FP,
				TranscriptHash:  s.transcript,
			},
		},
	)
	if err == nil {
		t.Fatalf(
			"expected stateful cert view to be rejected, got success for node %s",
			ctx.NodeID.String(),
		)
	}
}

func TestVerifyPeerCertRejectsOversizedBundle( // A
	t *testing.T,
) {
	s := buildScenario(t)
	peerCerts := make([]canonicalpkg.NodeCertLike, oversizedPeerCertBundleSize)
	caSignatures := make([][]byte, oversizedPeerCertBundleSize)
	for i := range peerCerts {
		peerCerts[i] = s.cert
		caSignatures[i] = s.caSig
	}
	bundleBytes, err := canonicalpkg.CanonicalNodeCertBundle(peerCerts)
	if err != nil {
		t.Fatal(err)
	}
	bundleHash := sha256.Sum256(bundleBytes)
	now := time.Now().Unix()
	proof := delegationpkg.NewDelegationProof(
		s.certHash,
		nil,
		s.transcript,
		s.x509FP,
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	exporterCtx, err := canonicalpkg.CanonicalDelegationProofForExporter(
		proof,
	)
	if err != nil {
		t.Fatal(err)
	}
	exporter := deriveTestExporter(exporterCtx)
	proof = delegationpkg.NewDelegationProof(
		s.certHash,
		exporter,
		s.transcript,
		s.x509FP,
		bundleHash[:],
		proof.NotBefore(),
		proof.NotAfter(),
	)
	proofCanonical, err := canonicalpkg.CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatal(err)
	}
	delegationMsg := canonicalpkg.DomainSeparate(
		delegationpkg.CTXNodeDelegationV1,
		proofCanonical,
	)
	delegationSig, err := s.nodeAC.Sign(delegationMsg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := mustVerifyPeerCertWithoutPanic(
		t,
		s.ca,
		PeerHandshake{
			Certs:           peerCerts,
			CASignatures:    caSignatures,
			DelegationProof: proof,
			DelegationSig:   delegationSig,
			TLS: TLSBindings{
				CertPubKeyHash:  s.certHash,
				ExporterBinding: exporter,
				X509Fingerprint: s.x509FP,
				TranscriptHash:  s.transcript,
			},
		},
	)
	if err == nil {
		t.Fatalf(
			"expected oversized bundle to be rejected, got success for node %s",
			ctx.NodeID.String(),
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
	if err := ca.AddAdminPubKey(combined); !errors.Is(err, ErrCAAlreadyExists) {
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
	if !errors.Is(err, ErrCANotFound) {
		t.Errorf("got %v, want ErrCANotFound", err)
	}
}

func TestRevokeUserCA(t *testing.T) { // A
	s := buildScenario(t)

	// Create and add a UserCA.
	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	kem, _ := userPub.MarshalBinaryKEM()
	sign, _ := userPub.MarshalBinarySign()
	combined := append(kem, sign...)
	anchorMsg := canonicalpkg.DomainSeparate(CTXUserCAAnchorV1, combined)
	anchorSig, _ := s.adminAC.Sign(anchorMsg)

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
	if !errors.Is(err, ErrCARevoked) {
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
	_, err = s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    [][]byte{s.caSig},
		DelegationProof: s.proof,
		DelegationSig:   s.delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  s.certHash,
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if !errors.Is(err, ErrNoValidCerts) {
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
	if !errors.Is(err, ErrInvalidAnchorSig) {
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
	if !errors.Is(err, ErrAnchorAdminNotFound) {
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
	userKEM, _ := userPub.MarshalBinaryKEM()
	userBytes := append(userKEM, userSign...)
	anchorMsg := canonicalpkg.DomainSeparate(CTXUserCAAnchorV1, userBytes)
	anchorSig, _ := adminAC.Sign(anchorMsg)
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
	canonical, _ := canonicalpkg.CanonicalNodeCert(cert)
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, _ := userAC.Sign(msg)

	// Build delegation.
	certHashSum := sha256.Sum256([]byte("cert-hash"))
	x509FPSum := sha256.Sum256([]byte("x509-fp"))
	transcriptSum := sha256.Sum256([]byte("transcript"))
	certHash := certHashSum[:]
	x509FP := x509FPSum[:]
	transcript := transcriptSum[:]
	certs := []canonicalpkg.NodeCertLike{cert}
	bundleBytes, _ := canonicalpkg.CanonicalNodeCertBundle(certs)
	bh := sha256.Sum256(bundleBytes)

	proof := delegationpkg.NewDelegationProof(
		certHash, nil, transcript, x509FP,
		bh[:], now-5, now+delegationpkg.MaxDelegationTTL-10,
	)
	expCtx, _ := canonicalpkg.CanonicalDelegationProofForExporter(
		proof,
	)
	exporter := deriveTestExporter(expCtx)
	proof = delegationpkg.NewDelegationProof(
		certHash, exporter, transcript, x509FP,
		bh[:], now-5, now+delegationpkg.MaxDelegationTTL-10,
	)
	delCanon, _ := canonicalpkg.CanonicalDelegationProof(proof)
	delMsg := canonicalpkg.DomainSeparate(
		delegationpkg.CTXNodeDelegationV1, delCanon,
	)
	delSig, _ := nodeAC.Sign(delMsg)

	ctx, err := ca.VerifyPeerCert(PeerHandshake{
		Certs:           certs,
		CASignatures:    [][]byte{caSig},
		DelegationProof: proof,
		DelegationSig:   delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  certHash,
			ExporterBinding: exporter,
			X509Fingerprint: x509FP,
			TranscriptHash:  transcript,
		},
	})
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

func TestUserScopedVerificationWithEmbeddedAuthorities( // A
	t *testing.T,
) {
	ca := NewCarrierAuth(testLogger())

	adminAC, _ := keys.NewAsyncCrypt()
	adminPub := adminAC.GetPublicKey()
	adminSign, _ := adminPub.MarshalBinarySign()
	adminKEM, _ := adminPub.MarshalBinaryKEM()
	adminBytes := append(adminKEM, adminSign...)
	if err := ca.AddAdminPubKey(adminBytes); err != nil {
		t.Fatal(err)
	}
	adminCA, _ := NewAdminCA(adminBytes)

	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	userSign, _ := userPub.MarshalBinarySign()
	userKEM, _ := userPub.MarshalBinaryKEM()
	userBytes := append(userKEM, userSign...)
	anchorMsg := canonicalpkg.DomainSeparate(CTXUserCAAnchorV1, userBytes)
	anchorSig, _ := adminAC.Sign(anchorMsg)
	userCA, _ := newUserCA(
		userBytes,
		anchorSig,
		adminCA.Hash(),
	)

	nodeAC, _ := keys.NewAsyncCrypt()
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
	canonical, _ := canonicalpkg.CanonicalNodeCert(cert)
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, _ := userAC.Sign(msg)

	certHashSum := sha256.Sum256([]byte("cert-hash"))
	x509FPSum := sha256.Sum256([]byte("x509-fp"))
	transcriptSum := sha256.Sum256([]byte("transcript"))
	certHash := certHashSum[:]
	x509FP := x509FPSum[:]
	transcript := transcriptSum[:]
	certs := []canonicalpkg.NodeCertLike{cert}
	bundleBytes, _ := canonicalpkg.CanonicalNodeCertBundle(certs)
	bh := sha256.Sum256(bundleBytes)

	proof := delegationpkg.NewDelegationProof(
		certHash, nil, transcript, x509FP,
		bh[:], now-5, now+delegationpkg.MaxDelegationTTL-10,
	)
	expCtx, _ := canonicalpkg.CanonicalDelegationProofForExporter(proof)
	exporter := deriveTestExporter(expCtx)
	proof = delegationpkg.NewDelegationProof(
		certHash, exporter, transcript, x509FP,
		bh[:], now-5, now+delegationpkg.MaxDelegationTTL-10,
	)
	delCanon, _ := canonicalpkg.CanonicalDelegationProof(proof)
	delMsg := canonicalpkg.DomainSeparate(delegationpkg.CTXNodeDelegationV1, delCanon)
	delSig, _ := nodeAC.Sign(delMsg)

	ctx, err := ca.VerifyPeerCert(PeerHandshake{
		Certs:        certs,
		CASignatures: [][]byte{caSig},
		Authorities: []EmbeddedCA{
			{
				Type:    "admin-ca",
				PubKEM:  adminKEM,
				PubSign: adminSign,
			},
			{
				Type:        "user-ca",
				PubKEM:      userKEM,
				PubSign:     userSign,
				AnchorSig:   anchorSig,
				AnchorAdmin: adminCA.Hash(),
			},
		},
		DelegationProof: proof,
		DelegationSig:   delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  certHash,
			ExporterBinding: exporter,
			X509Fingerprint: x509FP,
			TranscriptHash:  transcript,
		},
	})
	if err != nil {
		t.Fatalf("VerifyPeerCert: %v", err)
	}
	if ctx.EffectiveScope != ScopeUser {
		t.Errorf("scope = %v, want ScopeUser", ctx.EffectiveScope)
	}
	if len(ctx.AllowedUserCAOwners) != 1 {
		t.Fatalf("want 1 owner, got %d", len(ctx.AllowedUserCAOwners))
	}
	if ctx.AllowedUserCAOwners[0] != userCA.Hash() {
		t.Error("wrong UserCA owner hash")
	}
}

func TestVerifyPeerCertRejectsDifferentRootSignature( // A
	t *testing.T,
) {
	trustedCA := NewCarrierAuth(testLogger())

	rootAC1, _ := keys.NewAsyncCrypt()
	rootPub1 := rootAC1.GetPublicKey()
	rootKEM1, _ := rootPub1.MarshalBinaryKEM()
	rootSign1, _ := rootPub1.MarshalBinarySign()
	rootBytes1 := append(rootKEM1, rootSign1...)
	if err := trustedCA.AddAdminPubKey(rootBytes1); err != nil {
		t.Fatal(err)
	}
	rootCA1, _ := NewAdminCA(rootBytes1)

	rootAC2, _ := keys.NewAsyncCrypt()
	nodeAC, _ := keys.NewAsyncCrypt()
	nodePub := nodeAC.GetPublicKey()
	now := time.Now().Unix()
	cert, _ := NewNodeCert(
		nodePub,
		rootCA1.Hash(),
		now-10,
		now+600,
		[]byte("serial-root-mismatch"),
		[]byte("nonce-root-mismatch"),
	)
	canonical, _ := canonicalpkg.CanonicalNodeCert(cert)
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	wrongSig, _ := rootAC2.Sign(msg)

	certHashSum := sha256.Sum256([]byte("cert-hash-root-mismatch"))
	x509FPSum := sha256.Sum256([]byte("x509-fp-root-mismatch"))
	transcriptSum := sha256.Sum256([]byte("transcript-root-mismatch"))
	certHash := certHashSum[:]
	x509FP := x509FPSum[:]
	transcript := transcriptSum[:]
	bundleBytes, _ := canonicalpkg.CanonicalNodeCertBundle([]canonicalpkg.NodeCertLike{cert})
	bundleHash := sha256.Sum256(bundleBytes)
	proof := delegationpkg.NewDelegationProof(
		certHash,
		nil,
		transcript,
		x509FP,
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	expCtx, _ := canonicalpkg.CanonicalDelegationProofForExporter(proof)
	exporter := deriveTestExporter(expCtx)
	proof = delegationpkg.NewDelegationProof(
		certHash,
		exporter,
		transcript,
		x509FP,
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	delCanon, _ := canonicalpkg.CanonicalDelegationProof(proof)
	delMsg := canonicalpkg.DomainSeparate(delegationpkg.CTXNodeDelegationV1, delCanon)
	delSig, _ := nodeAC.Sign(delMsg)

	_, err := trustedCA.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{cert},
		CASignatures:    [][]byte{wrongSig},
		DelegationProof: proof,
		DelegationSig:   delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  certHash,
			ExporterBinding: exporter,
			X509Fingerprint: x509FP,
			TranscriptHash:  transcript,
		},
	})
	if !errors.Is(err, ErrNoValidCerts) {
		t.Fatalf("got %v, want ErrNoValidCerts", err)
	}
}

func TestVerifyPeerCertRejectsEmbeddedUserBrokenAnchorSig( // A
	t *testing.T,
) {
	ca := NewCarrierAuth(testLogger())

	adminAC, _ := keys.NewAsyncCrypt()
	adminPub := adminAC.GetPublicKey()
	adminKEM, _ := adminPub.MarshalBinaryKEM()
	adminSign, _ := adminPub.MarshalBinarySign()
	adminBytes := append(adminKEM, adminSign...)
	if err := ca.AddAdminPubKey(adminBytes); err != nil {
		t.Fatal(err)
	}
	adminCA, _ := NewAdminCA(adminBytes)

	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	userKEM, _ := userPub.MarshalBinaryKEM()
	userSign, _ := userPub.MarshalBinarySign()
	userBytes := append(userKEM, userSign...)
	userCA, _ := newUserCA(userBytes, []byte("ignored"), adminCA.Hash())

	nodeAC, _ := keys.NewAsyncCrypt()
	nodePub := nodeAC.GetPublicKey()
	now := time.Now().Unix()
	cert, _ := NewNodeCert(
		nodePub,
		userCA.Hash(),
		now-10,
		now+600,
		[]byte("serial-bad-anchor"),
		[]byte("nonce-bad-anchor"),
	)
	canonical, _ := canonicalpkg.CanonicalNodeCert(cert)
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, _ := userAC.Sign(msg)

	certHashSum := sha256.Sum256([]byte("cert-hash-bad-anchor"))
	x509FPSum := sha256.Sum256([]byte("x509-fp-bad-anchor"))
	transcriptSum := sha256.Sum256([]byte("transcript-bad-anchor"))
	certHash := certHashSum[:]
	x509FP := x509FPSum[:]
	transcript := transcriptSum[:]
	bundleBytes, _ := canonicalpkg.CanonicalNodeCertBundle([]canonicalpkg.NodeCertLike{cert})
	bundleHash := sha256.Sum256(bundleBytes)
	proof := delegationpkg.NewDelegationProof(
		certHash,
		nil,
		transcript,
		x509FP,
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	expCtx, _ := canonicalpkg.CanonicalDelegationProofForExporter(proof)
	exporter := deriveTestExporter(expCtx)
	proof = delegationpkg.NewDelegationProof(
		certHash,
		exporter,
		transcript,
		x509FP,
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	delCanon, _ := canonicalpkg.CanonicalDelegationProof(proof)
	delMsg := canonicalpkg.DomainSeparate(delegationpkg.CTXNodeDelegationV1, delCanon)
	delSig, _ := nodeAC.Sign(delMsg)

	_, err := ca.VerifyPeerCert(PeerHandshake{
		Certs:        []canonicalpkg.NodeCertLike{cert},
		CASignatures: [][]byte{caSig},
		Authorities: []EmbeddedCA{
			{
				Type:    "admin-ca",
				PubKEM:  adminKEM,
				PubSign: adminSign,
			},
			{
				Type:        "user-ca",
				PubKEM:      userKEM,
				PubSign:     userSign,
				AnchorSig:   []byte("broken-anchor-sig"),
				AnchorAdmin: adminCA.Hash(),
			},
		},
		DelegationProof: proof,
		DelegationSig:   delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  certHash,
			ExporterBinding: exporter,
			X509Fingerprint: x509FP,
			TranscriptHash:  transcript,
		},
	})
	if !errors.Is(err, ErrInvalidAnchorSig) {
		t.Fatalf("got %v, want ErrInvalidAnchorSig", err)
	}
}

func TestVerifyPeerCertRejectsEmbeddedDifferentRoot( // A
	t *testing.T,
) {
	ca := NewCarrierAuth(testLogger())

	trustedRootAC, _ := keys.NewAsyncCrypt()
	trustedRootPub := trustedRootAC.GetPublicKey()
	trustedRootKEM, _ := trustedRootPub.MarshalBinaryKEM()
	trustedRootSign, _ := trustedRootPub.MarshalBinarySign()
	trustedRootBytes := append(trustedRootKEM, trustedRootSign...)
	if err := ca.AddAdminPubKey(trustedRootBytes); err != nil {
		t.Fatal(err)
	}

	otherRootAC, _ := keys.NewAsyncCrypt()
	otherRootPub := otherRootAC.GetPublicKey()
	otherRootKEM, _ := otherRootPub.MarshalBinaryKEM()
	otherRootSign, _ := otherRootPub.MarshalBinarySign()
	otherRootBytes := append(otherRootKEM, otherRootSign...)
	otherRootCA, _ := NewAdminCA(otherRootBytes)

	userAC, _ := keys.NewAsyncCrypt()
	userPub := userAC.GetPublicKey()
	userKEM, _ := userPub.MarshalBinaryKEM()
	userSign, _ := userPub.MarshalBinarySign()
	userBytes := append(userKEM, userSign...)
	anchorMsg := canonicalpkg.DomainSeparate(CTXUserCAAnchorV1, userBytes)
	anchorSig, _ := otherRootAC.Sign(anchorMsg)
	userCA, _ := newUserCA(userBytes, anchorSig, otherRootCA.Hash())

	nodeAC, _ := keys.NewAsyncCrypt()
	nodePub := nodeAC.GetPublicKey()
	now := time.Now().Unix()
	cert, _ := NewNodeCert(
		nodePub,
		userCA.Hash(),
		now-10,
		now+600,
		[]byte("serial-different-root"),
		[]byte("nonce-different-root"),
	)
	canonical, _ := canonicalpkg.CanonicalNodeCert(cert)
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, _ := userAC.Sign(msg)

	certHashSum := sha256.Sum256([]byte("cert-hash-different-root"))
	x509FPSum := sha256.Sum256([]byte("x509-fp-different-root"))
	transcriptSum := sha256.Sum256([]byte("transcript-different-root"))
	certHash := certHashSum[:]
	x509FP := x509FPSum[:]
	transcript := transcriptSum[:]
	bundleBytes, _ := canonicalpkg.CanonicalNodeCertBundle([]canonicalpkg.NodeCertLike{cert})
	bundleHash := sha256.Sum256(bundleBytes)
	proof := delegationpkg.NewDelegationProof(
		certHash,
		nil,
		transcript,
		x509FP,
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	expCtx, _ := canonicalpkg.CanonicalDelegationProofForExporter(proof)
	exporter := deriveTestExporter(expCtx)
	proof = delegationpkg.NewDelegationProof(
		certHash,
		exporter,
		transcript,
		x509FP,
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	delCanon, _ := canonicalpkg.CanonicalDelegationProof(proof)
	delMsg := canonicalpkg.DomainSeparate(delegationpkg.CTXNodeDelegationV1, delCanon)
	delSig, _ := nodeAC.Sign(delMsg)

	_, err := ca.VerifyPeerCert(PeerHandshake{
		Certs:        []canonicalpkg.NodeCertLike{cert},
		CASignatures: [][]byte{caSig},
		Authorities: []EmbeddedCA{
			{
				Type:    "admin-ca",
				PubKEM:  otherRootKEM,
				PubSign: otherRootSign,
			},
			{
				Type:        "user-ca",
				PubKEM:      userKEM,
				PubSign:     userSign,
				AnchorSig:   anchorSig,
				AnchorAdmin: otherRootCA.Hash(),
			},
		},
		DelegationProof: proof,
		DelegationSig:   delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  certHash,
			ExporterBinding: exporter,
			X509Fingerprint: x509FP,
			TranscriptHash:  transcript,
		},
	})
	if !errors.Is(err, ErrUnknownIssuer) {
		t.Fatalf("got %v, want ErrUnknownIssuer", err)
	}
}

func TestDelegationTTLTooLong(t *testing.T) { // A
	s := buildScenario(t)

	now := time.Now().Unix()
	longProof := delegationpkg.NewDelegationProof(
		s.proof.TLSCertPubKeyHash(),
		s.proof.TLSExporterBinding(),
		s.proof.TLSTranscriptHash(),
		s.proof.X509Fingerprint(),
		s.proof.NodeCertBundleHash(),
		now-10, now+delegationpkg.MaxDelegationTTL+100,
	)

	canon, _ := canonicalpkg.CanonicalDelegationProof(longProof)
	msg := canonicalpkg.DomainSeparate(delegationpkg.CTXNodeDelegationV1, canon)
	sig, _ := s.nodeAC.Sign(msg)

	_, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    [][]byte{s.caSig},
		DelegationProof: longProof,
		DelegationSig:   sig,
		TLS: TLSBindings{
			CertPubKeyHash:  s.certHash,
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if !errors.Is(err, ErrDelegationTooLong) {
		t.Errorf(
			"got %v, want ErrDelegationTooLong", err,
		)
	}
}

func TestVerifyPeerCertSignatureCountMismatch( // A
	t *testing.T,
) {
	s := buildScenario(t)

	_, err := s.ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{s.cert},
		CASignatures:    nil,
		DelegationProof: s.proof,
		DelegationSig:   s.delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  s.certHash,
			ExporterBinding: s.exporter,
			X509Fingerprint: s.x509FP,
			TranscriptHash:  s.transcript,
		},
	})
	if !errors.Is(err, ErrSignatureCountMismatch) {
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
	userKEM, _ := userPub.MarshalBinaryKEM()
	userBytes := append(userKEM, userSign...)
	anchorMsg := canonicalpkg.DomainSeparate(CTXUserCAAnchorV1, userBytes)
	anchorSig, _ := adminAC.Sign(anchorMsg)
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
	canonical, _ := canonicalpkg.CanonicalNodeCert(cert)
	certMsg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	caSig, _ := userAC.Sign(certMsg)

	bundleBytes, _ := canonicalpkg.CanonicalNodeCertBundle(
		[]canonicalpkg.NodeCertLike{cert},
	)
	bundleHash := sha256.Sum256(bundleBytes)
	proof := delegationpkg.NewDelegationProof(
		[]byte("cert-hash"),
		nil,
		[]byte("transcript"),
		[]byte("x509-fp"),
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	expCtx, _ := canonicalpkg.CanonicalDelegationProofForExporter(
		proof,
	)
	exporter := deriveTestExporter(expCtx)
	proof = delegationpkg.NewDelegationProof(
		[]byte("cert-hash"),
		exporter,
		[]byte("transcript"),
		[]byte("x509-fp"),
		bundleHash[:],
		now-5,
		now+delegationpkg.MaxDelegationTTL-10,
	)
	proofCanon, _ := canonicalpkg.CanonicalDelegationProof(proof)
	proofMsg := canonicalpkg.DomainSeparate(
		delegationpkg.CTXNodeDelegationV1,
		proofCanon,
	)
	delSig, _ := nodeAC.Sign(proofMsg)

	_, err := ca.VerifyPeerCert(PeerHandshake{
		Certs:           []canonicalpkg.NodeCertLike{cert},
		CASignatures:    [][]byte{caSig},
		DelegationProof: proof,
		DelegationSig:   delSig,
		TLS: TLSBindings{
			CertPubKeyHash:  []byte("cert-hash"),
			ExporterBinding: exporter,
			X509Fingerprint: []byte("x509-fp"),
			TranscriptHash:  []byte("transcript"),
		},
	})
	if !errors.Is(err, ErrNoValidCerts) {
		t.Errorf("got %v, want ErrNoValidCerts", err)
	}
}

func TestAddUserPubKeyRejectsKEMSubstitution( // A
	t *testing.T,
) {
	s := buildScenario(t)

	userAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatal(err)
	}
	userPub := userAC.GetPublicKey()
	userKEM, err := userPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatal(err)
	}
	userSign, err := userPub.MarshalBinarySign()
	if err != nil {
		t.Fatal(err)
	}
	anchoredBytes := append(userKEM, userSign...)
	anchorMsg := canonicalpkg.DomainSeparate(
		CTXUserCAAnchorV1,
		anchoredBytes,
	)
	anchorSig, err := s.adminAC.Sign(anchorMsg)
	if err != nil {
		t.Fatal(err)
	}

	attackerAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatal(err)
	}
	attackerPub := attackerAC.GetPublicKey()
	attackerKEM, err := attackerPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatal(err)
	}
	substitutedBytes := append(attackerKEM, userSign...)

	err = s.ca.AddUserPubKey(
		substitutedBytes,
		anchorSig,
		s.adminCA.Hash(),
	)
	if !errors.Is(err, ErrInvalidAnchorSig) {
		t.Fatalf(
			"got %v, want ErrInvalidAnchorSig",
			err,
		)
	}
}
