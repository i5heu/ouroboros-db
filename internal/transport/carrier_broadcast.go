package transport

import (
	"errors"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func (c *carrierImpl) BroadcastReliable( // A
	message interfaces.Message,
) (success, failed []interfaces.PeerNode, err error) {
	c.mu.RLock()
	nodes := c.getNodesUnlocked()
	c.mu.RUnlock()
	if len(nodes) == 0 {
		return nil, nil, errors.New("no peers available")
	}
	maxRetries := c.config.BroadcastMaxRetries
	retryInterval := c.config.BroadcastRetryInterval
	pending := append(
		[]interfaces.PeerNode(nil), nodes...,
	)
	for attempt := range maxRetries + 1 {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}
		var stillFailed []interfaces.PeerNode
		for _, node := range pending {
			if sendErr := c.SendMessageToNodeReliable(
				node.NodeID,
				message,
			); sendErr != nil {
				stillFailed = append(
					stillFailed, node,
				)
				continue
			}
			success = append(success, node)
		}
		pending = stillFailed
		if len(pending) == 0 {
			break
		}
	}
	failed = pending
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
	if err := writeMessageStream(stream, message); err != nil {
		_ = stream.Close()
		return err
	}
	if err := stream.Close(); err != nil {
		return fmt.Errorf("close request stream: %w", err)
	}
	reply, err := readMessageReply(stream)
	if err != nil {
		return err
	}
	if !reply.Success {
		if reply.Error == "" {
			return fmt.Errorf(
				"message rejected by peer %s",
				nodeID.String(),
			)
		}
		return errors.New(reply.Error)
	}
	return nil
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
	data := marshalMessage(message)
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

func (c *carrierImpl) connectionForNode( // A
	nodeID keys.NodeID,
) (interfaces.Connection, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conn, ok := c.connections[nodeID]
	if !ok {
		return nil, errors.New("node not connected")
	}
	return conn, nil
}
