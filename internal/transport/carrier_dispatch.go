package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"google.golang.org/protobuf/proto"
)

// SetController wires the cluster controller used for
// inbound dispatch.
func (c *carrierImpl) SetController( // A
	controller interfaces.ClusterController,
) {
	c.mu.Lock()
	c.controller = controller
	c.mu.Unlock()
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
	responsePayload, dispatchErr := c.dispatchMessage(
		ctx,
		nodeID,
		msg,
	)
	if replyErr := writeMessageReply(
		stream,
		responsePayload,
		dispatchErr,
	); replyErr != nil {
		c.logger.WarnContext(
			ctx,
			"message reply failed",
			auth.LogKeyNodeID,
			nodeID.String(),
			auth.LogKeyReason,
			replyErr.Error(),
		)
	}
	if dispatchErr != nil {
		c.logger.WarnContext(
			ctx,
			"message dispatch failed",
			auth.LogKeyNodeID,
			nodeID.String(),
			logKeyMessageType,
			int(msg.Type),
			auth.LogKeyReason,
			dispatchErr.Error(),
		)
	}
}

func (c *carrierImpl) dispatchMessage( // A
	ctx context.Context,
	nodeID keys.NodeID,
	msg interfaces.Message,
) (proto.Message, error) {
	c.markPeerSeen(nodeID, time.Now())
	if msg.Type == interfaces.MessageTypeHeartbeat {
		if err := c.handleHeartbeatMessage(
			ctx,
			nodeID,
			msg,
		); err != nil {
			c.logger.WarnContext(
				ctx,
				"heartbeat handling failed",
				auth.LogKeyNodeID,
				nodeID.String(),
				auth.LogKeyReason,
				err.Error(),
			)
			return nil, err
		}
		return &interfaces.ResponseEmptyPayload{}, nil
	}
	c.mu.RLock()
	controller := c.controller
	scope := c.peerScopes[nodeID]
	c.mu.RUnlock()
	if controller == nil {
		return nil, fmt.Errorf(
			"no cluster controller configured for message type %d",
			msg.Type,
		)
	}
	responsePayload, err := controller.HandleIncomingMessage(
		msg,
		nodeID,
		scope,
	)
	return responsePayload, err
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
	_ = c.registry.SetStatus(
		nodeID,
		interfaces.ConnectionStatusDisconnected,
	)
}

func (c *carrierImpl) markPeerSeen( // A
	nodeID keys.NodeID,
	seenAt time.Time,
) {
	_ = c.registry.UpdateLastSeen(nodeID, seenAt)
	_ = c.registry.SetStatus(
		nodeID,
		interfaces.ConnectionStatusConnected,
	)
}

func (c *carrierImpl) markPeerFailed( // A
	nodeID keys.NodeID,
	conn interfaces.Connection,
) {
	c.dropConnection(nodeID, conn)
	_ = c.registry.SetStatus(
		nodeID,
		interfaces.ConnectionStatusFailed,
	)
}
