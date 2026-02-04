package carrier

import (
	"context"
	"errors"
	"fmt"
)

// Bootstrap attempts to join a cluster using the configured bootstrap
// addresses. If no bootstrap addresses were configured, this is a no-op
// and the node starts as a standalone cluster.
func (c *DefaultCarrier) Bootstrap(ctx context.Context) error { // A
	if len(c.bootstrapAddresses) == 0 {
		c.log.InfoContext(
			ctx,
			"no bootstrap addresses configured, starting as standalone",
		)
		return nil
	}
	return c.BootstrapFromAddresses(ctx, c.bootstrapAddresses)
}

// BootstrapFromAddresses attempts to join a cluster by connecting to one of
// the provided bootstrap addresses. It tries each address in order until one
// succeeds. The addresses can be in the format "host:port" or just "host"
// (in which case port 4242 is used by default).
func (c *DefaultCarrier) BootstrapFromAddresses( // A
	ctx context.Context,
	addresses []string,
) error {
	if len(addresses) == 0 {
		return errors.New("no bootstrap addresses provided")
	}

	c.log.InfoContext(ctx, "bootstrapping cluster from addresses",
		logKeyAddressCount, len(addresses))

	var lastErr error
	for _, addr := range addresses {
		// Normalize address: add default port if not specified
		normalizedAddr := normalizeAddress(addr, 4242)

		c.log.DebugContext(ctx, "attempting bootstrap from address",
			logKeyAddress, normalizedAddr)

		// Try to connect and get cluster info
		conn, err := c.transport.Connect(ctx, normalizedAddr)
		if err != nil {
			lastErr = fmt.Errorf("failed to connect to %s: %w", normalizedAddr, err)
			c.log.DebugContext(ctx, "bootstrap connection failed",
				logKeyAddress, normalizedAddr,
				logKeyError, err.Error())
			continue
		}

		// Send join request
		joinMsg := Message{
			Type:    MessageTypeNodeJoinRequest,
			Payload: nil, // TODO: serialize join request with local node info
		}

		if err := conn.Send(ctx, joinMsg); err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				c.log.DebugContext(ctx, "error closing connection",
					logKeyAddress, normalizedAddr,
					logKeyError, closeErr.Error())
			}
			lastErr = fmt.Errorf(
				"failed to send join request to %s: %w",
				normalizedAddr,
				err,
			)
			c.log.DebugContext(ctx, "bootstrap send failed",
				logKeyAddress, normalizedAddr,
				logKeyError, err.Error())
			continue
		}

		// Get the remote node info if available
		remoteID := conn.RemoteNodeID()
		remotePub := conn.RemotePublicKey()

		// Add the bootstrap node to known nodes if we have its info
		if remoteID != "" {
			bootstrapNode := Node{
				NodeID:    remoteID,
				Addresses: []string{normalizedAddr},
				PublicKey: remotePub,
			}
			c.mu.Lock()
			c.nodes[remoteID] = bootstrapNode
			c.mu.Unlock()

			// Add connection to pool for reuse by SyncNodes
			c.pool.addOutgoing(remoteID, conn)

			c.log.InfoContext(ctx, "added bootstrap node",
				logKeyNodeID, string(remoteID),
				logKeyAddress, normalizedAddr)

			// Sync node list from the bootstrap node
			if syncErr := c.SyncNodes(ctx, remoteID); syncErr != nil {
				c.log.WarnContext(ctx, "failed to sync nodes from bootstrap node",
					logKeyNodeID, string(remoteID),
					logKeyError, syncErr.Error())
			}

			// Announce ourselves to the cluster
			if announceErr := c.AnnounceSelf(ctx); announceErr != nil {
				c.log.WarnContext(ctx, "failed to announce self to cluster",
					logKeyError, announceErr.Error())
			}
		}

		// Don't close - the connection is now managed by the pool
		c.log.InfoContext(ctx, "successfully bootstrapped from address",
			logKeyAddress, normalizedAddr)

		// Now that bootstrap is complete, start receive loops for outgoing
		// connections to enable bidirectional communication
		c.pool.StartReceiveLoops()

		return nil
	}

	return fmt.Errorf("failed to bootstrap from any address: %w", lastErr)
}

// RequestNodeList requests the list of known nodes from a specific node.
// It uses the connection pool to reuse existing connections.
func (c *DefaultCarrier) RequestNodeList( // A
	ctx context.Context,
	nodeID NodeID,
) ([]Node, error) {
	c.mu.RLock()
	node, ok := c.nodes[nodeID]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown node: %s", nodeID)
	}

	if len(node.Addresses) == 0 {
		return nil, fmt.Errorf("node %s has no addresses", nodeID)
	}

	// Use pooled connection (don't close - pool manages lifecycle)
	conn, err := c.pool.getOrConnect(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	// Send node list request
	msg := Message{Type: MessageTypeNodeListRequest}
	if err := conn.Send(ctx, msg); err != nil {
		return nil, fmt.Errorf("failed to send node list request: %w", err)
	}

	// Receive response on the same connection
	// The server will send the response on a new stream back to us
	response, err := conn.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to receive node list response: %w", err)
	}

	if response.Type != MessageTypeNodeListResponse {
		return nil, fmt.Errorf("unexpected response type: %s", response.Type)
	}

	nodes, err := DeserializeNodeList(response.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize node list: %w", err)
	}

	return nodes, nil
}

// SyncNodes synchronizes the node list with a remote node, adding any
// nodes we don't know about.
func (c *DefaultCarrier) SyncNodes( // A
	ctx context.Context,
	nodeID NodeID,
) error {
	nodes, err := c.RequestNodeList(ctx, nodeID)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	added := 0
	for _, node := range nodes {
		if _, exists := c.nodes[node.NodeID]; !exists {
			c.nodes[node.NodeID] = node
			added++
			c.log.DebugContext(ctx, "synced new node",
				logKeyNodeID, string(node.NodeID))
		}
	}

	c.log.InfoContext(ctx, "node sync complete",
		logKeyViaNode, string(nodeID),
		logKeyNodesRecv, len(nodes),
		logKeyNodesAdded, added)

	return nil
}

// normalizeAddress ensures an address has a port. If no port is specified,
// the default port is appended.
func normalizeAddress(addr string, defaultPort uint16) string { // A
	// Check if address already has a port
	// Handle IPv6 addresses in brackets
	if hasPort(addr) {
		return addr
	}
	return fmt.Sprintf("%s:%d", addr, defaultPort)
}

// hasPort checks if an address string already contains a port.
func hasPort(addr string) bool { // A
	// Handle IPv6 addresses: [::1]:8080
	if len(addr) > 0 && addr[0] == '[' {
		// IPv6 in brackets - port comes after ]
		closeBracket := -1
		for i, c := range addr {
			if c == ']' {
				closeBracket = i
				break
			}
		}
		if closeBracket == -1 {
			return false // Malformed IPv6
		}
		// Check if there's a :port after the bracket
		return len(addr) > closeBracket+1 && addr[closeBracket+1] == ':'
	}
	// IPv4 or hostname: count colons
	colonCount := 0
	for _, c := range addr {
		if c == ':' {
			colonCount++
		}
	}
	// IPv4/hostname with port has exactly one colon
	// IPv6 without brackets has multiple colons (no port can be specified)
	return colonCount == 1
}
