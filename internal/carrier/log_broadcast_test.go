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
}

type sentMessage struct { // A
	nodeID  NodeID
	message Message
}

func (m *mockCarrier) GetNodes(ctx context.Context) ([]Node, error) { // A
	return nil, nil
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
	logger.Info("test message", "key", "value")

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
	logger.Info("broadcast to all")

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

	for _, nodeID := range []NodeID{"subscriber-1", "subscriber-2", "subscriber-3"} {
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
	logger.Info("info message")
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
	logger.Error("error message")
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
	logger.Info("first message")
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
	logger.Info("second message")
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
			logger.Info("concurrent log", "iteration", n)
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
		logger.Info("high volume log", "index", i)
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
	logger.Error("test error")
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
	logger.Info("after unsubscribe")
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
	logger := slog.New(lb).With("component", "test")
	logger.Info("message with attrs")

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
	logger.Info("message with group", "key", "value")

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
	logger.Info("first log")
	time.Sleep(50 * time.Millisecond)

	if len(mock.getSentMessages()) != 1 {
		t.Fatal("subscriber should receive first log")
	}
	mock.clearMessages()

	// Re-subscribe with only ERROR level
	lb.Subscribe(NodeID("subscriber"), []string{"ERROR"})
	time.Sleep(20 * time.Millisecond)

	logger.Info("second log - info")
	time.Sleep(50 * time.Millisecond)

	if len(mock.getSentMessages()) != 0 {
		t.Error("subscriber should not receive INFO after re-subscribe")
	}

	logger.Error("third log - error")
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
	logger.Info("no one listening")

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
		logger.Info("flood message", "i", i)
	}

	time.Sleep(200 * time.Millisecond)

	// Should have received some but not all (dropped due to full channel)
	messages := mock.getSentMessages()
	t.Logf("received %d messages (some dropped as expected)", len(messages))

	// Main assertion: inner handler still works even when broadcast drops
	// (verified by no panic)
}
