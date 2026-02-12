package transport

import (
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// QuicTransport manages QUIC listeners and outbound
// connections to peer nodes.
type QuicTransport interface { // A
	Dial(
		peer interfaces.PeerNode,
	) (Connection, error)
	Accept() (Connection, error)
	Close() error
	GetActiveConnections() []Connection
}
