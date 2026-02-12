// Package interfaces defines shared transport and
// messaging abstractions used across OuroborosDB
// cluster components.
package interfaces

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
)

// ConnectionStatus represents the connection state
// of a remote peer node.
type ConnectionStatus int // A

const ( // A
	ConnectionStatusDisconnected ConnectionStatus = iota
	ConnectionStatusConnecting
	ConnectionStatusConnected
	ConnectionStatusFailed
)

// String returns the human-readable name of the
// connection status.
func (s ConnectionStatus) String() string { // A
	switch s {
	case ConnectionStatusDisconnected:
		return "disconnected"
	case ConnectionStatusConnecting:
		return "connecting"
	case ConnectionStatusConnected:
		return "connected"
	case ConnectionStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// PeerNode describes a remote cluster peer as seen
// from the local node. It carries the peer identity,
// network addresses, and certificate material.
type PeerNode struct { // A
	NodeID    keys.NodeID
	Addresses []string
	Cert      *auth.NodeCert
}

// NodeInfo holds complete node data for registry
// storage and sync operations.
type NodeInfo struct { // A
	Peer             PeerNode
	CASignature      []byte
	LastSeen         time.Time
	ConnectionStatus ConnectionStatus
	TrustScope       auth.TrustScope
}
