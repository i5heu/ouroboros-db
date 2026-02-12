package interfaces

import (
	"testing"
)

func TestConnectionStatusString( // A
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name   string
		status ConnectionStatus
		want   string
	}{
		{
			name:   "disconnected",
			status: ConnectionStatusDisconnected,
			want:   "disconnected",
		},
		{
			name:   "connecting",
			status: ConnectionStatusConnecting,
			want:   "connecting",
		},
		{
			name:   "connected",
			status: ConnectionStatusConnected,
			want:   "connected",
		},
		{
			name:   "failed",
			status: ConnectionStatusFailed,
			want:   "failed",
		},
		{
			name:   "unknown",
			status: ConnectionStatus(99),
			want:   "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.status.String()
			if got != tc.want {
				t.Errorf(
					"String() = %q, want %q",
					got, tc.want,
				)
			}
		})
	}
}
