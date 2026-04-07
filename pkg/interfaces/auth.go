package interfaces

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
)

type Cluster struct { // A
	Nodes []Node
}

type Node struct { // A
	NodeID           keys.NodeID
	Addresses        []string
	NodeCerts        []NodeCert
	Role             NodeRole
	LastSeen         time.Time
	ConnectionStatus ConnectionStatus
}

// NodeCert aliases auth.NodeCertLike to keep carrier
// contract signatures exactly aligned across packages.
type NodeCert = auth.NodeCertLike // A

// DelegationProof aliases auth.DelegationProofLike to
// keep carrier contract signatures exactly aligned.
type DelegationProof = auth.DelegationProofLike // A

// AuthContext is a type alias for auth.AuthContext,
// kept here for backward compatibility.
type AuthContext = auth.AuthContext // A

// TLSBindings aliases auth.TLSBindings for use in
// carrier contracts.
type TLSBindings = auth.TLSBindings // A

// PeerHandshake aliases auth.PeerHandshake for use
// in carrier contracts.
type PeerHandshake = auth.PeerHandshake // A

type AdminCA interface { // A
	PubKey() []byte
	Hash() string
	VerifyNodeCert(
		nodeCert NodeCert,
		caSignature []byte,
	) (keys.NodeID, error)
}

type UserCA interface { // A
	PubKey() []byte
	Hash() string
	AnchorSig() []byte
	AnchorAdminHash() string
	VerifyNodeCert(
		nodeCert NodeCert,
		caSignature []byte,
	) (keys.NodeID, error)
}

// Verifier validates a peer's certificate bundle and
// delegation proof, returning the authenticated
// identity context.
type Verifier interface { // A
	VerifyPeerCert(
		hs PeerHandshake,
	) (AuthContext, error)
}

// TrustStore manages certificate authorities and
// revocation state.
type TrustStore interface { // A
	AddAdminPubKey(pubKey []byte) error
	AddUserPubKey(
		pubKey, anchorSig []byte,
		anchorAdminHash string,
	) error
	RemoveAdminPubKey(pubKeyHash string) error
	RemoveUserPubKey(pubKeyHash string) error
	RevokeAdminCA(adminCAHash string) error
	RevokeUserCA(userCAHash string) error
	RevokeNode(nodeID keys.NodeID) error
	SetRevocationHook(hook auth.RevocationHook)
}

// CarrierAuth combines verification and trust store
// management into one interface.
type CarrierAuth interface { // A
	Verifier
	TrustStore
}

// Compile-time interface compliance checks for
// auth.carrierAuth (unexported, so checked via
// NewCarrierAuth return type).
var ( // A
	_ Verifier    = auth.NewCarrierAuth(nil)
	_ TrustStore  = auth.NewCarrierAuth(nil)
	_ CarrierAuth = auth.NewCarrierAuth(nil)
)
