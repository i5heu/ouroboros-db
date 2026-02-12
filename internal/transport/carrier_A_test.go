package transport

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func testLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))
}

func newTestCarrier( // A
	t *testing.T,
	nodeID keys.NodeID,
) *Carrier {
	t.Helper()
	c, err := NewCarrier(CarrierConfig{
		ListenAddr:  "127.0.0.1:0",
		CarrierAuth: auth.NewCarrierAuth(),
		LocalNodeID: nodeID,
		Logger:      testLogger(),
	})
	if err != nil {
		t.Fatalf("NewCarrier: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestCarrierCreateAndClose( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	nodes := c.GetNodes()
	if len(nodes) != 0 {
		t.Fatalf(
			"expected 0 nodes, got %d",
			len(nodes),
		)
	}
}

func TestCarrierNilAuth(t *testing.T) { // A
	t.Parallel()
	_, err := NewCarrier(CarrierConfig{
		ListenAddr: "127.0.0.1:0",
		Logger:     testLogger(),
	})
	if err == nil {
		t.Fatal("expected error for nil auth")
	}
}

func TestCarrierNilLogger(t *testing.T) { // A
	t.Parallel()
	_, err := NewCarrier(CarrierConfig{
		ListenAddr:  "127.0.0.1:0",
		CarrierAuth: auth.NewCarrierAuth(),
	})
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
}

func TestCarrierSendMessageReliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	addrA := cA.ListenAddr()

	// Register A as a peer of B and establish
	// connection.
	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{addrA},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	// Send a reliable message from B to A.
	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("hello"),
	}
	err = cB.SendMessageToNode(peerA.NodeID, msg)
	if err != nil {
		t.Fatalf("SendMessageToNode: %v", err)
	}
}

func TestCarrierBroadcastReliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	// Connect B to A.
	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("broadcast"),
	}
	success, failed, err := cB.BroadcastReliable(msg)
	if err != nil {
		t.Fatalf("BroadcastReliable: %v", err)
	}
	if len(success) != 1 {
		t.Errorf(
			"expected 1 success, got %d",
			len(success),
		)
	}
	if len(failed) != 0 {
		t.Errorf(
			"expected 0 failed, got %d",
			len(failed),
		)
	}
}

func TestCarrierBroadcastUnreliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("dgram"),
	}
	attempted := cB.BroadcastUnreliable(msg)
	if len(attempted) != 1 {
		t.Errorf(
			"expected 1 attempted, got %d",
			len(attempted),
		)
	}
}

func TestCarrierJoinAndLeave(t *testing.T) { // A
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}

	// Join
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	nodes := cB.GetNodes()
	if len(nodes) != 1 {
		t.Fatalf(
			"expected 1 node after join, got %d",
			len(nodes),
		)
	}

	if !cB.IsConnected(peerA.NodeID) {
		t.Error("expected connected after join")
	}

	// Leave
	err = cB.LeaveCluster(peerA)
	if err != nil {
		t.Fatalf("LeaveCluster: %v", err)
	}

	nodes = cB.GetNodes()
	if len(nodes) != 0 {
		t.Fatalf(
			"expected 0 nodes after leave, got %d",
			len(nodes),
		)
	}
}

func TestCarrierRemoveNode(t *testing.T) { // A
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	err := cB.RemoveNode(peerA.NodeID)
	if err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}

	if cB.IsConnected(peerA.NodeID) {
		t.Error(
			"expected disconnected after remove",
		)
	}
}

func TestCarrierGetNodeNotFound( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	_, err := c.GetNode(keys.NodeID{99})
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestCarrierIsConnectedUnknown( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	if c.IsConnected(keys.NodeID{99}) {
		t.Error(
			"expected false for unknown node",
		)
	}
}

func TestCarrierImplementsInterface( // A
	t *testing.T,
) {
	t.Parallel()
	// Compile-time check that Carrier implements
	// interfaces.Carrier.
	var _ interfaces.Carrier = (*Carrier)(nil)
}

// TestCarrierSetMessageReceiver verifies that
// SetMessageReceiver installs the callback.
func TestCarrierSetMessageReceiver( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	called := false
	recv := MessageReceiver(
		func(
			_ interfaces.Message,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (interfaces.Response, error) {
			called = true
			return interfaces.Response{}, nil
		},
	)
	c.SetMessageReceiver(recv)

	if c.receiver.Load() == nil {
		t.Fatal("receiver not set")
	}
	_ = called
}

// TestCarrierInboundMessageDispatch verifies the
// full request/response cycle: sender opens a
// stream, writes a message, and reads the response
// from the receiver on the other carrier.
func TestCarrierInboundMessageDispatch( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	// Set up a receiver on A that echoes the
	// payload back.
	recv := MessageReceiver(
		func(
			msg interfaces.Message,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (interfaces.Response, error) {
			return interfaces.Response{
				Payload: msg.Payload,
				Metadata: map[string]string{
					"echo": "true",
				},
			}, nil
		},
	)
	cA.SetMessageReceiver(recv)

	// Connect B to A.
	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	// Give the accept loop time to spawn the
	// connection handler.
	time.Sleep(100 * time.Millisecond)

	// B sends a message to A and reads the response
	// by opening a stream directly.
	conn := cB.transport.GetConnection(peerA.NodeID)
	if conn == nil {
		t.Fatal("no connection to peer A")
	}

	stream, err := conn.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("ping"),
	}
	err = WriteMessage(stream, msg)
	if err != nil {
		t.Fatalf("write message: %v", err)
	}

	resp, err := ReadResponse(stream)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if string(resp.Payload) != "ping" {
		t.Fatalf(
			"payload: got %q, want %q",
			resp.Payload, "ping",
		)
	}
	if resp.Metadata["echo"] != "true" {
		t.Fatalf(
			"metadata: got %q, want %q",
			resp.Metadata["echo"], "true",
		)
	}
}

// TestCarrierInboundNoReceiver verifies that a
// message sent when no receiver is set returns an
// error response.
func TestCarrierInboundNoReceiver( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	// No receiver set on A.

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	conn := cB.transport.GetConnection(peerA.NodeID)
	if conn == nil {
		t.Fatal("no connection to peer A")
	}

	stream, err := conn.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("test"),
	}
	err = WriteMessage(stream, msg)
	if err != nil {
		t.Fatalf("write message: %v", err)
	}

	resp, err := ReadResponse(stream)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error response")
	}
}

// TestCarrierRegistry verifies that Registry()
// returns a non-nil NodeRegistry.
func TestCarrierRegistry(t *testing.T) { // A
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	reg := c.Registry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

// TestCarrierBroadcastWrapper verifies the Broadcast
// convenience method returns success and no error.
func TestCarrierBroadcastWrapper( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("wrap"),
	}
	success, err := cB.Broadcast(msg)
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if len(success) != 1 {
		t.Errorf(
			"expected 1 success, got %d",
			len(success),
		)
	}
}

// TestCarrierSendMessageToNodeUnreliable verifies
// the unreliable single-node send path.
func TestCarrierSendMessageToNodeUnreliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("dgram-single"),
	}
	err := cB.SendMessageToNodeUnreliable(
		peerA.NodeID, msg,
	)
	if err != nil {
		t.Fatalf(
			"SendMessageToNodeUnreliable: %v",
			err,
		)
	}
}

// TestCarrierSendUnreliableNoConn verifies that
// sending to an unconnected node returns an error.
func TestCarrierSendUnreliableNoConn( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("fail"),
	}
	err := c.SendMessageToNodeUnreliable(
		keys.NodeID{99}, msg,
	)
	if err == nil {
		t.Fatal(
			"expected error for unconnected node",
		)
	}
}

// TestCarrierGetNodeSuccess verifies GetNode returns
// the peer after JoinCluster.
func TestCarrierGetNodeSuccess( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	got, err := cB.GetNode(peerA.NodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.NodeID != peerA.NodeID {
		t.Fatalf(
			"NodeID = %v, want %v",
			got.NodeID, peerA.NodeID,
		)
	}
}

// TestCarrierJoinClusterDialError verifies that
// JoinCluster returns an error for a bad address.
func TestCarrierJoinClusterDialError( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	badPeer := interfaces.PeerNode{
		NodeID:    keys.NodeID{99},
		Addresses: []string{"127.0.0.1:1"},
	}
	err := c.JoinCluster(badPeer, nil)
	if err == nil {
		t.Fatal(
			"expected error for unreachable peer",
		)
	}
}

// TestCarrierEnsureConnectionUnknownNode verifies
// that sending to an unregistered node returns an
// error.
func TestCarrierEnsureConnectionUnknownNode( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("x"),
	}
	err := c.SendMessageToNodeReliable(
		keys.NodeID{99}, msg,
	)
	if err == nil {
		t.Fatal(
			"expected error for unknown node",
		)
	}
}
