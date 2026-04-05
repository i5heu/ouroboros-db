package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// trustSnapshot is a shallow copy of the trust store
// captured under RLock. Verification runs against
// this snapshot so the lock can be released before
// expensive cryptographic operations.
type trustSnapshot struct { // A
	adminCAs        map[string]*AdminCAImpl
	userCAs         map[string]*UserCAImpl
	revokedAdminCAs map[string]struct{}
	revokedUserCAs  map[string]struct{}
	revokedNodes    map[keys.NodeID]struct{}
	logger          *slog.Logger
}

// snapTrustStore copies the trust-store maps. Must be
// called with ca.mu held (at least RLock).
func (ca *carrierAuth) snapTrustStore() *trustSnapshot { // A
	admins := make(
		map[string]*AdminCAImpl, len(ca.adminCAs),
	)
	for k, v := range ca.adminCAs {
		admins[k] = v
	}
	users := make(
		map[string]*UserCAImpl, len(ca.userCAs),
	)
	for k, v := range ca.userCAs {
		users[k] = v
	}
	rAdmins := make(
		map[string]struct{}, len(ca.revokedAdminCAs),
	)
	for k, v := range ca.revokedAdminCAs {
		rAdmins[k] = v
	}
	rUsers := make(
		map[string]struct{}, len(ca.revokedUserCAs),
	)
	for k, v := range ca.revokedUserCAs {
		rUsers[k] = v
	}
	rNodes := make(
		map[keys.NodeID]struct{}, len(ca.revokedNodes),
	)
	for k, v := range ca.revokedNodes {
		rNodes[k] = v
	}
	return &trustSnapshot{
		adminCAs:        admins,
		userCAs:         users,
		revokedAdminCAs: rAdmins,
		revokedUserCAs:  rUsers,
		revokedNodes:    rNodes,
		logger:          ca.logger,
	}
}

// verifyChain performs checks 1-3: issuer discovery,
// validity/revocation filtering, authority
// verification.
func (ts *trustSnapshot) verifyChain( // A
	certs []NodeCertLike,
	sigs [][]byte,
	nowUnix int64,
) (*authResult, error) {
	validIdxs, err := ts.filterValidCerts(
		certs, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	return ts.verifyAuthority(
		certs, sigs, validIdxs,
	)
}

// lookupIssuer finds the CA for a cert's
// IssuerCAHash.
func (ts *trustSnapshot) lookupIssuer( // A
	caHash string,
) (issuerInfo, bool) {
	if admin, ok := ts.adminCAs[caHash]; ok {
		return issuerInfo{
			typ: issuerAdmin, verifier: admin,
		}, true
	}
	if user, ok := ts.userCAs[caHash]; ok {
		return issuerInfo{
			typ: issuerUser, verifier: user,
		}, true
	}
	return issuerInfo{}, false
}

// filterValidCerts performs checks 1-2: discovers
// issuers and filters by validity/revocation.
func (ts *trustSnapshot) filterValidCerts( // A
	certs []NodeCertLike,
	now int64,
) ([]int, error) {
	var valid []int
	for i, cert := range certs {
		if !ts.isCertValid(cert, now) {
			continue
		}
		valid = append(valid, i)
	}
	if len(valid) == 0 {
		return nil, ErrNoValidCerts
	}
	return valid, nil
}

// isCARevoked checks whether a CA hash is in either
// admin or user revocation lists.
func (ts *trustSnapshot) isCARevoked( // A
	caHash string,
) bool {
	if _, r := ts.revokedAdminCAs[caHash]; r {
		return true
	}
	_, r := ts.revokedUserCAs[caHash]
	return r
}

// isNodeRevoked checks whether a node is revoked.
func (ts *trustSnapshot) isNodeRevoked( // A
	nid keys.NodeID,
) bool {
	_, r := ts.revokedNodes[nid]
	return r
}

// isCertValid checks a single cert's time window,
// issuer existence, and revocation status.
func (ts *trustSnapshot) isCertValid( // A
	cert NodeCertLike,
	now int64,
) bool {
	if now < cert.ValidFrom() ||
		now > cert.ValidUntil() {
		return false
	}
	caHash := cert.IssuerCAHash()
	if _, found := ts.lookupIssuer(caHash); !found {
		return false
	}
	if ts.isCARevoked(caHash) {
		return false
	}
	if !ts.isUserCAAnchorValid(caHash) {
		return false
	}
	pubKey := cert.NodePubKey()
	nid, err := pubKey.NodeID()
	if err != nil {
		return false
	}
	return !ts.isNodeRevoked(nid)
}

// isUserCAAnchorValid checks whether the anchor
// AdminCA for a UserCA issuer is present and not
// revoked. Returns true if the issuer is not a
// UserCA.
func (ts *trustSnapshot) isUserCAAnchorValid( // A
	caHash string,
) bool {
	issuer, ok := ts.userCAs[caHash]
	if !ok {
		return true
	}
	ah := issuer.anchorAdminHash
	if _, revoked := ts.revokedAdminCAs[ah]; revoked {
		return false
	}
	_, found := ts.adminCAs[ah]
	return found
}

// withEmbeddedAuthorities returns a copy of the
// trust snapshot augmented with peer-supplied public
// CA chain data from the auth handshake.
func (ts *trustSnapshot) withEmbeddedAuthorities( // A
	authorities []EmbeddedCA,
) (*trustSnapshot, error) {
	clone := &trustSnapshot{
		adminCAs:        make(map[string]*AdminCAImpl, len(ts.adminCAs)),
		userCAs:         make(map[string]*UserCAImpl, len(ts.userCAs)),
		revokedAdminCAs: make(map[string]struct{}, len(ts.revokedAdminCAs)),
		revokedUserCAs:  make(map[string]struct{}, len(ts.revokedUserCAs)),
		revokedNodes:    make(map[keys.NodeID]struct{}, len(ts.revokedNodes)),
		logger:          ts.logger,
	}
	for key, value := range ts.adminCAs {
		clone.adminCAs[key] = value
	}
	for key, value := range ts.userCAs {
		clone.userCAs[key] = value
	}
	for key, value := range ts.revokedAdminCAs {
		clone.revokedAdminCAs[key] = value
	}
	for key, value := range ts.revokedUserCAs {
		clone.revokedUserCAs[key] = value
	}
	for key, value := range ts.revokedNodes {
		clone.revokedNodes[key] = value
	}
	for _, authority := range authorities {
		if authority.Type != "admin-ca" {
			continue
		}
		if err := clone.addEmbeddedAdmin(authority); err != nil {
			return nil, err
		}
	}
	for _, authority := range authorities {
		if authority.Type != "user-ca" {
			continue
		}
		if err := clone.addEmbeddedUser(authority); err != nil {
			return nil, err
		}
	}
	return clone, nil
}

func (ts *trustSnapshot) addEmbeddedAdmin( // A
	authority EmbeddedCA,
) error {
	pubBytes, err := embeddedAuthorityPubKeyBytes(authority)
	if err != nil {
		return err
	}
	admin, err := NewAdminCA(pubBytes)
	if err != nil {
		return err
	}
	hash := admin.Hash()
	if _, revoked := ts.revokedAdminCAs[hash]; revoked {
		return ErrCARevoked
	}
	if _, exists := ts.adminCAs[hash]; exists {
		return nil
	}
	ts.adminCAs[hash] = admin
	return nil
}

func (ts *trustSnapshot) addEmbeddedUser( // A
	authority EmbeddedCA,
) error {
	pubBytes, err := embeddedAuthorityPubKeyBytes(authority)
	if err != nil {
		return err
	}
	user, err := newUserCA(
		pubBytes,
		authority.AnchorSig,
		authority.AnchorAdmin,
	)
	if err != nil {
		return err
	}
	hash := user.Hash()
	if _, revoked := ts.revokedUserCAs[hash]; revoked {
		return ErrCARevoked
	}
	if _, exists := ts.userCAs[hash]; exists {
		return nil
	}
	admin, ok := ts.adminCAs[authority.AnchorAdmin]
	if !ok {
		return ErrAnchorAdminNotFound
	}
	if _, revoked := ts.revokedAdminCAs[authority.AnchorAdmin]; revoked {
		return ErrAnchorAdminRevoked
	}
	msg := DomainSeparate(CTXUserCAAnchorV1, pubBytes)
	if !admin.pubKey.Verify(msg, authority.AnchorSig) {
		return ErrInvalidAnchorSig
	}
	ts.userCAs[hash] = user
	return nil
}

func embeddedAuthorityPubKeyBytes( // A
	authority EmbeddedCA,
) ([]byte, error) {
	pub, err := keys.NewPublicKeyFromBinary(
		authority.PubKEM,
		authority.PubSign,
	)
	if err != nil {
		return nil, fmt.Errorf("parse embedded authority: %w", err)
	}
	pubBytes, err := marshalPubKeyBytes(pub)
	if err != nil {
		return nil, fmt.Errorf("marshal embedded authority: %w", err)
	}
	return pubBytes, nil
}

// verifyAuthority performs check 3: verifies CA
// signatures and ensures all certs share the same
// NodeID.
func (ts *trustSnapshot) verifyAuthority( // A
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
		nid, pub, iTyp, err := ts.verifySingleCert(
			certs[idx], sigs[idx],
		)
		if err != nil {
			// LOGGER: direct slog to avoid circular
			// import with pkg/clusterlog.
			ts.logger.WarnContext(
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
			return nil, authErr(
				ErrMismatchedNodeID,
				"certs bind to different nodes",
				LogKeyCertIndex, idx,
			)
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
func (ts *trustSnapshot) verifySingleCert( // A
	cert NodeCertLike,
	sig []byte,
) (keys.NodeID, *keys.PublicKey, issuerType, error) {
	issuer, found := ts.lookupIssuer(
		cert.IssuerCAHash(),
	)
	if !found {
		return keys.NodeID{}, nil, 0,
			ErrUnknownIssuer
	}
	nid, err := issuer.verifier.VerifyNodeCert(
		cert, sig,
	)
	if err != nil {
		return keys.NodeID{}, nil, 0, err
	}
	pubKey := cert.NodePubKey()
	pubKeyNID, err := pubKey.NodeID()
	if err != nil {
		return keys.NodeID{}, nil, 0, fmt.Errorf(
			"node ID derivation from pubkey: %w", err,
		)
	}
	if nid != pubKeyNID {
		return keys.NodeID{}, nil, 0,
			ErrMismatchedNodeID
	}
	return nid, &pubKey, issuer.typ, nil
}
