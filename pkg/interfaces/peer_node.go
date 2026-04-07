// Package interfaces defines shared transport and
// messaging abstractions used across OuroborosDB
// cluster components.
package interfaces

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
)

// NodeRole describes whether a node should be treated
// as a server-class or client-class peer.
type NodeRole int // A

const ( // A
	NodeRoleServer NodeRole = iota
	NodeRoleClient
)

// String returns the human-readable node role.
func (r NodeRole) String() string { // A
	switch r {
	case NodeRoleServer:
		return "server"
	case NodeRoleClient:
		return "client"
	default:
		return "unknown"
	}
}

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
	Cert      NodeCert
	Role      NodeRole
}

// NodeInfo holds complete node data for registry
// storage and sync operations.
type NodeInfo struct { // A
	Peer             PeerNode
	CASignature      []byte
	DelegationProof  DelegationProof
	DelegationSig    []byte
	LastSeen         time.Time
	ConnectionStatus ConnectionStatus
	Role             NodeRole
	TrustScope       auth.TrustScope
}

// NodeConnection wraps a PeerNode together with its
// active QUIC connection. It is returned by Carrier so
// callers can interact with a specific peer without a
// separate lookup step.
//
// The Connection field is nil when the peer is known to
// the registry but has no currently active transport
// connection (e.g. temporarily disconnected). Always
// check Connection != nil before using it.
//
// NodeConnection is a snapshot: the underlying connection
// may close concurrently. Callers should handle a nil or
// closed Connection gracefully.
type NodeConnection struct { // A
	// Peer is the authenticated peer identity.
	Peer PeerNode

	// Conn is the active QUIC connection to the peer,
	// or nil if the peer is not currently connected.
	Conn Connection
}
