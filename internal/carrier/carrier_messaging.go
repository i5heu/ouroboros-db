package carrier

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// Broadcast sends a message to all nodes in the cluster.
func (c *DefaultCarrier) Broadcast( // A
	ctx context.Context,
	message Message,
) (*BroadcastResult, error) {
	c.mu.RLock()
	nodes := make([]Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		// Skip self
		if node.NodeID == c.localNode.NodeID {
			continue
		}
		nodes = append(nodes, node)
	}
	c.mu.RUnlock()

	result := &BroadcastResult{
		SuccessNodes: make([]Node, 0),
		FailedNodes:  make(map[NodeID]error),
	}

	if len(nodes) == 0 {
		c.log.DebugContext(ctx, "broadcast: no remote nodes to send to",
			logKeyMessageType, message.Type.String())
		return result, nil
	}

	var wg sync.WaitGroup
	var resultMu sync.Mutex

	for _, node := range nodes {
		wg.Add(1)
		go func(n Node) {
			defer wg.Done()

			err := c.sendToNode(ctx, n, message)

			resultMu.Lock()
			defer resultMu.Unlock()

			if err != nil {
				result.FailedNodes[n.NodeID] = err
				c.log.WarnContext(ctx, "broadcast: failed to send to node",
					logKeyNodeID, string(n.NodeID),
					logKeyError, err.Error())
			} else {
				result.SuccessNodes = append(result.SuccessNodes, n)
			}
		}(node)
	}

	wg.Wait()

	c.log.DebugContext(ctx, "broadcast complete",
		logKeyMessageType, message.Type.String(),
		logKeySuccessCount, len(result.SuccessNodes),
		logKeyFailedCount, len(result.FailedNodes))

	return result, nil
}

// SendMessageToNode sends a message to a specific node.
func (c *DefaultCarrier) SendMessageToNode( // A
	ctx context.Context,
	nodeID NodeID,
	message Message,
) error {
	c.mu.RLock()
	node, ok := c.nodes[nodeID]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown node: %s", nodeID)
	}

	return c.sendToNode(ctx, node, message)
}

// sendToNode is the internal method for sending a message to a node.
func (c *DefaultCarrier) sendToNode( // A
	ctx context.Context,
	node Node,
	message Message,
) error {
	if len(node.Addresses) == 0 {
		return fmt.Errorf("node %s has no addresses", node.NodeID)
	}

	var lastErr error
	for _, addr := range node.Addresses {
		conn, err := c.transport.Connect(ctx, addr)
		if err != nil {
			lastErr = err
			c.log.DebugContext(ctx, "failed to connect to address",
				logKeyNodeID, string(node.NodeID),
				logKeyAddress, addr,
				logKeyError, err.Error())
			continue
		}

		err = conn.Send(ctx, message)
		closeErr := conn.Close()

		if err != nil {
			lastErr = err
			continue
		}
		if closeErr != nil {
			c.log.DebugContext(ctx, "error closing connection",
				logKeyNodeID, string(node.NodeID),
				logKeyError, closeErr.Error())
		}

		return nil // Success
	}

	return fmt.Errorf(
		"failed to send message to node %s: %w",
		node.NodeID,
		lastErr,
	)
}

// EncryptMessageFor encrypts a message for a specific recipient node.
func (c *DefaultCarrier) EncryptMessageFor( // A
	ctx context.Context,
	msg Message,
	recipient *Node,
) (*EncryptedMessage, error) {
	if recipient.PublicKey == nil {
		return nil, fmt.Errorf("recipient %s has no public key", recipient.NodeID)
	}

	// Serialize the message (Type + Payload)
	data := make([]byte, 1+len(msg.Payload))
	data[0] = byte(msg.Type)
	copy(data[1:], msg.Payload)

	// Encrypt for the recipient
	enc, err := c.nodeIdentity.EncryptFor(data, recipient.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	// Sign the encrypted data
	sig, err := c.nodeIdentity.Sign(enc.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}

	return &EncryptedMessage{
		SenderID:  c.localNode.NodeID,
		Encrypted: enc,
		Signature: sig,
	}, nil
}

// DecryptMessage decrypts a message that was encrypted for this node.
func (c *DefaultCarrier) DecryptMessage( // A
	ctx context.Context,
	enc *EncryptedMessage,
	senderPub *keys.PublicKey,
) (Message, error) {
	// Verify the signature if we have the sender's public key
	if senderPub != nil {
		valid := senderPub.Verify(enc.Encrypted.Ciphertext, enc.Signature)
		if !valid {
			return Message{}, errors.New("invalid message signature")
		}
	}

	// Decrypt the message
	data, err := c.nodeIdentity.Decrypt(enc.Encrypted)
	if err != nil {
		return Message{}, fmt.Errorf("decryption failed: %w", err)
	}

	if len(data) < 1 {
		return Message{}, errors.New("decrypted message too short")
	}

	return Message{
		Type:    MessageType(data[0]),
		Payload: data[1:],
	}, nil
}
