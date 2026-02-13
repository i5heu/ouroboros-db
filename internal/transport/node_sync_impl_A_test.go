package transport

import (
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func TestNodeSyncStartStop(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()
	ns := NewNodeSync(reg, nil, nil)

	ns.StartSyncInterval(100 * time.Millisecond)
	// Starting again should be a no-op.
	ns.StartSyncInterval(100 * time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	ns.StopSync()

	// Stopping again should be safe.
	ns.StopSync()
}

func TestNodeSyncHandleNodeListUpdate( // A
	t *testing.T,
) {
	t.Parallel()
	reg := NewNodeRegistry()
	ns := NewNodeSync(
		reg,
		nil,
		auth.NewCarrierAuth(auth.CarrierAuthConfig{}),
	)

	update := []interfaces.NodeInfo{
		{
			Peer: interfaces.PeerNode{
				NodeID: keys.NodeID{1},
				Addresses: []string{
					"127.0.0.1:9000",
				},
			},
			TrustScope: auth.ScopeAdmin,
		},
		{
			Peer: interfaces.PeerNode{
				NodeID: keys.NodeID{2},
				Addresses: []string{
					"127.0.0.1:9001",
				},
			},
			TrustScope: auth.ScopeUser,
		},
	}

	err := ns.HandleNodeListUpdate(update)
	if err != nil {
		t.Fatalf("HandleNodeListUpdate: %v", err)
	}

	all := reg.GetAllNodes()
	if len(all) != 2 {
		t.Fatalf("expected 2 nodes, got %d",
			len(all))
	}

	// Update again to trigger LastSeen path.
	err = ns.HandleNodeListUpdate(update)
	if err != nil {
		t.Fatalf(
			"HandleNodeListUpdate second: %v",
			err,
		)
	}
}

func TestNodeSyncTriggerNoConnections( // A
	t *testing.T,
) {
	t.Parallel()
	reg := NewNodeRegistry()
	_ = reg.AddNode(
		interfaces.PeerNode{
			NodeID: keys.NodeID{1},
		},
		nil,
		auth.ScopeAdmin,
	)

	// Create a real transport to provide empty
	// connections list.
	tr, err := NewQuicTransport(
		"127.0.0.1:0",
		auth.NewCarrierAuth(auth.CarrierAuthConfig{}),
		keys.NodeID{99},
	)
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	defer func() {
		_ = tr.Close()
	}()

	ns := NewNodeSync(
		reg,
		tr,
		auth.NewCarrierAuth(auth.CarrierAuthConfig{}),
	)

	// TriggerFullSync should fail because there
	// are no active connections to the registered
	// peer.
	err = ns.TriggerFullSync()
	if err == nil {
		t.Fatal("expected error for no connection")
	}
}
