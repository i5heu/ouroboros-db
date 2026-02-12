package clusterlog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/node"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"pgregory.net/rapid"
)

// mockCarrier records all messages sent via the
// Carrier interface so tests can inspect them.
type mockCarrier struct { // A
	mu       sync.Mutex
	nodes    []node.Node
	messages []sentMessage
}

type sentMessage struct { // A
	Target keys.NodeID
	Msg    interfaces.Message
}

func (m *mockCarrier) GetNodes() []node.Node { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

func (m *mockCarrier) Broadcast( // A
	msg interfaces.Message,
) ([]node.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		m.messages = append(m.messages, sentMessage{
			Target: n.ID(),
			Msg:    msg,
		})
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
	_ node.Node,
	_ *auth.NodeCert,
) error {
	return nil
}

func (m *mockCarrier) LeaveCluster( // A
	_ node.Node,
) error {
	return nil
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

func createTestNode(t *testing.T) node.Node { // A
	tmpDir := t.TempDir()
	n, err := node.New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create test node: %v", err)
	}
	return *n
}

func createTestNodeRapid(t *rapid.T) node.Node { // A
	tmpDir, err := os.MkdirTemp("", "node-test-*")
	if err != nil {
		panic(fmt.Sprintf("failed to create temp dir: %v", err))
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()
	n, err := node.New(tmpDir)
	if err != nil {
		panic(fmt.Sprintf("failed to create test node: %v", err))
	}
	return *n
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
	selfNode := createTestNode(t)
	sub := nodeID(3)

	carrier := &mockCarrier{
		nodes: []node.Node{
			selfNode,
			createTestNode(t),
		},
	}
	self := selfNode.ID()
	cl := New(newTestLogger(), carrier, self)
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
	selfNode := createTestNode(t)
	sub := nodeID(2)
	carrier := &mockCarrier{
		nodes: []node.Node{
			selfNode,
		},
	}
	self := selfNode.ID()
	cl := New(newTestLogger(), carrier, self)
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
