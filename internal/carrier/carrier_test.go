package carrier

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
)

// mockTransport is a test double for the Transport interface.
type mockTransport struct { // A
	mu          sync.Mutex
	connections map[string]*mockConnection
	connectErr  error
}

func newMockTransport() *mockTransport { // A
	return &mockTransport{
		connections: make(map[string]*mockConnection),
	}
}

func (m *mockTransport) Connect(
	ctx context.Context,
	address string,
) (Connection, error) { // A
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connectErr != nil {
		return nil, m.connectErr
	}

	conn := &mockConnection{
		address:  address,
		messages: make([]Message, 0),
	}
	m.connections[address] = conn
	return conn, nil
}

func (m *mockTransport) Listen(ctx context.Context, address string) error { // A
	return nil
}

func (m *mockTransport) Close() error { // A
	return nil
}

func (m *mockTransport) setConnectError(err error) { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectErr = err
}

// mockConnection is a test double for the Connection interface.
type mockConnection struct { // A
	mu       sync.Mutex
	address  string
	nodeID   NodeID
	messages []Message
	sendErr  error
	closed   bool
}

func (c *mockConnection) Send(ctx context.Context, msg Message) error { // A
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sendErr != nil {
		return c.sendErr
	}
	c.messages = append(c.messages, msg)
	return nil
}

func (c *mockConnection) Receive(ctx context.Context) (Message, error) { // A
	return Message{}, errors.New("not implemented")
}

func (c *mockConnection) Close() error { // A
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *mockConnection) RemoteNodeID() NodeID { // A
	return c.nodeID
}

func testLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func TestNewDefaultCarrier(t *testing.T) { // A
	transport := newMockTransport()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				LocalNode: Node{
					NodeID:    "node-1",
					Addresses: []string{"localhost:8080"},
				},
				Logger:    testLogger(),
				Transport: transport,
			},
			wantErr: false,
		},
		{
			name: "missing node ID",
			cfg: Config{
				LocalNode: Node{
					Addresses: []string{"localhost:8080"},
				},
				Logger:    testLogger(),
				Transport: transport,
			},
			wantErr: true,
		},
		{
			name: "missing addresses",
			cfg: Config{
				LocalNode: Node{
					NodeID: "node-1",
				},
				Logger:    testLogger(),
				Transport: transport,
			},
			wantErr: true,
		},
		{
			name: "missing transport",
			cfg: Config{
				LocalNode: Node{
					NodeID:    "node-1",
					Addresses: []string{"localhost:8080"},
				},
				Logger: testLogger(),
			},
			wantErr: true,
		},
		{
			name: "missing logger",
			cfg: Config{
				LocalNode: Node{
					NodeID:    "node-1",
					Addresses: []string{"localhost:8080"},
				},
				Transport: transport,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			carrier, err := NewDefaultCarrier(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDefaultCarrier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && carrier == nil {
				t.Error("NewDefaultCarrier() returned nil carrier without error")
			}
		})
	}
}

func TestDefaultCarrier_GetNodes(t *testing.T) { // A
	transport := newMockTransport()
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		Logger:    testLogger(),
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Initially should have only the local node
	nodes, err := carrier.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("GetNodes() returned %d nodes, want 1", len(nodes))
	}
	if nodes[0].NodeID != "node-1" {
		t.Errorf(
			"GetNodes() first node ID = %q, want %q",
			nodes[0].NodeID,
			"node-1",
		)
	}

	// Add another node
	err = carrier.AddNode(ctx, Node{
		NodeID:    "node-2",
		Addresses: []string{"localhost:8081"},
	})
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	nodes, err = carrier.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() error = %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("GetNodes() returned %d nodes, want 2", len(nodes))
	}
}

func TestDefaultCarrier_AddNode(t *testing.T) { // A
	transport := newMockTransport()
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		Logger:    testLogger(),
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Valid node
	err = carrier.AddNode(ctx, Node{
		NodeID:    "node-2",
		Addresses: []string{"localhost:8081"},
	})
	if err != nil {
		t.Errorf("AddNode() error = %v for valid node", err)
	}

	// Invalid node (no ID)
	err = carrier.AddNode(ctx, Node{
		Addresses: []string{"localhost:8082"},
	})
	if err == nil {
		t.Error("AddNode() should return error for node without ID")
	}

	// Invalid node (no addresses)
	err = carrier.AddNode(ctx, Node{
		NodeID: "node-3",
	})
	if err == nil {
		t.Error("AddNode() should return error for node without addresses")
	}
}

func TestDefaultCarrier_RemoveNode(t *testing.T) { // A
	transport := newMockTransport()
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		Logger:    testLogger(),
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Add a node
	err = carrier.AddNode(ctx, Node{
		NodeID:    "node-2",
		Addresses: []string{"localhost:8081"},
	})
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	// Remove it
	carrier.RemoveNode(ctx, "node-2")

	nodes, err := carrier.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf(
			"After RemoveNode, GetNodes() returned %d nodes, want 1",
			len(nodes),
		)
	}

	// Try to remove local node (should be prevented)
	carrier.RemoveNode(ctx, "node-1")
	nodes, err = carrier.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf(
			"After trying to remove local node, GetNodes() returned %d nodes, want 1",
			len(nodes),
		)
	}
}

func TestDefaultCarrier_SendMessageToNode(t *testing.T) { // A
	transport := newMockTransport()
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		Logger:    testLogger(),
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Add a remote node
	err = carrier.AddNode(ctx, Node{
		NodeID:    "node-2",
		Addresses: []string{"localhost:8081"},
	})
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	// Send message to known node
	msg := Message{
		Type:    MessageTypeHeartbeat,
		Payload: []byte("ping"),
	}
	err = carrier.SendMessageToNode(ctx, "node-2", msg)
	if err != nil {
		t.Errorf("SendMessageToNode() error = %v", err)
	}

	// Verify the message was sent
	transport.mu.Lock()
	conn := transport.connections["localhost:8081"]
	transport.mu.Unlock()
	if conn == nil {
		t.Fatal("No connection was made to the node")
	}
	conn.mu.Lock()
	if len(conn.messages) != 1 {
		t.Errorf("Connection received %d messages, want 1", len(conn.messages))
	}
	if conn.messages[0].Type != MessageTypeHeartbeat {
		t.Errorf(
			"Message type = %v, want %v",
			conn.messages[0].Type,
			MessageTypeHeartbeat,
		)
	}
	conn.mu.Unlock()

	// Send message to unknown node
	err = carrier.SendMessageToNode(ctx, "unknown-node", msg)
	if err == nil {
		t.Error("SendMessageToNode() should return error for unknown node")
	}
}

func TestDefaultCarrier_Broadcast(t *testing.T) { // A
	transport := newMockTransport()
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		Logger:    testLogger(),
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Add remote nodes
	for i := 2; i <= 4; i++ {
		err = carrier.AddNode(ctx, Node{
			NodeID:    NodeID("node-" + string(rune('0'+i))),
			Addresses: []string{"localhost:808" + string(rune('0'+i))},
		})
		if err != nil {
			t.Fatalf("AddNode() error = %v", err)
		}
	}

	// Broadcast
	msg := Message{
		Type:    MessageTypeNewNodeAnnouncement,
		Payload: []byte("new node"),
	}
	result, err := carrier.Broadcast(ctx, msg)
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}

	// Should have sent to 3 nodes (excluding self)
	if len(result.SuccessNodes) != 3 {
		t.Errorf(
			"Broadcast() succeeded for %d nodes, want 3",
			len(result.SuccessNodes),
		)
	}
	if len(result.FailedNodes) != 0 {
		t.Errorf("Broadcast() failed for %d nodes, want 0", len(result.FailedNodes))
	}
}

func TestDefaultCarrier_BroadcastWithFailures(t *testing.T) { // A
	transport := newMockTransport()
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		Logger:    testLogger(),
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Add a remote node
	err = carrier.AddNode(ctx, Node{
		NodeID:    "node-2",
		Addresses: []string{"localhost:8081"},
	})
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	// Make transport fail
	transport.setConnectError(errors.New("connection refused"))

	msg := Message{
		Type:    MessageTypeHeartbeat,
		Payload: []byte("ping"),
	}
	result, err := carrier.Broadcast(ctx, msg)
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}

	if len(result.SuccessNodes) != 0 {
		t.Errorf(
			"Broadcast() succeeded for %d nodes, want 0",
			len(result.SuccessNodes),
		)
	}
	if len(result.FailedNodes) != 1 {
		t.Errorf("Broadcast() failed for %d nodes, want 1", len(result.FailedNodes))
	}
}

func TestMessageType_String(t *testing.T) { // A
	tests := []struct {
		mt   MessageType
		want string
	}{
		{MessageTypeSealedSlicePayloadRequest, "SealedSlicePayloadRequest"},
		{MessageTypeChunkMetaRequest, "ChunkMetaRequest"},
		{MessageTypeBlobMetaRequest, "BlobMetaRequest"},
		{MessageTypeHeartbeat, "Heartbeat"},
		{MessageTypeNodeJoinRequest, "NodeJoinRequest"},
		{MessageTypeNodeLeaveNotification, "NodeLeaveNotification"},
		{MessageTypeUserAuthDecision, "UserAuthDecision"},
		{MessageTypeNewNodeAnnouncement, "NewNodeAnnouncement"},
		{MessageTypeChunkPayloadRequest, "ChunkPayloadRequest"},
		{MessageTypeBlobPayloadRequest, "BlobPayloadRequest"},
		{MessageType(255), "Unknown(255)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mt.String(); got != tt.want {
				t.Errorf("MessageType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNode_Validate(t *testing.T) { // A
	tests := []struct {
		name    string
		node    Node
		wantErr bool
	}{
		{
			name: "valid node",
			node: Node{
				NodeID:    "node-1",
				Addresses: []string{"localhost:8080"},
			},
			wantErr: false,
		},
		{
			name: "valid node with multiple addresses",
			node: Node{
				NodeID:    "node-1",
				Addresses: []string{"localhost:8080", "192.168.1.1:8080"},
			},
			wantErr: false,
		},
		{
			name: "empty node ID",
			node: Node{
				NodeID:    "",
				Addresses: []string{"localhost:8080"},
			},
			wantErr: true,
		},
		{
			name: "no addresses",
			node: Node{
				NodeID:    "node-1",
				Addresses: []string{},
			},
			wantErr: true,
		},
		{
			name: "nil addresses",
			node: Node{
				NodeID:    "node-1",
				Addresses: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Node.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultCarrier_LocalNode(t *testing.T) { // A
	transport := newMockTransport()
	localNode := Node{
		NodeID:    "node-1",
		Addresses: []string{"localhost:8080"},
	}
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: localNode,
		Logger:    testLogger(),
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	got := carrier.LocalNode()
	if got.NodeID != localNode.NodeID {
		t.Errorf("LocalNode().NodeID = %v, want %v", got.NodeID, localNode.NodeID)
	}
	if len(got.Addresses) != len(localNode.Addresses) {
		t.Errorf(
			"LocalNode().Addresses length = %d, want %d",
			len(got.Addresses),
			len(localNode.Addresses),
		)
	}
}
