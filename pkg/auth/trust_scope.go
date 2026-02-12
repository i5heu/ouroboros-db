package auth

// TrustScope identifies the authentication scope of a
// connected cluster peer.
type TrustScope int // A

const ( // A
	// ScopeAdmin marks an AdminCA-authorized node.
	ScopeAdmin TrustScope = iota
	// ScopeUser marks a UserCA-authorized node.
	ScopeUser
)

// String returns a human-readable label for the scope.
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
