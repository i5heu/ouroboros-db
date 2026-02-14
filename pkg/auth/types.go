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
