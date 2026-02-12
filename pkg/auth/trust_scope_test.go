package auth

import (
	"testing"
)

func TestTrustScopeString(t *testing.T) { // A
	tests := []struct {
		name  string
		scope TrustScope
		want  string
	}{
		{
			name:  "admin scope",
			scope: ScopeAdmin,
			want:  "admin",
		},
		{
			name:  "user scope",
			scope: ScopeUser,
			want:  "user",
		},
		{
			name:  "unknown scope",
			scope: TrustScope(99),
			want:  "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.scope.String()
			if got != tc.want {
				t.Errorf(
					"TrustScope(%d).String() "+
						"= %q, want %q",
					tc.scope, got, tc.want,
				)
			}
		})
	}
}
