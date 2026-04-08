package transport

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
)

// retryConn wraps scriptedConn to allow a dynamic
// OpenStream that fails a configurable number of
// times before succeeding.
type retryConn struct { // A
	scriptedConn
	failCount int32
	attempts  int32
}

func (rc *retryConn) OpenStream() ( // A
	interfaces.Stream, error,
) {
	n := atomic.AddInt32(&rc.attempts, 1)
	if n <= atomic.LoadInt32(&rc.failCount) {
		return nil, errors.New("connection closed")
	}
	replyBytes, err := proto.Marshal(&pb.MessageReply{
		Success: true,
	})
	if err != nil {
		return nil, err
	}
	return newTestStream(replyBytes), nil
}

func TestBroadcastReliableRetriesFailedPeers( // A
	t *testing.T,
) {
	t.Parallel()
	_, cert := newNodeIdentityForCarrierTest(t)
	carrier := newCarrierTestHarness(CarrierConfig{
		SelfCert:               cert,
		BroadcastMaxRetries:    3,
		BroadcastRetryInterval: 10 * time.Millisecond,
	})

	peerID := keys.NodeID{10}
	conn := &retryConn{
		scriptedConn: *newScriptedConn(
			newTLSBindingsForCarrierTest(),
			newExportersForCarrierTest(),
		),
		failCount: 2,
	}
	_ = carrier.registry.AddNode(&interfaces.Node{
		NodeID:           peerID,
		Addresses:        []string{"retry:1"},
		Role:             interfaces.NodeRoleServer,
		ConnectionStatus: interfaces.ConnectionStatusConnected,
		LastSeen:         time.Now(),
	}, nil, nil)
	carrier.connections[peerID] = conn

	success, failed, err := carrier.BroadcastReliable(
		interfaces.Message{
			Type:    interfaces.MessageTypeUserMessage,
			Payload: []byte("hello"),
		},
	)
	if err != nil {
		t.Fatalf("broadcast error: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf(
			"failed peers = %d, want 0", len(failed),
		)
	}
	if len(success) != 1 {
		t.Fatalf(
			"success peers = %d, want 1", len(success),
		)
	}
	got := atomic.LoadInt32(&conn.attempts)
	if got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestBroadcastReliableGivesUpAfterMaxRetries( // A
	t *testing.T,
) {
	t.Parallel()
	_, cert := newNodeIdentityForCarrierTest(t)
	carrier := newCarrierTestHarness(CarrierConfig{
		SelfCert:               cert,
		BroadcastMaxRetries:    2,
		BroadcastRetryInterval: 5 * time.Millisecond,
	})

	peerID := keys.NodeID{11}
	conn := &retryConn{
		scriptedConn: *newScriptedConn(
			newTLSBindingsForCarrierTest(),
			newExportersForCarrierTest(),
		),
		failCount: 100, // never succeeds
	}
	_ = carrier.registry.AddNode(&interfaces.Node{
		NodeID:           peerID,
		Addresses:        []string{"fail:1"},
		Role:             interfaces.NodeRoleServer,
		ConnectionStatus: interfaces.ConnectionStatusConnected,
		LastSeen:         time.Now(),
	}, nil, nil)
	carrier.connections[peerID] = conn

	success, failed, err := carrier.BroadcastReliable(
		interfaces.Message{
			Type:    interfaces.MessageTypeUserMessage,
			Payload: []byte("hello"),
		},
	)
	if err != nil {
		t.Fatalf("broadcast error: %v", err)
	}
	if len(success) != 0 {
		t.Fatalf(
			"success peers = %d, want 0",
			len(success),
		)
	}
	if len(failed) != 1 {
		t.Fatalf(
			"failed peers = %d, want 1", len(failed),
		)
	}
	// 1 initial + 2 retries = 3 total attempts
	got := atomic.LoadInt32(&conn.attempts)
	if got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestBroadcastReliableNoRetryOnSuccess( // A
	t *testing.T,
) {
	t.Parallel()
	_, cert := newNodeIdentityForCarrierTest(t)
	carrier := newCarrierTestHarness(CarrierConfig{
		SelfCert:               cert,
		BroadcastMaxRetries:    3,
		BroadcastRetryInterval: 5 * time.Millisecond,
	})

	peerID := keys.NodeID{12}
	conn := &retryConn{
		scriptedConn: *newScriptedConn(
			newTLSBindingsForCarrierTest(),
			newExportersForCarrierTest(),
		),
		failCount: 0, // succeeds immediately
	}
	_ = carrier.registry.AddNode(&interfaces.Node{
		NodeID:           peerID,
		Addresses:        []string{"ok:1"},
		Role:             interfaces.NodeRoleServer,
		ConnectionStatus: interfaces.ConnectionStatusConnected,
		LastSeen:         time.Now(),
	}, nil, nil)
	carrier.connections[peerID] = conn

	success, failed, err := carrier.BroadcastReliable(
		interfaces.Message{
			Type:    interfaces.MessageTypeUserMessage,
			Payload: []byte("hello"),
		},
	)
	if err != nil {
		t.Fatalf("broadcast error: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf(
			"failed peers = %d, want 0", len(failed),
		)
	}
	if len(success) != 1 {
		t.Fatalf(
			"success peers = %d, want 1", len(success),
		)
	}
	got := atomic.LoadInt32(&conn.attempts)
	if got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestMergeHeartbeatNodesReturnsDiscoveredPeers( // A
	t *testing.T,
) {
	t.Parallel()
	_, cert := newNodeIdentityForCarrierTest(t)
	carrier := newCarrierTestHarness(
		CarrierConfig{SelfCert: cert},
	)
	existingID := keys.NodeID{20}
	_ = carrier.registry.AddNode(&interfaces.Node{
		NodeID:           existingID,
		Addresses:        []string{"existing:1"},
		Role:             interfaces.NodeRoleServer,
		ConnectionStatus: interfaces.ConnectionStatusConnected,
	}, nil, nil)

	newID := keys.NodeID{21}
	discovered, err := carrier.mergeHeartbeatNodes(
		[]heartbeatNodeEntry{
			{
				NodeID:    existingID,
				Addresses: []string{"existing:2"},
				Role:      interfaces.NodeRoleServer,
			},
			{
				NodeID:    newID,
				Addresses: []string{"new:1"},
				Role:      interfaces.NodeRoleServer,
			},
		},
	)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf(
			"discovered = %d, want 1",
			len(discovered),
		)
	}
	if discovered[0].NodeID != newID {
		t.Fatalf(
			"discovered ID = %v, want %v",
			discovered[0].NodeID, newID,
		)
	}
}

func TestConnectDiscoveredPeersSkipsClients( // A
	t *testing.T,
) {
	t.Parallel()
	transport := &stubDialTransport{}
	carrier := newCarrierTestHarness(CarrierConfig{})
	carrier.transport = transport

	carrier.connectDiscoveredPeers(
		context.Background(),
		[]interfaces.Node{
			{
				NodeID:    keys.NodeID{30},
				Addresses: []string{"client:1"},
				Role:      interfaces.NodeRoleClient,
			},
		},
	)

	if len(transport.attempts) != 0 {
		t.Fatalf(
			"dial attempts = %d, want 0",
			len(transport.attempts),
		)
	}
}
