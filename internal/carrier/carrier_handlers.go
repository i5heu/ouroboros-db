package carrier

import (
	"context"
)

// RegisterDefaultHandlers registers the built-in message handlers for
// node announcements, join requests, and node list sync.
func (c *DefaultCarrier) RegisterDefaultHandlers() { // A
	// Handle new node announcements
	c.RegisterHandler(MessageTypeNewNodeAnnouncement, c.handleNodeAnnouncement)

	// Handle node list requests
	c.RegisterHandler(MessageTypeNodeListRequest, c.handleNodeListRequest)

	// Handle node join requests
	c.RegisterHandler(MessageTypeNodeJoinRequest, c.handleNodeJoinRequest)

	// Handle block distribution messages: announcements and slice deliveries
	c.RegisterHandler(MessageTypeBlockAnnouncement, c.handleBlockAnnouncement)
	c.RegisterHandler(MessageTypeBlockSliceDelivery, c.handleBlockSliceDelivery)

	// Accept log subscription requests (no-op unless LogBroadcaster present)
	c.RegisterHandler(MessageTypeLogSubscribe, c.handleLogSubscribe)

	// Handle dashboard announcements so nodes can subscribe to dashboards
	c.RegisterHandler(MessageTypeDashboardAnnounce, c.handleDashboardAnnounce)
}

// handleNodeAnnouncement processes incoming node announcements.
func (c *DefaultCarrier) handleNodeAnnouncement( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	ann, err := DeserializeNodeAnnouncement(msg.Payload)
	if err != nil {
		c.log.WarnContext(ctx, "failed to deserialize node announcement",
			logKeyNodeID, string(senderID),
			logKeyError, err.Error())
		return nil, err
	}

	// Don't add ourselves
	if ann.Node.NodeID == c.localNode.NodeID {
		return nil, nil
	}

	c.mu.Lock()
	_, exists := c.nodes[ann.Node.NodeID]
	if !exists {
		c.nodes[ann.Node.NodeID] = ann.Node
	}
	c.mu.Unlock()

	if !exists {
		c.log.InfoContext(ctx, "discovered new node via announcement",
			logKeyNodeID, string(ann.Node.NodeID),
			logKeyViaNode, string(senderID))
	}

	return nil, nil
}

// handleNodeListRequest responds with our list of known nodes.
func (c *DefaultCarrier) handleNodeListRequest( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	c.mu.RLock()
	nodes := make([]Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	c.mu.RUnlock()

	payload, err := SerializeNodeList(nodes)
	if err != nil {
		c.log.WarnContext(ctx, "failed to serialize node list",
			logKeyNodeID, string(senderID),
			logKeyError, err.Error())
		return nil, err
	}

	c.log.DebugContext(ctx, "responding to node list request",
		logKeyNodeID, string(senderID),
		logKeyNodeCount, len(nodes))

	return &Message{
		Type:    MessageTypeNodeListResponse,
		Payload: payload,
	}, nil
}

// handleNodeJoinRequest processes a node join request.
// It adds the requesting node and announces it to the cluster.
func (c *DefaultCarrier) handleNodeJoinRequest( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	c.log.InfoContext(ctx, "received node join request",
		logKeyNodeID, string(senderID))

	// For now, we auto-accept join requests
	// TODO: Implement user approval flow as per architecture

	// The sender's info should come from the connection or the message payload
	// For now, we just acknowledge that we received it
	// The actual node addition happens when we get their announcement

	return nil, nil
}

// handleBlockAnnouncement logs incoming block announcements.
func (c *DefaultCarrier) handleBlockAnnouncement( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	p, err := DeserializeBlockAnnouncement(msg.Payload)
	if err != nil {
		c.log.WarnContext(ctx, "failed to deserialize block announcement",
			logKeyNodeID, string(senderID),
			logKeyError, err.Error())
		return nil, err
	}

	c.log.DebugContext(ctx, "received block announcement",
		"blockHash", p.BlockHash.String(),
		"timestamp", p.Timestamp,
		"chunkCount", p.ChunkCount,
		"vertexCount", p.VertexCount,
		logKeyNodeID, string(senderID))

	return nil, nil
}

// handleBlockSliceDelivery logs incoming block slice deliveries.
func (c *DefaultCarrier) handleBlockSliceDelivery( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	p, err := DeserializeBlockSliceDelivery(msg.Payload)
	if err != nil {
		c.log.WarnContext(ctx, "failed to deserialize block slice delivery",
			logKeyNodeID, string(senderID),
			logKeyError, err.Error())
		return nil, err
	}

	c.log.DebugContext(ctx, "received block slice",
		"sliceHash", p.Slice.Hash.String(),
		"blockHash", p.Slice.BlockHash.String(),
		"requestID", p.RequestID,
		logKeyNodeID, string(senderID))

	// Acknowledge receipt (best-effort)
	ack := BlockSliceAckPayload{
		SliceHash: p.Slice.Hash,
		BlockHash: p.Slice.BlockHash,
		RequestID: p.RequestID,
		Success:   true,
	}
	ackData, _ := SerializeBlockSliceAck(&ack)
	return &Message{Type: MessageTypeBlockSliceAck, Payload: ackData}, nil
}

// handleLogSubscribe accepts log subscription requests (no-op here) and logs them.
func (c *DefaultCarrier) handleLogSubscribe( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	c.log.DebugContext(ctx, "received log subscribe request",
		logKeyNodeID, string(senderID))
	// We don't send log entries unless LogBroadcaster is configured.
	return nil, nil
}

// handleDashboardAnnounce reacts to dashboard announcements by subscribing the
// local node to the announced dashboard's logs (best-effort).
func (c *DefaultCarrier) handleDashboardAnnounce( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	p, err := DeserializeDashboardAnnounce(msg.Payload)
	if err != nil {
		c.log.WarnContext(ctx, "failed to deserialize dashboard announce",
			logKeyNodeID, string(senderID),
			logKeyError, err.Error())
		return nil, err
	}

	c.log.InfoContext(ctx, "dashboard announced",
		logKeyNodeID, string(p.NodeID),
		logKeyAddress, p.DashboardAddress,
		"via", string(senderID))

	// Send a LogSubscribe to the announcing dashboard node so it will receive
	// forwarded logs from this node (if the dashboard registers a LogBroadcaster).
	payload := LogSubscribePayload{SubscriberNodeID: c.localNode.NodeID}
	data, err := SerializeLogSubscribe(payload)
	if err != nil {
		c.log.WarnContext(ctx, "serialize log subscribe failed",
			logKeyError, err.Error())
		return nil, err
	}

	msgOut := Message{Type: MessageTypeLogSubscribe, Payload: data}
	// Use the senderID (connection provenance) as the target dashboard node
	if err := c.SendMessageToNode(ctx, senderID, msgOut); err != nil {
		c.log.WarnContext(ctx, "failed to send log subscribe to dashboard",
			logKeyNodeID, string(senderID),
			logKeyError, err.Error())
	}
	return nil, nil
}
