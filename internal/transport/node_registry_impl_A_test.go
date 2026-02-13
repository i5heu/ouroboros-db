package transport

import (
	"sync"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"pgregory.net/rapid"
)

func testPeer(b byte) interfaces.PeerNode { // A
	var id keys.NodeID
	id[0] = b
	return interfaces.PeerNode{
		NodeID:    id,
		Addresses: []string{"127.0.0.1:9000"},
	}
}

func TestRegistryAddAndGet(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()
	peer := testPeer(1)

	err := reg.AddNode(
		peer,
		[]byte("sig"),
		auth.ScopeAdmin,
	)
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	info, err := reg.GetNode(peer.NodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if info.Peer.NodeID != peer.NodeID {
		t.Errorf("NodeID mismatch")
	}
	if info.TrustScope != auth.ScopeAdmin {
		t.Errorf("scope mismatch")
	}
}

func TestRegistryRemoveNode(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()
	peer := testPeer(1)

	_ = reg.AddNode(
		peer,
		[]byte("sig"),
		auth.ScopeAdmin,
	)
	err := reg.RemoveNode(peer.NodeID)
	if err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}

	_, err = reg.GetNode(peer.NodeID)
	if err == nil {
		t.Fatal("expected error after remove")
	}
}

func TestRegistryRemoveNotFound(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()
	err := reg.RemoveNode(testPeer(99).NodeID)
	if err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestRegistryGetAllNodes(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()
	for i := byte(1); i <= 3; i++ {
		_ = reg.AddNode(
			testPeer(i),
			nil,
			auth.ScopeAdmin,
		)
	}
	all := reg.GetAllNodes()
	if len(all) != 3 {
		t.Fatalf("expected 3 nodes, got %d",
			len(all))
	}
}

func TestRegistryScopeFilter(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()

	_ = reg.AddNode(
		testPeer(1),
		nil,
		auth.ScopeAdmin,
	)
	_ = reg.AddNode(
		testPeer(2),
		nil,
		auth.ScopeUser,
	)
	_ = reg.AddNode(
		testPeer(3),
		nil,
		auth.ScopeAdmin,
	)

	admins := reg.GetAdminNodes()
	if len(admins) != 2 {
		t.Fatalf("expected 2 admins, got %d",
			len(admins))
	}

	users := reg.GetUserNodes()
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d",
			len(users))
	}
}

func TestRegistryUpdateConnectionStatus( // A
	t *testing.T,
) {
	t.Parallel()
	reg := NewNodeRegistry()
	peer := testPeer(1)
	_ = reg.AddNode(
		peer,
		nil,
		auth.ScopeAdmin,
	)

	err := reg.UpdateConnectionStatus(
		peer.NodeID,
		interfaces.ConnectionStatusConnected,
	)
	if err != nil {
		t.Fatalf("UpdateConnectionStatus: %v", err)
	}

	info, _ := reg.GetNode(peer.NodeID)
	if info.ConnectionStatus !=
		interfaces.ConnectionStatusConnected {
		t.Errorf("status not updated")
	}
}

func TestRegistryUpdateLastSeen(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()
	peer := testPeer(1)
	_ = reg.AddNode(
		peer,
		nil,
		auth.ScopeAdmin,
	)

	before, _ := reg.GetNode(peer.NodeID)
	err := reg.UpdateLastSeen(peer.NodeID)
	if err != nil {
		t.Fatalf("UpdateLastSeen: %v", err)
	}
	after, _ := reg.GetNode(peer.NodeID)
	if after.LastSeen.Before(before.LastSeen) {
		t.Errorf("LastSeen not updated")
	}
}

func TestRegistryConcurrentAccess( // A
	t *testing.T,
) {
	t.Parallel()
	reg := NewNodeRegistry()
	var wg sync.WaitGroup

	for i := byte(0); i < 50; i++ {
		wg.Add(1)
		go func(b byte) {
			defer wg.Done()
			peer := testPeer(b)
			_ = reg.AddNode(
				peer,
				nil,
				auth.ScopeAdmin,
			)
			_, _ = reg.GetNode(peer.NodeID)
			_ = reg.GetAllNodes()
		}(i)
	}
	wg.Wait()

	all := reg.GetAllNodes()
	if len(all) != 50 {
		t.Fatalf("expected 50 nodes, got %d",
			len(all))
	}
}

func TestRegistryPropertyAddRemove( // A
	t *testing.T,
) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		reg := NewNodeRegistry()
		added := make(map[keys.NodeID]bool)

		n := rapid.IntRange(1, 30).Draw(t, "ops")
		for range n {
			b := byte(
				rapid.IntRange(0, 20).Draw(t, "b"),
			)
			peer := testPeer(b)
			op := rapid.IntRange(0, 1).Draw(t, "op")

			switch op {
			case 0:
				_ = reg.AddNode(
					peer,
					nil,
					auth.ScopeAdmin,
				)
				added[peer.NodeID] = true
			case 1:
				err := reg.RemoveNode(peer.NodeID)
				if added[peer.NodeID] {
					if err != nil {
						t.Fatalf(
							"remove existing: %v",
							err,
						)
					}
					delete(added, peer.NodeID)
				}
			}
		}

		all := reg.GetAllNodes()
		if len(all) != len(added) {
			t.Fatalf(
				"expected %d nodes, got %d",
				len(added),
				len(all),
			)
		}
	})
}

func TestRegistryAdminVsUserFilterCorrectness(t *testing.T) { // A
	t.Parallel()
	reg := NewNodeRegistry()

	// Add 3 admin + 2 user nodes
	for i := byte(1); i <= 3; i++ {
		_ = reg.AddNode(testPeer(i), nil, auth.ScopeAdmin)
	}
	for i := byte(4); i <= 5; i++ {
		_ = reg.AddNode(testPeer(i), nil, auth.ScopeUser)
	}

	admins := reg.GetAdminNodes()
	users := reg.GetUserNodes()

	if len(admins) != 3 {
		t.Errorf("expected 3 admins, got %d", len(admins))
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	// Verify no overlap
	for _, a := range admins {
		for _, u := range users {
			if a.NodeID == u.NodeID {
				t.Errorf("node %s found in both admin and user lists", a.NodeID)
			}
		}
	}
}
