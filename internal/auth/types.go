package auth

import "github.com/i5heu/ouroboros-crypt/pkg/keys"

// TrustScope represents the authenticated
// authorization scope of a peer.
type TrustScope int // A

const ( // A
	ScopeAdmin TrustScope = iota
	ScopeUser
)

// String returns the textual scope name.
func (s TrustScope) String() string { // A
	switch s {
	case ScopeAdmin:
		return "admin"
	case ScopeUser:
		return "user"
	default:
		return "unknown"
	}
}

// TLSBindings groups the four TLS channel-binding
// fields extracted from the transport layer. Each
// field must be exactly 32 bytes (SHA-256).
type TLSBindings struct { // A
	CertPubKeyHash  []byte
	ExporterBinding []byte
	X509Fingerprint []byte
	TranscriptHash  []byte
}

// EmbeddedCA carries public-only CA chain material
// that can be shipped alongside a node certificate
// bundle during peer authentication.
type EmbeddedCA struct { // A
	Type        string
	PubKEM      []byte
	PubSign     []byte
	AnchorSig   []byte
	AnchorAdmin string
}

// PeerHandshake bundles all inputs required by
// VerifyPeerCert into a single value. This prevents
// argument-ordering mistakes and makes the API
// easier to use safely.
type PeerHandshake struct { // A
	Certs           []NodeCertLike
	CASignatures    [][]byte
	Authorities     []EmbeddedCA
	DelegationProof DelegationProofLike
	DelegationSig   []byte
	TLS             TLSBindings
}

// RevocationType distinguishes the kind of entity
// being revoked.
type RevocationType int // A

const ( // A
	RevocationAdmin RevocationType = iota
	RevocationUser
	RevocationNode
)

// RevocationEvent carries the details of a
// revocation action for notification hooks.
type RevocationEvent struct { // A
	Type   RevocationType
	CAHash string
	NodeID keys.NodeID
}

// RevocationHook is called after a revocation is
// committed. Implementations MUST NOT call back into
// the CarrierAuth that fired the hook (deadlock).
type RevocationHook func(event RevocationEvent) // A
