package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// carrierAuth implements interfaces.CarrierAuth.
// LOGGER: Uses *slog.Logger directly because
// pkg/auth -> pkg/clusterlog -> pkg/interfaces ->
// pkg/auth would create a circular import.
type carrierAuth struct { // A
	logger          *slog.Logger
	mu              sync.RWMutex
	adminCAs        map[string]*AdminCAImpl
	userCAs         map[string]*UserCAImpl
	revokedAdminCAs map[string]struct{}
	revokedUserCAs  map[string]struct{}
	revokedNodes    map[keys.NodeID]struct{}
}

// NewCarrierAuth creates a new CarrierAuth instance.
func NewCarrierAuth( // A
	logger *slog.Logger,
) *carrierAuth {
	return &carrierAuth{
		logger:          logger,
		adminCAs:        make(map[string]*AdminCAImpl),
		userCAs:         make(map[string]*UserCAImpl),
		revokedAdminCAs: make(map[string]struct{}),
		revokedUserCAs:  make(map[string]struct{}),
		revokedNodes:    make(map[keys.NodeID]struct{}),
	}
}

// authResult holds intermediate verification state.
type authResult struct { // A
	nodePubKey *keys.PublicKey
	nodeID     keys.NodeID
	adminValid bool
	userHashes []string
}

// VerifyPeerCert validates all certs in the bundle
// and derives effective authorization. Each step is a
// separate method to stay under complexity limits.
func (ca *carrierAuth) VerifyPeerCert( // A
	peerCerts []NodeCertLike,
	caSignatures [][]byte,
	delegationProof DelegationProofLike,
	delegationSig []byte,
	tlsCertPubKeyHash []byte,
	tlsExporterBinding []byte,
	tlsX509Fingerprint []byte,
	tlsTranscriptHash []byte,
) (AuthContext, error) {
	if len(peerCerts) == 0 {
		return AuthContext{}, ErrNoCerts
	}
	if len(caSignatures) != len(peerCerts) {
		return AuthContext{}, ErrSignatureCountMismatch
	}
	nowUnix := time.Now().Unix()
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	result, err := ca.verifyChain(
		peerCerts, caSignatures, nowUnix,
	)
	if err != nil {
		return AuthContext{}, err
	}
	err = ca.verifyDelegation(
		result, delegationProof, delegationSig,
		peerCerts, tlsCertPubKeyHash,
		tlsX509Fingerprint,
	)
	if err != nil {
		return AuthContext{}, err
	}
	err = ca.verifyFreshness(
		delegationProof, tlsExporterBinding,
		tlsTranscriptHash, nowUnix,
	)
	if err != nil {
		return AuthContext{}, err
	}
	return ca.deriveScope(result), nil
}

// verifyChain performs checks 1-3: issuer discovery,
// validity/revocation filtering, authority verification.
func (ca *carrierAuth) verifyChain( // A
	certs []NodeCertLike,
	sigs [][]byte,
	nowUnix int64,
) (*authResult, error) {
	validIdxs, err := ca.filterValidCerts(certs, nowUnix)
	if err != nil {
		return nil, err
	}
	return ca.verifyAuthority(
		certs, sigs, validIdxs,
	)
}

// issuerType tracks whether a cert was issued by an
// Admin or User CA.
type issuerType int // A

const ( // A
	issuerAdmin issuerType = iota
	issuerUser
)

// issuerInfo holds the CA reference for a cert.
type issuerInfo struct { // A
	typ     issuerType
	adminCA *AdminCAImpl
	userCA  *UserCAImpl
}

// lookupIssuer finds the CA for a cert's IssuerCAHash.
func (ca *carrierAuth) lookupIssuer( // A
	caHash string,
) (issuerInfo, bool) {
	if admin, ok := ca.adminCAs[caHash]; ok {
		return issuerInfo{
			typ: issuerAdmin, adminCA: admin,
		}, true
	}
	if user, ok := ca.userCAs[caHash]; ok {
		return issuerInfo{
			typ: issuerUser, userCA: user,
		}, true
	}
	return issuerInfo{}, false
}

// filterValidCerts performs checks 1-2: discovers
// issuers and filters by validity/revocation.
func (ca *carrierAuth) filterValidCerts( // A
	certs []NodeCertLike,
	now int64,
) ([]int, error) {
	var valid []int
	for i, cert := range certs {
		if !ca.isCertValid(cert, now) {
			continue
		}
		valid = append(valid, i)
	}
	if len(valid) == 0 {
		return nil, ErrNoValidCerts
	}
	return valid, nil
}

// isCertValid checks a single cert's time window,
// issuer existence, and revocation status.
func (ca *carrierAuth) isCertValid( // A
	cert NodeCertLike,
	now int64,
) bool {
	if now < cert.ValidFrom() || now > cert.ValidUntil() {
		return false
	}
	caHash := cert.IssuerCAHash()
	_, found := ca.lookupIssuer(caHash)
	if !found {
		return false
	}
	if _, revoked := ca.revokedAdminCAs[caHash]; revoked {
		return false
	}
	if _, revoked := ca.revokedUserCAs[caHash]; revoked {
		return false
	}
	if issuer, ok := ca.userCAs[caHash]; ok {
		if _, revoked := ca.revokedAdminCAs[issuer.anchorAdminHash]; revoked {
			return false
		}
		if _, found := ca.adminCAs[issuer.anchorAdminHash]; !found {
			return false
		}
	}
	pubKey := cert.NodePubKey()
	nid, err := pubKey.NodeID()
	if err != nil {
		return false
	}
	if _, revoked := ca.revokedNodes[nid]; revoked {
		return false
	}
	return true
}

// verifyAuthority performs check 3: verifies CA
// signatures and ensures all certs share the same
// NodeID.
func (ca *carrierAuth) verifyAuthority( // A
	certs []NodeCertLike,
	sigs [][]byte,
	validIdxs []int,
) (*authResult, error) {
	var (
		firstNID   keys.NodeID
		firstPub   *keys.PublicKey
		nidSet     bool
		adminValid bool
		userHashes []string
	)
	for _, idx := range validIdxs {
		nid, pub, iTyp, err := ca.verifySingleCert(
			certs[idx], sigs[idx],
		)
		if err != nil {
			ca.logger.WarnContext(
				context.TODO(),
				"certificate authority verification failed",
				LogKeyStep, "verifyAuthority",
				"certIndex", idx,
				LogKeyReason, err.Error(),
			)
			continue
		}
		if !nidSet {
			firstNID = nid
			firstPub = pub
			nidSet = true
		} else if firstNID != nid {
			return nil, ErrMismatchedNodeID
		}
		if iTyp == issuerAdmin {
			adminValid = true
		} else {
			userHashes = append(
				userHashes,
				certs[idx].IssuerCAHash(),
			)
		}
	}
	if !nidSet {
		return nil, ErrNoValidCerts
	}
	return &authResult{
		nodePubKey: firstPub,
		nodeID:     firstNID,
		adminValid: adminValid,
		userHashes: userHashes,
	}, nil
}

// verifySingleCert verifies one cert's CA signature
// and returns its NodeID and issuer type.
func (ca *carrierAuth) verifySingleCert( // A
	cert NodeCertLike,
	sig []byte,
) (keys.NodeID, *keys.PublicKey, issuerType, error) {
	issuer, found := ca.lookupIssuer(cert.IssuerCAHash())
	if !found {
		return keys.NodeID{}, nil, 0, ErrUnknownIssuer
	}
	var (
		nid keys.NodeID
		err error
	)
	if issuer.typ == issuerAdmin {
		nid, err = issuer.adminCA.VerifyNodeCert(cert, sig)
	} else {
		nid, err = issuer.userCA.VerifyNodeCert(cert, sig)
	}
	if err != nil {
		return keys.NodeID{}, nil, 0, err
	}
	pubKey := cert.NodePubKey()
	return nid, &pubKey, issuer.typ, nil
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
	msg := DomainSeparate(CTXNodeDelegationV1, canonical)
	if !result.nodePubKey.Verify(msg, sig) {
		return ErrInvalidDelegationSig
	}
	if !secureEqual(
		proof.TLSCertPubKeyHash(), tlsCertPubKeyHash,
	) {
		return ErrTLSBindingMismatch
	}
	if !secureEqual(
		proof.X509Fingerprint(), tlsX509Fingerprint,
	) {
		return ErrTLSBindingMismatch
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

// verifyFreshness performs check 5: replay/UKS defense.
func (ca *carrierAuth) verifyFreshness( // A
	proof DelegationProofLike,
	tlsExporterBinding []byte,
	tlsTranscriptHash []byte,
	now int64,
) error {
	if now < proof.NotBefore() || now > proof.NotAfter() {
		return ErrDelegationExpired
	}
	ttl := proof.NotAfter() - proof.NotBefore()
	if ttl > MaxDelegationTTL {
		return ErrDelegationTooLong
	}
	if !secureEqual(
		proof.TLSExporterBinding(), tlsExporterBinding,
	) {
		return ErrTLSBindingMismatch
	}
	if !secureEqual(
		proof.TLSTranscriptHash(), tlsTranscriptHash,
	) {
		return ErrTLSBindingMismatch
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
	ca.logger.InfoContext(
		context.TODO(),
		"peer verified",
		LogKeyNodeID, result.nodeID.String(),
		LogKeyScope, ctx.EffectiveScope.String(),
	)
	return ctx
}

// AddAdminPubKey adds an AdminCA to the trust store.
func (ca *carrierAuth) AddAdminPubKey( // A
	pubKey []byte,
) error {
	admin, err := NewAdminCA(pubKey)
	if err != nil {
		return err
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	h := admin.Hash()
	if _, exists := ca.adminCAs[h]; exists {
		return ErrCAAlreadyExists
	}
	if _, revoked := ca.revokedAdminCAs[h]; revoked {
		return ErrCARevoked
	}
	ca.adminCAs[h] = admin
	return nil
}

// AddUserPubKey adds a UserCA after verifying its
// anchor signature against the referenced AdminCA.
func (ca *carrierAuth) AddUserPubKey( // A
	pubKey []byte,
	anchorSig []byte,
	anchorAdminHash string,
) error {
	user, err := newUserCA(
		pubKey, anchorSig, anchorAdminHash,
	)
	if err != nil {
		return err
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	h := user.Hash()
	if _, exists := ca.userCAs[h]; exists {
		return ErrCAAlreadyExists
	}
	if _, revoked := ca.revokedUserCAs[h]; revoked {
		return ErrCARevoked
	}
	err = ca.verifyAnchor(user, anchorAdminHash)
	if err != nil {
		return err
	}
	ca.userCAs[h] = user
	return nil
}

// verifyAnchor checks the anchor signature against
// the referenced AdminCA. Must be called under lock.
func (ca *carrierAuth) verifyAnchor( // A
	user *UserCAImpl,
	anchorAdminHash string,
) error {
	admin, ok := ca.adminCAs[anchorAdminHash]
	if !ok {
		return ErrAnchorAdminNotFound
	}
	if _, rev := ca.revokedAdminCAs[anchorAdminHash]; rev {
		return ErrAnchorAdminRevoked
	}
	signBytes, err := user.pubKey.MarshalBinarySign()
	if err != nil {
		return fmt.Errorf(
			"user CA sign key marshal: %w", err,
		)
	}
	msg := DomainSeparate(CTXUserCAAnchorV1, signBytes)
	if !admin.pubKey.Verify(msg, user.anchorSig) {
		return ErrInvalidAnchorSig
	}
	return nil
}

// secureEqual compares two byte slices in constant
// time and returns true if they are equal.
func secureEqual(a []byte, b []byte) bool { // A
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

// RemoveAdminPubKey removes an AdminCA from the trust
// store.
func (ca *carrierAuth) RemoveAdminPubKey( // A
	pubKeyHash string,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if _, ok := ca.adminCAs[pubKeyHash]; !ok {
		return ErrCANotFound
	}
	delete(ca.adminCAs, pubKeyHash)
	return nil
}

// RemoveUserPubKey removes a UserCA from the trust
// store.
func (ca *carrierAuth) RemoveUserPubKey( // A
	pubKeyHash string,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if _, ok := ca.userCAs[pubKeyHash]; !ok {
		return ErrCANotFound
	}
	delete(ca.userCAs, pubKeyHash)
	return nil
}

// RevokeUserCA marks a UserCA as revoked, preventing
// re-addition.
func (ca *carrierAuth) RevokeUserCA( // A
	userCAHash string,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.revokedUserCAs[userCAHash] = struct{}{}
	delete(ca.userCAs, userCAHash)
	ca.logger.InfoContext(
		context.TODO(),
		"user CA revoked",
		LogKeyCAHash, userCAHash,
	)
	return nil
}

// RevokeAdminCA marks an AdminCA as revoked,
// preventing re-addition and invalidating anchors.
func (ca *carrierAuth) RevokeAdminCA( // A
	adminCAHash string,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.revokedAdminCAs[adminCAHash] = struct{}{}
	delete(ca.adminCAs, adminCAHash)
	ca.logger.InfoContext(
		context.TODO(),
		"admin CA revoked",
		LogKeyCAHash, adminCAHash,
	)
	return nil
}

// RevokeNode marks a node as revoked by its NodeID.
func (ca *carrierAuth) RevokeNode( // A
	nodeID keys.NodeID,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.revokedNodes[nodeID] = struct{}{}
	ca.logger.InfoContext(
		context.TODO(),
		"node revoked",
		LogKeyNodeID, nodeID.String(),
	)
	return nil
}

// AuthContext holds the verified peer authorization.
type AuthContext struct { // A
	NodeID              keys.NodeID
	EffectiveScope      TrustScope
	AllowedUserCAOwners []string
	HasValidAdminCert   bool
}
