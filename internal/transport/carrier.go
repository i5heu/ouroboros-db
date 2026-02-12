package transport

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// MessageReceiver is the callback invoked when an
// inbound message arrives from an authenticated peer.
// The Carrier writes the returned Response back on
// the same stream.
type MessageReceiver func( // A
	msg interfaces.Message,
	peer keys.NodeID,
	scope auth.TrustScope,
) (interfaces.Response, error)

// CarrierConfig holds configuration for creating a
// Carrier instance.
type CarrierConfig struct { // A
	ListenAddr      string
	BootstrapConfig *BootstrapConfig
	CarrierAuth     *auth.CarrierAuth
	LocalNodeID     keys.NodeID
	// LOGGER: Using slog directly because Carrier
	// is a dependency of ClusterLog. Using
	// pkg/clusterlog here would create a
	// subscription loop.
	Logger *slog.Logger
}

// Carrier is the concrete QUIC-based cluster
// transport that implements interfaces.Carrier.
type Carrier struct { // A
	transport    *quicTransportImpl
	registry     NodeRegistry
	nodeSync     NodeSync
	bootstrapper *BootStrapper
	auth         *auth.CarrierAuth
	receiver     atomic.Pointer[MessageReceiver]
	// LOGGER: Using slog directly because Carrier
	// is a dependency of ClusterLog. Using
	// pkg/clusterlog here would create a
	// subscription loop.
	logger *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCarrier creates and starts a new Carrier.
func NewCarrier( // A
	cfg CarrierConfig,
) (*Carrier, error) {
	if cfg.CarrierAuth == nil {
		return nil, fmt.Errorf(
			"carrier auth must not be nil",
		)
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf(
			"logger must not be nil",
		)
	}

	qt, err := NewQuicTransport(
		cfg.ListenAddr,
		cfg.CarrierAuth,
		cfg.LocalNodeID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create transport: %w", err,
		)
	}

	impl := qt.(*quicTransportImpl)
	registry := NewNodeRegistry()
	ns := NewNodeSync(
		registry,
		qt,
		cfg.CarrierAuth,
	)
	bs := NewBootStrapper(
		cfg.BootstrapConfig,
		qt,
		registry,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	c := &Carrier{
		transport:    impl,
		registry:     registry,
		nodeSync:     ns,
		bootstrapper: bs,
		auth:         cfg.CarrierAuth,
		logger:       cfg.Logger,
		ctx:          ctx,
		cancel:       cancel,
	}

	c.wg.Add(1)
	go c.acceptLoop()

	return c, nil
}

// Close gracefully shuts down the Carrier.
func (c *Carrier) Close() error { // A
	c.cancel()
	c.nodeSync.StopSync()
	err := c.transport.Close()
	c.wg.Wait()
	return err
}

// ListenAddr returns the address the Carrier is
// listening on.
func (c *Carrier) ListenAddr() string { // A
	return c.transport.ListenAddr()
}

// Registry returns the node registry for external
// inspection.
func (c *Carrier) Registry() NodeRegistry { // A
	return c.registry
}

func (c *Carrier) GetNodes() []interfaces.PeerNode { // A
	return c.registry.GetAllNodes()
}

func (c *Carrier) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.PeerNode, error) {
	info, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.PeerNode{}, err
	}
	return info.Peer, nil
}

func (c *Carrier) BroadcastReliable( // A
	message interfaces.Message,
) (
	success []interfaces.PeerNode,
	failed []interfaces.PeerNode,
	err error,
) {
	nodes := c.registry.GetAllNodes()
	for _, peer := range nodes {
		sendErr := c.SendMessageToNodeReliable(
			peer.NodeID,
			message,
		)
		if sendErr != nil {
			failed = append(failed, peer)
		} else {
			success = append(success, peer)
		}
	}
	return success, failed, nil
}

func (c *Carrier) SendMessageToNodeReliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	conn, err := c.ensureConnection(nodeID)
	if err != nil {
		return err
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	return WriteMessage(stream, message)
}

func (c *Carrier) BroadcastUnreliable( // A
	message interfaces.Message,
) (attempted []interfaces.PeerNode) {
	data, err := SerializeMessage(message)
	if err != nil {
		return nil
	}

	nodes := c.registry.GetAllNodes()
	for _, peer := range nodes {
		conn := c.transport.GetConnection(
			peer.NodeID,
		)
		if conn == nil {
			continue
		}
		_ = conn.SendDatagram(data)
		attempted = append(attempted, peer)
	}
	return attempted
}

func (c *Carrier) SendMessageToNodeUnreliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	data, err := SerializeMessage(message)
	if err != nil {
		return fmt.Errorf(
			"serialize message: %w", err,
		)
	}

	conn := c.transport.GetConnection(nodeID)
	if conn == nil {
		return fmt.Errorf(
			"no connection to %s", nodeID,
		)
	}
	return conn.SendDatagram(data)
}

func (c *Carrier) Broadcast( // A
	message interfaces.Message,
) (success []interfaces.PeerNode, err error) {
	s, _, err := c.BroadcastReliable(message)
	return s, err
}

func (c *Carrier) SendMessageToNode( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	return c.SendMessageToNodeReliable(
		nodeID,
		message,
	)
}

func (c *Carrier) JoinCluster( // A
	clusterNode interfaces.PeerNode,
	cert *auth.NodeCert,
) error {
	conn, err := c.transport.Dial(clusterNode)
	if err != nil {
		return fmt.Errorf(
			"dial cluster node: %w", err,
		)
	}

	err = c.registry.AddNode(
		clusterNode,
		nil,
		0,
	)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"register cluster node: %w", err,
		)
	}

	return c.registry.UpdateConnectionStatus(
		clusterNode.NodeID,
		interfaces.ConnectionStatusConnected,
	)
}

func (c *Carrier) LeaveCluster( // A
	clusterNode interfaces.PeerNode,
) error {
	conn := c.transport.GetConnection(
		clusterNode.NodeID,
	)
	if conn != nil {
		_ = conn.Close()
	}
	c.transport.RemoveConnection(
		clusterNode.NodeID,
	)
	return c.registry.RemoveNode(
		clusterNode.NodeID,
	)
}

func (c *Carrier) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	conn := c.transport.GetConnection(nodeID)
	if conn != nil {
		_ = conn.Close()
	}
	c.transport.RemoveConnection(nodeID)
	return c.registry.RemoveNode(nodeID)
}

func (c *Carrier) IsConnected( // A
	nodeID keys.NodeID,
) bool {
	info, err := c.registry.GetNode(nodeID)
	if err != nil {
		return false
	}
	return info.ConnectionStatus ==
		interfaces.ConnectionStatusConnected
}

// SetMessageReceiver sets the callback invoked for
// each inbound message from authenticated peers.
// Must be called before connections arrive to avoid
// missing messages. Thread-safe.
func (c *Carrier) SetMessageReceiver( // A
	recv MessageReceiver,
) {
	c.receiver.Store(&recv)
}

func (c *Carrier) acceptLoop() { // A
	defer c.wg.Done()
	for {
		conn, err := c.transport.Accept()
		if err != nil {
			select {
			case <-c.ctx.Done():
				return
			default:
				continue
			}
		}

		// Register accepted peer.
		peer := interfaces.PeerNode{
			NodeID: conn.NodeID(),
		}
		_ = c.registry.AddNode(peer, nil, 0)
		_ = c.registry.UpdateConnectionStatus(
			peer.NodeID,
			interfaces.ConnectionStatusConnected,
		)

		// Spawn a goroutine to handle inbound
		// streams from this connection.
		c.wg.Add(1)
		go c.handleConnection(conn)
	}
}

// handleConnection processes inbound streams from a
// single peer connection. Each stream carries one
// request/response exchange.
func (c *Carrier) handleConnection( // A
	conn Connection,
) {
	defer c.wg.Done()
	for {
		stream, err := conn.AcceptStream()
		if err != nil {
			select {
			case <-c.ctx.Done():
				return
			default:
				// Connection closed or error;
				// stop processing this peer.
				return
			}
		}

		c.wg.Add(1)
		go c.handleStream(conn, stream)
	}
}

// handleStream reads a single message from the
// stream, dispatches it to the MessageReceiver,
// and writes the response back.
func (c *Carrier) handleStream( // A
	conn Connection,
	stream Stream,
) {
	defer c.wg.Done()
	defer func() { _ = stream.Close() }()

	msg, err := ReadMessage(stream)
	if err != nil {
		return
	}

	_ = c.registry.UpdateLastSeen(conn.NodeID())

	resp := c.dispatchMessage(msg, conn.NodeID())

	_ = WriteResponse(stream, resp)
}

// dispatchMessage invokes the MessageReceiver or
// returns an error response if no receiver is set.
func (c *Carrier) dispatchMessage( // A
	msg interfaces.Message,
	peer keys.NodeID,
) interfaces.Response {
	recv := c.receiver.Load()
	if recv == nil {
		return interfaces.Response{
			Error: fmt.Errorf(
				"no message receiver configured",
			),
		}
	}

	info, err := c.registry.GetNode(peer)
	if err != nil {
		return interfaces.Response{
			Error: fmt.Errorf(
				"unknown peer %s", peer,
			),
		}
	}

	resp, err := (*recv)(
		msg, peer, info.TrustScope,
	)
	if err != nil {
		return interfaces.Response{
			Error: err,
		}
	}
	return resp
}

// ensureConnection returns an existing connection
// or dials a new one.
func (c *Carrier) ensureConnection( // A
	nodeID keys.NodeID,
) (Connection, error) {
	existing := c.transport.GetConnection(nodeID)
	if existing != nil {
		return existing, nil
	}

	info, err := c.registry.GetNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf(
			"node %s not in registry", nodeID,
		)
	}

	conn, err := c.transport.Dial(info.Peer)
	if err != nil {
		return nil, fmt.Errorf(
			"dial %s: %w", nodeID, err,
		)
	}

	_ = c.registry.UpdateConnectionStatus(
		nodeID,
		interfaces.ConnectionStatusConnected,
	)

	return conn, nil
}
