package carrier

import (
	"testing"
)

func TestSerializeDeserializeNodeAnnouncement(t *testing.T) { // A
	tests := []struct {
		name string
		ann  *NodeAnnouncement
	}{
		{
			name: "basic announcement",
			ann: &NodeAnnouncement{
				Node: Node{
					NodeID:    "test-node-123",
					Addresses: []string{"localhost:4242"},
				},
				Timestamp: 1234567890,
			},
		},
		{
			name: "multiple addresses",
			ann: &NodeAnnouncement{
				Node: Node{
					NodeID:    "multi-addr-node",
					Addresses: []string{"192.168.1.1:4242", "10.0.0.1:4242", "node.example.com:4242"},
				},
				Timestamp: 9876543210,
			},
		},
		{
			name: "long node ID",
			ann: &NodeAnnouncement{
				Node: Node{
					NodeID:    NodeID("abcdefghijklmnopqrstuvwxyz0123456789"),
					Addresses: []string{"localhost:8080"},
				},
				Timestamp: 0,
			},
		},
		{
			name: "empty addresses",
			ann: &NodeAnnouncement{
				Node: Node{
					NodeID:    "no-addr-node",
					Addresses: []string{},
				},
				Timestamp: 1111111111,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := SerializeNodeAnnouncement(tt.ann)
			if err != nil {
				t.Fatalf("SerializeNodeAnnouncement() error = %v", err)
			}

			got, err := DeserializeNodeAnnouncement(data)
			if err != nil {
				t.Fatalf("DeserializeNodeAnnouncement() error = %v", err)
			}

			if got.Node.NodeID != tt.ann.Node.NodeID {
				t.Errorf("NodeID = %v, want %v", got.Node.NodeID, tt.ann.Node.NodeID)
			}

			if got.Timestamp != tt.ann.Timestamp {
				t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.ann.Timestamp)
			}

			if len(got.Node.Addresses) != len(tt.ann.Node.Addresses) {
				t.Errorf("Addresses count = %d, want %d",
					len(got.Node.Addresses), len(tt.ann.Node.Addresses))
			}

			for i, addr := range tt.ann.Node.Addresses {
				if got.Node.Addresses[i] != addr {
					t.Errorf("Addresses[%d] = %v, want %v", i, got.Node.Addresses[i], addr)
				}
			}
		})
	}
}

func TestSerializeNodeAnnouncement_NilInput(t *testing.T) { // A
	_, err := SerializeNodeAnnouncement(nil)
	if err == nil {
		t.Error("SerializeNodeAnnouncement(nil) should return error")
	}
}

func TestDeserializeNodeAnnouncement_InvalidData(t *testing.T) { // A
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte{1, 2, 3}},
		{"invalid nodeID length", []byte{0, 0, 0, 0, 0, 0, 0, 1, 255, 255, 255, 255}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeserializeNodeAnnouncement(tt.data)
			if err == nil {
				t.Error("DeserializeNodeAnnouncement should return error for invalid data")
			}
		})
	}
}

func TestSerializeDeserializeNodeList(t *testing.T) { // A
	nodes := []Node{
		{NodeID: "node-1", Addresses: []string{"localhost:4242"}},
		{NodeID: "node-2", Addresses: []string{"192.168.1.1:4242", "10.0.0.1:4242"}},
		{NodeID: "node-3", Addresses: []string{"node3.example.com:4242"}},
	}

	data, err := SerializeNodeList(nodes)
	if err != nil {
		t.Fatalf("SerializeNodeList() error = %v", err)
	}

	got, err := DeserializeNodeList(data)
	if err != nil {
		t.Fatalf("DeserializeNodeList() error = %v", err)
	}

	if len(got) != len(nodes) {
		t.Fatalf("node count = %d, want %d", len(got), len(nodes))
	}

	for i, node := range nodes {
		if got[i].NodeID != node.NodeID {
			t.Errorf("node[%d].NodeID = %v, want %v", i, got[i].NodeID, node.NodeID)
		}
		if len(got[i].Addresses) != len(node.Addresses) {
			t.Errorf("node[%d].Addresses count = %d, want %d",
				i, len(got[i].Addresses), len(node.Addresses))
		}
	}
}

func TestSerializeDeserializeNodeList_Empty(t *testing.T) { // A
	nodes := []Node{}

	data, err := SerializeNodeList(nodes)
	if err != nil {
		t.Fatalf("SerializeNodeList() error = %v", err)
	}

	got, err := DeserializeNodeList(data)
	if err != nil {
		t.Fatalf("DeserializeNodeList() error = %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty node list, got %d nodes", len(got))
	}
}

func TestDeserializeNodeList_InvalidData(t *testing.T) { // A
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeserializeNodeList(tt.data)
			if err == nil {
				t.Error("DeserializeNodeList should return error for invalid data")
			}
		})
	}
}
