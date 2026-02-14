package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// CarrierAuth manages trust roots and authenticates
// peers during QUIC/TLS handshakes.
type CarrierAuth struct { // H
	mu              sync.RWMutex
	adminCAs        map[CaHash]*AdminCA
	userCAs         map[CaHash]*UserCA
	revokedNodes    map[keys.NodeID]struct{}
	revokedAdmin    map[CaHash]struct{}
	revokedUser     map[CaHash]struct{}
	nonceCache      *NonceCache
	clock           Clock
	revocationFresh map[CaHash]time.Time
	freshnessTTL    time.Duration
	maxDelegTTL     time.Duration
}

// CarrierAuthConfig holds configuration for creating a
// CarrierAuth.
type CarrierAuthConfig struct { // A
	Clock           Clock
	NonceTTL        time.Duration
	NonceMaxEntries int
	FreshnessTTL    time.Duration
	MaxDelegTTL     time.Duration
}

// NewCarrierAuth creates a CarrierAuth with the given
// configuration.
func NewCarrierAuth( // H
	cfg CarrierAuthConfig,
) *CarrierAuth {
	clk := cfg.Clock
	if clk == nil {
		clk = realClock{}
	}
	nonceTTL := cfg.NonceTTL
	if nonceTTL == 0 {
		nonceTTL = 5 * time.Minute
	}
	nonceMaxEntries := cfg.NonceMaxEntries
	if nonceMaxEntries <= 0 {
		nonceMaxEntries = 10_000
	}
	freshnessTTL := cfg.FreshnessTTL
	if freshnessTTL == 0 {
		freshnessTTL = time.Hour
	}
	maxDelegTTL := cfg.MaxDelegTTL
	if maxDelegTTL == 0 {
		maxDelegTTL = 5 * time.Minute
	}
	return &CarrierAuth{
		adminCAs: make(
			map[CaHash]*AdminCA,
		),
		userCAs: make(
			map[CaHash]*UserCA,
		),
		revokedNodes: make(
			map[keys.NodeID]struct{},
		),
		revokedAdmin: make(
			map[CaHash]struct{},
		),
		revokedUser: make(
			map[CaHash]struct{},
		),
		nonceCache: NewNonceCacheWithMaxEntries(
			nonceTTL,
			clk,
			nonceMaxEntries,
		),
		clock: clk,
		revocationFresh: make(
			map[CaHash]time.Time,
		),
		freshnessTTL: freshnessTTL,
		maxDelegTTL:  maxDelegTTL,
	}
}

// VerifyPeerCertParams holds all inputs for the 6-step
// peer certificate verification pipeline.
type VerifyPeerCertParams struct { // A
	PeerCert           *NodeCert
	CASignature        []byte
	DelegationProof    *DelegationProof
	DelegationSig      []byte
	TLSCertPubKeyHash  [32]byte
	TLSExporterBinding [32]byte
	TLSX509Fingerprint [32]byte
	TLSTranscriptHash  []byte
}

// VerifyPeerCert authenticates a peer through a 6-step
// pipeline: issuer discovery, validity/revocation,
// CA signature, delegation binding, replay defense,
// and role extraction.
func (ca *CarrierAuth) VerifyPeerCert( // PAP
	params VerifyPeerCertParams,
) (keys.NodeID, TrustScope, error) {
	if err := validateVerifyInputs(
		ca, params,
	); err != nil {
		return keys.NodeID{}, 0, err
	}

	ca.mu.RLock()
	defer ca.mu.RUnlock()

	caType, caPubKey, err := ca.step1IssuerDiscovery(
		params.PeerCert.IssuerCAHash(),
	)
	if err != nil {
		return keys.NodeID{}, 0, errors.New("authentication failed")
	}

	if err := ca.step2ValidityRevocation(
		params.PeerCert,
	); err != nil {
		return keys.NodeID{}, 0, errors.New("authentication failed")
	}

	nodeID, err := step3AuthorityVerify(
		caPubKey, params.PeerCert, params.CASignature,
	)
	if err != nil {
		return keys.NodeID{}, 0, errors.New("authentication failed")
	}

	if err := step4DelegationBinding(
		params,
	); err != nil {
		return keys.NodeID{}, 0, errors.New("authentication failed")
	}

	if err := ca.step5ReplayDefense(
		params,
	); err != nil {
		return keys.NodeID{}, 0, errors.New("authentication failed")
	}

	scope, err := step6RoleExtraction(
		params.PeerCert, caType,
	)
	if err != nil {
		return keys.NodeID{}, 0, errors.New("authentication failed")
	}

	return nodeID, scope, nil
}

// caType distinguishes admin from user CAs during
// verification.
type caType int // A

const ( // A
	caTypeAdmin caType = iota
	caTypeUser
)

// step1IssuerDiscovery looks up the issuer CA by hash.
// Returns the CA type and public key. Must be called
// with mu held.
func (ca *CarrierAuth) step1IssuerDiscovery( // PAP
	issuer CaHash,
) (caType, *keys.PublicKey, error) {
	if admin, ok := ca.adminCAs[issuer]; ok {
		return caTypeAdmin, admin.PubKey(), nil
	}
	if user, ok := ca.userCAs[issuer]; ok {
		anchorAdmin := user.AnchorAdminHash()
		if _, ok := ca.adminCAs[anchorAdmin]; !ok {
			return 0, nil, errors.New(
				"user CA anchor admin not in trust store",
			)
		}
		if _, revoked := ca.revokedAdmin[anchorAdmin]; revoked {
			return 0, nil, errors.New(
				"user CA anchor admin is revoked",
			)
		}
		return caTypeUser, user.PubKey(), nil
	}
	return 0, nil, errors.New(
		"no trusted CA found for issuer",
	)
}

// step2ValidityRevocation checks time validity,
// node revocation, CA revocation, and revocation
// freshness. Must be called with mu held.
func (ca *CarrierAuth) step2ValidityRevocation( // A
	cert *NodeCert,
) error {
	now := ca.clock.Now()
	if now.Before(cert.ValidFrom()) {
		return errors.New("cert is not yet valid")
	}
	if now.After(cert.ValidUntil()) {
		return errors.New("cert has expired")
	}

	nodeID, err := cert.NodeID()
	if err != nil {
		return fmt.Errorf("derive node ID: %w", err)
	}
	if _, ok := ca.revokedNodes[nodeID]; ok {
		return errors.New("node is revoked")
	}

	issuer := cert.IssuerCAHash()
	if _, ok := ca.revokedAdmin[issuer]; ok {
		return errors.New("issuer CA is revoked")
	}
	if _, ok := ca.revokedUser[issuer]; ok {
		return errors.New("issuer CA is revoked")
	}

	return ca.checkRevocationFreshness(issuer)
}

// checkRevocationFreshness verifies that revocation
// data for the given CA has been refreshed within
// the freshness TTL. Must be called with mu held.
func (ca *CarrierAuth) checkRevocationFreshness( // A
	issuer CaHash,
) error {
	lastRefresh, ok := ca.revocationFresh[issuer]
	if !ok {
		return errors.New(
			"revocation state not yet refreshed",
		)
	}
	if ca.clock.Now().After(
		lastRefresh.Add(ca.freshnessTTL),
	) {
		return errors.New(
			"revocation state is stale",
		)
	}
	return nil
}

// step3AuthorityVerify verifies the CA signature over
// the NodeCert.
func step3AuthorityVerify( // A
	caPubKey *keys.PublicKey,
	cert *NodeCert,
	caSignature []byte,
) (keys.NodeID, error) {
	return verifyNodeCert(
		caPubKey, cert, caSignature,
	)
}

// step4DelegationBinding verifies that the delegation
// proof is correctly bound to the NodeCert and TLS
// session.
func step4DelegationBinding( // PAP
	params VerifyPeerCertParams,
) error {
	proof := params.DelegationProof
	if proof == nil {
		return errors.New(
			"delegation proof must not be nil",
		)
	}

	payload, err := delegationSigningPayload(proof)
	if err != nil {
		return fmt.Errorf(
			"build delegation payload: %w", err,
		)
	}

	certPub := params.PeerCert.NodePubKey()
	if !certPub.Verify(payload, params.DelegationSig) {
		return errors.New(
			"delegation signature verification failed",
		)
	}

	proofPubKeyHash := proof.TLSCertPubKeyHash()

	if subtle.ConstantTimeCompare(
		proofPubKeyHash[:],
		params.TLSCertPubKeyHash[:],
	) != 1 {
		return errors.New("TLS cert pubkey hash mismatch")
	}

	if proof.X509Fingerprint() != params.TLSX509Fingerprint {
		return errors.New(
			"x509 fingerprint mismatch",
		)
	}

	certHash, err := params.PeerCert.Hash()
	if err != nil {
		return fmt.Errorf(
			"compute cert hash: %w", err,
		)
	}
	if !proof.NodeCertHash().Equal(certHash) {
		return errors.New(
			"node cert hash mismatch",
		)
	}

	return nil
}

// step5ReplayDefense checks delegation lifetime,
// validity window, nonce freshness, and transcript
// hash binding.
func (ca *CarrierAuth) step5ReplayDefense( // PAP
	params VerifyPeerCertParams,
) error {
	proof := params.DelegationProof
	delegTTL := proof.NotAfter().Sub(
		proof.NotBefore(),
	)
	if delegTTL > ca.maxDelegTTL {
		return errors.New(
			"delegation TTL exceeds maximum",
		)
	}

	now := ca.clock.Now()
	if now.After(proof.NotAfter()) {
		return errors.New(
			"delegation proof has expired",
		)
	}
	if now.Before(proof.NotBefore()) {
		return errors.New(
			"delegation proof not yet valid",
		)
	}

	if !ca.nonceCache.RecordNonce(
		proof.HandshakeNonce(),
	) {
		return errors.New("replayed handshake nonce")
	}

	proofExporter := proof.TLSExporterBinding()

	if subtle.ConstantTimeCompare(
		proofExporter[:],
		params.TLSExporterBinding[:],
	) != 1 {
		return errors.New("TLS exporter binding mismatch")
	}

	if len(params.TLSTranscriptHash) == 0 {
		return errors.New(
			"TLS transcript hash must not be empty",
		)
	}

	return nil
}

// step6RoleExtraction extracts and validates the role
// claim, ensuring it matches the CA type.
func step6RoleExtraction( // A
	cert *NodeCert,
	ct caType,
) (TrustScope, error) {
	role := cert.RoleClaims()
	if err := validateRole(role); err != nil {
		return 0, fmt.Errorf(
			"invalid role claim: %w", err,
		)
	}

	switch {
	case ct == caTypeAdmin && role == ScopeAdmin:
		return ScopeAdmin, nil
	case ct == caTypeUser && role == ScopeUser:
		return ScopeUser, nil
	default:
		return 0, errors.New(
			"role claim does not match CA type",
		)
	}
}

// validateVerifyInputs performs nil checks on all
// required VerifyPeerCert parameters.
func validateVerifyInputs( // A
	ca *CarrierAuth,
	params VerifyPeerCertParams,
) error {
	if ca == nil {
		return errors.New(
			"carrier auth must not be nil",
		)
	}
	if params.PeerCert == nil {
		return errors.New(
			"peer cert must not be nil",
		)
	}
	if params.PeerCert.NodePubKey() == nil {
		return errors.New(
			"peer cert node public key must not be nil",
		)
	}
	if params.TLSCertPubKeyHash == [32]byte{} {
		return errors.New("TLS cert pubkey hash must not be zero")
	}
	if params.TLSExporterBinding == [32]byte{} {
		return errors.New("TLS exporter binding must not be zero")
	}
	return nil
}

// RefreshRevocationState marks a CA's revocation data
// as freshly synchronized. Called by external CRL/sync
// mechanisms.
func (ca *CarrierAuth) RefreshRevocationState( // A
	caHash CaHash,
) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.revocationFresh[caHash] = ca.clock.Now()
}

// AddAdminPubKey registers a new admin CA trust root.
func (ca *CarrierAuth) AddAdminPubKey( // AP
	pubKey *keys.PublicKey,
) error {
	admin, err := NewAdminCA(pubKey)
	if err != nil {
		return err
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if _, revoked := ca.revokedAdmin[admin.Hash()]; revoked {
		return fmt.Errorf(
			"admin CA hash is revoked: %s",
			admin.Hash().Hex(),
		)
	}
	ca.adminCAs[admin.Hash()] = admin
	return nil
}

// RemoveAdminPubKey removes an admin CA by its hash.
func (ca *CarrierAuth) RemoveAdminPubKey( // A
	pubKeyHash CaHash,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	delete(ca.adminCAs, pubKeyHash)
	return nil
}

// RevokeAdminCA permanently revokes an admin CA by
// hash.
func (ca *CarrierAuth) RevokeAdminCA( // AP
	adminCAHash CaHash,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.revokedAdmin[adminCAHash] = struct{}{}
	return nil
}

// AddUserPubKey registers a new user CA trust root.
func (ca *CarrierAuth) AddUserPubKey( // PAP
	pubKey *keys.PublicKey,
	anchorSig []byte,
	anchorAdminHash CaHash,
) error {
	if pubKey == nil {
		return errors.New("user CA public key must not be nil")
	}
	if len(anchorSig) == 0 {
		return errors.New("anchor signature must not be empty")
	}
	if anchorAdminHash.IsZero() {
		return errors.New("anchor admin hash must not be zero")
	}

	ca.mu.Lock()
	defer ca.mu.Unlock()

	adminCA, ok := ca.adminCAs[anchorAdminHash]
	if !ok {
		return fmt.Errorf(
			"anchor admin CA %s not found in trust store",
			anchorAdminHash.Hex(),
		)
	}

	if _, revoked := ca.revokedAdmin[anchorAdminHash]; revoked {
		return fmt.Errorf(
			"anchor admin CA %s is revoked",
			anchorAdminHash.Hex(),
		)
	}

	if err := verifyUserCAAnchor(
		adminCA.PubKey(),
		pubKey,
		anchorSig,
	); err != nil {
		return fmt.Errorf(
			"anchor signature verification failed: %w",
			err,
		)
	}

	user, err := NewUserCA(pubKey, anchorSig, anchorAdminHash)
	if err != nil {
		return err
	}

	if _, revoked := ca.revokedUser[user.Hash()]; revoked {
		return fmt.Errorf(
			"user CA hash is revoked: %s",
			user.Hash().Hex(),
		)
	}
	ca.userCAs[user.Hash()] = user
	return nil
}

// RemoveUserPubKey removes a user CA by its hash.
func (ca *CarrierAuth) RemoveUserPubKey( // A
	pubKeyHash CaHash,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	delete(ca.userCAs, pubKeyHash)
	return nil
}

// RevokeUserCA permanently revokes a user CA by hash.
func (ca *CarrierAuth) RevokeUserCA( // AP
	userCAHash CaHash,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.revokedUser[userCAHash] = struct{}{}
	return nil
}

// RevokeNode permanently revokes a node by its ID.
func (ca *CarrierAuth) RevokeNode( // AP
	nodeID keys.NodeID,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.revokedNodes[nodeID] = struct{}{}
	return nil
}
