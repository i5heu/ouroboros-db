// Package carrier implements the Carrier interface for inter-node communication
// in the OuroborosDB cluster.
//
// The Carrier is responsible for:
//   - Managing the list of known cluster nodes
//   - Broadcasting messages to all nodes
//   - Sending messages to specific nodes
//   - Handling cluster join/leave operations
//
// Communication between nodes uses the QUIC protocol for reliable,
// multiplexed streams with built-in encryption. Each node has its own
// cryptographic identity using post-quantum algorithms (ML-KEM for key
// encapsulation, ML-DSA for signatures) from ouroboros-crypt.
//
// It works in conjunction with the BootStrapper to initialize new nodes and
// uses the Message protocol for all inter-node communication.
//
// File organization:
//   - carrier.go: DefaultCarrier implementation (this file)
//   - carrier_config.go: Config and QUICConfig types
//   - interfaces.go: Carrier, Transport, Connection, Listener interfaces
//   - message.go: Message, EncryptedMessage, BroadcastResult types
//   - node.go: Node, NodeID, NodeCert, NodeIdentity types
//   - types.go: MessageType enum and constants
package carrier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// DefaultCarrier is the default implementation of the Carrier interface.
// It uses QUIC for transport and post-quantum encryption for message security.
type DefaultCarrier struct { // A
	localNode    Node
	nodeIdentity *NodeIdentity
	log          *slog.Logger
	transport    Transport
	bootStrapper BootStrapper
	qConfig      QUICConfig

	mu    sync.RWMutex
	nodes map[NodeID]Node

	// Listener state
	listener   Listener
	listenerMu sync.Mutex
	running    bool
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Message handlers
	handlersMu sync.RWMutex
	handlers   map[MessageType][]MessageHandler
}

// NewDefaultCarrier creates a new DefaultCarrier with the given configuration.
// Each node must have its own NodeIdentity for encryption and signing.
func NewDefaultCarrier(cfg Config) (*DefaultCarrier, error) { // A
	if err := cfg.LocalNode.Validate(); err != nil {
		return nil, fmt.Errorf("invalid local node: %w", err)
	}
	if cfg.NodeIdentity == nil {
		return nil, errors.New("node identity is required for encryption")
	}
	if cfg.Transport == nil {
		return nil, errors.New("transport is required")
	}
	if cfg.Logger == nil {
		return nil, errors.New("logger is required")
	}

	qConfig := cfg.QUICConfig
	if qConfig.MaxIdleTimeout == 0 {
		qConfig = DefaultQUICConfig()
	}

	c := &DefaultCarrier{
		localNode:    cfg.LocalNode,
		nodeIdentity: cfg.NodeIdentity,
		log:          cfg.Logger,
		transport:    cfg.Transport,
		bootStrapper: cfg.BootStrapper,
		qConfig:      qConfig,
		nodes:        make(map[NodeID]Node),
		handlers:     make(map[MessageType][]MessageHandler),
	}

	// Add self to known nodes
	c.nodes[cfg.LocalNode.NodeID] = cfg.LocalNode

	return c, nil
}

// GetNodes returns all known nodes in the cluster.
func (c *DefaultCarrier) GetNodes(ctx context.Context) ([]Node, error) { // A
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// Broadcast sends a message to all nodes in the cluster.
func (c *DefaultCarrier) Broadcast(
	ctx context.Context,
	message Message,
) (*BroadcastResult, error) { // A
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
func (c *DefaultCarrier) SendMessageToNode(
	ctx context.Context,
	nodeID NodeID,
	message Message,
) error { // A
	c.mu.RLock()
	node, ok := c.nodes[nodeID]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown node: %s", nodeID)
	}

	return c.sendToNode(ctx, node, message)
}

// sendToNode is the internal method for sending a message to a node.
func (c *DefaultCarrier) sendToNode(
	ctx context.Context,
	node Node,
	message Message,
) error { // A
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

// JoinCluster requests to join a cluster via the specified node.
func (c *DefaultCarrier) JoinCluster(
	ctx context.Context,
	clusterNode Node,
	cert NodeCert,
) error { // A
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

// LocalNode returns the local node's identity.
func (c *DefaultCarrier) LocalNode() Node { // A
	return c.localNode
}

// NodeIdentity returns the cryptographic identity of this node.
func (c *DefaultCarrier) NodeIdentity() *NodeIdentity { // A
	return c.nodeIdentity
}

// EncryptMessageFor encrypts a message for a specific recipient node.
func (c *DefaultCarrier) EncryptMessageFor(
	ctx context.Context,
	msg Message,
	recipient *Node,
) (*EncryptedMessage, error) { // A
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
func (c *DefaultCarrier) DecryptMessage(
	ctx context.Context,
	enc *EncryptedMessage,
	senderPub *keys.PublicKey,
) (Message, error) { // A
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

// GetNodePublicKey returns the public key for a known node.
func (c *DefaultCarrier) GetNodePublicKey(
	nodeID NodeID,
) (*keys.PublicKey, bool) { // A
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, ok := c.nodes[nodeID]
	if !ok {
		return nil, false
	}
	return node.PublicKey, node.PublicKey != nil
}

// RegisterHandler registers a handler for a specific message type.
// Multiple handlers can be registered for the same message type; they will
// be called in order of registration.
func (c *DefaultCarrier) RegisterHandler(
	msgType MessageType,
	handler MessageHandler,
) { // A
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()

	c.handlers[msgType] = append(c.handlers[msgType], handler)
	c.log.DebugContext(context.Background(), "registered message handler",
		logKeyMessageType, msgType.String())
}

// Start begins listening for incoming connections on the local node's address.
// It returns immediately; connections are handled in background goroutines.
func (c *DefaultCarrier) Start(ctx context.Context) error { // A
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()

	if c.running {
		return errors.New("carrier is already running")
	}

	if len(c.localNode.Addresses) == 0 {
		return errors.New("local node has no addresses to listen on")
	}

	// Use the first address for listening
	listenAddr := c.localNode.Addresses[0]

	listener, err := c.transport.Listen(ctx, listenAddr)
	if err != nil {
		return fmt.Errorf("failed to start listener on %s: %w", listenAddr, err)
	}

	c.listener = listener
	c.running = true
	c.stopCh = make(chan struct{})

	c.log.InfoContext(ctx, "carrier started listening",
		logKeyAddress, listenAddr)

	// Start the accept loop in a goroutine
	c.wg.Add(1)
	go c.acceptLoop(ctx)

	return nil
}

// Stop gracefully shuts down the carrier, closing all connections and
// stopping the listener.
func (c *DefaultCarrier) Stop(ctx context.Context) error { // A
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()

	if !c.running {
		return nil // Already stopped
	}

	c.log.InfoContext(ctx, "stopping carrier")

	// Signal the accept loop to stop
	close(c.stopCh)

	// Close the listener to unblock Accept()
	if c.listener != nil {
		if err := c.listener.Close(); err != nil {
			c.log.WarnContext(ctx, "error closing listener",
				logKeyError, err.Error())
		}
	}

	// Wait for all goroutines to finish
	c.wg.Wait()

	// Close the transport
	if err := c.transport.Close(); err != nil {
		c.log.WarnContext(ctx, "error closing transport",
			logKeyError, err.Error())
	}

	c.running = false
	c.listener = nil

	c.log.InfoContext(ctx, "carrier stopped")
	return nil
}

// acceptLoop continuously accepts incoming connections.
func (c *DefaultCarrier) acceptLoop(ctx context.Context) { // A
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		conn, err := c.listener.Accept(ctx)
		if err != nil {
			// Check if we're shutting down
			select {
			case <-c.stopCh:
				return
			default:
			}

			c.log.WarnContext(ctx, "error accepting connection",
				logKeyError, err.Error())
			continue
		}

		// Handle the connection in a new goroutine
		c.wg.Add(1)
		go c.handleConnection(ctx, conn)
	}
}

// handleConnection processes messages from an incoming connection.
func (c *DefaultCarrier) handleConnection(
	ctx context.Context,
	conn Connection,
) { // A
	defer c.wg.Done()
	defer func() {
		if err := conn.Close(); err != nil {
			c.log.DebugContext(ctx, "error closing connection",
				logKeyError, err.Error())
		}
	}()

	remoteID := conn.RemoteNodeID()
	c.log.DebugContext(ctx, "accepted connection",
		logKeyNodeID, string(remoteID))

	// Process messages until connection closes or carrier stops
	for {
		select {
		case <-c.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		if !c.processNextMessage(ctx, conn, remoteID) {
			return
		}
	}
}

// processNextMessage receives and handles a single message from a connection.
// Returns false if the connection should be closed.
func (c *DefaultCarrier) processNextMessage(
	ctx context.Context,
	conn Connection,
	remoteID NodeID,
) bool { // A
	msg, err := conn.Receive(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			c.log.DebugContext(ctx, "error receiving message",
				logKeyNodeID, string(remoteID),
				logKeyError, err.Error())
		}
		return false
	}

	response, err := c.dispatchMessage(ctx, remoteID, msg)
	if err != nil {
		c.log.WarnContext(ctx, "error handling message",
			logKeyNodeID, string(remoteID),
			logKeyMessageType, msg.Type.String(),
			logKeyError, err.Error())
		return true // continue processing despite handler error
	}

	if response != nil {
		if err := conn.Send(ctx, *response); err != nil {
			c.log.WarnContext(ctx, "error sending response",
				logKeyNodeID, string(remoteID),
				logKeyError, err.Error())
		}
	}
	return true
}

// dispatchMessage calls registered handlers for a message type.
func (c *DefaultCarrier) dispatchMessage(
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) { // A
	c.handlersMu.RLock()
	handlers := c.handlers[msg.Type]
	c.handlersMu.RUnlock()

	if len(handlers) == 0 {
		c.log.DebugContext(ctx, "no handlers for message type",
			logKeyMessageType, msg.Type.String(),
			logKeyNodeID, string(senderID))
		return nil, nil
	}

	var lastResponse *Message
	for _, handler := range handlers {
		response, err := handler(ctx, senderID, msg)
		if err != nil {
			return nil, err
		}
		if response != nil {
			lastResponse = response
		}
	}

	return lastResponse, nil
}

// IsRunning returns whether the carrier is currently running and accepting
// connections.
func (c *DefaultCarrier) IsRunning() bool { // A
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()
	return c.running
}

// Ensure DefaultCarrier implements Carrier interface.
var _ Carrier = (*DefaultCarrier)(nil)
