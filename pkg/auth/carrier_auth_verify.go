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

// lookupIssuer finds the CA for a cert's IssuerCAHash.
func (ca *carrierAuth) lookupIssuer( // A
	caHash string,
) (issuerInfo, bool) {
	if admin, ok := ca.adminCAs[caHash]; ok {
		return issuerInfo{
			typ: issuerAdmin, verifier: admin,
		}, true
	}
	if user, ok := ca.userCAs[caHash]; ok {
		return issuerInfo{
			typ: issuerUser, verifier: user,
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
	if !ca.isUserCAAnchorValid(caHash) {
		return false
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

// isUserCAAnchorValid checks whether the anchor
// AdminCA for a UserCA issuer is present and not
// revoked. Returns true if the issuer is not a UserCA.
func (ca *carrierAuth) isUserCAAnchorValid( // A
	caHash string,
) bool {
	issuer, ok := ca.userCAs[caHash]
	if !ok {
		return true
	}
	if _, revoked := ca.revokedAdminCAs[issuer.anchorAdminHash]; revoked {
		return false
	}
	_, found := ca.adminCAs[issuer.anchorAdminHash]
	return found
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
				LogKeyCertIndex, idx,
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
	nid, err := issuer.verifier.VerifyNodeCert(cert, sig)
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

// secureEqual compares two byte slices in constant
// time and returns true if they are equal.
func secureEqual(a, b []byte) bool { // A
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
