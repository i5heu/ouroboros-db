package dashboard

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLogStreamHub_StartStop(t *testing.T) { // A
	hub := NewLogStreamHub()
	hub.Start()
	time.Sleep(10 * time.Millisecond)
	hub.Stop()
	// Should not hang or panic
}

func TestLogStreamHub_Broadcast(t *testing.T) { // A
	hub := NewLogStreamHub()
	hub.Start()
	defer hub.Stop()

	// Create a mock client
	client := &Client{
		sendCh: make(chan []byte, 10),
	}

	// Register client
	hub.registerCh <- client
	time.Sleep(10 * time.Millisecond)

	// Broadcast a message
	msg := LogStreamMessage{
		SourceNodeID: "test-node",
		Timestamp:    time.Now().UnixNano(),
		Level:        "INFO",
		Message:      "test message",
	}
	hub.Broadcast(msg)

	// Wait for message
	select {
	case data := <-client.sendCh:
		var received LogStreamMessage
		if err := json.Unmarshal(data, &received); err != nil {
			t.Fatalf("Failed to unmarshal message: %v", err)
		}
		if received.Message != "test message" {
			t.Errorf("Expected 'test message', got '%s'", received.Message)
		}
		if received.Type != "log" {
			t.Errorf("Expected type 'log', got '%s'", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for broadcast message")
	}
}

func TestLogStreamHub_MultipleClients(t *testing.T) { // A
	hub := NewLogStreamHub()
	hub.Start()
	defer hub.Stop()

	// Create multiple clients
	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = &Client{
			sendCh: make(chan []byte, 10),
		}
		hub.registerCh <- clients[i]
	}
	time.Sleep(20 * time.Millisecond)

	// Broadcast a message
	hub.Broadcast(LogStreamMessage{
		SourceNodeID: "test-node",
		Level:        "INFO",
		Message:      "broadcast to all",
	})

	// All clients should receive the message
	for i, client := range clients {
		select {
		case data := <-client.sendCh:
			var msg LogStreamMessage
			json.Unmarshal(data, &msg)
			if msg.Message != "broadcast to all" {
				t.Errorf("Client %d: wrong message", i)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Client %d: timeout waiting for message", i)
		}
	}
}

func TestLogStreamHub_ClientUnregister(t *testing.T) { // A
	hub := NewLogStreamHub()
	hub.Start()
	defer hub.Stop()

	client := &Client{
		sendCh: make(chan []byte, 10),
	}

	// Register
	hub.registerCh <- client
	time.Sleep(10 * time.Millisecond)

	// Unregister
	hub.unregisterCh <- client
	time.Sleep(10 * time.Millisecond)

	// Client's send channel should be closed
	_, ok := <-client.sendCh
	if ok {
		t.Error("Expected client's send channel to be closed")
	}
}

func TestLogStreamHub_SlowClient(t *testing.T) { // A
	hub := NewLogStreamHub()
	hub.Start()
	defer hub.Stop()

	// Create a slow client with a tiny buffer
	slowClient := &Client{
		sendCh: make(chan []byte, 1), // Tiny buffer
	}
	fastClient := &Client{
		sendCh: make(chan []byte, 100),
	}

	hub.registerCh <- slowClient
	hub.registerCh <- fastClient
	time.Sleep(10 * time.Millisecond)

	// Flood with messages
	for i := 0; i < 50; i++ {
		hub.Broadcast(LogStreamMessage{
			SourceNodeID: "test",
			Level:        "INFO",
			Message:      "flood",
		})
	}

	time.Sleep(50 * time.Millisecond)

	// Fast client should have most messages
	fastCount := 0
	for {
		select {
		case <-fastClient.sendCh:
			fastCount++
		default:
			goto done
		}
	}
done:

	if fastCount < 10 {
		t.Errorf("Fast client should have received many messages, got %d", fastCount)
	}
	// Slow client won't receive all due to dropped messages, which is fine
}

func TestLogStreamHub_BroadcastAfterStop(t *testing.T) { // A
	hub := NewLogStreamHub()
	hub.Start()
	hub.Stop()

	// Broadcast after stop should not panic
	hub.Broadcast(LogStreamMessage{
		Message: "after stop",
	})
}

func TestLogStreamMessage_JSON(t *testing.T) { // A
	msg := LogStreamMessage{
		Type:         "log",
		SourceNodeID: "node-123",
		Timestamp:    1234567890,
		Level:        "ERROR",
		Message:      "something failed",
		Attributes: map[string]string{
			"key": "value",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded LogStreamMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.SourceNodeID != msg.SourceNodeID {
		t.Errorf("SourceNodeID mismatch")
	}
	if decoded.Message != msg.Message {
		t.Errorf("Message mismatch")
	}
	if decoded.Attributes["key"] != "value" {
		t.Errorf("Attributes mismatch")
	}
}
