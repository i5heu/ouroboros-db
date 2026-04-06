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
// NO special privileges - they are connection seeds.
// Each dialed peer is authenticated normally through
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

// RuntimeCarrier extends the public carrier contract
// with runtime wiring helpers needed by commands.
type RuntimeCarrier interface { // A
	interfaces.Carrier
	SetController(controller interfaces.ClusterController)
	ListenAddress() string
}

const logKeyMessageType = "messageType" // A

// Compile-time interface compliance check.
var (
	_ interfaces.Carrier = (*carrierImpl)(nil)
	_ RuntimeCarrier     = (*carrierImpl)(nil)
)

// carrierImpl implements interfaces.Carrier. It owns the
// QUIC transport, node registry, and connection state.
type carrierImpl struct { // A
	mu          sync.RWMutex
	logger      *slog.Logger
	config      CarrierConfig
	registry    interfaces.NodeRegistry
	transport   interfaces.QuicTransport
	connections map[keys.NodeID]interfaces.Connection
	controller  interfaces.ClusterController
	peerScopes  map[keys.NodeID]auth.TrustScope
}

// New creates a new Carrier from the given configuration.
func New(conf CarrierConfig) (*carrierImpl, error) { // A
	if conf.SelfCert == nil {
		return nil, fmt.Errorf("SelfCert must not be nil")
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
		registry:    newNodeRegistry(),
		connections: make(map[keys.NodeID]interfaces.Connection),
		peerScopes:  make(map[keys.NodeID]auth.TrustScope),
	}
	if conf.NodeIdentity != nil {
		qt, err := newQuicTransport(
			conf.ListenAddress,
			conf.NodeIdentity,
		)
		if err != nil {
			return nil, fmt.Errorf("init transport: %w", err)
		}
		c.transport = qt
	}
	return c, nil
}

// SetController wires the cluster controller used for inbound dispatch.
func (c *carrierImpl) SetController( // A
	controller interfaces.ClusterController,
) {
	c.mu.Lock()
	c.controller = controller
	c.mu.Unlock()
}

// ListenAddress returns the bound local QUIC address.
func (c *carrierImpl) ListenAddress() string { // A
	c.mu.RLock()
	tp := c.transport
	c.mu.RUnlock()
	qt, ok := tp.(*quicTransport)
	if !ok {
		return c.config.ListenAddress
	}
	return qt.listenAddress()
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
	node, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.PeerNode{}, err
	}
	return toPeerNode(node), nil
}

func (c *carrierImpl) GetNodeConnection( // A
	nodeID keys.NodeID,
) (interfaces.NodeConnection, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	node, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.NodeConnection{}, err
	}
	return interfaces.NodeConnection{
		Peer: toPeerNode(node),
		Conn: c.connections[nodeID],
	}, nil
}

func (c *carrierImpl) BroadcastReliable( // A
	message interfaces.Message,
) (success, failed []interfaces.PeerNode, err error) {
	c.mu.RLock()
	nodes := c.getNodesUnlocked()
	c.mu.RUnlock()
	if len(nodes) == 0 {
		return nil, nil, fmt.Errorf("no peers available")
	}
	for _, node := range nodes {
		if err := c.SendMessageToNodeReliable(
			node.NodeID,
			message,
		); err != nil {
			failed = append(failed, node)
			continue
		}
		success = append(success, node)
	}
	return success, failed, nil
}

func (c *carrierImpl) SendMessageToNodeReliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	conn, err := c.connectionForNode(nodeID)
	if err != nil {
		return err
	}
	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()
	return writeMessageStream(stream, message)
}

func (c *carrierImpl) BroadcastUnreliable( // A
	message interfaces.Message,
) (attempted []interfaces.PeerNode) {
	c.mu.RLock()
	nodes := c.getNodesUnlocked()
	c.mu.RUnlock()
	for _, node := range nodes {
		_ = c.SendMessageToNodeUnreliable(
			node.NodeID,
			message,
		)
		attempted = append(attempted, node)
	}
	return attempted
}

func (c *carrierImpl) SendMessageToNodeUnreliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	conn, err := c.connectionForNode(nodeID)
	if err != nil {
		return err
	}
	data, err := marshalMessage(message)
	if err != nil {
		return err
	}
	return conn.SendDatagram(data)
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

func (c *carrierImpl) OpenPeerChannel( // A
	peer interfaces.PeerNode,
	_ interfaces.NodeCert,
) error {
	ni := c.config.NodeIdentity
	if ni == nil {
		return fmt.Errorf("NodeIdentity is required to open peer channel")
	}
	c.mu.RLock()
	tp := c.transport
	c.mu.RUnlock()
	if tp == nil {
		return fmt.Errorf("transport is not initialized")
	}
	conn, err := tp.Dial(interfaces.Node{
		NodeID:    peer.NodeID,
		Addresses: peer.Addresses,
	})
	if err != nil {
		return fmt.Errorf("dial peer: %w", err)
	}
	if err := c.dialAndAuth(conn, ni); err != nil {
		_ = conn.Close()
		return fmt.Errorf("auth handshake: %w", err)
	}
	authCtx, certs, err := c.awaitPeerAuth(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("verify peer auth: %w", err)
	}
	if err := c.registerPeer(
		authCtx,
		conn,
		certs,
		[]string{peerAddress(peer, conn)},
	); err != nil {
		_ = conn.Close()
		return fmt.Errorf("register peer: %w", err)
	}
	c.startConnectionLoops(
		context.Background(),
		authCtx.NodeID,
		conn,
	)
	c.logger.InfoContext(
		context.Background(),
		"joined cluster peer",
		auth.LogKeyNodeID,
		authCtx.NodeID.String(),
	)
	return nil
}

// dialAndAuth performs the prover-side auth handshake.
func (c *carrierImpl) dialAndAuth( // A
	conn interfaces.Connection,
	ni *auth.NodeIdentity,
) error {
	exporterFn := func(
		label string,
		ctx []byte,
		length int,
	) ([]byte, error) {
		return conn.ExportKeyingMaterial(label, ctx, length)
	}
	proof, sig, err := auth.SignDelegation(
		ni.Key(),
		ni.Certs(),
		ni.Session(),
		exporterFn,
	)
	if err != nil {
		return fmt.Errorf("sign delegation: %w", err)
	}
	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf("open auth stream: %w", err)
	}
	defer func() { _ = stream.Close() }()
	return writeAuthHandshake(
		stream,
		ni.Certs(),
		ni.CASigs(),
		ni.Authorities(),
		proof,
		sig,
	)
}

func (c *carrierImpl) LeaveCluster( // A
	peer interfaces.PeerNode,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn, ok := c.connections[peer.NodeID]
	if !ok {
		return fmt.Errorf("node not connected")
	}
	if err := conn.Close(); err != nil {
		return err
	}
	delete(c.connections, peer.NodeID)
	delete(c.peerScopes, peer.NodeID)
	return nil
}

func (c *carrierImpl) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if conn, ok := c.connections[nodeID]; ok {
		_ = conn.Close()
		delete(c.connections, nodeID)
	}
	delete(c.peerScopes, nodeID)
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
// configured ListenAddress.
func (c *carrierImpl) StartListener( // A
	ctx context.Context,
) error {
	if c.config.ListenAddress == "" {
		return fmt.Errorf("ListenAddress is not configured")
	}
	c.mu.RLock()
	tp := c.transport
	c.mu.RUnlock()
	if tp == nil {
		return fmt.Errorf("transport is not initialized")
	}
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
				return fmt.Errorf("accept: %w", err)
			}
		}
		go c.handleIncomingConn(ctx, conn)
	}
}

// handleIncomingConn authenticates an inbound connection
// and adds the peer to the registry on success.
func (c *carrierImpl) handleIncomingConn( // A
	ctx context.Context,
	conn interfaces.Connection,
) {
	if c.config.Auth == nil {
		c.logger.WarnContext(
			ctx,
			"rejecting connection, no CarrierAuth configured",
		)
		_ = conn.Close()
		return
	}
	stream, err := conn.AcceptStream()
	if err != nil {
		c.logger.WarnContext(
			ctx,
			"auth stream accept failed",
			auth.LogKeyReason,
			err.Error(),
		)
		_ = conn.Close()
		return
	}
	hs, err := readAuthHandshake(stream, conn)
	if err != nil {
		c.logger.WarnContext(
			ctx,
			"auth handshake read failed",
			auth.LogKeyReason,
			err.Error(),
		)
		_ = stream.Close()
		_ = conn.Close()
		return
	}
	_ = stream.Close()
	authCtx, err := c.config.Auth.VerifyPeerCert(hs)
	if err != nil {
		c.logger.WarnContext(
			ctx,
			"peer auth verification failed",
			auth.LogKeyReason,
			err.Error(),
		)
		_ = conn.Close()
		return
	}
	if err := c.registerPeer(
		authCtx,
		conn,
		hs.Certs,
		[]string{conn.RemoteAddr()},
	); err != nil {
		c.logger.WarnContext(
			ctx,
			"peer registration failed",
			auth.LogKeyReason,
			err.Error(),
		)
		_ = conn.Close()
		return
	}
	if err := c.writeLocalAuth(conn); err != nil {
		c.logger.WarnContext(
			ctx,
			"peer auth response failed",
			auth.LogKeyReason,
			err.Error(),
		)
		_ = conn.Close()
		return
	}
	c.logger.InfoContext(
		ctx,
		"peer authenticated and registered",
		auth.LogKeyNodeID,
		authCtx.NodeID.String(),
		auth.LogKeyScope,
		authCtx.EffectiveScope.String(),
	)
	c.startConnectionLoops(ctx, authCtx.NodeID, conn)
}

// getNodesUnlocked returns all registry nodes as PeerNode slices.
func (c *carrierImpl) getNodesUnlocked() []interfaces.PeerNode { // A
	nodes := c.registry.GetAllNodes()
	out := make([]interfaces.PeerNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, toPeerNode(node))
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

func (c *carrierImpl) connectionForNode( // A
	nodeID keys.NodeID,
) (interfaces.Connection, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conn, ok := c.connections[nodeID]
	if !ok {
		return nil, fmt.Errorf("node not connected")
	}
	return conn, nil
}

func (c *carrierImpl) awaitPeerAuth( // A
	conn interfaces.Connection,
) (auth.AuthContext, []auth.NodeCertLike, error) {
	if c.config.Auth == nil {
		return auth.AuthContext{}, nil, fmt.Errorf(
			"carrier auth is required to verify peers",
		)
	}
	stream, err := conn.AcceptStream()
	if err != nil {
		return auth.AuthContext{}, nil, fmt.Errorf(
			"accept peer auth stream: %w",
			err,
		)
	}
	defer func() { _ = stream.Close() }()
	hs, err := readAuthHandshake(stream, conn)
	if err != nil {
		return auth.AuthContext{}, nil, err
	}
	authCtx, err := c.config.Auth.VerifyPeerCert(hs)
	if err != nil {
		return auth.AuthContext{}, nil, err
	}
	return authCtx, hs.Certs, nil
}

func (c *carrierImpl) writeLocalAuth( // A
	conn interfaces.Connection,
) error {
	ni := c.config.NodeIdentity
	if ni == nil {
		return fmt.Errorf("node identity is not configured")
	}
	proof, sig, err := auth.SignDelegation(
		ni.Key(),
		ni.Certs(),
		ni.Session(),
		func(
			label string,
			ctx []byte,
			length int,
		) ([]byte, error) {
			return conn.ExportKeyingMaterial(label, ctx, length)
		},
	)
	if err != nil {
		return fmt.Errorf("sign delegation: %w", err)
	}
	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf("open auth response stream: %w", err)
	}
	defer func() { _ = stream.Close() }()
	return writeAuthHandshake(
		stream,
		ni.Certs(),
		ni.CASigs(),
		ni.Authorities(),
		proof,
		sig,
	)
}

func (c *carrierImpl) registerPeer( // A
	authCtx auth.AuthContext,
	conn interfaces.Connection,
	certs []auth.NodeCertLike,
	addresses []string,
) error {
	nodeCerts := make([]interfaces.NodeCert, len(certs))
	copy(nodeCerts, certs)
	node := interfaces.Node{
		NodeID:    authCtx.NodeID,
		Addresses: compactAddresses(addresses),
		NodeCerts: nodeCerts,
	}
	if err := c.registry.AddNode(node, nodeCerts, nil); err != nil {
		return err
	}
	c.mu.Lock()
	c.connections[authCtx.NodeID] = conn
	c.peerScopes[authCtx.NodeID] = authCtx.EffectiveScope
	c.mu.Unlock()
	return nil
}

func (c *carrierImpl) startConnectionLoops( // A
	ctx context.Context,
	nodeID keys.NodeID,
	conn interfaces.Connection,
) {
	// Streams and datagrams are only enabled after
	// the peer has been authenticated and registered.
	// This gates datagrams behind the same TLS-backed
	// connection established for the auth handshake.
	// Freshness and revocation re-checks are still a
	// separate hardening concern for long-lived peers.
	go c.handleReliableStreams(ctx, nodeID, conn)
	go c.handleDatagrams(ctx, nodeID, conn)
}

func (c *carrierImpl) handleReliableStreams( // A
	ctx context.Context,
	nodeID keys.NodeID,
	conn interfaces.Connection,
) {
	for {
		stream, err := conn.AcceptStream()
		if err != nil {
			c.dropConnection(nodeID, conn)
			return
		}
		go c.handleMessageStream(ctx, nodeID, stream)
	}
}

func (c *carrierImpl) handleDatagrams( // A
	ctx context.Context,
	nodeID keys.NodeID,
	conn interfaces.Connection,
) {
	// Datagrams inherit transport confidentiality and
	// integrity from the authenticated QUIC/TLS session,
	// but they remain unordered and best-effort. Higher
	// layers must treat them as unsuitable for operations
	// that require replay protection, ordering, or strong
	// backpressure unless those properties are added
	// separately at the message layer.
	for {
		data, err := conn.ReceiveDatagram()
		if err != nil {
			c.dropConnection(nodeID, conn)
			return
		}
		msg, err := unmarshalMessage(data)
		if err != nil {
			c.logger.WarnContext(
				ctx,
				"discarding invalid datagram",
				auth.LogKeyNodeID,
				nodeID.String(),
				auth.LogKeyReason,
				err.Error(),
			)
			continue
		}
		c.dispatchMessage(ctx, nodeID, msg)
	}
}

func (c *carrierImpl) handleMessageStream( // A
	ctx context.Context,
	nodeID keys.NodeID,
	stream interfaces.Stream,
) {
	defer func() { _ = stream.Close() }()
	msg, err := readMessageStream(stream)
	if err != nil {
		c.logger.WarnContext(
			ctx,
			"discarding invalid stream message",
			auth.LogKeyNodeID,
			nodeID.String(),
			auth.LogKeyReason,
			err.Error(),
		)
		return
	}
	c.dispatchMessage(ctx, nodeID, msg)
}

func (c *carrierImpl) dispatchMessage( // A
	ctx context.Context,
	nodeID keys.NodeID,
	msg interfaces.Message,
) {
	c.mu.RLock()
	controller := c.controller
	scope := c.peerScopes[nodeID]
	c.mu.RUnlock()
	if controller == nil {
		c.logger.WarnContext(
			ctx,
			"no cluster controller configured",
			auth.LogKeyNodeID,
			nodeID.String(),
			logKeyMessageType,
			int(msg.Type),
		)
		return
	}
	if _, err := controller.HandleIncomingMessage(
		msg,
		nodeID,
		scope,
	); err != nil {
		c.logger.WarnContext(
			ctx,
			"message dispatch failed",
			auth.LogKeyNodeID,
			nodeID.String(),
			logKeyMessageType,
			int(msg.Type),
			auth.LogKeyReason,
			err.Error(),
		)
	}
}

func (c *carrierImpl) dropConnection( // A
	nodeID keys.NodeID,
	conn interfaces.Connection,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, ok := c.connections[nodeID]
	if !ok || current != conn {
		return
	}
	delete(c.connections, nodeID)
	delete(c.peerScopes, nodeID)
	_ = conn.Close()
}

func peerAddress( // A
	peer interfaces.PeerNode,
	conn interfaces.Connection,
) string {
	if len(peer.Addresses) > 0 && peer.Addresses[0] != "" {
		return peer.Addresses[0]
	}
	return conn.RemoteAddr()
}

func compactAddresses(addresses []string) []string { // A
	out := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		out = append(out, address)
	}
	return out
}
