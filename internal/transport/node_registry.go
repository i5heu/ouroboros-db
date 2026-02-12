package transport

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// NodeRegistry tracks all known peer nodes with
// their certificates and connection status.
type NodeRegistry interface { // A
	AddNode(
		peer interfaces.PeerNode,
		caSignature []byte,
		scope auth.TrustScope,
	) error
	RemoveNode(nodeID keys.NodeID) error
	GetNode(
		nodeID keys.NodeID,
	) (interfaces.NodeInfo, error)
	GetAllNodes() []interfaces.PeerNode
	GetAdminNodes() []interfaces.PeerNode
	GetUserNodes() []interfaces.PeerNode
	UpdateConnectionStatus(
		nodeID keys.NodeID,
		status interfaces.ConnectionStatus,
	) error
	UpdateLastSeen(nodeID keys.NodeID) error
}
