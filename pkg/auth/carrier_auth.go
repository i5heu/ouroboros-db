package auth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// carrierAuth implements interfaces.CarrierAuth.
// Compile-time checks live in pkg/interfaces/auth.go
// to avoid circular imports.
// LOGGER: Uses *slog.Logger directly because
// pkg/auth -> pkg/clusterlog -> pkg/interfaces ->
// pkg/auth would create a circular import.
//
// ─── HOW TO USE pkg/auth FOR NODE AUTHENTICATION ───
//
// This package provides the VERIFIER side of the
// auth.mmd 4-phase protocol. A complete integration
// requires the transport layer to supply the missing
// pieces. The steps below describe what each layer
// must do.
//
// ┌──────────────────────────────────────────────────┐
// │  PHASE 1: IDENTITY BOOTSTRAP (offline/setup)    │
// └──────────────────────────────────────────────────┘
//
//  1. Generate a persistent ML-DSA-87 key pair for
//     each node using ouroboros-crypt:
//
//     ac, _ := keys.NewAsyncCrypt()
//     nodePub := ac.GetPublicKey()
//     // persist ac (private key) securely
//
//  2. Submit nodePub to one or more issuing CAs.
//     Each CA produces a signed NodeCert:
//
//     cert, _ := auth.NewNodeCert(
//     nodePub, caHash,
//     validFrom, validUntil,
//     serial, nonce,
//     )
//     canonical, _ := auth.CanonicalNodeCert(cert)
//     msg := auth.DomainSeparate(
//     auth.CTXNodeAdmissionV1, canonical,
//     )
//     caSig, _ := caPrivKey.Sign(msg)
//
//  3. On the VERIFIER node, populate the trust store
//     BEFORE accepting connections:
//
//     ca := auth.NewCarrierAuth(logger)
//     ca.AddAdminPubKey(adminPubKeyBytes)
//     // optional: ca.AddUserPubKey(...)
//     // optional: ca.SetRevocationHook(hook)
//
// ┌──────────────────────────────────────────────────┐
// │  PHASE 2: SESSION DELEGATION (node startup)     │
// └──────────────────────────────────────────────────┘
//
// NOT YET IMPLEMENTED — transport layer must do this.
//
//  4. The PROVER node generates an ephemeral session
//     key pair (X25519Kyber768 hybrid PQ) and creates
//     a short-lived self-signed X.509 certificate
//     containing the session public key.
//
//  5. The node configures the QUIC/TLS stack to use
//     this X.509 cert with PQ-hybrid cipher suites
//     ONLY. No downgrade is allowed.
//
// ┌──────────────────────────────────────────────────┐
// │  PHASE 3: SECURE CONNECTION (runtime)           │
// └──────────────────────────────────────────────────┘
//
// NOT YET IMPLEMENTED — transport layer must do this.
//
//  6. Prover dials verifier; QUIC/TLS handshake runs.
//     After TLS Finished, the prover extracts four
//     binding values from the TLS stack:
//
//     tlsBindings := auth.TLSBindings{
//     CertPubKeyHash:  sha256(TLS cert SPKI),
//     ExporterBinding: tls.ExportKeyingMaterial(
//     auth.ExporterLabel, exporterCtx, 32,
//     ),
//     X509Fingerprint: sha256(x509 DER),
//     TranscriptHash:  tls handshake hash,
//     }
//
//     Connection.TLSBindings() must return these.
//
// 7. Prover builds and signs a DelegationProof:
//
//		bundleHash := sha256(
//		    auth.CanonicalNodeCertBundle(certs),
//		)
//		proof := auth.NewDelegationProof(
//		    certPubKeyHash, nil, transcriptHash,
//		    x509FP, bundleHash[:],
//		    now, now+300,
//		)
//		// Derive exporter context from proof-without
//		// -exporter:
//		expCtx, _ := auth.CanonicalDelegationProofForExporter(proof)
//		exporter := tls.ExportKeyingMaterial(
//		    auth.ExporterLabel, expCtx, 32,
//		)
//		// Rebuild proof WITH exporter:
//		proof = auth.NewDelegationProof(
//		    certPubKeyHash, exporter,
//		    transcriptHash, x509FP,
//		    bundleHash[:], now, now+300,
//		)
//		delCanon, _ := auth.CanonicalDelegationProof(proof)
//		delMsg := auth.DomainSeparate(
//		    auth.CTXNodeDelegationV1, delCanon,
//		)
//		delSig, _ := nodePrivKey.Sign(delMsg)
//
//	 8. Prover sends over a QUIC auth stream:
//	    [4-byte big-endian length][CBOR wireAuthMessage]
//	    containing: certs, CA signatures, proof, delSig.
//	    (see pkg/carrier/auth_handshake.go for the
//	    receiver side; a writeAuthHandshake() must be
//	    implemented to mirror it)
//
// ┌──────────────────────────────────────────────────┐
// │  PHASE 4: CHAIN VALIDATION (verifier side)      │
// └──────────────────────────────────────────────────┘
//
// THIS IS THE PART pkg/auth IMPLEMENTS TODAY.
//
//  9. Verifier receives the auth message (see
//     readAuthHandshake in pkg/carrier), combines it
//     with TLS bindings from the connection, and
//     calls:
//
//     authCtx, err := ca.VerifyPeerCert(
//     auth.PeerHandshake{
//     Certs:           certs,
//     CASignatures:    sigs,
//     DelegationProof: proof,
//     DelegationSig:   delSig,
//     TLS:             conn.TLSBindings(),
//     },
//     )
//
//     VerifyPeerCert runs CHECKs 1-5 from auth.mmd:
//     issuer discovery, validity/revocation, CA sig
//     verification, delegation binding, and freshness.
//
// 10. On success authCtx contains:
//   - NodeID of the authenticated peer
//   - EffectiveScope (ScopeAdmin or ScopeUser)
//   - AllowedUserCAOwners (for data ACL checks)
//     Register the connection and scope; reject on
//     any error by closing the QUIC connection.
//
// ┌──────────────────────────────────────────────────┐
// │  WHAT IS MISSING (transport layer TODOs)         │
// └──────────────────────────────────────────────────┘
//
//   - QuicTransport implementation with PQ-hybrid TLS
//   - Connection.TLSBindings() wired to real TLS stack
//   - Prover-side writeAuthHandshake()
//   - JoinCluster: dial → TLS → send auth → register
//   - Periodic re-auth / scope refresh on long-lived
//     connections (spec lines 130-131)
//   - Cluster-wide revocation propagation via Carrier
//     broadcast (the RevocationHook fires locally;
//     the integrator must broadcast the event)
type carrierAuth struct { // A
	logger          *slog.Logger
	mu              sync.RWMutex
	adminCAs        map[string]*AdminCAImpl
	userCAs         map[string]*UserCAImpl
	revokedAdminCAs map[string]struct{}
	revokedUserCAs  map[string]struct{}
	revokedNodes    map[keys.NodeID]struct{}
	onRevoke        RevocationHook
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

// SetRevocationHook registers a callback that fires
// after every revocation. The hook runs outside the
// lock and MUST NOT call back into this carrierAuth.
func (ca *carrierAuth) SetRevocationHook( // A
	hook RevocationHook,
) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.onRevoke = hook
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
// the referenced AdminCA over the full composite
// UserCA public key. Must be called under lock.
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
	pubKeyBytes, err := marshalPubKeyBytes(user.pubKey)
	if err != nil {
		return fmt.Errorf(
			"user CA public key marshal: %w", err,
		)
	}
	msg := DomainSeparate(CTXUserCAAnchorV1, pubKeyBytes)
	if !admin.pubKey.Verify(msg, user.anchorSig) {
		return ErrInvalidAnchorSig
	}
	return nil
}

// RemoveAdminPubKey removes an AdminCA from the trust
// store and revokes it to prevent re-addition.
func (ca *carrierAuth) RemoveAdminPubKey( // A
	pubKeyHash string,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if _, ok := ca.adminCAs[pubKeyHash]; !ok {
		return ErrCANotFound
	}
	delete(ca.adminCAs, pubKeyHash)
	if len(ca.revokedAdminCAs) < MaxRevocationEntries {
		ca.revokedAdminCAs[pubKeyHash] = struct{}{}
	}
	return nil
}

// RemoveUserPubKey removes a UserCA from the trust
// store and revokes it to prevent re-addition.
func (ca *carrierAuth) RemoveUserPubKey( // A
	pubKeyHash string,
) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if _, ok := ca.userCAs[pubKeyHash]; !ok {
		return ErrCANotFound
	}
	delete(ca.userCAs, pubKeyHash)
	if len(ca.revokedUserCAs) < MaxRevocationEntries {
		ca.revokedUserCAs[pubKeyHash] = struct{}{}
	}
	return nil
}

// RevokeUserCA marks a UserCA as revoked, preventing
// re-addition.
func (ca *carrierAuth) RevokeUserCA( // A
	userCAHash string,
) error {
	ca.mu.Lock()
	if len(ca.revokedUserCAs) >= MaxRevocationEntries {
		ca.mu.Unlock()
		return ErrRevocationStoreFull
	}
	ca.revokedUserCAs[userCAHash] = struct{}{}
	delete(ca.userCAs, userCAHash)
	hook := ca.onRevoke
	ca.mu.Unlock()
	ca.logger.InfoContext(
		context.TODO(),
		"user CA revoked",
		LogKeyCAHash, userCAHash,
	)
	if hook != nil {
		hook(RevocationEvent{
			Type: RevocationUser, CAHash: userCAHash,
		})
	}
	return nil
}

// RevokeAdminCA marks an AdminCA as revoked,
// preventing re-addition and invalidating anchors.
func (ca *carrierAuth) RevokeAdminCA( // A
	adminCAHash string,
) error {
	ca.mu.Lock()
	if len(ca.revokedAdminCAs) >= MaxRevocationEntries {
		ca.mu.Unlock()
		return ErrRevocationStoreFull
	}
	ca.revokedAdminCAs[adminCAHash] = struct{}{}
	delete(ca.adminCAs, adminCAHash)
	hook := ca.onRevoke
	ca.mu.Unlock()
	ca.logger.InfoContext(
		context.TODO(),
		"admin CA revoked",
		LogKeyCAHash, adminCAHash,
	)
	if hook != nil {
		hook(RevocationEvent{
			Type: RevocationAdmin, CAHash: adminCAHash,
		})
	}
	return nil
}

// RevokeNode marks a node as revoked by its NodeID.
func (ca *carrierAuth) RevokeNode( // A
	nodeID keys.NodeID,
) error {
	ca.mu.Lock()
	if len(ca.revokedNodes) >= MaxRevocationEntries {
		ca.mu.Unlock()
		return ErrRevocationStoreFull
	}
	ca.revokedNodes[nodeID] = struct{}{}
	hook := ca.onRevoke
	ca.mu.Unlock()
	ca.logger.InfoContext(
		context.TODO(),
		"node revoked",
		LogKeyNodeID, nodeID.String(),
	)
	if hook != nil {
		hook(RevocationEvent{
			Type: RevocationNode, NodeID: nodeID,
		})
	}
	return nil
}
