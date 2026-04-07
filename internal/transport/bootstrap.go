package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// runBootstrapLoop attempts to connect to all
// configured bootstrap addresses. If no peers are
// reachable after BootstrapMaxRetries rounds the
// node transitions to offline mode.
func (c *carrierImpl) runBootstrapLoop( // A
	ctx context.Context,
) {
	addrs := c.config.BootstrapAddresses
	if len(addrs) == 0 {
		return
	}
	c.attemptBootstrap(ctx, addrs)
}

// attemptBootstrap runs the retry loop for the
// given bootstrap addresses. Returns true if at
// least one peer was connected.
func (c *carrierImpl) attemptBootstrap( // A
	ctx context.Context,
	addrs []string,
) bool {
	maxRetries := c.bootstrapMaxRetries()
	interval := c.bootstrapRetryInterval()
	for attempt := range maxRetries + 1 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(interval):
			}
		}
		connected := c.dialBootstrapAll(ctx, addrs)
		if connected > 0 {
			c.logger.DebugContext(
				ctx,
				"bootstrap connected",
				logKeyPeers,
				connected,
				logKeyAttempt,
				attempt+1,
			)
			return true
		}
		c.logger.WarnContext(
			ctx,
			"bootstrap attempt failed",
			logKeyAttempt,
			attempt+1,
			logKeyMaxAttempts,
			maxRetries+1,
		)
	}
	c.logger.ErrorContext(
		ctx,
		"bootstrap exhausted, node offline",
	)
	c.setOffline()
	return false
}

// dialBootstrapAll tries every bootstrap address
// once and returns the count of successful peers.
func (c *carrierImpl) dialBootstrapAll( // A
	ctx context.Context,
	addrs []string,
) int {
	connected := 0
	for _, addr := range addrs {
		err := c.dialBootstrapAddr(addr)
		if err != nil {
			c.logger.WarnContext(
				ctx,
				"bootstrap dial failed",
				logKeyAddress,
				addr,
				auth.LogKeyReason,
				err.Error(),
			)
			continue
		}
		connected++
	}
	return connected
}

// dialBootstrapAddr dials a single bootstrap
// address, performs mutual auth, and registers
// the discovered peer. The NodeID is unknown
// until the auth handshake completes.
func (c *carrierImpl) dialBootstrapAddr( // A
	addr string,
) error {
	ni := c.config.NodeIdentity
	if ni == nil {
		return fmt.Errorf(
			"%w", ErrNodeIdentityRequired,
		)
	}
	c.mu.RLock()
	tp := c.transport
	c.mu.RUnlock()
	if tp == nil {
		return fmt.Errorf("%w", ErrTransportNotInitialized)
	}
	conn, err := tp.Dial(interfaces.Node{
		Addresses: []string{addr},
	})
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	if err := c.dialAndAuth(conn, ni); err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"auth %s: %w: %w", addr,
			ErrBootstrapDialAuthFailed, err,
		)
	}
	authCtx, certs, err := c.awaitPeerAuth(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"verify %s: %w: %w", addr,
			ErrBootstrapVerifyFailed, err,
		)
	}
	if c.IsConnected(authCtx.NodeID) {
		_ = conn.Close()
		return nil
	}
	if err := c.registerPeer(
		authCtx, conn, certs, []string{addr},
	); err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"register %s: %w: %w", addr,
			ErrBootstrapRegisterFailed, err,
		)
	}
	c.startConnectionLoops(
		context.Background(),
		authCtx.NodeID,
		conn,
	)
	c.logger.InfoContext(
		context.Background(),
		"bootstrap peer connected",
		auth.LogKeyNodeID,
		shortID(authCtx.NodeID),
		logKeyAddress,
		addr,
	)
	return nil
}

func (c *carrierImpl) bootstrapRetryInterval() time.Duration { // A
	if c.config.BootstrapRetryInterval > 0 {
		return c.config.BootstrapRetryInterval
	}
	return defaultBootstrapRetryInterval
}

func (c *carrierImpl) bootstrapMaxRetries() int { // A
	if c.config.BootstrapMaxRetries > 0 {
		return c.config.BootstrapMaxRetries
	}
	return defaultBootstrapMaxRetries
}
