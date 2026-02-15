package auth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

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

// AuthContext holds the verified peer authorization.
type AuthContext struct { // A
	NodeID              keys.NodeID
	EffectiveScope      TrustScope
	AllowedUserCAOwners []string
	HasValidAdminCert   bool
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
