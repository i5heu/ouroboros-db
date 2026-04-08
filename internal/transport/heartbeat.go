package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
)

type heartbeatNodeEntry struct { // A
	NodeID    keys.NodeID
	Addresses []string
	Role      interfaces.NodeRole
}

type heartbeatPayload struct { // A
	SentAtUnix       int64
	SenderRole       interfaces.NodeRole
	KnownNodes       []heartbeatNodeEntry
	Stats            map[string]uint64
	StorageFreeBytes uint64
	CustomFields     map[string][]byte
}

func marshalHeartbeatPayload( // A
	payload heartbeatPayload,
) ([]byte, error) {
	nodes := make(
		[]*pb.HeartbeatNodeEntry,
		len(payload.KnownNodes),
	)
	for i, n := range payload.KnownNodes {
		nodes[i] = &pb.HeartbeatNodeEntry{
			NodeId:    n.NodeID[:],
			Addresses: n.Addresses,
			Role:      int32(n.Role), //nolint:gosec // G115: role enum fits in int32
		}
	}
	msg := &pb.HeartbeatPayload{
		SentAtUnix:       payload.SentAtUnix,
		SenderRole:       int32(payload.SenderRole), //nolint:gosec // G115: role enum fits in int32
		KnownNodes:       nodes,
		Stats:            payload.Stats,
		StorageFreeBytes: payload.StorageFreeBytes,
		CustomFields:     payload.CustomFields,
	}
	return proto.Marshal(msg)
}

func unmarshalHeartbeatPayload( // A
	data []byte,
) (heartbeatPayload, error) {
	var msg pb.HeartbeatPayload
	if err := proto.Unmarshal(data, &msg); err != nil {
		return heartbeatPayload{}, err
	}
	nodes := make(
		[]heartbeatNodeEntry,
		len(msg.KnownNodes),
	)
	for i, n := range msg.KnownNodes {
		var nodeID keys.NodeID
		copy(nodeID[:], n.NodeId)
		nodes[i] = heartbeatNodeEntry{
			NodeID:    nodeID,
			Addresses: n.Addresses,
			Role:      interfaces.NodeRole(n.Role),
		}
	}
	return heartbeatPayload{
		SentAtUnix:       msg.SentAtUnix,
		SenderRole:       interfaces.NodeRole(msg.SenderRole),
		KnownNodes:       nodes,
		Stats:            msg.Stats,
		StorageFreeBytes: msg.StorageFreeBytes,
		CustomFields:     msg.CustomFields,
	}, nil
}

func (c *carrierImpl) runHeartbeatLoop( // A
	ctx context.Context,
) {
	ticker := time.NewTicker(c.heartbeatInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendHeartbeatBatch(ctx)
			c.failStalePeers(time.Now())
		}
	}
}

func (c *carrierImpl) runReconnectLoop( // A
	ctx context.Context,
) {
	ticker := time.NewTicker(c.reconnectInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.retryUnreachablePeers(ctx)
		}
	}
}

func (c *carrierImpl) sendHeartbeatBatch( // A
	ctx context.Context,
) {
	for _, peer := range c.GetNodes() {
		if !c.shouldHeartbeatPeer(peer) {
			continue
		}
		if err := c.sendHeartbeat(ctx, peer.NodeID); err != nil {
			conn, connErr := c.connectionForNode(peer.NodeID)
			if connErr == nil {
				c.markPeerFailed(peer.NodeID, conn)
			}
			c.logger.WarnContext(
				ctx,
				"heartbeat send failed",
				auth.LogKeyNodeID,
				shortID(peer.NodeID),
				auth.LogKeyReason,
				err.Error(),
			)
		}
	}
}

func (c *carrierImpl) shouldHeartbeatPeer( // A
	peer interfaces.PeerNode,
) bool {
	return c.IsConnected(peer.NodeID) && peer.Role != interfaces.NodeRoleClient
}

func (c *carrierImpl) sendHeartbeat( // A
	_ context.Context,
	nodeID keys.NodeID,
) error {
	payload, err := c.buildHeartbeatPayload()
	if err != nil {
		return err
	}
	encoded, err := marshalHeartbeatPayload(payload)
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}
	return c.SendMessageToNodeReliable(nodeID, interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: encoded,
	})
}

func (c *carrierImpl) buildHeartbeatPayload( // A
) (heartbeatPayload, error) {
	knownNodes := c.heartbeatKnownNodes()
	stats := map[string]uint64{
		"connectedNodes": uint64( //nolint:gosec // G115: count is always non-negative
			c.connectedCount(),
		),
		"knownNodes": uint64(len(knownNodes)),
	}
	return heartbeatPayload{
		SentAtUnix: time.Now().Unix(),
		SenderRole: c.config.NodeRole,
		KnownNodes: knownNodes,
		Stats:      stats,
	}, nil
}

func (c *carrierImpl) heartbeatKnownNodes() []heartbeatNodeEntry { // A
	peers := c.registry.GetAllNodes()
	entries := make([]heartbeatNodeEntry, 0, len(peers)+1)
	entries = append(entries, heartbeatNodeEntry{
		NodeID:    c.selfNodeID(),
		Addresses: c.selfAddresses(),
		Role:      c.config.NodeRole,
	})
	for _, peer := range peers {
		entries = append(entries, heartbeatNodeEntry{
			NodeID:    peer.NodeID,
			Addresses: append([]string(nil), peer.Addresses...),
			Role:      peer.Role,
		})
	}
	return entries
}

func (c *carrierImpl) selfNodeID() keys.NodeID { // A
	return c.config.SelfCert.NodeID()
}

func (c *carrierImpl) selfAddresses() []string { // A
	addresses := []string{c.ListenAddress()}
	return compactAddresses(addresses)
}

func (c *carrierImpl) connectedCount() int { // A
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.connections)
}

func (c *carrierImpl) handleHeartbeatMessage( // A
	ctx context.Context,
	peerID keys.NodeID,
	msg interfaces.Message,
) error {
	payload, err := unmarshalHeartbeatPayload(msg.Payload)
	if err != nil {
		return fmt.Errorf("unmarshal heartbeat: %w", err)
	}
	c.markPeerSeen(peerID, time.Now())
	if err := c.registry.SetRole(peerID, payload.SenderRole); err != nil {
		return err
	}
	newPeers, mergeErr := c.mergeHeartbeatNodes(
		payload.KnownNodes,
	)
	if mergeErr != nil {
		return mergeErr
	}
	if len(newPeers) > 0 {
		go c.connectDiscoveredPeers(ctx, newPeers)
	}
	return nil
}

func (c *carrierImpl) mergeHeartbeatNodes( // A
	nodes []heartbeatNodeEntry,
) ([]interfaces.Node, error) {
	selfID := c.selfNodeID()
	var discovered []interfaces.Node
	for _, entry := range nodes {
		if entry.NodeID.IsZero() || entry.NodeID == selfID {
			continue
		}
		node, err := c.registry.GetNode(entry.NodeID)
		if err != nil {
			newNode := interfaces.Node{
				NodeID: entry.NodeID,
				Addresses: compactAddresses(
					entry.Addresses,
				),
				Role: entry.Role,
				ConnectionStatus: interfaces.
					ConnectionStatusDisconnected,
			}
			addErr := c.registry.AddNode(
				newNode, nil, nil,
			)
			if addErr != nil {
				return discovered, addErr
			}
			discovered = append(discovered, newNode)
			continue
		}
		node.Addresses = compactAddresses(append(
			node.Addresses,
			entry.Addresses...,
		))
		node.Role = entry.Role
		if addErr := c.registry.AddNode(
			node,
			node.NodeCerts,
			nil,
		); addErr != nil {
			return discovered, addErr
		}
	}
	return discovered, nil
}

// connectDiscoveredPeers dials peers that were just
// learned via heartbeat gossip so that the mesh forms
// without waiting for the reconnect loop.
func (c *carrierImpl) connectDiscoveredPeers( // A
	ctx context.Context,
	peers []interfaces.Node,
) {
	for _, node := range peers {
		if node.Role == interfaces.NodeRoleClient {
			continue
		}
		if c.IsConnected(node.NodeID) {
			continue
		}
		if err := c.connectNode(node); err != nil {
			c.logger.WarnContext(
				ctx,
				"gossip peer connect failed",
				auth.LogKeyNodeID,
				shortID(node.NodeID),
				auth.LogKeyReason,
				err.Error(),
			)
			continue
		}
		c.logger.InfoContext(
			ctx,
			"gossip peer connected",
			auth.LogKeyNodeID,
			shortID(node.NodeID),
		)
	}
}

func (c *carrierImpl) failStalePeers( // A
	now time.Time,
) {
	peers := c.registry.GetAllNodes()
	for _, peer := range peers {
		if peer.LastSeen.IsZero() {
			continue
		}
		if now.Sub(peer.LastSeen) <= c.heartbeatTimeout() {
			continue
		}
		conn, err := c.connectionForNode(peer.NodeID)
		if err != nil {
			continue
		}
		c.markPeerFailed(peer.NodeID, conn)
		c.logger.WarnContext(
			context.Background(),
			"peer heartbeat timed out",
			auth.LogKeyNodeID,
			shortID(peer.NodeID),
		)
	}
}

func (c *carrierImpl) retryUnreachablePeers( // A
	ctx context.Context,
) {
	for _, node := range c.registry.GetUnreachableNodes() {
		if node.Role == interfaces.NodeRoleClient {
			continue
		}
		if c.IsConnected(node.NodeID) {
			continue
		}
		if err := c.connectNode(node); err != nil {
			c.logger.WarnContext(
				ctx,
				"peer reconnect failed",
				auth.LogKeyNodeID,
				shortID(node.NodeID),
				auth.LogKeyReason,
				err.Error(),
			)
			continue
		}
		c.logger.InfoContext(
			ctx,
			"peer reconnected",
			auth.LogKeyNodeID,
			shortID(node.NodeID),
		)
	}
}

func (c *carrierImpl) heartbeatInterval() time.Duration { // A
	if c.config.HeartbeatInterval > 0 {
		return c.config.HeartbeatInterval
	}
	return defaultHeartbeatInterval
}

func (c *carrierImpl) heartbeatTimeout() time.Duration { // A
	if c.config.HeartbeatTimeout > 0 {
		return c.config.HeartbeatTimeout
	}
	return defaultHeartbeatTimeout
}

func (c *carrierImpl) reconnectInterval() time.Duration { // A
	if c.config.ReconnectInterval > 0 {
		return c.config.ReconnectInterval
	}
	return defaultReconnectInterval
}
