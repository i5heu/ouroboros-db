package cas

import "github.com/i5heu/ouroboros-db/pkg/auth"

// AccessContext contains the authenticated scope
// and the data-owner set allowed for user scope.
type AccessContext struct { // A
	Scope                auth.TrustScope
	AllowedOwnerCAHashes map[string]struct{}
}

// NewAccessContext builds an access context from
// scope and owner hashes.
func NewAccessContext( // A
	scope auth.TrustScope,
	ownerCAHashes []string,
) AccessContext {
	allowed := make(map[string]struct{}, len(ownerCAHashes))
	for _, ownerCAHash := range ownerCAHashes {
		if ownerCAHash == "" {
			continue
		}
		allowed[ownerCAHash] = struct{}{}
	}
	return AccessContext{
		Scope:                scope,
		AllowedOwnerCAHashes: allowed,
	}
}

// PrivilegeChecker centralizes CAS privilege checks.
type PrivilegeChecker interface { // A
	CanAccessData(
		access AccessContext,
		ownerCAHash string,
	) bool
}

// DefaultPrivilegeChecker is the default CAS
// privilege checker implementation.
type DefaultPrivilegeChecker struct{} // A

// NewPrivilegeChecker creates the default
// PrivilegeChecker.
func NewPrivilegeChecker() *DefaultPrivilegeChecker { // A
	return &DefaultPrivilegeChecker{}
}

// CanAccessData allows admin scope for all data.
// User scope is allowed only for owned data.
func (c *DefaultPrivilegeChecker) CanAccessData( // A
	access AccessContext,
	ownerCAHash string,
) bool {
	if access.Scope == auth.ScopeAdmin {
		return true
	}
	if access.Scope != auth.ScopeUser {
		return false
	}
	if ownerCAHash == "" {
		return false
	}
	_, ok := access.AllowedOwnerCAHashes[ownerCAHash]
	return ok
}
