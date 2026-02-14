package cas
package cas

import (
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/auth"
)

func TestCanAccessDataAdminAllowsAny( // A
	t *testing.T,
) {
	checker := NewPrivilegeChecker()
	access := NewAccessContext(
		auth.ScopeAdmin,
		nil,
	)

	if !checker.CanAccessData(access, "user-ca-1") {
		t.Fatalf("expected admin access to be allowed")
	}
}

func TestCanAccessDataUserOwnedAllowed( // A
	t *testing.T,
) {
	checker := NewPrivilegeChecker()
	access := NewAccessContext(
		auth.ScopeUser,
		[]string{"user-ca-1", "user-ca-2"},
	)

	if !checker.CanAccessData(access, "user-ca-2") {
		t.Fatalf("expected owned user data access")
	}
}

func TestCanAccessDataUserUnownedDenied( // A
	t *testing.T,
) {
	checker := NewPrivilegeChecker()
	access := NewAccessContext(
		auth.ScopeUser,
		[]string{"user-ca-1"},
	)

	if checker.CanAccessData(access, "user-ca-2") {
		t.Fatalf("expected unowned user data access denial")
	}
}

func TestCanAccessDataUnknownScopeDenied( // A
	t *testing.T,
) {
	checker := NewPrivilegeChecker()
	access := NewAccessContext(
		auth.TrustScope(99),
		[]string{"user-ca-1"},
	)

	if checker.CanAccessData(access, "user-ca-1") {
		t.Fatalf("expected unknown scope access denial")
	}
}
