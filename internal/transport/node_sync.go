package transport

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// NodeSync handles periodic full sync of the node
// registry between cluster peers.
type NodeSync interface { // A
	StartSyncInterval(d time.Duration)
	StopSync()
	TriggerFullSync() error
	ExchangeNodeList(
		peerID keys.NodeID,
	) error
	HandleNodeListUpdate(
		nodes []interfaces.NodeInfo,
	) error
}
