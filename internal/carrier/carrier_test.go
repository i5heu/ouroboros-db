package carrier

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// mockTransport is a test double for the Transport interface.
type mockTransport struct { // A
	mu          sync.Mutex
	connections map[string]*mockConnection
	connectErr  error
	// addrToNodeID maps addresses to node IDs for proper RemoteNodeID() returns
	addrToNodeID map[string]NodeID
}

func newMockTransport() *mockTransport { // A
	return &mockTransport{
		connections:  make(map[string]*mockConnection),
		addrToNodeID: make(map[string]NodeID),
	}
}

// registerNode maps an address to a node ID for connection validation.
func (m *mockTransport) registerNode(address string, nodeID NodeID) { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addrToNodeID[address] = nodeID
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

	// Look up the node ID for this address
	nodeID := m.addrToNodeID[address]

	conn := &mockConnection{
		address:  address,
		nodeID:   nodeID,
		messages: make([]Message, 0),
	}
	m.connections[address] = conn
	return conn, nil
}

func (m *mockTransport) Listen(
	ctx context.Context,
	address string,
) (Listener, error) { // A
	return &mockListener{address: address}, nil
}

// mockListener is a test double for the Listener interface.
type mockListener struct { // A
	address    string
	acceptErr  error
	closed     bool
	mu         sync.Mutex
	acceptChan chan Connection
}

func (l *mockListener) Accept(ctx context.Context) (Connection, error) { // A
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, errors.New("listener closed")
	}
	if l.acceptErr != nil {
		err := l.acceptErr
		l.mu.Unlock()
		return nil, err
	}
	l.mu.Unlock()

	// Block until context cancelled or connection arrives
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-l.acceptChan:
		return conn, nil
	}
}

func (l *mockListener) Addr() string { // A
	return l.address
}

func (l *mockListener) Close() error { // A
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

func (m *mockTransport) Close() error { // A
	return nil
}

func (m *mockTransport) SetLogger(logger *slog.Logger) {
}

func (m *mockTransport) setConnectError(err error) { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectErr = err
}

// mockConnection is a test double for the Connection interface.
type mockConnection struct { // A
	mu                sync.Mutex
	address           string
	nodeID            NodeID
	messages          []Message
	encryptedMessages []*EncryptedMessage
	sendErr           error
	closed            bool
	remotePubKey      *keys.PublicKey
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

func (c *mockConnection) SendEncrypted(
	ctx context.Context,
	enc *EncryptedMessage,
) error { // A
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sendErr != nil {
		return c.sendErr
	}
	c.encryptedMessages = append(c.encryptedMessages, enc)
	return nil
}

func (c *mockConnection) Receive(ctx context.Context) (Message, error) { // A
	return Message{}, errors.New("not implemented")
}

func (c *mockConnection) ReceiveEncrypted(
	ctx context.Context,
) (*EncryptedMessage, error) { // A
	return nil, errors.New("not implemented")
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

func (c *mockConnection) RemotePublicKey() *keys.PublicKey { // A
	return c.remotePubKey
}

func testLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// testNodeIdentity creates a test node identity.
func testNodeIdentity(t *testing.T) *NodeIdentity { // A
	t.Helper()
	ni, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("NewNodeIdentity() error = %v", err)
	}
	return ni
}

func TestNewDefaultCarrier(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

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
				NodeIdentity: nodeIdentity,
				Logger:       testLogger(),
				Transport:    transport,
			},
			wantErr: false,
		},
		{
			name: "missing node ID",
			cfg: Config{
				LocalNode: Node{
					Addresses: []string{"localhost:8080"},
				},
				NodeIdentity: nodeIdentity,
				Logger:       testLogger(),
				Transport:    transport,
			},
			wantErr: true,
		},
		{
			name: "missing addresses",
			cfg: Config{
				LocalNode: Node{
					NodeID: "node-1",
				},
				NodeIdentity: nodeIdentity,
				Logger:       testLogger(),
				Transport:    transport,
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
				NodeIdentity: nodeIdentity,
				Logger:       testLogger(),
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
				NodeIdentity: nodeIdentity,
				Transport:    transport,
			},
			wantErr: true,
		},
		{
			name: "missing node identity",
			cfg: Config{
				LocalNode: Node{
					NodeID:    "node-1",
					Addresses: []string{"localhost:8080"},
				},
				Logger:    testLogger(),
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
	nodeIdentity := testNodeIdentity(t)
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
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
	nodeIdentity := testNodeIdentity(t)
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
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
	nodeIdentity := testNodeIdentity(t)
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
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
	nodeIdentity := testNodeIdentity(t)
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Register node-2's address so transport returns correct RemoteNodeID
	transport.registerNode("localhost:8081", "node-2")

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
	nodeIdentity := testNodeIdentity(t)
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Register and add remote nodes
	for i := 2; i <= 4; i++ {
		nodeID := NodeID("node-" + string(rune('0'+i)))
		addr := "localhost:808" + string(rune('0'+i))
		transport.registerNode(addr, nodeID)
		err = carrier.AddNode(ctx, Node{
			NodeID:    nodeID,
			Addresses: []string{addr},
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
	nodeIdentity := testNodeIdentity(t)
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
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
	nodeIdentity := testNodeIdentity(t)
	localNode := Node{
		NodeID:    "node-1",
		Addresses: []string{"localhost:8080"},
	}
	carrier, err := NewDefaultCarrier(Config{
		LocalNode:    localNode,
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
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

// TestNodeIdentity tests the node identity creation and crypto operations.
func TestNodeIdentity(t *testing.T) { // A
	ni, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("NewNodeIdentity() error = %v", err)
	}

	if ni.Crypt == nil {
		t.Error("NodeIdentity.Crypt should not be nil")
	}
	// PublicKey is a struct field, not a method
	if ni.PublicKey.Equal(&keys.PublicKey{}) {
		t.Error("NodeIdentity.PublicKey should not be empty")
	}
}

// TestNodeIdentity_SignVerify tests signing and verification.
func TestNodeIdentity_SignVerify(t *testing.T) { // A
	ni, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("NewNodeIdentity() error = %v", err)
	}

	message := []byte("test message to sign")
	signature, err := ni.Sign(message)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify with our own public key (Verify only takes 2 args on NodeIdentity)
	valid := ni.Verify(message, signature)
	if !valid {
		t.Error("Verify() returned false for valid signature")
	}

	// Tamper with message
	tamperedMessage := []byte("tampered message")
	valid = ni.Verify(tamperedMessage, signature)
	if valid {
		t.Error("Verify() returned true for tampered message")
	}
}

// TestNodeIdentity_EncryptDecrypt tests encryption and decryption.
func TestNodeIdentity_EncryptDecrypt(t *testing.T) { // A
	// Create two node identities
	sender, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("NewNodeIdentity() for sender error = %v", err)
	}

	receiver, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("NewNodeIdentity() for receiver error = %v", err)
	}

	plaintext := []byte("secret message")

	// Sender encrypts for receiver (needs pointer to receiver's public key)
	ciphertext, err := sender.EncryptFor(plaintext, &receiver.PublicKey)
	if err != nil {
		t.Fatalf("EncryptFor() error = %v", err)
	}

	// Receiver decrypts
	decrypted, err := receiver.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted = %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestDefaultCarrier_SetLogger(t *testing.T) {
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)
	// Create an initial logger
	initialLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "node-1",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       initialLogger,
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	// Create a new "updated" logger
	newLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Call SetLogger
	carrier.SetLogger(newLogger)

	// Verify carrier's logger is updated (we can't easily access private field 'log',
	// but we can ensure the method runs without panic and sub-components are called.
	// Since we mock transport, we can verify SetLogger was called on transport if we wanted,
	// but mockTransport.SetLogger is a no-op currently.
	// However, we can check if it updates the components we can access or just ensure it doesn't panic.
	// For a real test of propagation, we'd need to inspect private fields or use a spy transport.
	// Since we are inside the 'carrier' package test, we DO have access to private fields!

	if carrier.log != newLogger {
		t.Error("SetLogger did not update carrier.log")
	}

	// Check connection pool logger
	// c.pool is private, but we are in package carrier so we can access it
	if carrier.pool == nil {
		t.Fatal("carrier.pool is nil")
	}
	// pool.log is private too
	// We need to cast pool.log to compare or check if it matches
	// pool.log is an interface, so direct comparison works if the underlying object is the same
	/*
		// NOTE: pool.log is 'interface{ Debug(...); Warn(...) }', matching slog.Logger satisfies this.
		// However, in conn_pool.go:
		// type connPool struct { log interface{...} ... }
		// so carrier.pool.log should be == newLogger
	*/
	// Reflection or just direct access since we are in same package
	// Actually we are in 'carrier' package so we can access private fields of carrier types
	if carrier.pool.log != newLogger { // direct access to private field
		t.Error("SetLogger did not update pool.log")
	}

	// Check transport logger
	// We used a mockTransport which has a SetLogger that does nothing.
	// But if we used a real transport or verified the call...
	// Let's rely on the fact that we saw the code call c.transport.SetLogger(logger).
	// But since mockTransport.SetLogger is a no-op in our mock, we can't check a side effect there easily
	// without updating the mock.
	// Let's update the mock to verify it received the call.
}

// TestDefaultCarrier_EncryptMessageFor tests message encryption between
// carriers.
func TestDefaultCarrier_EncryptMessageFor(t *testing.T) { // A
	transport := newMockTransport()
	senderIdentity := testNodeIdentity(t)
	receiverIdentity := testNodeIdentity(t)

	sender, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "sender-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: senderIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() for sender error = %v", err)
	}

	receiver, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "receiver-node",
			Addresses: []string{"localhost:8081"},
			PublicKey: &receiverIdentity.PublicKey,
		},
		NodeIdentity: receiverIdentity,
		Logger:       testLogger(),
		Transport:    newMockTransport(),
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() for receiver error = %v", err)
	}

	// Create a message
	msg := Message{
		Type:    MessageTypeHeartbeat,
		Payload: []byte("encrypted payload"),
	}

	ctx := context.Background()

	// Create a recipient node with the receiver's public key
	recipientNode := &Node{
		NodeID:    "receiver-node",
		Addresses: []string{"localhost:8081"},
		PublicKey: &receiverIdentity.PublicKey,
	}

	// Encrypt message for receiver
	encMsg, err := sender.EncryptMessageFor(ctx, msg, recipientNode)
	if err != nil {
		t.Fatalf("EncryptMessageFor() error = %v", err)
	}

	if encMsg.SenderID != "sender-node" {
		t.Errorf(
			"EncryptedMessage.SenderID = %q, want %q",
			encMsg.SenderID,
			"sender-node",
		)
	}

	// Decrypt message
	decMsg, err := receiver.DecryptMessage(ctx, encMsg, &senderIdentity.PublicKey)
	if err != nil {
		t.Fatalf("DecryptMessage() error = %v", err)
	}

	if decMsg.Type != msg.Type {
		t.Errorf("Decrypted message Type = %v, want %v", decMsg.Type, msg.Type)
	}
	if string(decMsg.Payload) != string(msg.Payload) {
		t.Errorf(
			"Decrypted message Payload = %q, want %q",
			decMsg.Payload,
			msg.Payload,
		)
	}
}

// TestDefaultCarrier_RegisterHandler tests registering message handlers.
func TestDefaultCarrier_RegisterHandler(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "test-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	handlerCalled := false
	handler := func(
		ctx context.Context,
		senderID NodeID,
		msg Message,
	) (*Message, error) {
		handlerCalled = true
		return &Message{Type: MessageTypeChunkMetaRequest}, nil
	}

	carrier.RegisterHandler(MessageTypeHeartbeat, handler)

	// Verify handler was registered
	carrier.handlersMu.RLock()
	_, exists := carrier.handlers[MessageTypeHeartbeat]
	carrier.handlersMu.RUnlock()

	if !exists {
		t.Error("Handler was not registered")
	}

	// Test dispatchMessage calls the handler
	ctx := context.Background()
	msg := Message{Type: MessageTypeHeartbeat, Payload: []byte("test")}
	response, err := carrier.dispatchMessage(ctx, "sender-node", msg)
	if err != nil {
		t.Errorf("dispatchMessage() error = %v", err)
	}

	if !handlerCalled {
		t.Error("Handler was not called")
	}

	if response == nil || response.Type != MessageTypeChunkMetaRequest {
		t.Error("Unexpected response from handler")
	}
}

// TestDefaultCarrier_RegisterHandler_NoHandler tests dispatching without a
// registered handler.
func TestDefaultCarrier_RegisterHandler_NoHandler(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "test-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()
	msg := Message{Type: MessageTypeHeartbeat, Payload: []byte("test")}

	// Should return nil response when no handler is registered
	response, err := carrier.dispatchMessage(ctx, "sender-node", msg)
	if err != nil {
		t.Errorf("dispatchMessage() error = %v", err)
	}

	if response != nil {
		t.Error("Expected nil response when no handler registered")
	}
}

// TestDefaultCarrier_StartStop tests starting and stopping the carrier.
func TestDefaultCarrier_StartStop(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "test-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Initially not running
	if carrier.IsRunning() {
		t.Error("Carrier should not be running initially")
	}

	// Start the carrier
	err = carrier.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !carrier.IsRunning() {
		t.Error("Carrier should be running after Start()")
	}

	// Starting again should return error
	err = carrier.Start(ctx)
	if err == nil {
		t.Error("Start() should return error when already running")
	}

	// Stop the carrier
	err = carrier.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if carrier.IsRunning() {
		t.Error("Carrier should not be running after Stop()")
	}

	// Stopping again should be idempotent (no error)
	err = carrier.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() should be idempotent, got error = %v", err)
	}
}

// TestDefaultCarrier_IsRunning tests the IsRunning method.
func TestDefaultCarrier_IsRunning(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "test-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	// Should be false initially
	if carrier.IsRunning() {
		t.Error("IsRunning() should return false initially")
	}
}

// TestDefaultQUICConfig tests the default QUIC configuration values.
func TestDefaultQUICConfig(t *testing.T) { // A
	cfg := DefaultQUICConfig()

	// Verify default values are sensible
	if cfg.MaxIdleTimeout != 30000 {
		t.Errorf("MaxIdleTimeout = %d, want 30000", cfg.MaxIdleTimeout)
	}
	if cfg.KeepAlivePeriod != 10000 {
		t.Errorf("KeepAlivePeriod = %d, want 10000", cfg.KeepAlivePeriod)
	}
	if cfg.MaxIncomingStreams != 100 {
		t.Errorf("MaxIncomingStreams = %d, want 100", cfg.MaxIncomingStreams)
	}
}

// TestQUICConfig_CustomValues tests custom QUIC configuration.
func TestQUICConfig_CustomValues(t *testing.T) { // A
	cfg := QUICConfig{
		MaxIdleTimeout:     60000,
		KeepAlivePeriod:    5000,
		MaxIncomingStreams: 200,
	}

	if cfg.MaxIdleTimeout != 60000 {
		t.Errorf("MaxIdleTimeout = %d, want 60000", cfg.MaxIdleTimeout)
	}
	if cfg.KeepAlivePeriod != 5000 {
		t.Errorf("KeepAlivePeriod = %d, want 5000", cfg.KeepAlivePeriod)
	}
	if cfg.MaxIncomingStreams != 200 {
		t.Errorf("MaxIncomingStreams = %d, want 200", cfg.MaxIncomingStreams)
	}
}

// TestListener_Interface tests the mockListener implementation.
func TestListener_Interface(t *testing.T) { // A
	listener := &mockListener{
		address:    "localhost:9000",
		acceptChan: make(chan Connection, 1),
	}

	// Test Addr()
	if addr := listener.Addr(); addr != "localhost:9000" {
		t.Errorf("Addr() = %q, want %q", addr, "localhost:9000")
	}

	// Test Close()
	if err := listener.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify closed state
	listener.mu.Lock()
	closed := listener.closed
	listener.mu.Unlock()
	if !closed {
		t.Error("listener should be closed after Close()")
	}
}

// TestListener_AcceptCancellation tests Accept behavior with context
// cancellation.
func TestListener_AcceptCancellation(t *testing.T) { // A
	listener := &mockListener{
		address:    "localhost:9000",
		acceptChan: make(chan Connection),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start Accept in goroutine
	done := make(chan struct{})
	var acceptErr error
	go func() {
		_, acceptErr = listener.Accept(ctx)
		close(done)
	}()

	// Cancel context
	cancel()

	// Wait for Accept to return
	<-done

	if !errors.Is(acceptErr, context.Canceled) {
		t.Errorf("Accept() error = %v, want context.Canceled", acceptErr)
	}
}

// TestListener_AcceptConnection tests Accept receiving a connection.
func TestListener_AcceptConnection(t *testing.T) { // A
	listener := &mockListener{
		address:    "localhost:9000",
		acceptChan: make(chan Connection, 1),
	}

	// Create a mock connection
	mockConn := &mockConnection{
		nodeID:  "remote-node",
		address: "localhost:9001",
	}

	// Send connection to accept channel
	listener.acceptChan <- mockConn

	ctx := context.Background()
	conn, err := listener.Accept(ctx)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}

	if conn.RemoteNodeID() != "remote-node" {
		t.Errorf("RemoteNodeID() = %q, want %q", conn.RemoteNodeID(), "remote-node")
	}
}

// TestListener_AcceptAfterClose tests Accept returns error when listener is
// closed.
func TestListener_AcceptAfterClose(t *testing.T) { // A
	listener := &mockListener{
		address:    "localhost:9000",
		acceptChan: make(chan Connection),
	}

	// Close the listener first
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Accept should return error
	ctx := context.Background()
	_, err := listener.Accept(ctx)
	if err == nil {
		t.Error("Accept() should return error when listener is closed")
	}
}

// TestTransport_Listen tests the Transport.Listen interface.
func TestTransport_Listen(t *testing.T) { // A
	transport := newMockTransport()
	ctx := context.Background()

	listener, err := transport.Listen(ctx, "localhost:8080")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	if listener == nil {
		t.Fatal("Listen() returned nil listener")
	}

	if addr := listener.Addr(); addr != "localhost:8080" {
		t.Errorf("Addr() = %q, want %q", addr, "localhost:8080")
	}

	if err := listener.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// TestEncryptedMessage_Fields tests EncryptedMessage struct fields.
func TestEncryptedMessage_Fields(t *testing.T) { // A
	senderIdentity := testNodeIdentity(t)
	receiverIdentity := testNodeIdentity(t)

	plaintext := []byte("test message")
	encrypted, err := senderIdentity.EncryptFor(
		plaintext,
		&receiverIdentity.PublicKey,
	)
	if err != nil {
		t.Fatalf("EncryptFor() error = %v", err)
	}

	signature, err := senderIdentity.Sign(encrypted.Ciphertext)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	encMsg := &EncryptedMessage{
		SenderID:  "sender-node",
		Encrypted: encrypted,
		Signature: signature,
	}

	if encMsg.SenderID != "sender-node" {
		t.Errorf("SenderID = %q, want %q", encMsg.SenderID, "sender-node")
	}
	if encMsg.Encrypted == nil {
		t.Error("Encrypted should not be nil")
	}
	if len(encMsg.Signature) == 0 {
		t.Error("Signature should not be empty")
	}
}

// TestConnection_SendEncrypted tests sending encrypted messages over a
// connection.
func TestConnection_SendEncrypted(t *testing.T) { // A
	senderIdentity := testNodeIdentity(t)
	receiverIdentity := testNodeIdentity(t)

	conn := &mockConnection{
		nodeID:       "receiver-node",
		address:      "localhost:9001",
		remotePubKey: &receiverIdentity.PublicKey,
	}

	plaintext := []byte("encrypted payload")
	encrypted, err := senderIdentity.EncryptFor(
		plaintext,
		&receiverIdentity.PublicKey,
	)
	if err != nil {
		t.Fatalf("EncryptFor() error = %v", err)
	}

	signature, err := senderIdentity.Sign(encrypted.Ciphertext)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	encMsg := &EncryptedMessage{
		SenderID:  "sender-node",
		Encrypted: encrypted,
		Signature: signature,
	}

	ctx := context.Background()
	if err := conn.SendEncrypted(ctx, encMsg); err != nil {
		t.Fatalf("SendEncrypted() error = %v", err)
	}

	// Verify message was stored
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.encryptedMessages) != 1 {
		t.Errorf(
			"encryptedMessages count = %d, want 1",
			len(conn.encryptedMessages),
		)
	}
}

// TestConnection_RemotePublicKey tests retrieving remote node's public key.
func TestConnection_RemotePublicKey(t *testing.T) { // A
	remoteIdentity := testNodeIdentity(t)

	conn := &mockConnection{
		nodeID:       "remote-node",
		address:      "localhost:9001",
		remotePubKey: &remoteIdentity.PublicKey,
	}

	pubKey := conn.RemotePublicKey()
	if pubKey == nil {
		t.Fatal("RemotePublicKey() returned nil")
	}

	// Verify we got the correct public key
	if pubKey != &remoteIdentity.PublicKey {
		t.Error("RemotePublicKey() returned wrong key")
	}
}

// TestCarrier_StartWithListener tests carrier start with transport listener.
func TestCarrier_StartWithListener(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "test-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	ctx := context.Background()

	// Start should use transport.Listen
	if err := carrier.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !carrier.IsRunning() {
		t.Error("Carrier should be running after Start()")
	}

	// Clean up
	if err := carrier.Stop(ctx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

// TestCarrier_MultipleHandlersForSameType tests multiple handlers for one
// message type.
func TestCarrier_MultipleHandlersForSameType(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "test-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	var handler1Called, handler2Called bool

	handler1 := func(
		ctx context.Context,
		senderID NodeID,
		msg Message,
	) (*Message, error) {
		handler1Called = true
		return nil, nil
	}

	handler2 := func(
		ctx context.Context,
		senderID NodeID,
		msg Message,
	) (*Message, error) {
		handler2Called = true
		return &Message{Type: MessageTypeChunkMetaRequest}, nil
	}

	carrier.RegisterHandler(MessageTypeHeartbeat, handler1)
	carrier.RegisterHandler(MessageTypeHeartbeat, handler2)

	// Dispatch a message
	ctx := context.Background()
	msg := Message{Type: MessageTypeHeartbeat}
	response, err := carrier.dispatchMessage(ctx, "sender-node", msg)
	if err != nil {
		t.Fatalf("dispatchMessage() error = %v", err)
	}

	if !handler1Called {
		t.Error("handler1 was not called")
	}
	if !handler2Called {
		t.Error("handler2 was not called")
	}

	// Last handler's response should be returned
	if response == nil || response.Type != MessageTypeChunkMetaRequest {
		t.Error("Expected last handler's response")
	}
}

// TestCarrier_ProcessNextMessage tests message processing helper.
func TestCarrier_ProcessNextMessage(t *testing.T) { // A
	transport := newMockTransport()
	nodeIdentity := testNodeIdentity(t)

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    "test-node",
			Addresses: []string{"localhost:8080"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       testLogger(),
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("NewDefaultCarrier() error = %v", err)
	}

	handlerCalled := false
	carrier.RegisterHandler(MessageTypeHeartbeat, func(
		ctx context.Context,
		senderID NodeID,
		msg Message,
	) (*Message, error) {
		handlerCalled = true
		return nil, nil
	})

	// Create mock connection with a message queue
	msgChan := make(chan Message, 1)
	conn := &mockConnectionWithReceive{
		mockConnection: mockConnection{
			nodeID:  "sender-node",
			address: "localhost:9001",
		},
		receiveChan: msgChan,
	}

	// Send a message
	msgChan <- Message{Type: MessageTypeHeartbeat, Payload: []byte("test")}

	ctx := context.Background()
	result := carrier.processNextMessage(ctx, conn, "sender-node")

	if !result {
		t.Error("processNextMessage() should return true for successful message")
	}
	if !handlerCalled {
		t.Error("Handler should have been called")
	}
}

// mockConnectionWithReceive extends mockConnection with working Receive.
type mockConnectionWithReceive struct { // A
	mockConnection
	receiveChan chan Message
	receiveErr  error
}

func (c *mockConnectionWithReceive) Receive(
	ctx context.Context,
) (Message, error) { // A
	if c.receiveErr != nil {
		return Message{}, c.receiveErr
	}
	select {
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case msg := <-c.receiveChan:
		return msg, nil
	}
}

// TestHasPort verifies the hasPort function correctly detects port presence.
func TestHasPort(t *testing.T) { // A
	tests := []struct {
		name     string
		addr     string
		expected bool
	}{
		{"IPv4 with port", "192.168.1.1:8080", true},
		{"IPv4 without port", "192.168.1.1", false},
		{"hostname with port", "example.com:4242", true},
		{"hostname without port", "example.com", false},
		{"localhost with port", "localhost:8080", true},
		{"localhost without port", "localhost", false},
		{"IPv6 bracketed with port", "[::1]:8080", true},
		{"IPv6 bracketed without port", "[::1]", false},
		{"IPv6 full bracketed with port", "[2001:db8::1]:8080", true},
		{"IPv6 full bracketed without port", "[2001:db8::1]", false},
		{"IPv6 unbracketed", "::1", false},
		{"IPv6 full unbracketed", "2001:db8::1", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasPort(tt.addr)
			if result != tt.expected {
				t.Errorf("hasPort(%q) = %v, want %v", tt.addr, result, tt.expected)
			}
		})
	}
}

// TestNormalizeAddress verifies address normalization with default ports.
func TestNormalizeAddress(t *testing.T) { // A
	tests := []struct {
		name        string
		addr        string
		defaultPort uint16
		expected    string
	}{
		{"IPv4 with port", "192.168.1.1:8080", 4242, "192.168.1.1:8080"},
		{"IPv4 without port", "192.168.1.1", 4242, "192.168.1.1:4242"},
		{"hostname with port", "example.com:8080", 4242, "example.com:8080"},
		{"hostname without port", "example.com", 4242, "example.com:4242"},
		{"localhost with port", "localhost:9000", 4242, "localhost:9000"},
		{"localhost without port", "localhost", 4242, "localhost:4242"},
		{"IPv6 bracketed with port", "[::1]:8080", 4242, "[::1]:8080"},
		{"IPv6 bracketed without port", "[::1]", 4242, "[::1]:4242"},
		{"different default port", "example.com", 5555, "example.com:5555"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeAddress(tt.addr, tt.defaultPort)
			if result != tt.expected {
				t.Errorf("normalizeAddress(%q, %d) = %q, want %q",
					tt.addr, tt.defaultPort, result, tt.expected)
			}
		})
	}
}

// TestBootstrapFromAddresses_NoAddresses verifies error when no addresses.
func TestBootstrapFromAddresses_NoAddresses(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	err = carrier.BootstrapFromAddresses(context.Background(), []string{})
	if err == nil {
		t.Error("Expected error when no bootstrap addresses provided")
	}
}

// TestBootstrap_NoConfiguredAddresses verifies no-op when no addresses
// configured.
func TestBootstrap_NoConfiguredAddresses(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
		// No BootstrapAddresses configured
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	// Should not error - just no-op
	err = carrier.Bootstrap(context.Background())
	if err != nil {
		t.Errorf("Bootstrap() should not error with no addresses: %v", err)
	}
}

// TestDefaultPortConfiguration verifies default port is set correctly.
func TestDefaultPortConfiguration(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	// Test default port when not configured
	carrier1, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	if carrier1.DefaultPort() != 4242 {
		t.Errorf("DefaultPort() = %d, want 4242", carrier1.DefaultPort())
	}

	// Test custom port
	carrier2, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
		DefaultPort:  5555,
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	if carrier2.DefaultPort() != 5555 {
		t.Errorf("DefaultPort() = %d, want 5555", carrier2.DefaultPort())
	}
}

// TestBootstrapAddressesConfiguration verifies bootstrap addresses are stored.
func TestBootstrapAddressesConfiguration(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	addresses := []string{"node1.example.com:4242", "node2.example.com"}

	carrier, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity:       nodeIdentity,
		Logger:             logger,
		Transport:          newMockTransport(),
		BootstrapAddresses: addresses,
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	got := carrier.BootstrapAddresses()
	if len(got) != len(addresses) {
		t.Errorf("BootstrapAddresses() len = %d, want %d", len(got), len(addresses))
	}
	for i, addr := range addresses {
		if got[i] != addr {
			t.Errorf("BootstrapAddresses()[%d] = %q, want %q", i, got[i], addr)
		}
	}
}

// TestRegisterDefaultHandlers verifies that default handlers are registered.
func TestRegisterDefaultHandlers(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	c, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	c.RegisterDefaultHandlers()

	// Check that handlers are registered by checking the handlers map
	c.handlersMu.RLock()
	defer c.handlersMu.RUnlock()

	if len(c.handlers[MessageTypeNewNodeAnnouncement]) == 0 {
		t.Error("Expected handler for MessageTypeNewNodeAnnouncement")
	}
	if len(c.handlers[MessageTypeNodeListRequest]) == 0 {
		t.Error("Expected handler for MessageTypeNodeListRequest")
	}
	if len(c.handlers[MessageTypeNodeJoinRequest]) == 0 {
		t.Error("Expected handler for MessageTypeNodeJoinRequest")
	}
}

// TestHandleNodeAnnouncement verifies node announcement handling.
func TestHandleNodeAnnouncement(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	c, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	c.RegisterDefaultHandlers()

	// Create a node announcement
	newNode := Node{
		NodeID:    "new-node-123",
		Addresses: []string{"192.168.1.100:4242"},
	}
	ann := &NodeAnnouncement{
		Node:      newNode,
		Timestamp: 1234567890,
	}
	payload, err := SerializeNodeAnnouncement(ann)
	if err != nil {
		t.Fatalf("Failed to serialize announcement: %v", err)
	}

	msg := Message{
		Type:    MessageTypeNewNodeAnnouncement,
		Payload: payload,
	}

	// Dispatch the message
	ctx := context.Background()
	_, err = c.dispatchMessage(ctx, "sender-node", msg)
	if err != nil {
		t.Fatalf("dispatchMessage() error = %v", err)
	}

	// Verify the node was added
	c.mu.RLock()
	_, exists := c.nodes[newNode.NodeID]
	c.mu.RUnlock()

	if !exists {
		t.Error("Expected new node to be added to nodes map")
	}
}

// TestHandleNodeAnnouncement_IgnoresSelf verifies self-announcements are
// ignored.
func TestHandleNodeAnnouncement_IgnoresSelf(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	c, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	c.RegisterDefaultHandlers()

	// Create a self-announcement (should be ignored)
	ann := &NodeAnnouncement{
		Node: Node{
			NodeID:    nodeID, // Same as local node
			Addresses: []string{"different-addr:4242"},
		},
		Timestamp: 1234567890,
	}
	payload, err := SerializeNodeAnnouncement(ann)
	if err != nil {
		t.Fatalf("Failed to serialize announcement: %v", err)
	}

	// Get initial node count
	c.mu.RLock()
	initialCount := len(c.nodes)
	c.mu.RUnlock()

	msg := Message{
		Type:    MessageTypeNewNodeAnnouncement,
		Payload: payload,
	}

	ctx := context.Background()
	_, err = c.dispatchMessage(ctx, "sender-node", msg)
	if err != nil {
		t.Fatalf("dispatchMessage() error = %v", err)
	}

	// Verify node count hasn't changed
	c.mu.RLock()
	finalCount := len(c.nodes)
	c.mu.RUnlock()

	if finalCount != initialCount {
		t.Errorf("Node count changed from %d to %d, expected no change",
			initialCount, finalCount)
	}
}

// TestHandleNodeListRequest verifies node list request handling.
func TestHandleNodeListRequest(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	c, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    newMockTransport(),
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	// Add some nodes
	ctx := context.Background()
	_ = c.AddNode(
		ctx,
		Node{NodeID: "node-2", Addresses: []string{"localhost:4243"}},
	)
	_ = c.AddNode(
		ctx,
		Node{NodeID: "node-3", Addresses: []string{"localhost:4244"}},
	)

	c.RegisterDefaultHandlers()

	// Send node list request
	msg := Message{Type: MessageTypeNodeListRequest}
	response, err := c.dispatchMessage(ctx, "requester-node", msg)
	if err != nil {
		t.Fatalf("dispatchMessage() error = %v", err)
	}

	if response == nil {
		t.Fatal("Expected response, got nil")
	}

	if response.Type != MessageTypeNodeListResponse {
		t.Errorf("Response type = %v, want %v",
			response.Type, MessageTypeNodeListResponse)
	}

	// Deserialize and verify
	nodes, err := DeserializeNodeList(response.Payload)
	if err != nil {
		t.Fatalf("DeserializeNodeList() error = %v", err)
	}

	// Should have 3 nodes (self + 2 added)
	if len(nodes) != 3 {
		t.Errorf("Node count = %d, want 3", len(nodes))
	}
}

// TestAnnounceNode verifies node announcement broadcast.
func TestAnnounceNode(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	transport := newMockTransport()
	c, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	// Register and add a remote node to broadcast to
	ctx := context.Background()
	remoteNode := Node{
		NodeID:    "remote-node",
		Addresses: []string{"localhost:4243"},
	}
	transport.registerNode("localhost:4243", remoteNode.NodeID)
	_ = c.AddNode(ctx, remoteNode)

	// Announce a new node
	newNode := Node{
		NodeID:    "announced-node",
		Addresses: []string{"192.168.1.50:4242"},
	}

	err = c.AnnounceNode(ctx, newNode)
	if err != nil {
		t.Fatalf("AnnounceNode() error = %v", err)
	}

	// Verify a message was sent to the remote node
	transport.mu.Lock()
	conn, exists := transport.connections["localhost:4243"]
	transport.mu.Unlock()

	if !exists {
		t.Error("Expected connection to remote node")
	}

	if len(conn.messages) == 0 {
		t.Error("Expected message to be sent")
	}

	if conn.messages[0].Type != MessageTypeNewNodeAnnouncement {
		t.Errorf("Message type = %v, want %v",
			conn.messages[0].Type, MessageTypeNewNodeAnnouncement)
	}
}

// TestAnnounceSelf verifies self-announcement.
func TestAnnounceSelf(t *testing.T) { // A
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	nodeIdentity, err := NewNodeIdentity()
	if err != nil {
		t.Fatalf("Failed to create node identity: %v", err)
	}

	nodeID, err := NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create node ID: %v", err)
	}

	transport := newMockTransport()
	c, err := NewDefaultCarrier(Config{
		LocalNode: Node{
			NodeID:    nodeID,
			Addresses: []string{"localhost:4242"},
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    transport,
	})
	if err != nil {
		t.Fatalf("Failed to create carrier: %v", err)
	}

	// Register and add a remote node
	ctx := context.Background()
	transport.registerNode("localhost:4243", "remote-node")
	_ = c.AddNode(
		ctx,
		Node{NodeID: "remote-node", Addresses: []string{"localhost:4243"}},
	)

	err = c.AnnounceSelf(ctx)
	if err != nil {
		t.Fatalf("AnnounceSelf() error = %v", err)
	}

	// Verify announcement was sent
	transport.mu.Lock()
	conn, exists := transport.connections["localhost:4243"]
	transport.mu.Unlock()

	if !exists {
		t.Error("Expected connection to remote node")
	}

	if len(conn.messages) == 0 {
		t.Error("Expected message to be sent")
	}

	// Verify the announcement contains our node info
	ann, err := DeserializeNodeAnnouncement(conn.messages[0].Payload)
	if err != nil {
		t.Fatalf("DeserializeNodeAnnouncement() error = %v", err)
	}

	if ann.Node.NodeID != nodeID {
		t.Errorf("Announced NodeID = %v, want %v", ann.Node.NodeID, nodeID)
	}
}
