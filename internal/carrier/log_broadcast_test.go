package carrier

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// mockCarrier is a test double for the Carrier interface.
type mockCarrier struct { // A
	sentMessages []sentMessage
	mu           sync.Mutex
	localNode    Node
}

type sentMessage struct { // A
	nodeID  NodeID
	message Message
}

func (m *mockCarrier) GetNodes(ctx context.Context) ([]Node, error) { // A
	return nil, nil
}

func (m *mockCarrier) LocalNode() Node { // A
	if m.localNode.NodeID != "" {
		return m.localNode
	}
	return Node{NodeID: "mock-local-node"}
}

func (m *mockCarrier) Broadcast(
	ctx context.Context,
	message Message,
) (*BroadcastResult, error) { // A
	return nil, nil
}

func (m *mockCarrier) SendMessageToNode(
	ctx context.Context,
	nodeID NodeID,
	message Message,
) error { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = append(m.sentMessages, sentMessage{
		nodeID:  nodeID,
		message: message,
	})
	return nil
}

func (m *mockCarrier) JoinCluster(
	ctx context.Context,
	clusterNode Node,
	cert NodeCert,
) error { // A
	return nil
}

func (m *mockCarrier) LeaveCluster(ctx context.Context) error { // A
	return nil
}

func (m *mockCarrier) Start(ctx context.Context) error { // A
	return nil
}

func (m *mockCarrier) Stop(ctx context.Context) error { // A
	return nil
}

func (m *mockCarrier) RegisterHandler(
	msgType MessageType,
	handler MessageHandler,
) { // A
}

func (m *mockCarrier) SetLogger(logger *slog.Logger) {
}

func (m *mockCarrier) BootstrapFromAddresses(
	ctx context.Context,
	addresses []string,
) error { // A
	return nil
}

func (m *mockCarrier) getSentMessages() []sentMessage { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sentMessage, len(m.sentMessages))
	copy(result, m.sentMessages)
	return result
}

func (m *mockCarrier) clearMessages() { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = nil
}

func newTestBroadcaster(carrier Carrier) *LogBroadcaster { // A
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	return NewLogBroadcaster(LogBroadcasterConfig{
		Carrier:     carrier,
		LocalNodeID: NodeID("test-node"),
		Inner:       inner,
	})
}

func TestLogBroadcaster_StartStop(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)

	lb.Start()
	time.Sleep(10 * time.Millisecond) // Let goroutine start
	lb.Stop()
	// Should not hang
}

func TestLogBroadcaster_SingleSubscriber(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	// Subscribe a node
	lb.Subscribe(NodeID("subscriber-1"), nil)
	time.Sleep(10 * time.Millisecond) // Let command process

	// Send a log
	logger := slog.New(lb)
	//nolint:sloglint // testing log functionality requires raw keys
	logger.InfoContext(context.Background(), "test message", "key", "value")

	// Wait for async send
	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0].nodeID != NodeID("subscriber-1") {
		t.Errorf("expected nodeID subscriber-1, got %s", messages[0].nodeID)
	}

	if messages[0].message.Type != MessageTypeLogEntry {
		t.Errorf("expected MessageTypeLogEntry, got %v", messages[0].message.Type)
	}
}

func TestLogBroadcaster_MultipleSubscribers(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	// Subscribe multiple nodes
	lb.Subscribe(NodeID("subscriber-1"), nil)
	lb.Subscribe(NodeID("subscriber-2"), nil)
	lb.Subscribe(NodeID("subscriber-3"), nil)
	time.Sleep(20 * time.Millisecond)

	// Send a log
	logger := slog.New(lb)
	logger.InfoContext(context.Background(), "broadcast to all")

	// Wait for async sends
	time.Sleep(100 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Verify all subscribers received the message
	received := make(map[NodeID]bool)
	for _, msg := range messages {
		received[msg.nodeID] = true
	}

	for _, nodeID := range []NodeID{
		"subscriber-1",
		"subscriber-2",
		"subscriber-3",
	} {
		if !received[nodeID] {
			t.Errorf("subscriber %s did not receive message", nodeID)
		}
	}
}

func TestLogBroadcaster_LevelFiltering(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	// Subscriber only wants errors
	lb.Subscribe(NodeID("error-only"), []string{"ERROR"})
	// Subscriber wants all
	lb.Subscribe(NodeID("all-levels"), nil)
	time.Sleep(20 * time.Millisecond)

	logger := slog.New(lb)

	// Send info log
	logger.InfoContext(context.Background(), "info message")
	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	// Only "all-levels" should receive the info message
	if len(messages) != 1 {
		t.Fatalf("expected 1 message for INFO, got %d", len(messages))
	}
	if messages[0].nodeID != NodeID("all-levels") {
		t.Errorf("expected all-levels to receive INFO, got %s", messages[0].nodeID)
	}

	mock.clearMessages()

	// Send error log
	logger.ErrorContext(context.Background(), "error message")
	time.Sleep(50 * time.Millisecond)

	messages = mock.getSentMessages()
	// Both should receive error
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages for ERROR, got %d", len(messages))
	}
}

func TestLogBroadcaster_Unsubscribe(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	lb.Subscribe(NodeID("subscriber-1"), nil)
	lb.Subscribe(NodeID("subscriber-2"), nil)
	time.Sleep(20 * time.Millisecond)

	logger := slog.New(lb)

	// Both should receive
	logger.InfoContext(context.Background(), "first message")
	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	mock.clearMessages()

	// Unsubscribe one
	lb.Unsubscribe(NodeID("subscriber-1"))
	time.Sleep(20 * time.Millisecond)

	// Only subscriber-2 should receive
	logger.InfoContext(context.Background(), "second message")
	time.Sleep(50 * time.Millisecond)

	messages = mock.getSentMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after unsubscribe, got %d", len(messages))
	}
	if messages[0].nodeID != NodeID("subscriber-2") {
		t.Errorf("expected subscriber-2, got %s", messages[0].nodeID)
	}
}

func TestLogBroadcaster_ConcurrentSubscriptions(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	var wg sync.WaitGroup
	logger := slog.New(lb)

	// Concurrent subscribes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			nodeID := NodeID("concurrent-" + string(rune('A'+n)))
			lb.Subscribe(nodeID, nil)
		}(i)
	}

	// Concurrent logging
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			//nolint:sloglint // testing log functionality
			logger.InfoContext(context.Background(), "concurrent log", "iteration", n)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// Should not panic or deadlock
}

func TestLogBroadcaster_HighVolume(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	lb.Subscribe(NodeID("subscriber-1"), nil)
	time.Sleep(20 * time.Millisecond)

	logger := slog.New(lb)

	// Send many logs quickly
	for i := 0; i < 1000; i++ {
		//nolint:sloglint // testing log functionality
		logger.InfoContext(context.Background(), "high volume log", "index", i)
	}

	time.Sleep(500 * time.Millisecond)

	messages := mock.getSentMessages()
	// We might drop some due to channel buffer, but should get most
	if len(messages) < 100 {
		t.Errorf("expected at least 100 messages, got %d", len(messages))
	}
}

func TestLogBroadcaster_HandleLogSubscribe(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	// Create subscription payload
	payload, err := SerializeLogSubscribe(LogSubscribePayload{
		SubscriberNodeID: NodeID("remote-subscriber"),
		Levels:           []string{"ERROR", "WARN"},
	})
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	msg := Message{
		Type:    MessageTypeLogSubscribe,
		Payload: payload,
	}

	// Handle the message
	_, err = lb.HandleLogSubscribe(context.Background(), NodeID("sender"), msg)
	if err != nil {
		t.Fatalf("HandleLogSubscribe failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Verify subscription by sending an error log
	logger := slog.New(lb)
	logger.ErrorContext(context.Background(), "test error")
	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].nodeID != NodeID("remote-subscriber") {
		t.Errorf("expected remote-subscriber, got %s", messages[0].nodeID)
	}
}

func TestLogBroadcaster_HandleLogUnsubscribe(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	// Subscribe first
	lb.Subscribe(NodeID("to-unsubscribe"), nil)
	time.Sleep(20 * time.Millisecond)

	// Create unsubscription payload
	payload, err := SerializeLogUnsubscribe(LogUnsubscribePayload{
		SubscriberNodeID: NodeID("to-unsubscribe"),
	})
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	msg := Message{
		Type:    MessageTypeLogUnsubscribe,
		Payload: payload,
	}

	// Handle the unsubscribe message
	_, err = lb.HandleLogUnsubscribe(context.Background(), NodeID("sender"), msg)
	if err != nil {
		t.Fatalf("HandleLogUnsubscribe failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Verify unsubscription by sending a log - should get nothing
	logger := slog.New(lb)
	logger.InfoContext(context.Background(), "after unsubscribe")
	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 0 {
		t.Fatalf("expected 0 messages after unsubscribe, got %d", len(messages))
	}
}

func TestLogBroadcaster_WithAttrs(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	lb.Subscribe(NodeID("subscriber"), nil)
	time.Sleep(20 * time.Millisecond)

	// Create logger with attributes
	//nolint:sloglint // testing log functionality with attributes
	logger := slog.New(lb).With(slog.String("component", "test"))
	logger.InfoContext(context.Background(), "message with attrs")

	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// Verify the entry contains the attribute
	entry, err := DeserializeLogEntry(messages[0].message.Payload)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if entry.Attributes["component"] != "test" {
		t.Errorf("expected component=test, got %v", entry.Attributes)
	}
}

func TestLogBroadcaster_WithGroup(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	lb.Subscribe(NodeID("subscriber"), nil)
	time.Sleep(20 * time.Millisecond)

	// Create logger with group
	logger := slog.New(lb).WithGroup("mygroup")
	//nolint:sloglint // testing log functionality with groups
	logger.InfoContext(context.Background(), "message with group", "key", "value")

	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

func TestLogBroadcaster_SubscriberReplace(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	// Subscribe with all levels
	lb.Subscribe(NodeID("subscriber"), nil)
	time.Sleep(20 * time.Millisecond)

	logger := slog.New(lb)
	logger.InfoContext(context.Background(), "first log")
	time.Sleep(50 * time.Millisecond)

	if len(mock.getSentMessages()) != 1 {
		t.Fatal("subscriber should receive first log")
	}
	mock.clearMessages()

	// Re-subscribe with only ERROR level
	lb.Subscribe(NodeID("subscriber"), []string{"ERROR"})
	time.Sleep(20 * time.Millisecond)

	logger.InfoContext(context.Background(), "second log - info")
	time.Sleep(50 * time.Millisecond)

	if len(mock.getSentMessages()) != 0 {
		t.Error("subscriber should not receive INFO after re-subscribe")
	}

	logger.ErrorContext(context.Background(), "third log - error")
	time.Sleep(50 * time.Millisecond)

	if len(mock.getSentMessages()) != 1 {
		t.Error("subscriber should receive ERROR after re-subscribe")
	}
}

func TestLogBroadcaster_NoSubscribers(t *testing.T) { // A
	mock := &mockCarrier{}
	lb := newTestBroadcaster(mock)
	lb.Start()
	defer lb.Stop()

	logger := slog.New(lb)
	logger.InfoContext(context.Background(), "no one listening")

	time.Sleep(50 * time.Millisecond)

	messages := mock.getSentMessages()
	if len(messages) != 0 {
		t.Errorf("expected 0 messages with no subscribers, got %d", len(messages))
	}
}

func TestLogBroadcaster_DropsWhenChannelFull(t *testing.T) { // A
	mock := &mockCarrier{}

	// Create broadcaster with tiny buffer
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	lb := &LogBroadcaster{
		carrier:       mock,
		localNodeID:   NodeID("test-node"),
		inner:         inner,
		subscribeCh:   make(chan subscribeCmd, 16),
		unsubscribeCh: make(chan NodeID, 16),
		logCh:         make(chan LogEntryPayload, 1), // Tiny buffer
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	lb.Start()
	defer lb.Stop()

	lb.Subscribe(NodeID("subscriber"), nil)
	time.Sleep(20 * time.Millisecond)

	logger := slog.New(lb)

	// Flood the channel
	for i := 0; i < 100; i++ {
		//nolint:sloglint // testing log functionality
		logger.InfoContext(context.Background(), "flood message", "i", i)
	}

	time.Sleep(200 * time.Millisecond)

	// Should have received some but not all (dropped due to full channel)
	messages := mock.getSentMessages()
	t.Logf("received %d messages (some dropped as expected)", len(messages))

	// Main assertion: inner handler still works even when broadcast drops
	// (verified by no panic)
}

func TestLogBroadcaster_LocalHandler(t *testing.T) { // A
	mock := &mockCarrier{}
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	localNodeID := NodeID("local-node")

	lb := NewLogBroadcaster(LogBroadcasterConfig{
		Carrier:     mock,
		LocalNodeID: localNodeID,
		Inner:       inner,
	})

	// Track entries received via local handler
	var receivedEntries []LogEntryPayload
	var mu sync.Mutex

	lb.SetLocalHandler(func(entry LogEntryPayload) {
		mu.Lock()
		receivedEntries = append(receivedEntries, entry)
		mu.Unlock()
	})

	lb.Start()
	defer lb.Stop()

	// Subscribe the local node to its own logs
	lb.Subscribe(localNodeID, nil)
	time.Sleep(20 * time.Millisecond)

	// Send a log
	logger := slog.New(lb)
	logger.InfoContext(context.Background(), "self-subscription test")

	time.Sleep(100 * time.Millisecond)

	// Verify local handler was called
	mu.Lock()
	count := len(receivedEntries)
	mu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 entry via local handler, got %d", count)
	}

	// Verify no messages were sent via carrier (should use local handler)
	carrierMessages := mock.getSentMessages()
	if len(carrierMessages) != 0 {
		t.Errorf(
			"expected 0 carrier messages for self-subscription, got %d",
			len(carrierMessages),
		)
	}

	// Verify the entry content
	mu.Lock()
	entry := receivedEntries[0]
	mu.Unlock()

	if entry.Message != "self-subscription test" {
		t.Errorf("expected message 'self-subscription test', got '%s'", entry.Message)
	}
	if entry.SourceNodeID != localNodeID {
		t.Errorf("expected source node %s, got %s", localNodeID, entry.SourceNodeID)
	}
}

// TestLogBroadcaster_LocalHandlerReceivesUploadLogs verifies that the local
// handler receives logs when the local node is subscribed to itself. This is
// the correct behavior for dashboard log streaming during file uploads.
func TestLogBroadcaster_LocalHandlerReceivesUploadLogs(t *testing.T) {
	mock := &mockCarrier{}
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	localNodeID := NodeID("dashboard-node")

	lb := NewLogBroadcaster(LogBroadcasterConfig{
		Carrier:     mock,
		LocalNodeID: localNodeID,
		Inner:       inner,
	})

	// Simulate dashboard setting up local handler
	var receivedEntries []LogEntryPayload
	var mu sync.Mutex

	lb.SetLocalHandler(func(entry LogEntryPayload) {
		mu.Lock()
		receivedEntries = append(receivedEntries, entry)
		mu.Unlock()
	})

	lb.Start()
	defer lb.Stop()

	// Subscribe the local node to itself (this is what the fix should do)
	lb.Subscribe(localNodeID, nil)
	time.Sleep(20 * time.Millisecond)

	// Simulate upload logs
	logger := slog.New(lb)
	//nolint:sloglint // testing log functionality with attributes
	logger.InfoContext(context.Background(), "starting upload", "filename", "test.dat")
	//nolint:sloglint // testing log functionality with attributes
	logger.InfoContext(context.Background(), "processing chunk", "chunk", 1, "total", 10)
	//nolint:sloglint // testing log functionality with attributes
	logger.InfoContext(context.Background(), "upload complete", "bytes", 1024)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(receivedEntries)
	mu.Unlock()

	if count != 3 {
		t.Fatalf("expected 3 upload log entries, got %d", count)
	}

	// Verify all expected messages were received (order may vary due to
	// goroutine-based delivery)
	mu.Lock()
	messageSet := make(map[string]bool)
	for _, e := range receivedEntries {
		messageSet[e.Message] = true
	}
	mu.Unlock()

	expected := []string{"starting upload", "processing chunk", "upload complete"}
	for _, exp := range expected {
		if !messageSet[exp] {
			t.Errorf("expected message %q not found in received entries", exp)
		}
	}
}

// TestLogBroadcaster_SetLocalHandlerEnablesLocalDelivery verifies that
// calling SetLocalHandler should be sufficient to receive local logs.
// The local handler should automatically receive logs without requiring
// an explicit Subscribe call for the local node.
func TestLogBroadcaster_SetLocalHandlerEnablesLocalDelivery(t *testing.T) {
	mock := &mockCarrier{}
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	localNodeID := NodeID("dashboard-node")

	lb := NewLogBroadcaster(LogBroadcasterConfig{
		Carrier:     mock,
		LocalNodeID: localNodeID,
		Inner:       inner,
	})

	// Set local handler - this should be sufficient for local delivery
	var receivedEntries []LogEntryPayload
	var mu sync.Mutex

	lb.SetLocalHandler(func(entry LogEntryPayload) {
		mu.Lock()
		receivedEntries = append(receivedEntries, entry)
		mu.Unlock()
	})

	lb.Start()
	defer lb.Stop()

	time.Sleep(20 * time.Millisecond)

	// Send logs - these should reach the local handler
	logger := slog.New(lb)
	logger.InfoContext(context.Background(), "upload started")
	logger.InfoContext(context.Background(), "upload complete")

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(receivedEntries)
	mu.Unlock()

	// Expected behavior: local handler should receive logs when set
	if count != 2 {
		t.Fatalf("expected 2 entries via local handler, got %d", count)
	}

	// Verify message content
	mu.Lock()
	msgSet := make(map[string]bool)
	for _, e := range receivedEntries {
		msgSet[e.Message] = true
	}
	mu.Unlock()

	if !msgSet["upload started"] {
		t.Error("expected 'upload started' message")
	}
	if !msgSet["upload complete"] {
		t.Error("expected 'upload complete' message")
	}

	// Local delivery should not go through the carrier
	carrierMessages := mock.getSentMessages()
	if len(carrierMessages) != 0 {
		t.Errorf("expected 0 carrier messages for local delivery, got %d",
			len(carrierMessages))
	}
}

// TestLogBroadcaster_LocalHandlerCalledConcurrentlyWithRemote verifies that
// local handler delivery works correctly alongside remote subscriber delivery.
func TestLogBroadcaster_LocalHandlerCalledConcurrentlyWithRemote(t *testing.T) {
	mock := &mockCarrier{}
	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	localNodeID := NodeID("local-node")
	remoteNodeID := NodeID("remote-node")

	lb := NewLogBroadcaster(LogBroadcasterConfig{
		Carrier:     mock,
		LocalNodeID: localNodeID,
		Inner:       inner,
	})

	// Set up local handler
	var localEntries []LogEntryPayload
	var mu sync.Mutex

	lb.SetLocalHandler(func(entry LogEntryPayload) {
		mu.Lock()
		localEntries = append(localEntries, entry)
		mu.Unlock()
	})

	lb.Start()
	defer lb.Stop()

	// Subscribe both local and remote nodes
	lb.Subscribe(localNodeID, nil)
	lb.Subscribe(remoteNodeID, nil)
	time.Sleep(20 * time.Millisecond)

	// Send a log
	logger := slog.New(lb)
	logger.InfoContext(context.Background(), "broadcast to both")

	time.Sleep(100 * time.Millisecond)

	// Verify local handler received the entry
	mu.Lock()
	localCount := len(localEntries)
	mu.Unlock()

	if localCount != 1 {
		t.Fatalf("expected 1 local entry, got %d", localCount)
	}

	// Verify remote subscriber received via carrier
	carrierMessages := mock.getSentMessages()
	if len(carrierMessages) != 1 {
		t.Fatalf("expected 1 carrier message to remote, got %d", len(carrierMessages))
	}

	if carrierMessages[0].nodeID != remoteNodeID {
		t.Errorf("expected message to %s, got %s",
			remoteNodeID, carrierMessages[0].nodeID)
	}

	// Verify both received the same message content
	mu.Lock()
	localMsg := localEntries[0].Message
	mu.Unlock()

	remoteEntry, err := DeserializeLogEntry(carrierMessages[0].message.Payload)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if localMsg != remoteEntry.Message {
		t.Errorf("message mismatch: local=%q, remote=%q", localMsg, remoteEntry.Message)
	}
	if localMsg != "broadcast to both" {
		t.Errorf("expected 'broadcast to both', got %q", localMsg)
	}
}
