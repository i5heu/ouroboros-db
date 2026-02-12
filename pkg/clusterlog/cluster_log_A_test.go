package clusterlog

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// mockCarrier records all messages sent via the
// Carrier interface so tests can inspect them.
type mockCarrier struct { // A
	mu       sync.Mutex
	nodes    []interfaces.PeerNode
	messages []sentMessage
}

type sentMessage struct { // A
	Target keys.NodeID
	Msg    interfaces.Message
}

func (m *mockCarrier) GetNodes() []interfaces.PeerNode { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

func (m *mockCarrier) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.PeerNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.NodeID == nodeID {
			return n, nil
		}
	}
	return interfaces.PeerNode{}, nil
}

func (m *mockCarrier) BroadcastReliable( // A
	msg interfaces.Message,
) (
	[]interfaces.PeerNode,
	[]interfaces.PeerNode,
	error,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		m.messages = append(
			m.messages,
			sentMessage{
				Target: n.NodeID,
				Msg:    msg,
			},
		)
	}
	return m.nodes, nil, nil
}

func (m *mockCarrier) SendMessageToNodeReliable( // A
	nodeID keys.NodeID,
	msg interfaces.Message,
) error {
	return m.SendMessageToNode(nodeID, msg)
}

func (m *mockCarrier) BroadcastUnreliable( // A
	msg interfaces.Message,
) []interfaces.PeerNode {
	s, _, _ := m.BroadcastReliable(msg)
	return s
}

func (m *mockCarrier) SendMessageToNodeUnreliable( // A
	nodeID keys.NodeID,
	msg interfaces.Message,
) error {
	return m.SendMessageToNode(nodeID, msg)
}

func (m *mockCarrier) Broadcast( // A
	msg interfaces.Message,
) ([]interfaces.PeerNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		m.messages = append(
			m.messages,
			sentMessage{
				Target: n.NodeID,
				Msg:    msg,
			},
		)
	}
	return m.nodes, nil
}

func (m *mockCarrier) SendMessageToNode( // A
	nodeID keys.NodeID,
	msg interfaces.Message,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{
		Target: nodeID,
		Msg:    msg,
	})
	return nil
}

func (m *mockCarrier) JoinCluster( // A
	_ interfaces.PeerNode,
	_ *auth.NodeCert,
) error {
	return nil
}

func (m *mockCarrier) LeaveCluster( // A
	_ interfaces.PeerNode,
) error {
	return nil
}

func (m *mockCarrier) RemoveNode( // A
	_ keys.NodeID,
) error {
	return nil
}

func (m *mockCarrier) IsConnected( // A
	_ keys.NodeID,
) bool {
	return true
}

func (m *mockCarrier) getMessages() []sentMessage { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sentMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

func newTestLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))
}

func nodeID(b byte) keys.NodeID { // A
	var id keys.NodeID
	id[0] = b
	return id
}

func peerNode(id keys.NodeID) interfaces.PeerNode { // A
	return interfaces.PeerNode{NodeID: id}
}

func TestNewAndStop(t *testing.T) { // A
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, nodeID(1))
	cl.Stop()
}

func TestLogAndTail(t *testing.T) { // A
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, nodeID(1))
	defer cl.Stop()

	ctx := context.Background()
	cl.Info(ctx, "hello", nil)
	cl.Warn(ctx, "warning", map[string]string{
		"k": "v",
	})
	cl.Debug(ctx, "debug", nil)
	cl.Err(ctx, "error", nil)

	entries := cl.Tail(0)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d",
			len(entries))
	}

	entries = cl.Tail(2)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d",
			len(entries))
	}
	if entries[0].Message != "debug" {
		t.Errorf("expected 'debug', got %q",
			entries[0].Message)
	}
	if entries[1].Message != "error" {
		t.Errorf("expected 'error', got %q",
			entries[1].Message)
	}
}

func TestQuery(t *testing.T) { // A
	carrier := &mockCarrier{}
	self := nodeID(1)
	cl := New(newTestLogger(), carrier, self)
	defer cl.Stop()

	ctx := context.Background()
	cl.Info(ctx, "msg1", nil)

	before := time.Now()
	time.Sleep(2 * time.Millisecond)
	cl.Info(ctx, "msg2", nil)

	results := cl.Query(self, before)
	if len(results) != 1 {
		t.Fatalf("expected 1 entry, got %d",
			len(results))
	}
	if results[0].Message != "msg2" {
		t.Errorf("expected 'msg2', got %q",
			results[0].Message)
	}
}

func TestQueryAll(t *testing.T) { // A
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, nodeID(1))
	defer cl.Stop()

	ctx := context.Background()
	cl.Info(ctx, "a", nil)
	cl.Warn(ctx, "b", nil)

	all := cl.QueryAll(time.Time{})
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestQueryByLevel(t *testing.T) { // A
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, nodeID(1))
	defer cl.Stop()

	ctx := context.Background()
	cl.Info(ctx, "info1", nil)
	cl.Warn(ctx, "warn1", nil)
	cl.Info(ctx, "info2", nil)

	results := cl.QueryByLevel(
		LogLevelInfo, time.Time{},
	)
	if len(results) != 2 {
		t.Fatalf("expected 2 info entries, got %d",
			len(results))
	}
}

func TestSubscribeAndPush(t *testing.T) { // A
	self := nodeID(1)
	sub := nodeID(2)
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, self)
	defer cl.Stop()

	ctx := context.Background()
	cl.SubscribeLog(ctx, self, sub)

	cl.Info(ctx, "pushed", nil)

	msgs := carrier.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 push, got %d",
			len(msgs))
	}
	if msgs[0].Target != sub {
		t.Errorf("expected target %v, got %v",
			sub, msgs[0].Target)
	}
	if msgs[0].Msg.Type !=
		interfaces.MessageTypeLogPush {
		t.Errorf("expected LogPush type, got %d",
			msgs[0].Msg.Type)
	}

	var entries []LogEntry
	if err := json.Unmarshal(
		msgs[0].Msg.Payload, &entries,
	); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(entries) != 1 ||
		entries[0].Message != "pushed" {
		t.Errorf("unexpected payload: %v", entries)
	}
}

func TestSubscribeAllAndPush(t *testing.T) { // A
	selfID := nodeID(1)
	otherID := nodeID(2)
	sub := nodeID(3)

	carrier := &mockCarrier{
		nodes: []interfaces.PeerNode{
			peerNode(selfID),
			peerNode(otherID),
		},
	}
	cl := New(newTestLogger(), carrier, selfID)
	defer cl.Stop()

	ctx := context.Background()
	cl.SubscribeLogAll(ctx, sub)

	cl.Info(ctx, "broadcast-test", nil)

	msgs := carrier.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 push, got %d",
			len(msgs))
	}
	if msgs[0].Target != sub {
		t.Errorf("push should target subscriber")
	}
}

func TestUnsubscribeLog(t *testing.T) { // A
	self := nodeID(1)
	sub := nodeID(2)
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, self)
	defer cl.Stop()

	ctx := context.Background()
	cl.SubscribeLog(ctx, self, sub)
	cl.UnsubscribeLog(ctx, self, sub)

	cl.Info(ctx, "no-push", nil)

	msgs := carrier.getMessages()
	if len(msgs) != 0 {
		t.Fatalf(
			"expected 0 pushes after unsub, got %d",
			len(msgs),
		)
	}
}

func TestUnsubscribeLogAll(t *testing.T) { // A
	selfID := nodeID(1)
	sub := nodeID(2)
	carrier := &mockCarrier{
		nodes: []interfaces.PeerNode{
			peerNode(selfID),
		},
	}
	cl := New(newTestLogger(), carrier, selfID)
	defer cl.Stop()

	ctx := context.Background()
	cl.SubscribeLogAll(ctx, sub)
	cl.UnsubscribeLogAll(ctx, sub)

	cl.Info(ctx, "no-push", nil)

	msgs := carrier.getMessages()
	if len(msgs) != 0 {
		t.Fatalf(
			"expected 0 pushes after unsub, got %d",
			len(msgs),
		)
	}
}

func TestSelfPushSkipped(t *testing.T) { // A
	self := nodeID(1)
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, self)
	defer cl.Stop()

	ctx := context.Background()
	cl.SubscribeLog(ctx, self, self)

	cl.Info(ctx, "self-msg", nil)

	msgs := carrier.getMessages()
	if len(msgs) != 0 {
		t.Fatalf(
			"expected self-push skipped, got %d",
			len(msgs),
		)
	}
}

func TestSendLog(t *testing.T) { // A
	self := nodeID(1)
	target := nodeID(2)
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, self)
	defer cl.Stop()

	ctx := context.Background()
	cl.Info(ctx, "historical", nil)

	err := cl.SendLog(
		ctx, target, self, time.Time{},
	)
	if err != nil {
		t.Fatalf("SendLog: %v", err)
	}

	msgs := carrier.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 send, got %d",
			len(msgs))
	}
	if msgs[0].Msg.Type !=
		interfaces.MessageTypeLogSendResponse {
		t.Errorf("expected LogSendResponse, got %d",
			msgs[0].Msg.Type)
	}
}

func TestSendLogEmpty(t *testing.T) { // A
	self := nodeID(1)
	target := nodeID(2)
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, self)
	defer cl.Stop()

	ctx := context.Background()
	err := cl.SendLog(
		ctx, target, self, time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("SendLog should return nil: %v", err)
	}

	msgs := carrier.getMessages()
	if len(msgs) != 0 {
		t.Fatalf("expected 0 sends, got %d",
			len(msgs))
	}
}

func TestCleanup(t *testing.T) { // A
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, nodeID(1))
	defer cl.Stop()

	expired := LogEntry{
		Timestamp: time.Now().Add(-48 * time.Hour),
		NodeID:    nodeID(1),
		Level:     LogLevelDebug,
		Message:   "old",
	}
	fresh := LogEntry{
		Timestamp: time.Now(),
		NodeID:    nodeID(1),
		Level:     LogLevelDebug,
		Message:   "new",
	}

	cl.mu.Lock()
	cl.entries = append(cl.entries, expired, fresh)
	cl.mu.Unlock()

	cl.cleanup()

	cl.mu.RLock()
	defer cl.mu.RUnlock()
	if len(cl.entries) != 1 {
		t.Fatalf("expected 1 entry after cleanup, "+
			"got %d", len(cl.entries))
	}
	if cl.entries[0].Message != "new" {
		t.Errorf("wrong entry survived cleanup")
	}
}

func TestLogEntryIsExpired(t *testing.T) { // A
	now := time.Now()

	tests := []struct {
		name    string
		entry   LogEntry
		expired bool
	}{
		{
			name: "debug expired",
			entry: LogEntry{
				Timestamp: now.Add(-25 * time.Hour),
				Level:     LogLevelDebug,
			},
			expired: true,
		},
		{
			name: "debug fresh",
			entry: LogEntry{
				Timestamp: now.Add(-23 * time.Hour),
				Level:     LogLevelDebug,
			},
			expired: false,
		},
		{
			name: "info expired",
			entry: LogEntry{
				Timestamp: now.Add(-4 * 24 * time.Hour),
				Level:     LogLevelInfo,
			},
			expired: true,
		},
		{
			name: "info fresh",
			entry: LogEntry{
				Timestamp: now.Add(-2 * 24 * time.Hour),
				Level:     LogLevelInfo,
			},
			expired: false,
		},
		{
			name: "error expired",
			entry: LogEntry{
				Timestamp: now.Add(-6 * 24 * time.Hour),
				Level:     LogLevelError,
			},
			expired: true,
		},
		{
			name: "error fresh",
			entry: LogEntry{
				Timestamp: now.Add(-4 * 24 * time.Hour),
				Level:     LogLevelError,
			},
			expired: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.entry.isExpired(now)
			if got != tc.expired {
				t.Errorf("isExpired = %v, want %v",
					got, tc.expired)
			}
		})
	}
}

func TestLogLevelString(t *testing.T) { // A
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LogLevelDebug, "DEBUG"},
		{LogLevelInfo, "INFO"},
		{LogLevelWarn, "WARN"},
		{LogLevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}
	for _, tc := range tests {
		if got := tc.level.String(); got != tc.want {
			t.Errorf("LogLevel(%d).String() = %q, "+
				"want %q", tc.level, got, tc.want)
		}
	}
}

func TestLogEntryIsExpiredUnknownLevel( // A
	t *testing.T,
) {
	t.Parallel()

	now := time.Now()
	entry := LogEntry{
		Timestamp: now.Add(-4 * 24 * time.Hour),
		Level:     LogLevel(99),
	}
	// Unknown level falls back to Info TTL (3d),
	// so 4d old should be expired.
	if !entry.isExpired(now) {
		t.Error(
			"unknown level should fall back to " +
				"Info TTL and be expired",
		)
	}

	fresh := LogEntry{
		Timestamp: now.Add(-2 * 24 * time.Hour),
		Level:     LogLevel(99),
	}
	if fresh.isExpired(now) {
		t.Error(
			"unknown level 2d old should not be " +
				"expired (Info TTL = 3d)",
		)
	}
}

func TestConcurrentLogAccess(t *testing.T) { // A
	carrier := &mockCarrier{}
	cl := New(newTestLogger(), carrier, nodeID(1))
	defer cl.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cl.Info(ctx, "concurrent", nil)
		}()
	}
	wg.Wait()

	entries := cl.Tail(0)
	if len(entries) != 50 {
		t.Fatalf("expected 50 entries, got %d",
			len(entries))
	}
}
