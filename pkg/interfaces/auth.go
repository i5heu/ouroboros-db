package interfaces

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
)

type Cluster struct { // A
	Nodes []Node
}

type Node struct { // A
	NodeID    keys.NodeID
	Addresses []string
	NodeCerts []NodeCert
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

type AdminCA interface { // A
	PubKey() []byte
	Hash() string
	VerifyNodeCert(nodeCert NodeCert, caSignature []byte) (keys.NodeID, error)
}

type UserCA interface { // A
	PubKey() []byte
	Hash() string
	AnchorSig() []byte
	AnchorAdminHash() string
	VerifyNodeCert(nodeCert NodeCert, caSignature []byte) (keys.NodeID, error)
}

type CarrierAuth interface { // A
	VerifyPeerCert(
		peerCerts []NodeCert,
		caSignatures [][]byte,
		delegationProof DelegationProof,
		delegationSig []byte,
		tlsCertPubKeyHash []byte,
		tlsExporterBinding []byte,
		tlsX509Fingerprint []byte,
		tlsTranscriptHash []byte,
	) (AuthContext, error)
	AddAdminPubKey(pubKey []byte) error
	AddUserPubKey(pubKey, anchorSig []byte, anchorAdminHash string) error
	RemoveUserPubKey(pubKeyHash string) error
	RemoveAdminPubKey(pubKeyHash string) error
	RevokeAdminCA(adminCAHash string) error
	RevokeUserCA(userCAHash string) error
	RevokeNode(nodeID keys.NodeID) error
}
