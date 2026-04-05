package auth

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

// PeerHandshake bundles all inputs required by
// VerifyPeerCert into a single value. This prevents
// argument-ordering mistakes and makes the API
// easier to use safely.
type PeerHandshake struct { // A
	Certs           []NodeCertLike
	CASignatures    [][]byte
	DelegationProof DelegationProofLike
	DelegationSig   []byte
	TLS             TLSBindings
}
