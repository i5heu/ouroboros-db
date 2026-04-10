package transport

import (
	"encoding/hex"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// Slog key constants for carrier-specific structured
// logging. Use these instead of raw string literals to
// satisfy the no-raw-keys lint rule.
const ( // A
	logKeyAddress     = "address"
	logKeyPeers       = "peers"
	logKeyAttempt     = "attempt"
	logKeyMaxAttempts = "maxAttempts"
)

// shortID returns the first 8 bytes of a NodeID as
// 16 hex characters. This produces a compact, human-
// readable peer identifier for log output that stays
// consistent with the interactive CLI's shortNodeID.
func shortID(nodeID keys.NodeID) string { // A
	return hex.EncodeToString(nodeID[:8])
}
