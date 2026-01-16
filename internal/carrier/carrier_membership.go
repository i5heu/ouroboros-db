package carrier

import (
	"context"
	"fmt"
)

// JoinCluster requests to join a cluster via the specified node.
func (c *DefaultCarrier) JoinCluster( // A
	ctx context.Context,
	clusterNode Node,
	cert NodeCert,
) error {
	if err := clusterNode.Validate(); err != nil {
		return fmt.Errorf("invalid cluster node: %w", err)
	}

	c.log.InfoContext(ctx, "attempting to join cluster",
		logKeyViaNode, string(clusterNode.NodeID))

	// Bootstrap the local node if bootstrapper is available
	if c.bootStrapper != nil {
		if err := c.bootStrapper.BootstrapNode(ctx, c.localNode); err != nil {
			return fmt.Errorf("bootstrap failed: %w", err)
		}
	}

	// Send join request to the cluster node
	joinMsg := Message{
		Type:    MessageTypeNodeJoinRequest,
		Payload: nil, // TODO: serialize join request with cert
	}

	if err := c.sendToNode(ctx, clusterNode, joinMsg); err != nil {
		return fmt.Errorf("failed to send join request: %w", err)
	}

	// Add the cluster node to known nodes
	c.mu.Lock()
	c.nodes[clusterNode.NodeID] = clusterNode
	c.mu.Unlock()

	c.log.InfoContext(ctx, "successfully joined cluster",
		logKeyViaNode, string(clusterNode.NodeID))

	return nil
}

// LeaveCluster notifies the cluster that this node is leaving.
func (c *DefaultCarrier) LeaveCluster(ctx context.Context) error { // A
	c.log.InfoContext(ctx, "leaving cluster",
		logKeyNodeID, string(c.localNode.NodeID))

	leaveMsg := Message{
		Type:    MessageTypeNodeLeaveNotification,
		Payload: nil, // TODO: serialize leave notification
	}

	result, err := c.Broadcast(ctx, leaveMsg)
	if err != nil {
		return fmt.Errorf("failed to broadcast leave notification: %w", err)
	}

	if len(result.FailedNodes) > 0 {
		c.log.WarnContext(ctx, "some nodes did not receive leave notification",
			logKeyFailedCount, len(result.FailedNodes))
	}

	// Clear all remote nodes
	c.mu.Lock()
	c.nodes = map[NodeID]Node{
		c.localNode.NodeID: c.localNode,
	}
	c.mu.Unlock()

	c.log.InfoContext(ctx, "left cluster successfully")
	return nil
}

// AddNode adds a node to the list of known nodes.
func (c *DefaultCarrier) AddNode(ctx context.Context, node Node) error { // A
	if err := node.Validate(); err != nil {
		return fmt.Errorf("invalid node: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.nodes[node.NodeID] = node
	c.log.DebugContext(ctx, "added node",
		logKeyNodeID, string(node.NodeID))

	return nil
}

// RemoveNode removes a node from the list of known nodes.
func (c *DefaultCarrier) RemoveNode(ctx context.Context, nodeID NodeID) { // A
	c.mu.Lock()
	defer c.mu.Unlock()

	// Don't allow removing self
	if nodeID == c.localNode.NodeID {
		c.log.WarnContext(ctx, "attempted to remove local node from known nodes")
		return
	}

	delete(c.nodes, nodeID)
	c.log.DebugContext(ctx, "removed node",
		logKeyNodeID, string(nodeID))
}

// AnnounceNode broadcasts a new node announcement to all known nodes.
// This should be called when a new node joins the cluster so all nodes
// can update their node stores.
func (c *DefaultCarrier) AnnounceNode( // A
	ctx context.Context,
	node Node,
) error {
	ann := &NodeAnnouncement{
		Node:      node,
		Timestamp: timeNow().UnixNano(),
	}

	payload, err := SerializeNodeAnnouncement(ann)
	if err != nil {
		return fmt.Errorf("failed to serialize node announcement: %w", err)
	}

	msg := Message{
		Type:    MessageTypeNewNodeAnnouncement,
		Payload: payload,
	}

	result, err := c.Broadcast(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to broadcast node announcement: %w", err)
	}

	c.log.InfoContext(ctx, "announced node to cluster",
		logKeyNodeID, string(node.NodeID),
		logKeySuccessCount, len(result.SuccessNodes),
		logKeyFailedCount, len(result.FailedNodes))

	return nil
}

// AnnounceSelf broadcasts this node's information to all known nodes.
func (c *DefaultCarrier) AnnounceSelf(ctx context.Context) error { // A
	return c.AnnounceNode(ctx, c.localNode)
}
