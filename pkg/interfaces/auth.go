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

type NodeCert interface { // A
	NodePubKey() keys.PublicKey
	IssuerCAHash() string
	ValidFrom() int64
	ValidUntil() int64
	Serial() []byte
	CertNonce() []byte
	NodeID() keys.NodeID
}

type DelegationProof interface { // A
	TLSCertPubKeyHash() []byte
	TLSExporterBinding() []byte
	TLSTranscriptHash() []byte
	X509Fingerprint() []byte
	NodeCertBundleHash() []byte
	NotBefore() int64
	NotAfter() int64
}

type AuthContext struct { // A
	NodeID              keys.NodeID
	EffectiveScope      auth.TrustScope
	AllowedUserCAOwners []string
	HasValidAdminCert   bool
}

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
	RemoveAdminPubKey(pubKeyHash string) error
	RevokeAdminCA(adminCAHash string) error
	AddUserPubKey(pubKey, anchorSig []byte, anchorAdminHash string) error
	RemoveUserPubKey(pubKeyHash string) error
	RevokeUserCA(userCAHash string) error
	RevokeNode(nodeID keys.NodeID) error
}
