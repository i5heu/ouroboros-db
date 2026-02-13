package auth

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// CarrierAuth manages trust roots and authenticates
// peers during QUIC/TLS handshakes.
type CarrierAuth struct { // H
	mu           sync.RWMutex
	adminCAs     map[CaHash]*AdminCA
	userCAs      map[CaHash]*UserCA
	revokedNodes map[keys.NodeID]struct{}
	revokedAdmin map[CaHash]struct{}
	revokedUser  map[CaHash]struct{}
}

// NewCarrierAuth creates an empty CarrierAuth with no
// trusted CAs or revocations.
func NewCarrierAuth() *CarrierAuth { // H
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
	}
}

// VerifyPeerCert authenticates a peer by checking
// proof-of-possession, CA signature validity, and
// revocation status. Returns the NodeID and TrustScope.
func (ca *CarrierAuth) VerifyPeerCert( // AP
	peerCert *NodeCert,
	caSignature []byte,
	tlsPeerPubKey *keys.PublicKey,
) (keys.NodeID, TrustScope, error) {
	if err := validateVerifyPeerInputs(
		ca,
		peerCert,
		tlsPeerPubKey,
	); err != nil {
		return keys.NodeID{}, 0, err
	}

	if err := verifyProofOfPossession(
		tlsPeerPubKey,
		peerCert,
	); err != nil {
		return keys.NodeID{}, 0, err
	}

	nodeID, err := peerCert.NodeID()
	if err != nil {
		return keys.NodeID{}, 0, fmt.Errorf(
			"derive node ID: %w", err,
		)
	}

	ca.mu.RLock()
	defer ca.mu.RUnlock()

	if _, ok := ca.revokedNodes[nodeID]; ok {
		return keys.NodeID{}, 0, errors.New(
			"node is revoked",
		)
	}

	issuer := peerCert.IssuerCAHash()

	scope, vErr := ca.tryVerify(
		peerCert, caSignature, issuer,
	)
	if vErr != nil {
		return keys.NodeID{}, 0, vErr
	}

	return nodeID, scope, nil
}

func validateVerifyPeerInputs( // A
	ca *CarrierAuth,
	peerCert *NodeCert,
	tlsPeerPubKey *keys.PublicKey,
) error {
	if ca == nil {
		return errors.New("carrier auth must not be nil")
	}
	if peerCert == nil {
		return errors.New("peer cert must not be nil")
	}
	if tlsPeerPubKey == nil {
		return errors.New("TLS peer public key must not be nil")
	}
	if peerCert.NodePubKey() == nil {
		return errors.New(
			"peer cert node public key must not be nil",
		)
	}
	return nil
}

func verifyProofOfPossession( // A
	tlsPeerPubKey *keys.PublicKey,
	peerCert *NodeCert,
) error {
	tlsSign, err := tlsPeerPubKey.MarshalBinarySign()
	if err != nil {
		return fmt.Errorf(
			"marshal TLS peer sign key: %w", err,
		)
	}

	certSign, err := peerCert.NodePubKey().MarshalBinarySign()
	if err != nil {
		return fmt.Errorf(
			"marshal cert sign key: %w", err,
		)
	}

	if !bytes.Equal(tlsSign, certSign) {
		return errors.New(
			"TLS peer sign key does not match cert",
		)
	}

	return nil
}

// tryVerify attempts verification against admin CAs
// first, then user CAs. Must be called with mu held.
func (ca *CarrierAuth) tryVerify( // AP
	cert *NodeCert,
	sig []byte,
	issuer CaHash,
) (TrustScope, error) {
	if admin, ok := ca.adminCAs[issuer]; ok {
		if _, rev := ca.revokedAdmin[issuer]; !rev {
			_, err := admin.VerifyNodeCert(
				cert, sig,
			)
			if err != nil {
				return 0, err
			}
			return ScopeAdmin, nil
		}
	}

	if user, ok := ca.userCAs[issuer]; ok {
		if _, rev := ca.revokedUser[issuer]; !rev {
			_, err := user.VerifyNodeCert(
				cert, sig,
			)
			if err != nil {
				return 0, err
			}
			return ScopeUser, nil
		}
	}

	return 0, errors.New(
		"no trusted CA found for issuer",
	)
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
func (ca *CarrierAuth) AddUserPubKey( // AP
	pubKey *keys.PublicKey,
) error {
	user, err := NewUserCA(pubKey)
	if err != nil {
		return err
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
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
