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
