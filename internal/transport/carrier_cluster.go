package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
	"github.com/i5heu/ouroboros-db/internal/auth/delegation"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

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

// StartListener accepts inbound QUIC connections on the
// configured ListenAddress.
func (c *carrierImpl) StartListener( // A
	ctx context.Context,
) error {
	c.ensureBackgroundLoops()
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
	c.ensureBackgroundLoops()
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
		"inbound peer connected",
		auth.LogKeyNodeID,
		shortID(authCtx.NodeID),
		auth.LogKeyScope,
		authCtx.EffectiveScope.String(),
	)
	c.startConnectionLoops(ctx, authCtx.NodeID, conn)
}

// dialAndAuth performs the prover-side auth handshake.
func (c *carrierImpl) dialAndAuth( // A
	conn interfaces.Connection,
	ni *certpkg.NodeIdentity,
) error {
	exporterFn := func(
		label string,
		ctx []byte,
		length int,
	) ([]byte, error) {
		return conn.ExportKeyingMaterial(label, ctx, length)
	}
	proof, sig, err := delegation.SignDelegation(
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

func (c *carrierImpl) awaitPeerAuth( // A
	conn interfaces.Connection,
) (auth.AuthContext, []canonical.NodeCertLike, error) {
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
	proof, sig, err := delegation.SignDelegation(
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
	certs []canonical.NodeCertLike,
	addresses []string,
) error {
	nodeCerts := make([]interfaces.NodeCert, len(certs))
	copy(nodeCerts, certs)
	seenAt := time.Now()
	node := interfaces.Node{
		NodeID:           authCtx.NodeID,
		Addresses:        compactAddresses(addresses),
		NodeCerts:        nodeCerts,
		Role:             interfaces.NodeRoleServer,
		LastSeen:         seenAt,
		ConnectionStatus: interfaces.ConnectionStatusConnected,
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
