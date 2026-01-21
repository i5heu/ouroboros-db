package carrier

import (
	"testing"
)

func TestLogSubscribePayload_Roundtrip(t *testing.T) { // A
	tests := []struct {
		name    string
		payload LogSubscribePayload
	}{
		{
			name: "empty levels",
			payload: LogSubscribePayload{
				SubscriberNodeID: NodeID("node-123"),
				Levels:           nil,
			},
		},
		{
			name: "single level",
			payload: LogSubscribePayload{
				SubscriberNodeID: NodeID("node-456"),
				Levels:           []string{"error"},
			},
		},
		{
			name: "multiple levels",
			payload: LogSubscribePayload{
				SubscriberNodeID: NodeID("node-789"),
				Levels:           []string{"debug", "info", "warn", "error"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := SerializeLogSubscribe(tt.payload)
			if err != nil {
				t.Fatalf("serialize failed: %v", err)
			}

			got, err := DeserializeLogSubscribe(data)
			if err != nil {
				t.Fatalf("deserialize failed: %v", err)
			}

			if got.SubscriberNodeID != tt.payload.SubscriberNodeID {
				t.Errorf("SubscriberNodeID = %v, want %v",
					got.SubscriberNodeID, tt.payload.SubscriberNodeID)
			}

			if len(got.Levels) != len(tt.payload.Levels) {
				t.Errorf("Levels len = %d, want %d",
					len(got.Levels), len(tt.payload.Levels))
			}

			for i, level := range got.Levels {
				if level != tt.payload.Levels[i] {
					t.Errorf("Levels[%d] = %v, want %v",
						i, level, tt.payload.Levels[i])
				}
			}
		})
	}
}

func TestLogUnsubscribePayload_Roundtrip(t *testing.T) { // A
	payload := LogUnsubscribePayload{
		SubscriberNodeID: NodeID("node-to-unsubscribe"),
	}

	data, err := SerializeLogUnsubscribe(payload)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	got, err := DeserializeLogUnsubscribe(data)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if got.SubscriberNodeID != payload.SubscriberNodeID {
		t.Errorf("SubscriberNodeID = %v, want %v",
			got.SubscriberNodeID, payload.SubscriberNodeID)
	}
}

func TestLogEntryPayload_Roundtrip(t *testing.T) { // A
	tests := []struct {
		name    string
		payload LogEntryPayload
	}{
		{
			name: "minimal entry",
			payload: LogEntryPayload{
				SourceNodeID: NodeID("source-node"),
				Timestamp:    1234567890,
				Level:        "info",
				Message:      "test message",
				Attributes:   nil,
			},
		},
		{
			name: "with attributes",
			payload: LogEntryPayload{
				SourceNodeID: NodeID("source-node-2"),
				Timestamp:    9876543210,
				Level:        "error",
				Message:      "something went wrong",
				Attributes: map[string]string{
					"requestId": "req-123",
					"userId":    "user-456",
					"component": "storage",
				},
			},
		},
		{
			name: "empty attributes map",
			payload: LogEntryPayload{
				SourceNodeID: NodeID("node"),
				Timestamp:    0,
				Level:        "debug",
				Message:      "",
				Attributes:   map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := SerializeLogEntry(tt.payload)
			if err != nil {
				t.Fatalf("serialize failed: %v", err)
			}

			got, err := DeserializeLogEntry(data)
			if err != nil {
				t.Fatalf("deserialize failed: %v", err)
			}

			if got.SourceNodeID != tt.payload.SourceNodeID {
				t.Errorf("SourceNodeID = %v, want %v",
					got.SourceNodeID, tt.payload.SourceNodeID)
			}
			if got.Timestamp != tt.payload.Timestamp {
				t.Errorf("Timestamp = %v, want %v",
					got.Timestamp, tt.payload.Timestamp)
			}
			if got.Level != tt.payload.Level {
				t.Errorf("Level = %v, want %v", got.Level, tt.payload.Level)
			}
			if got.Message != tt.payload.Message {
				t.Errorf("Message = %v, want %v", got.Message, tt.payload.Message)
			}

			if len(got.Attributes) != len(tt.payload.Attributes) {
				t.Errorf("Attributes len = %d, want %d",
					len(got.Attributes), len(tt.payload.Attributes))
			}

			for k, v := range tt.payload.Attributes {
				if got.Attributes[k] != v {
					t.Errorf("Attributes[%s] = %v, want %v", k, got.Attributes[k], v)
				}
			}
		})
	}
}

func TestDashboardAnnouncePayload_Roundtrip(t *testing.T) { // A
	tests := []struct {
		name    string
		payload DashboardAnnouncePayload
	}{
		{
			name: "localhost",
			payload: DashboardAnnouncePayload{
				NodeID:           NodeID("local-node"),
				DashboardAddress: "127.0.0.1:8080",
			},
		},
		{
			name: "hostname with port",
			payload: DashboardAnnouncePayload{
				NodeID:           NodeID("remote-node"),
				DashboardAddress: "node1.cluster.local:9090",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := SerializeDashboardAnnounce(tt.payload)
			if err != nil {
				t.Fatalf("serialize failed: %v", err)
			}

			got, err := DeserializeDashboardAnnounce(data)
			if err != nil {
				t.Fatalf("deserialize failed: %v", err)
			}

			if got.NodeID != tt.payload.NodeID {
				t.Errorf("NodeID = %v, want %v", got.NodeID, tt.payload.NodeID)
			}
			if got.DashboardAddress != tt.payload.DashboardAddress {
				t.Errorf("DashboardAddress = %v, want %v",
					got.DashboardAddress, tt.payload.DashboardAddress)
			}
		})
	}
}

func TestDeserialize_InvalidData(t *testing.T) { // A
	invalidData := []byte("not valid gob data")

	_, err := DeserializeLogSubscribe(invalidData)
	if err == nil {
		t.Error("DeserializeLogSubscribe should fail with invalid data")
	}

	_, err = DeserializeLogUnsubscribe(invalidData)
	if err == nil {
		t.Error("DeserializeLogUnsubscribe should fail with invalid data")
	}

	_, err = DeserializeLogEntry(invalidData)
	if err == nil {
		t.Error("DeserializeLogEntry should fail with invalid data")
	}

	_, err = DeserializeDashboardAnnounce(invalidData)
	if err == nil {
		t.Error("DeserializeDashboardAnnounce should fail with invalid data")
	}
}
