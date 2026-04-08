package transport

import (
	"context"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func (c *carrierImpl) GetNodes() []interfaces.PeerNode { // A
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getNodesUnlocked()
}

func (c *carrierImpl) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.PeerNode, error) {
	node, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.PeerNode{}, err
	}
	return toPeerNode(&node), nil
}

func (c *carrierImpl) GetNodeConnection( // A
	nodeID keys.NodeID,
) (interfaces.NodeConnection, error) {
	node, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.NodeConnection{}, err
	}
	c.mu.RLock()
	conn := c.connections[nodeID]
	c.mu.RUnlock()
	return interfaces.NodeConnection{
		Peer: toPeerNode(&node),
		Conn: conn,
	}, nil
}

func (c *carrierImpl) OpenPeerChannel( // A
	peer *interfaces.PeerNode,
	_ interfaces.NodeCert,
) error {
	c.ensureBackgroundLoops()
	return c.connectNode(&interfaces.Node{
		NodeID:    peer.NodeID,
		Addresses: peer.Addresses,
		NodeCerts: nil,
		Role:      peer.Role,
	})
}

func (c *carrierImpl) connectNode( // A
	node *interfaces.Node,
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
	conn, usedAddress, err := c.dialKnownAddresses(node)
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
		[]string{usedAddress},
	); err != nil {
		_ = conn.Close()
		return fmt.Errorf("register peer: %w", err)
	}
	c.startConnectionLoops(
		context.Background(),
		authCtx.NodeID,
		conn,
	)
	c.logger.DebugContext(
		context.Background(),
		"outbound peer connected",
		auth.LogKeyNodeID,
		shortID(authCtx.NodeID),
	)
	return nil
}

// getNodesUnlocked returns all registry nodes as PeerNode slices.
func (c *carrierImpl) getNodesUnlocked() []interfaces.PeerNode { // A
	nodes := c.registry.GetAllNodes()
	out := make([]interfaces.PeerNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, toPeerNode(&node))
	}
	return out
}

// toPeerNode converts an interfaces.Node to a PeerNode.
func toPeerNode(n *interfaces.Node) interfaces.PeerNode { // A
	return interfaces.PeerNode{
		NodeID:    n.NodeID,
		Addresses: n.Addresses,
		Cert:      nil,
		Role:      n.Role,
	}
}

func (c *carrierImpl) IsConnected( // A
	nodeID keys.NodeID,
) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.connections[nodeID]
	return ok
}

func (c *carrierImpl) dialKnownAddresses( // A
	node *interfaces.Node,
) (interfaces.Connection, string, error) {
	if len(node.Addresses) == 0 {
		return nil, "", fmt.Errorf("node has no addresses")
	}
	var lastErr error
	for _, address := range compactAddresses(node.Addresses) {
		conn, err := c.transport.Dial(&interfaces.Node{
			NodeID:    node.NodeID,
			Addresses: []string{address},
			NodeCerts: node.NodeCerts,
			Role:      node.Role,
		})
		if err == nil {
			return conn, address, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no dial attempts were made")
	}
	return nil, "", lastErr
}
