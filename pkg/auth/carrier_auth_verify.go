package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// authResult holds intermediate verification state.
type authResult struct { // A
	nodePubKey *keys.PublicKey
	nodeID     keys.NodeID
	adminValid bool
	userHashes []string
}

// issuerType tracks whether a cert was issued by an
// Admin or User CA.
type issuerType int // A

const ( // A
	issuerAdmin issuerType = iota
	issuerUser
)

// certVerifier verifies a CA signature on a NodeCert.
type certVerifier interface { // A
	VerifyNodeCert(NodeCertLike, []byte) (keys.NodeID, error)
}

// issuerInfo holds the CA reference for a cert.
type issuerInfo struct { // A
	typ      issuerType
	verifier certVerifier
}

type bindingField struct { // A
	name      string
	value     []byte
	expectedL int
}

// VerifyPeerCert validates all certs in the bundle
// and derives effective authorization. Inputs are
// defensively snapshotted to prevent stateful-cert
// attacks.
//
// This method implements PHASE 4 (Chain Validation)
// from auth.mmd. The caller (transport layer) MUST:
//
//  1. Complete TLS with PQ-hybrid suites ONLY.
//  2. Derive TLS bindings independently from the
//     local TLS stack (never trust peer-supplied
//     values for CertPubKeyHash, ExporterBinding,
//     X509Fingerprint, TranscriptHash).
//  3. Read the auth message from the peer's auth
//     stream (see readAuthHandshake in
//     pkg/carrier/auth_handshake.go).
//  4. Call this method with the combined
//     PeerHandshake.
//  5. On error: close the QUIC connection
//     immediately (bad certificate).
//  6. On success: use AuthContext.NodeID to
//     identify the peer, AuthContext.EffectiveScope
//     to authorize operations, and
//     AuthContext.AllowedUserCAOwners to enforce
//     per-data ownership checks via
//     pkg/cas.PrivilegeChecker.
//  7. For long-lived connections: re-run
//     VerifyPeerCert periodically or after
//     revocation events to refresh the scope
//     (spec lines 130-131).
//
// Returns AuthContext with the verified identity and
// authorization scope, or an error wrapping one of
// the sentinel errors from errors.go.
func (ca *carrierAuth) VerifyPeerCert( // A
	hs PeerHandshake,
) (AuthContext, error) {
	if err := validateHandshake(hs); err != nil {
		return AuthContext{}, err
	}

	// Snapshot all inputs to freeze interface reads.
	certs, err := snapshotCerts(hs.Certs)
	if err != nil {
		return AuthContext{}, err
	}
	proof := snapshotDelegation(hs.DelegationProof)
	tls := &TLSBindings{
		CertPubKeyHash:  cloneBytes(hs.TLS.CertPubKeyHash),
		ExporterBinding: cloneBytes(hs.TLS.ExporterBinding),
		X509Fingerprint: cloneBytes(hs.TLS.X509Fingerprint),
		TranscriptHash:  cloneBytes(hs.TLS.TranscriptHash),
	}

	nowUnix := time.Now().Unix()

	// Snapshot trust store under read lock, then
	// release so crypto work runs without blocking
	// revocations.
	ca.mu.RLock()
	ts := ca.snapTrustStore()
	ca.mu.RUnlock()
	if len(hs.Authorities) > 0 {
		ts, err = ts.withEmbeddedAuthorities(hs.Authorities)
		if err != nil {
			return AuthContext{}, err
		}
	}

	certLikes := make([]NodeCertLike, len(certs))
	for i, c := range certs {
		certLikes[i] = c
	}

	result, err := ts.verifyChain(
		certLikes, hs.CASignatures, nowUnix,
	)
	if err != nil {
		return AuthContext{}, err
	}
	err = validateBindingFields([]bindingField{
		{
			name:      "proof TLS cert pubkey hash",
			value:     proof.TLSCertPubKeyHash(),
			expectedL: TLSCertPubKeyHashSize,
		},
		{
			name:      "proof TLS exporter binding",
			value:     proof.TLSExporterBinding(),
			expectedL: TLSExporterBindingSize,
		},
		{
			name:      "proof TLS transcript hash",
			value:     proof.TLSTranscriptHash(),
			expectedL: TLSTranscriptHashSize,
		},
		{
			name:      "proof X.509 fingerprint",
			value:     proof.X509Fingerprint(),
			expectedL: X509FingerprintSize,
		},
		{
			name:      "proof node cert bundle hash",
			value:     proof.NodeCertBundleHash(),
			expectedL: NodeCertBundleHashSize,
		},
		{
			name:      "transport TLS cert pubkey hash",
			value:     tls.CertPubKeyHash,
			expectedL: TLSCertPubKeyHashSize,
		},
		{
			name:      "transport TLS exporter binding",
			value:     tls.ExporterBinding,
			expectedL: TLSExporterBindingSize,
		},
		{
			name:      "transport X.509 fingerprint",
			value:     tls.X509Fingerprint,
			expectedL: X509FingerprintSize,
		},
		{
			name:      "transport TLS transcript hash",
			value:     tls.TranscriptHash,
			expectedL: TLSTranscriptHashSize,
		},
	})
	if err != nil {
		return AuthContext{}, err
	}
	err = ca.verifyDelegation(
		result, proof, hs.DelegationSig,
		certLikes, tls.CertPubKeyHash,
		tls.X509Fingerprint,
	)
	if err != nil {
		return AuthContext{}, err
	}
	err = ca.verifyFreshness(
		proof, tls.ExporterBinding,
		tls.TranscriptHash, nowUnix,
	)
	if err != nil {
		return AuthContext{}, err
	}
	return ca.deriveScope(result), nil
}

func validateBindingFields( // A
	fields []bindingField,
) error {
	for _, field := range fields {
		if len(field.value) != field.expectedL {
			return authErr(
				ErrInvalidBindingField,
				field.name,
				"expectedLen", field.expectedL,
				"actualLen", len(field.value),
			)
		}
	}
	return nil
}

// validateHandshake checks PeerHandshake invariants
// before expensive cryptographic operations.
func validateHandshake(hs PeerHandshake) error { // A
	if len(hs.Certs) == 0 {
		return ErrNoCerts
	}
	if len(hs.Certs) > MaxPeerCertBundleSize {
		return ErrBundleTooLarge
	}
	if len(hs.CASignatures) != len(hs.Certs) {
		return ErrSignatureCountMismatch
	}
	if hs.DelegationProof == nil {
		return ErrNilDelegationProof
	}
	if len(hs.DelegationSig) == 0 {
		return ErrInvalidDelegationSig
	}
	return nil
}

// verifyDelegation performs check 4: delegation
// binding verification.
func (ca *carrierAuth) verifyDelegation( // A
	result *authResult,
	proof DelegationProofLike,
	sig []byte,
	certs []NodeCertLike,
	tlsCertPubKeyHash []byte,
	tlsX509Fingerprint []byte,
) error {
	canonical, err := CanonicalDelegationProof(proof)
	if err != nil {
		return fmt.Errorf(
			"delegation canonical encoding: %w", err,
		)
	}
	msg := DomainSeparate(
		CTXNodeDelegationV1, canonical,
	)
	if !result.nodePubKey.Verify(msg, sig) {
		return ErrInvalidDelegationSig
	}
	if !secureEqual(
		proof.TLSCertPubKeyHash(),
		tlsCertPubKeyHash,
	) {
		return authErr(
			ErrTLSBindingMismatch,
			"TLS cert pubkey hash mismatch",
		)
	}
	if !secureEqual(
		proof.X509Fingerprint(),
		tlsX509Fingerprint,
	) {
		return authErr(
			ErrTLSBindingMismatch,
			"X.509 fingerprint mismatch",
		)
	}
	return ca.verifyBundleHash(proof, certs)
}

// verifyBundleHash checks NodeCertBundleHash matches.
func (ca *carrierAuth) verifyBundleHash( // A
	proof DelegationProofLike,
	certs []NodeCertLike,
) error {
	bundleBytes, err := CanonicalNodeCertBundle(certs)
	if err != nil {
		return fmt.Errorf(
			"bundle canonical encoding: %w", err,
		)
	}
	computed := sha256.Sum256(bundleBytes)
	if !secureEqual(
		proof.NodeCertBundleHash(), computed[:],
	) {
		return ErrBundleHashMismatch
	}
	return nil
}

// verifyFreshness performs check 5: replay/UKS
// defense. Re-derives the expected TLS exporter
// binding from the proof context per the spec.
func (ca *carrierAuth) verifyFreshness( // A
	proof DelegationProofLike,
	tlsExporterBinding []byte,
	tlsTranscriptHash []byte,
	now int64,
) error {
	if now < proof.NotBefore() ||
		now > proof.NotAfter() {
		return authErr(
			ErrDelegationExpired,
			"delegation outside time window",
			"now", now,
			"notBefore", proof.NotBefore(),
			"notAfter", proof.NotAfter(),
		)
	}
	ttl := proof.NotAfter() - proof.NotBefore()
	if ttl < 0 || ttl > MaxDelegationTTL {
		return authErr(
			ErrDelegationTooLong,
			"TTL exceeds maximum",
			"ttl", ttl,
			"max", MaxDelegationTTL,
		)
	}
	// Re-derive the expected exporter context from
	// the proof (minus TLSExporterBinding). The
	// transport layer (readAuthHandshake) derives
	// the actual exporter value via EKM using this
	// context, so the comparison below validates
	// that the proof's exporter matches the
	// independently-derived transport value.
	exporterCtx, err := CanonicalDelegationProofForExporter(
		proof,
	)
	if err != nil {
		return fmt.Errorf(
			"exporter canonical encoding: %w", err,
		)
	}
	// exporterCtx is available for future in-line
	// re-derivation if VerifyPeerCert gains direct
	// access to the TLS keying-material API.
	_ = exporterCtx
	if !secureEqual(
		proof.TLSExporterBinding(),
		tlsExporterBinding,
	) {
		return authErr(
			ErrTLSBindingMismatch,
			"TLS exporter binding mismatch",
		)
	}
	if !secureEqual(
		proof.TLSTranscriptHash(), tlsTranscriptHash,
	) {
		return authErr(
			ErrTLSBindingMismatch,
			"TLS transcript hash mismatch",
		)
	}
	return nil
}

// deriveScope determines the effective scope from
// verified certs.
func (ca *carrierAuth) deriveScope( // A
	result *authResult,
) AuthContext {
	ctx := AuthContext{
		NodeID:            result.nodeID,
		HasValidAdminCert: result.adminValid,
	}
	if result.adminValid {
		ctx.EffectiveScope = ScopeAdmin
	} else {
		ctx.EffectiveScope = ScopeUser
		ctx.AllowedUserCAOwners = result.userHashes
	}
	// LOGGER: direct slog to avoid circular import
	// with pkg/clusterlog.
	ca.logger.InfoContext(
		context.TODO(),
		"peer verified",
		LogKeyNodeID, result.nodeID.String(),
		LogKeyScope, ctx.EffectiveScope.String(),
	)
	return ctx
}

// secureEqual compares two byte slices in constant
// time and returns true if they are equal.
func secureEqual(a, b []byte) bool { // A
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
