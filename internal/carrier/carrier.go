// Package carrier implements the Carrier interface for inter-node communication
// in the OuroborosDB cluster.
//
// The Carrier is responsible for:
//   - Managing the list of known cluster nodes
//   - Broadcasting messages to all nodes
//   - Sending messages to specific nodes
//   - Handling cluster join/leave operations
//
// Communication between nodes uses the QUIC protocol for reliable,
// multiplexed streams with built-in encryption. Each node has its own
// cryptographic identity using post-quantum algorithms (ML-KEM for key
// encapsulation, ML-DSA for signatures) from ouroboros-crypt.
//
// It works in conjunction with the BootStrapper to initialize new nodes and
// uses the Message protocol for all inter-node communication.
//
// File organization:
//   - carrier.go: DefaultCarrier implementation (this file)
//   - carrier_membership.go: Cluster membership and node management
//   - carrier_messaging.go: Broadcasting and message sending
//   - carrier_bootstrap.go: Bootstrapping and node sync
//   - carrier_server.go: Server lifecycle and connection handling
//   - carrier_handlers.go: Built-in message handlers
//   - carrier_config.go: Config and QUICConfig types
//   - interfaces.go: Carrier, Transport, Connection, Listener interfaces
//   - message.go: Message, EncryptedMessage, BroadcastResult types
//   - node.go: Node, NodeID, NodeCert, NodeIdentity types
//   - types.go: MessageType enum and constants
package carrier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// timeNow is a variable for time.Now to allow testing.
var timeNow = time.Now // A

// DefaultCarrier is the default implementation of the Carrier interface.
// It uses QUIC for transport and post-quantum encryption for message security.
// Connections to remote nodes are maintained persistently and reused for all
// messages, with QUIC streams providing multiplexed communication.
type DefaultCarrier struct { // A
	localNode    Node
	nodeIdentity *NodeIdentity
	log          *slog.Logger
	transport    Transport
	bootStrapper BootStrapper
	qConfig      QUICConfig

	// Bootstrap configuration
	bootstrapAddresses []string
	defaultPort        uint16

	mu    sync.RWMutex
	nodes map[NodeID]Node

	// Connection pool for persistent connections
	pool *connPool

	// Listener state
	listener   Listener
	listenerMu sync.Mutex
	running    bool
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Message handlers
	handlersMu sync.RWMutex
	handlers   map[MessageType][]MessageHandler
}

// NewDefaultCarrier creates a new DefaultCarrier with the given configuration.
// Each node must have its own NodeIdentity for encryption and signing.
func NewDefaultCarrier(cfg Config) (*DefaultCarrier, error) { // A
	if err := cfg.LocalNode.Validate(); err != nil {
		return nil, fmt.Errorf("invalid local node: %w", err)
	}
	if cfg.NodeIdentity == nil {
		return nil, errors.New("node identity is required for encryption")
	}
	if cfg.Transport == nil {
		return nil, errors.New("transport is required")
	}
	if cfg.Logger == nil {
		return nil, errors.New("logger is required")
	}

	qConfig := cfg.QUICConfig
	if qConfig.MaxIdleTimeout == 0 {
		qConfig = DefaultQUICConfig()
	}

	defaultPort := cfg.DefaultPort
	if defaultPort == 0 {
		defaultPort = 4242 // Default OuroborosDB port
	}

	c := &DefaultCarrier{
		localNode:          cfg.LocalNode,
		nodeIdentity:       cfg.NodeIdentity,
		log:                cfg.Logger,
		transport:          cfg.Transport,
		bootStrapper:       cfg.BootStrapper,
		qConfig:            qConfig,
		bootstrapAddresses: cfg.BootstrapAddresses,
		defaultPort:        defaultPort,
		nodes:              make(map[NodeID]Node),
		handlers:           make(map[MessageType][]MessageHandler),
	}

	// Initialize connection pool
	c.pool = newConnPool(cfg.Transport, cfg.LocalNode.NodeID, cfg.Logger)

	// Add self to known nodes
	c.nodes[cfg.LocalNode.NodeID] = cfg.LocalNode

	return c, nil
}

// GetNodes returns all known nodes in the cluster.
func (c *DefaultCarrier) GetNodes(ctx context.Context) ([]Node, error) { // A
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// LocalNode returns the local node's identity.
func (c *DefaultCarrier) LocalNode() Node { // A
	return c.localNode
}

// NodeIdentity returns the cryptographic identity of this node.
func (c *DefaultCarrier) NodeIdentity() *NodeIdentity {
	return c.nodeIdentity
}

// SetLogger updates the logger used by the carrier.
func (c *DefaultCarrier) SetLogger(logger *slog.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = logger
	if c.pool != nil {
		c.pool.SetLogger(logger)
	}
	if c.transport != nil {
		c.transport.SetLogger(logger)
	}
}

// BootstrapAddresses returns the configured bootstrap addresses.
func (c *DefaultCarrier) BootstrapAddresses() []string { // A
	return c.bootstrapAddresses
}

// DefaultPort returns the configured default port for bootstrap addresses.
func (c *DefaultCarrier) DefaultPort() uint16 { // A
	return c.defaultPort
}

// GetNodePublicKey returns the public key for a known node.
func (c *DefaultCarrier) GetNodePublicKey(
	nodeID NodeID,
) (*keys.PublicKey, bool) { // A
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, ok := c.nodes[nodeID]
	if !ok {
		return nil, false
	}
	return node.PublicKey, node.PublicKey != nil
}

// Ensure DefaultCarrier implements Carrier interface.
var _ Carrier = (*DefaultCarrier)(nil)
