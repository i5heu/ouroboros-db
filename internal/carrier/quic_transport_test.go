package carrier

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// testQUICLogger creates a logger for QUIC transport tests.
func testQUICLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// TestNewQUICTransport tests creating a new QUIC transport.
func TestNewQUICTransport(t *testing.T) { // A
	logger := testQUICLogger()

	transport, err := NewQUICTransport(logger, "test-node", DefaultQUICConfig())
	if err != nil {
		t.Fatalf("NewQUICTransport() error = %v", err)
	}
	defer transport.Close()

	if transport == nil {
		t.Fatal("NewQUICTransport() returned nil")
	}

	if transport.localID != "test-node" {
		t.Errorf("localID = %q, want %q", transport.localID, "test-node")
	}
}

// TestQUICTransport_ConnectAndHandshake tests connection and handshake.
func TestQUICTransport_ConnectAndHandshake(t *testing.T) { // A
	logger := testQUICLogger()

	// Create server transport
	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	// Create client transport
	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	// Start listener
	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	// Accept in goroutine
	var serverConn Connection
	var acceptErr error
	acceptDone := make(chan struct{})
	go func() {
		serverConn, acceptErr = listener.Accept(ctx)
		close(acceptDone)
	}()

	// Connect from client
	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer clientConn.Close()

	// Wait for accept
	select {
	case <-acceptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timed out")
	}

	if acceptErr != nil {
		t.Fatalf("Accept() error = %v", acceptErr)
	}
	defer serverConn.Close()

	// Verify node IDs from handshake
	if clientConn.RemoteNodeID() != "server-node" {
		t.Errorf(
			"client RemoteNodeID() = %q, want %q",
			clientConn.RemoteNodeID(),
			"server-node",
		)
	}
	if serverConn.RemoteNodeID() != "client-node" {
		t.Errorf(
			"server RemoteNodeID() = %q, want %q",
			serverConn.RemoteNodeID(),
			"client-node",
		)
	}
}

// TestQUICConn_SendReceive tests sending and receiving messages.
func TestQUICConn_SendReceive(t *testing.T) { // A
	logger := testQUICLogger()

	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	var serverConn Connection
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept(ctx)
		close(acceptDone)
	}()

	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer clientConn.Close()

	<-acceptDone
	defer serverConn.Close()

	// Test sending from client to server
	testMsg := Message{
		Type:    MessageTypeHeartbeat,
		Payload: []byte("hello from client"),
	}

	// Send in goroutine (Receive blocks)
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- clientConn.Send(ctx, testMsg)
	}()

	// Receive on server
	receivedMsg, err := serverConn.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}

	// Check send completed
	select {
	case err := <-sendDone:
		if err != nil {
			t.Fatalf("Send() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send timed out")
	}

	// Verify message
	if receivedMsg.Type != testMsg.Type {
		t.Errorf("Type = %v, want %v", receivedMsg.Type, testMsg.Type)
	}
	if !bytes.Equal(receivedMsg.Payload, testMsg.Payload) {
		t.Errorf("Payload = %q, want %q", receivedMsg.Payload, testMsg.Payload)
	}
}

// TestQUICConn_ConcurrentStreams tests multiple concurrent streams.
func TestQUICConn_ConcurrentStreams(t *testing.T) { // A
	logger := testQUICLogger()

	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	var serverConn Connection
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept(ctx)
		close(acceptDone)
	}()

	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer clientConn.Close()

	<-acceptDone
	defer serverConn.Close()

	// Send multiple messages concurrently
	const numMessages = 10
	var wg sync.WaitGroup
	messages := make([]Message, numMessages)
	for i := 0; i < numMessages; i++ {
		messages[i] = Message{
			Type:    MessageTypeHeartbeat,
			Payload: []byte{byte(i)},
		}
	}

	// Start receivers
	received := make(chan Message, numMessages)
	receiverDone := make(chan struct{})
	go func() {
		for i := 0; i < numMessages; i++ {
			msg, err := serverConn.Receive(ctx)
			if err != nil {
				t.Logf("Receive error: %v", err)
				continue
			}
			received <- msg
		}
		close(receiverDone)
	}()

	// Send concurrently
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(msg Message) {
			defer wg.Done()
			if err := clientConn.Send(ctx, msg); err != nil {
				t.Logf("Send error: %v", err)
			}
		}(messages[i])
	}

	wg.Wait()

	// Wait for all receives
	select {
	case <-receiverDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Receivers timed out")
	}

	close(received)

	// Count received messages
	count := 0
	for range received {
		count++
	}

	if count != numMessages {
		t.Errorf("Received %d messages, want %d", count, numMessages)
	}
}

// TestQUICConn_LargePayload tests sending large payloads.
func TestQUICConn_LargePayload(t *testing.T) { // A
	logger := testQUICLogger()

	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	var serverConn Connection
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept(ctx)
		close(acceptDone)
	}()

	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer clientConn.Close()

	<-acceptDone
	defer serverConn.Close()

	// Create a 1MB payload
	largePayload := make([]byte, 1024*1024)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	testMsg := Message{
		Type:    MessageTypeChunkPayloadRequest,
		Payload: largePayload,
	}

	sendDone := make(chan error, 1)
	go func() {
		sendDone <- clientConn.Send(ctx, testMsg)
	}()

	receivedMsg, err := serverConn.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}

	select {
	case err := <-sendDone:
		if err != nil {
			t.Fatalf("Send() error = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Send timed out")
	}

	if !bytes.Equal(receivedMsg.Payload, largePayload) {
		t.Error("Large payload was corrupted in transit")
	}
}

// TestQUICConn_Close tests connection close behavior.
func TestQUICConn_Close(t *testing.T) { // A
	logger := testQUICLogger()

	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	var serverConn Connection
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept(ctx)
		close(acceptDone)
	}()

	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	<-acceptDone

	// Close client connection
	if err := clientConn.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Second close should be idempotent
	if err := clientConn.Close(); err != nil {
		t.Errorf("Second Close() error = %v", err)
	}

	// IsClosed should return true
	if qc, ok := clientConn.(*quicConn); ok {
		if !qc.IsClosed() {
			t.Error("IsClosed() = false after Close()")
		}
	}

	// Sending on closed connection should fail
	err = clientConn.Send(ctx, Message{Type: MessageTypeHeartbeat})
	if err == nil {
		t.Error("Send on closed connection should fail")
	}

	if serverConn != nil {
		serverConn.Close()
	}
}

// TestQUICTransport_Close tests transport close behavior.
func TestQUICTransport_Close(t *testing.T) { // A
	logger := testQUICLogger()

	transport, err := NewQUICTransport(logger, "test-node", DefaultQUICConfig())
	if err != nil {
		t.Fatalf("NewQUICTransport() error = %v", err)
	}

	ctx := context.Background()

	listener, err := transport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	// Close transport
	if err := transport.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Second close should be idempotent
	if err := transport.Close(); err != nil {
		t.Errorf("Second Close() error = %v", err)
	}

	// Connect on closed transport should fail
	_, err = transport.Connect(ctx, "127.0.0.1:9999")
	if err == nil {
		t.Error("Connect on closed transport should fail")
	}

	// Listen on closed transport should fail
	_, err = transport.Listen(ctx, "127.0.0.1:0")
	if err == nil {
		t.Error("Listen on closed transport should fail")
	}
}

// TestQUICListener_Addr tests listener address reporting.
func TestQUICListener_Addr(t *testing.T) { // A
	logger := testQUICLogger()

	transport, err := NewQUICTransport(logger, "test-node", DefaultQUICConfig())
	if err != nil {
		t.Fatalf("NewQUICTransport() error = %v", err)
	}
	defer transport.Close()

	ctx := context.Background()

	listener, err := transport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	addr := listener.Addr()
	if addr == "" {
		t.Error("Addr() returned empty string")
	}
	// Should include a port
	if len(addr) < 10 {
		t.Errorf("Addr() = %q, expected address with port", addr)
	}
}

// TestQUICConn_SendEncrypted tests that SendEncrypted returns error.
func TestQUICConn_SendEncrypted(t *testing.T) { // A
	logger := testQUICLogger()

	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	var serverConn Connection
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept(ctx)
		close(acceptDone)
	}()

	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer clientConn.Close()

	<-acceptDone
	if serverConn != nil {
		defer serverConn.Close()
	}

	// SendEncrypted should return error (QUIC provides encryption)
	err = clientConn.SendEncrypted(ctx, &EncryptedMessage{})
	if err == nil {
		t.Error("SendEncrypted() should return error")
	}

	// ReceiveEncrypted should return error
	_, err = clientConn.ReceiveEncrypted(ctx)
	if err == nil {
		t.Error("ReceiveEncrypted() should return error")
	}
}

// TestQUICConn_RemotePublicKey tests that RemotePublicKey returns nil.
func TestQUICConn_RemotePublicKey(t *testing.T) { // A
	logger := testQUICLogger()

	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	var serverConn Connection
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept(ctx)
		close(acceptDone)
	}()

	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer clientConn.Close()

	<-acceptDone
	if serverConn != nil {
		defer serverConn.Close()
	}

	// RemotePublicKey returns nil (not implemented for QUIC transport)
	if clientConn.RemotePublicKey() != nil {
		t.Error("RemotePublicKey() should return nil")
	}
}

// TestQUICConn_PayloadTooLarge tests rejection of oversized payloads.
func TestQUICConn_PayloadTooLarge(t *testing.T) { // A
	logger := testQUICLogger()

	serverTransport, err := NewQUICTransport(
		logger,
		"server-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(server) error = %v", err)
	}
	defer serverTransport.Close()

	clientTransport, err := NewQUICTransport(
		logger,
		"client-node",
		DefaultQUICConfig(),
	)
	if err != nil {
		t.Fatalf("NewQUICTransport(client) error = %v", err)
	}
	defer clientTransport.Close()

	ctx := context.Background()

	listener, err := serverTransport.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr()

	var serverConn Connection
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept(ctx)
		close(acceptDone)
	}()

	clientConn, err := clientTransport.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer clientConn.Close()

	<-acceptDone
	if serverConn != nil {
		defer serverConn.Close()
	}

	// Create a payload larger than maxPayloadLength (16 MB)
	hugePayload := make([]byte, 17*1024*1024)

	err = clientConn.Send(ctx, Message{
		Type:    MessageTypeHeartbeat,
		Payload: hugePayload,
	})
	if err == nil {
		t.Error("Send() should reject payload larger than 16 MB")
	}
}

// TestWriteReadNodeID tests the nodeID wire protocol helpers.
func TestWriteReadNodeID(t *testing.T) { // A
	tests := []struct {
		name   string
		nodeID NodeID
	}{
		{"short ID", "n1"},
		{"typical ID", "node-abc123"},
		{"hash ID", "519e909acb5205b3bcda497a5f724f44"},
		{"empty ID", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}

			err := writeNodeID(buf, tt.nodeID)
			if err != nil {
				t.Fatalf("writeNodeID() error = %v", err)
			}

			readID, err := readNodeID(buf)
			if err != nil {
				t.Fatalf("readNodeID() error = %v", err)
			}

			if readID != tt.nodeID {
				t.Errorf("readNodeID() = %q, want %q", readID, tt.nodeID)
			}
		})
	}
}

// TestWriteNodeID_TooLong tests rejection of node IDs that are too long.
func TestWriteNodeID_TooLong(t *testing.T) { // A
	longID := make([]byte, maxNodeIDLength+1)
	for i := range longID {
		longID[i] = 'a'
	}

	buf := &bytes.Buffer{}
	err := writeNodeID(buf, NodeID(longID))
	if err == nil {
		t.Error("writeNodeID() should reject ID longer than maxNodeIDLength")
	}
}

// TestGenerateTLSConfig tests TLS config generation.
func TestGenerateTLSConfig(t *testing.T) { // A
	config, err := generateTLSConfig()
	if err != nil {
		t.Fatalf("generateTLSConfig() error = %v", err)
	}

	if config == nil {
		t.Fatal("generateTLSConfig() returned nil")
	}

	if len(config.Certificates) != 1 {
		t.Errorf(
			"Certificates count = %d, want 1",
			len(config.Certificates),
		)
	}

	if len(config.NextProtos) != 1 || config.NextProtos[0] != "ouroboros-quic" {
		t.Errorf("NextProtos = %v, want [ouroboros-quic]", config.NextProtos)
	}
}

// TestTruncateNodeID tests the node ID truncation helper.
func TestTruncateNodeID(t *testing.T) { // A
	tests := []struct {
		input    NodeID
		expected string
	}{
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678..."},
		{"519e909acb5205b3bcda497a5f724f44", "519e909a..."},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := truncateNodeID(tt.input)
			if result != tt.expected {
				t.Errorf("truncateNodeID(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}
