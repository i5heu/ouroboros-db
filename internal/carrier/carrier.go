// Package carrier implements the Carrier interface for inter-node communication
// in the OuroborosDB cluster.
//
// The Carrier is responsible for:
//   - Managing the list of known cluster nodes
//   - Broadcasting messages to all nodes
//   - Sending messages to specific nodes
//   - Handling cluster join/leave operations
//
// It works in conjunction with the BootStrapper to initialize new nodes and
// uses the Message protocol for all inter-node communication.
package carrier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// MessageType defines the type of message being sent between nodes.
type MessageType uint8 // A

const (
	// MessageTypeSealedSlicePayloadRequest requests a sealed slice payload.
	MessageTypeSealedSlicePayloadRequest MessageType = iota + 1
	// MessageTypeChunkMetaRequest requests chunk metadata.
	MessageTypeChunkMetaRequest
	// MessageTypeBlobMetaRequest requests blob metadata.
	MessageTypeBlobMetaRequest
	// MessageTypeHeartbeat is used for health monitoring and cluster status.
	MessageTypeHeartbeat
	// MessageTypeNodeJoinRequest is sent when a node wants to join the cluster.
	MessageTypeNodeJoinRequest
	// MessageTypeNodeLeaveNotification is sent when a node leaves the cluster.
	MessageTypeNodeLeaveNotification
	// MessageTypeUserAuthDecision communicates authentication decisions.
	MessageTypeUserAuthDecision
	// MessageTypeNewNodeAnnouncement announces a new node to the cluster.
	MessageTypeNewNodeAnnouncement
	// MessageTypeChunkPayloadRequest requests chunk data (reserved for future
	// use).
	MessageTypeChunkPayloadRequest
	// MessageTypeBlobPayloadRequest requests blob data (reserved for future use).
	MessageTypeBlobPayloadRequest
)

// Slog attribute keys used throughout the carrier package.
const (
	logKeyMessageType  = "messageType"
	logKeyNodeID       = "nodeId"
	logKeyAddress      = "address"
	logKeyError        = "error"
	logKeySuccessCount = "successCount"
	logKeyFailedCount  = "failedCount"
	logKeyViaNode      = "viaNode"
)

// messageTypeNames maps MessageType values to their string representations.
var messageTypeNames = map[MessageType]string{ // A
	MessageTypeSealedSlicePayloadRequest: "SealedSlicePayloadRequest",
	MessageTypeChunkMetaRequest:          "ChunkMetaRequest",
	MessageTypeBlobMetaRequest:           "BlobMetaRequest",
	MessageTypeHeartbeat:                 "Heartbeat",
	MessageTypeNodeJoinRequest:           "NodeJoinRequest",
	MessageTypeNodeLeaveNotification:     "NodeLeaveNotification",
	MessageTypeUserAuthDecision:          "UserAuthDecision",
	MessageTypeNewNodeAnnouncement:       "NewNodeAnnouncement",
	MessageTypeChunkPayloadRequest:       "ChunkPayloadRequest",
	MessageTypeBlobPayloadRequest:        "BlobPayloadRequest",
}

// String returns the string representation of a MessageType.
func (mt MessageType) String() string { // A
	if name, ok := messageTypeNames[mt]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", mt)
}

// Message represents a message exchanged between nodes in the cluster.
type Message struct { // A
	// Type identifies what kind of message this is.
	Type MessageType
	// Payload is the message data, format depends on Type.
	Payload []byte
}

// NodeID is a unique identifier for a node in the cluster.
type NodeID string // A

// NodeCert represents the certificate used to authenticate a node.
// This is a placeholder type that will be expanded when crypto integration
// is implemented.
type NodeCert struct { // A
	// PubKeyHash is the hash of the node's public key.
	PubKeyHash hash.Hash
	// CertData holds the raw certificate bytes.
	CertData []byte
}

// Node represents a node in the OuroborosDB cluster.
type Node struct { // A
	// NodeID is the unique identifier for this node.
	NodeID NodeID
	// Addresses are the network addresses where this node can be reached.
	Addresses []string
	// Cert is the node's certificate for authentication.
	Cert NodeCert
}

// Validate checks if the Node has valid configuration.
func (n Node) Validate() error { // A
	if n.NodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if len(n.Addresses) == 0 {
		return errors.New("node must have at least one address")
	}
	return nil
}

// BroadcastResult contains the result of a broadcast operation.
type BroadcastResult struct { // A
	// SuccessNodes are nodes that successfully received the message.
	SuccessNodes []Node
	// FailedNodes maps failed nodes to their error.
	FailedNodes map[NodeID]error
}

// Carrier defines the interface for inter-node communication.
type Carrier interface { // A
	// GetNodes returns all known nodes in the cluster.
	GetNodes(ctx context.Context) ([]Node, error)

	// Broadcast sends a message to all nodes in the cluster.
	// Returns the nodes that successfully received the message and any error.
	Broadcast(ctx context.Context, message Message) (*BroadcastResult, error)

	// SendMessageToNode sends a message to a specific node.
	SendMessageToNode(ctx context.Context, nodeID NodeID, message Message) error

	// JoinCluster requests to join a cluster via the specified node.
	JoinCluster(ctx context.Context, clusterNode Node, cert NodeCert) error

	// LeaveCluster notifies the cluster that this node is leaving.
	LeaveCluster(ctx context.Context) error
}

// BootStrapper handles the initialization of nodes joining the cluster.
type BootStrapper interface { // A
	// BootstrapNode initializes a node for cluster participation.
	BootstrapNode(ctx context.Context, node Node) error
}

// Transport defines the low-level network operations for the carrier.
// Implementations may use different protocols (TCP, QUIC, etc.).
type Transport interface { // A
	// Connect establishes a connection to a node.
	Connect(ctx context.Context, address string) (Connection, error)
	// Listen starts accepting incoming connections.
	Listen(ctx context.Context, address string) error
	// Close shuts down the transport.
	Close() error
}

// Connection represents a network connection to another node.
type Connection interface { // A
	// Send transmits a message over the connection.
	Send(ctx context.Context, msg Message) error
	// Receive waits for and returns the next message.
	Receive(ctx context.Context) (Message, error)
	// Close terminates the connection.
	Close() error
	// RemoteNodeID returns the ID of the connected node.
	RemoteNodeID() NodeID
}

// Config holds configuration for the DefaultCarrier.
type Config struct { // A
	// LocalNode is this node's identity.
	LocalNode Node
	// Logger is the structured logger for the carrier.
	Logger *slog.Logger
	// Transport is the network transport implementation.
	Transport Transport
	// BootStrapper handles node initialization.
	BootStrapper BootStrapper
}

// DefaultCarrier is the default implementation of the Carrier interface.
type DefaultCarrier struct { // A
	localNode    Node
	log          *slog.Logger
	transport    Transport
	bootStrapper BootStrapper

	mu    sync.RWMutex
	nodes map[NodeID]Node
}

// NewDefaultCarrier creates a new DefaultCarrier with the given configuration.
func NewDefaultCarrier(cfg Config) (*DefaultCarrier, error) { // A
	if err := cfg.LocalNode.Validate(); err != nil {
		return nil, fmt.Errorf("invalid local node: %w", err)
	}
	if cfg.Transport == nil {
		return nil, errors.New("transport is required")
	}
	if cfg.Logger == nil {
		return nil, errors.New("logger is required")
	}

	c := &DefaultCarrier{
		localNode:    cfg.LocalNode,
		log:          cfg.Logger,
		transport:    cfg.Transport,
		bootStrapper: cfg.BootStrapper,
		nodes:        make(map[NodeID]Node),
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

// Ensure DefaultCarrier implements Carrier interface.
var _ Carrier = (*DefaultCarrier)(nil)
