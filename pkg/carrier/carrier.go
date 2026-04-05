package carrier

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// CarrierConfig holds the configuration needed to
// construct a Carrier. It is consumed by New().
//
// # Bootstrap Addresses
//
// BootstrapAddresses is a list of "<host>:<port>"
// endpoints used only for initial cluster discovery.
// These addresses are NOT trusted identities and grant
// NO special privileges—they are connection seeds. Each
// dialed peer is authenticated normally through
// CarrierAuth before being admitted to the registry.
//
// # Self Identity
//
// SelfCert is this node's identity certificate. The
// NodeID is derived automatically from the cert:
//
//	NodeID = SHA-256(SelfCert.NodePubKey())
//
// Callers should retrieve it via SelfCert.NodeID()
// rather than computing it independently.
//
// # Logger
//
// If Logger is nil, the implementation should create a
// default structured logger writing to stderr.
type CarrierConfig struct { // A
	// BootstrapAddresses is a list of "<host>:<port>"
	// endpoints for initial cluster discovery.
	BootstrapAddresses []string

	// SelfCert is this node's identity certificate.
	// NodeID is derived from it via SelfCert.NodeID().
	SelfCert interfaces.NodeCert

	// ListenAddress is the local "<host>:<port>" address
	// the QUIC transport binds to. If empty the Carrier
	// will not accept inbound connections (dial-only).
	ListenAddress string

	// Logger is an optional structured logger. If nil a
	// default stderr logger is used.
	Logger *slog.Logger

	// Auth is the CarrierAuth used to verify inbound
	// peers. If nil, all inbound connections are
	// rejected.
	Auth interfaces.CarrierAuth

	// NodeIdentity holds the node's persistent key,
	// ephemeral session identity, cert bundle, and
	// CA signatures. Required for dialing peers
	// (prover side). If nil, the carrier cannot
	// initiate outbound authenticated connections.
	NodeIdentity *auth.NodeIdentity
}

// Compile-time interface compliance check.
var _ interfaces.Carrier = (*carrierImpl)(nil)

// carrierImpl implements interfaces.Carrier. It owns the
// QUIC transport, node registry, periodic sync, and
// delegates authentication to CarrierAuth.
type carrierImpl struct { // A
	mu          sync.RWMutex
	logger      *slog.Logger
	config      CarrierConfig
	registry    interfaces.NodeRegistry
	transport   interfaces.QuicTransport
	connections map[keys.NodeID]interfaces.Connection
}

// New creates a new Carrier from the given configuration.
// It validates required fields, applies defaults, and
// initializes internal state. The returned Carrier is
// ready to accept inbound connections (if ListenAddress
// is set) and dial bootstrap peers.
func New(conf CarrierConfig) (*carrierImpl, error) { // A
	if conf.SelfCert == nil {
		return nil, fmt.Errorf(
			"SelfCert must not be nil",
		)
	}
	if conf.Logger == nil {
		h := slog.NewTextHandler(
			os.Stderr,
			&slog.HandlerOptions{Level: slog.LevelInfo},
		)
		conf.Logger = slog.New(h)
	}
	c := &carrierImpl{
		logger:      conf.Logger,
		config:      conf,
		connections: make(map[keys.NodeID]interfaces.Connection),
	}
	// Initialize the QUIC transport if a
	// NodeIdentity is provided (needed for
	// TLS configuration).
	if conf.NodeIdentity != nil {
		qt, err := newQuicTransport(
			conf.ListenAddress, conf.NodeIdentity,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"init transport: %w", err,
			)
		}
		c.transport = qt
	}
	return c, nil
}

func (c *carrierImpl) GetNodes() []interfaces.PeerNode { // A
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getNodesUnlocked()
}

func (c *carrierImpl) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.PeerNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.registry == nil {
		return interfaces.PeerNode{}, fmt.Errorf(
			"node not found",
		)
	}
	n, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.PeerNode{}, err
	}
	return toPeerNode(n), nil
}

func (c *carrierImpl) GetNodeConnection( // A
	nodeID keys.NodeID,
) (interfaces.NodeConnection, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.registry == nil {
		return interfaces.NodeConnection{}, fmt.Errorf(
			"node not found",
		)
	}
	n, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.NodeConnection{}, err
	}
	peer := toPeerNode(n)
	var conn interfaces.Connection
	if c.connections != nil {
		conn = c.connections[nodeID]
	}
	return interfaces.NodeConnection{
		Peer: peer,
		Conn: conn,
	}, nil
}

func (c *carrierImpl) BroadcastReliable( // A
	message interfaces.Message,
) (success, failed []interfaces.PeerNode, err error) {
	c.mu.RLock()
	nodes := c.getNodesUnlocked()
	c.mu.RUnlock()
	for _, n := range nodes {
		sendErr := c.SendMessageToNodeReliable(
			n.NodeID, message,
		)
		if sendErr != nil {
			failed = append(failed, n)
			continue
		}
		success = append(success, n)
	}
	return success, failed, nil
}

func (c *carrierImpl) SendMessageToNodeReliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	return fmt.Errorf("not implemented")
}

func (c *carrierImpl) BroadcastUnreliable( // A
	message interfaces.Message,
) (attempted []interfaces.PeerNode) {
	c.mu.RLock()
	nodes := c.getNodesUnlocked()
	c.mu.RUnlock()
	for _, n := range nodes {
		_ = c.SendMessageToNodeUnreliable(
			n.NodeID, message,
		)
		attempted = append(attempted, n)
	}
	return attempted
}

func (c *carrierImpl) SendMessageToNodeUnreliable( // A
	_ keys.NodeID,
	_ interfaces.Message,
) error {
	return fmt.Errorf("not implemented")
}

func (c *carrierImpl) Broadcast( // A
	message interfaces.Message,
) ([]interfaces.PeerNode, error) {
	success, _, err := c.BroadcastReliable(message)
	return success, err
}

func (c *carrierImpl) SendMessageToNode( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	return c.SendMessageToNodeReliable(nodeID, message)
}

func (c *carrierImpl) JoinCluster( // A
	peer interfaces.PeerNode,
	_ interfaces.NodeCert,
) error {
	ni := c.config.NodeIdentity
	if ni == nil {
		return fmt.Errorf(
			"NodeIdentity is required to join",
		)
	}
	c.mu.RLock()
	tp := c.transport
	c.mu.RUnlock()
	if tp == nil {
		return fmt.Errorf(
			"transport is not initialized",
		)
	}

	// Dial the peer via QUIC/TLS.
	node := interfaces.Node{
		NodeID:    peer.NodeID,
		Addresses: peer.Addresses,
	}
	conn, err := tp.Dial(node)
	if err != nil {
		return fmt.Errorf("dial peer: %w", err)
	}

	// Perform prover-side auth handshake.
	if err := c.dialAndAuth(conn, ni); err != nil {
		_ = conn.Close()
		return fmt.Errorf("auth handshake: %w", err)
	}

	// Register this node's connection.
	c.mu.Lock()
	c.connections[peer.NodeID] = conn
	c.mu.Unlock()

	c.logger.Info(
		"joined cluster peer",
		"nodeID", peer.NodeID.String(),
	)
	return nil
}

// dialAndAuth performs the prover-side auth
// handshake: signs a DelegationProof and sends it
// over a QUIC auth stream.
func (c *carrierImpl) dialAndAuth( // A
	conn interfaces.Connection,
	ni *auth.NodeIdentity,
) error {
	// Build exporter function from the connection.
	exporterFn := func(
		label string, ctx []byte, length int,
	) ([]byte, error) {
		return conn.ExportKeyingMaterial(
			label, ctx, length,
		)
	}

	// Sign the DelegationProof.
	proof, sig, err := auth.SignDelegation(
		ni.Key(),
		ni.Certs(),
		ni.Session(),
		exporterFn,
	)
	if err != nil {
		return fmt.Errorf(
			"sign delegation: %w", err,
		)
	}

	// Open an auth stream and send the handshake.
	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf(
			"open auth stream: %w", err,
		)
	}
	defer func() { _ = stream.Close() }()

	return writeAuthHandshake(
		stream,
		ni.Certs(),
		ni.CASigs(),
		proof,
		sig,
	)
}

func (c *carrierImpl) LeaveCluster( // A
	_ interfaces.PeerNode,
) error {
	return fmt.Errorf("not implemented")
}

func (c *carrierImpl) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.registry == nil {
		return fmt.Errorf("node not found")
	}
	if conn, ok := c.connections[nodeID]; ok {
		_ = conn.Close()
		delete(c.connections, nodeID)
	}
	return c.registry.RemoveNode(nodeID)
}

func (c *carrierImpl) IsConnected( // A
	nodeID keys.NodeID,
) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.connections[nodeID]
	return ok
}

// StartListener accepts inbound QUIC connections on the
// configured ListenAddress. It blocks until ctx is
// cancelled or a fatal accept error occurs. Each accepted
// connection is authenticated via CarrierAuth and, on
// success, added to the node registry.
func (c *carrierImpl) StartListener( // A
	ctx context.Context,
) error {
	if c.config.ListenAddress == "" {
		return fmt.Errorf(
			"ListenAddress is not configured",
		)
	}
	c.mu.RLock()
	tp := c.transport
	c.mu.RUnlock()
	if tp == nil {
		return fmt.Errorf(
			"transport is not initialized",
		)
	}
	// Ensure the QUIC listener is started.
	if qt, ok := tp.(*quicTransport); ok {
		if err := qt.startListener(); err != nil {
			return err
		}
	}
	for {
		conn, err := tp.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf(
					"accept: %w", err,
				)
			}
		}
		go c.handleIncomingConn(ctx, conn)
	}
}

// handleIncomingConn authenticates an inbound connection
// and adds the peer to the registry on success.
// Connections are always rejected when no CarrierAuth is
// configured.
func (c *carrierImpl) handleIncomingConn( // A
	ctx context.Context,
	conn interfaces.Connection,
) {
	if c.config.Auth == nil {
		c.logger.WarnContext(ctx,
			"rejecting connection, no CarrierAuth configured",
		)
		_ = conn.Close()
		return
	}

	// Accept the peer's auth stream.
	stream, err := conn.AcceptStream()
	if err != nil {
		c.logger.WarnContext(ctx,
			"auth stream accept failed",
			"error", err.Error(),
		)
		_ = conn.Close()
		return
	}

	// Read the handshake message from the stream.
	hs, err := readAuthHandshake(stream, conn)
	if err != nil {
		c.logger.WarnContext(ctx,
			"auth handshake read failed",
			"error", err.Error(),
		)
		_ = stream.Close()
		_ = conn.Close()
		return
	}
	_ = stream.Close()

	// Verify peer identity and authorization.
	authCtx, err := c.config.Auth.VerifyPeerCert(hs)
	if err != nil {
		c.logger.WarnContext(ctx,
			"peer auth verification failed",
			"error", err.Error(),
		)
		_ = conn.Close()
		return
	}

	// Register authenticated peer.
	c.mu.Lock()
	c.connections[authCtx.NodeID] = conn
	c.mu.Unlock()

	c.logger.InfoContext(ctx,
		"peer authenticated and registered",
		"nodeID", authCtx.NodeID.String(),
		"scope", authCtx.EffectiveScope.String(),
	)
}

// getNodesUnlocked returns all registry nodes as
// PeerNode slices. Caller must hold at least a read
// lock.
func (c *carrierImpl) getNodesUnlocked() []interfaces.PeerNode { // A
	if c.registry == nil {
		return nil
	}
	nodes := c.registry.GetAllNodes()
	out := make(
		[]interfaces.PeerNode, 0, len(nodes),
	)
	for _, n := range nodes {
		out = append(out, toPeerNode(n))
	}
	return out
}

// toPeerNode converts an interfaces.Node to a PeerNode.
func toPeerNode(n interfaces.Node) interfaces.PeerNode { // A
	return interfaces.PeerNode{
		NodeID:    n.NodeID,
		Addresses: n.Addresses,
		Cert:      nil,
	}
}
