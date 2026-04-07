package carrier

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

type stubDialTransport struct { // A
	dialErrByAddr map[string]error
	connByAddr    map[string]interfaces.Connection
	attempts      []string
}

func (s *stubDialTransport) Dial( // A
	node interfaces.Node,
) (interfaces.Connection, error) {
	if len(node.Addresses) == 0 {
		return nil, errors.New("missing address")
	}
	address := node.Addresses[0]
	s.attempts = append(s.attempts, address)
	if err := s.dialErrByAddr[address]; err != nil {
		return nil, err
	}
	conn, ok := s.connByAddr[address]
	if !ok {
		return nil, errors.New("missing conn")
	}
	return conn, nil
}

func (s *stubDialTransport) Accept() (interfaces.Connection, error) { // A
	return nil, errors.New("not implemented")
}

func (s *stubDialTransport) Close() error { // A
	return nil
}

func (s *stubDialTransport) GetActiveConnections( // A
) []interfaces.Connection {
	return nil
}

func TestHeartbeatPayloadRoundTripSenderRole( // A
	t *testing.T,
) {
	t.Parallel()
	decoded := decodeHeartbeatPayloadForTest(t)
	if decoded.SenderRole != interfaces.NodeRoleClient {
		t.Fatalf("sender role = %v, want client", decoded.SenderRole)
	}
}

func TestHeartbeatPayloadRoundTripKnownNodes( // A
	t *testing.T,
) {
	t.Parallel()
	decoded := decodeHeartbeatPayloadForTest(t)
	if len(decoded.KnownNodes) != 1 {
		t.Fatalf("known nodes len = %d, want 1", len(decoded.KnownNodes))
	}
	if decoded.KnownNodes[0].Role != interfaces.NodeRoleServer {
		t.Fatalf("node role = %v, want server", decoded.KnownNodes[0].Role)
	}
}

func TestHeartbeatPayloadRoundTripCustomFields( // A
	t *testing.T,
) {
	t.Parallel()
	decoded := decodeHeartbeatPayloadForTest(t)
	if string(decoded.CustomFields["stats"]) != "blob" {
		t.Fatalf("custom field mismatch: %q", decoded.CustomFields["stats"])
	}
}

func decodeHeartbeatPayloadForTest( // A
	t *testing.T,
) heartbeatPayload {
	t.Helper()
	encoded, err := marshalHeartbeatPayload(heartbeatPayload{
		SentAtUnix: 123,
		SenderRole: interfaces.NodeRoleClient,
		KnownNodes: []heartbeatNodeEntry{{
			NodeID:    keys.NodeID{1},
			Addresses: []string{"127.0.0.1:1000"},
			Role:      interfaces.NodeRoleServer,
		}},
		Stats: map[string]uint64{"knownNodes": 2},
		CustomFields: map[string][]byte{
			"stats": []byte("blob"),
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := unmarshalHeartbeatPayload(encoded)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return decoded
}

func TestHeartbeatBatchSkipsClientPeers( // A
	t *testing.T,
) {
	t.Parallel()
	_, cert := newNodeIdentityForCarrierTest(t)
	serverStream := newTestStream(nil)
	clientStream := newTestStream(nil)
	serverConn := newScriptedConn(
		newTLSBindingsForCarrierTest(),
		newExportersForCarrierTest(),
	)
	clientConn := newScriptedConn(
		newTLSBindingsForCarrierTest(),
		newExportersForCarrierTest(),
	)
	serverConn.openStream = serverStream
	clientConn.openStream = clientStream
	carrier := newCarrierTestHarness(CarrierConfig{SelfCert: cert})
	serverID := keys.NodeID{2}
	clientID := keys.NodeID{3}
	_ = carrier.registry.AddNode(interfaces.Node{
		NodeID:           serverID,
		Addresses:        []string{"server:1"},
		Role:             interfaces.NodeRoleServer,
		ConnectionStatus: interfaces.ConnectionStatusConnected,
		LastSeen:         time.Now(),
	}, nil, nil)
	_ = carrier.registry.AddNode(interfaces.Node{
		NodeID:           clientID,
		Addresses:        []string{"client:1"},
		Role:             interfaces.NodeRoleClient,
		ConnectionStatus: interfaces.ConnectionStatusConnected,
		LastSeen:         time.Now(),
	}, nil, nil)
	carrier.connections[serverID] = serverConn
	carrier.connections[clientID] = clientConn

	carrier.sendHeartbeatBatch(context.Background())

	serverOpens, _, _, _, _ := serverConn.counts()
	clientOpens, _, _, _, _ := clientConn.counts()
	if serverOpens != 1 {
		t.Fatalf("server open stream calls = %d, want 1", serverOpens)
	}
	if clientOpens != 0 {
		t.Fatalf("client open stream calls = %d, want 0", clientOpens)
	}
	if serverStream.writes.Len() == 0 {
		t.Fatal("expected heartbeat bytes for server peer")
	}
	if clientStream.writes.Len() != 0 {
		t.Fatal("unexpected heartbeat bytes for client peer")
	}
}

func TestHandleHeartbeatUpdatesRegistryAndMergesNodes( // A
	t *testing.T,
) {
	t.Parallel()
	_, cert := newNodeIdentityForCarrierTest(t)
	carrier := newCarrierTestHarness(CarrierConfig{SelfCert: cert})
	peerID := keys.NodeID{7}
	newNodeID := keys.NodeID{8}
	_ = carrier.registry.AddNode(interfaces.Node{
		NodeID:           peerID,
		Addresses:        []string{"peer:1"},
		Role:             interfaces.NodeRoleServer,
		ConnectionStatus: interfaces.ConnectionStatusDisconnected,
	}, nil, nil)
	payloadBytes, err := marshalHeartbeatPayload(heartbeatPayload{
		SentAtUnix: time.Now().Unix(),
		SenderRole: interfaces.NodeRoleClient,
		KnownNodes: []heartbeatNodeEntry{{
			NodeID:    newNodeID,
			Addresses: []string{"new:1", "new:2"},
			Role:      interfaces.NodeRoleServer,
		}},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	err = carrier.handleHeartbeatMessage(context.Background(), peerID, interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: payloadBytes,
	})
	if err != nil {
		t.Fatalf("handle heartbeat: %v", err)
	}
	peer, err := carrier.registry.GetNode(peerID)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.Role != interfaces.NodeRoleClient {
		t.Fatalf("peer role = %v, want client", peer.Role)
	}
	if peer.ConnectionStatus != interfaces.ConnectionStatusConnected {
		t.Fatalf("peer status = %v, want connected", peer.ConnectionStatus)
	}
	if peer.LastSeen.IsZero() {
		t.Fatal("expected peer last seen to be updated")
	}
	merged, err := carrier.registry.GetNode(newNodeID)
	if err != nil {
		t.Fatalf("get merged node: %v", err)
	}
	if len(merged.Addresses) != 2 {
		t.Fatalf("merged addresses len = %d, want 2", len(merged.Addresses))
	}
	if merged.Role != interfaces.NodeRoleServer {
		t.Fatalf("merged role = %v, want server", merged.Role)
	}
}

func TestFailStalePeersMarksNodeFailed( // A
	t *testing.T,
) {
	t.Parallel()
	_, cert := newNodeIdentityForCarrierTest(t)
	carrier := newCarrierTestHarness(CarrierConfig{SelfCert: cert})
	peerID := keys.NodeID{9}
	conn := newScriptedConn(
		newTLSBindingsForCarrierTest(),
		newExportersForCarrierTest(),
	)
	carrier.connections[peerID] = conn
	_ = carrier.registry.AddNode(interfaces.Node{
		NodeID:           peerID,
		Addresses:        []string{"stale:1"},
		Role:             interfaces.NodeRoleServer,
		LastSeen:         time.Now().Add(-time.Minute),
		ConnectionStatus: interfaces.ConnectionStatusConnected,
	}, nil, nil)

	carrier.failStalePeers(time.Now())

	if carrier.IsConnected(peerID) {
		t.Fatal("expected stale peer connection to be dropped")
	}
	node, err := carrier.registry.GetNode(peerID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if node.ConnectionStatus != interfaces.ConnectionStatusFailed {
		t.Fatalf("status = %v, want failed", node.ConnectionStatus)
	}
	_, _, _, _, closeCalls := conn.counts()
	if closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", closeCalls)
	}
}

func TestDialKnownAddressesTriesAllAddresses( // A
	t *testing.T,
) {
	t.Parallel()
	transport := &stubDialTransport{
		dialErrByAddr: map[string]error{
			"bad:1": errors.New("refused"),
		},
		connByAddr: map[string]interfaces.Connection{
			"good:1": newScriptedConn(
				newTLSBindingsForCarrierTest(),
				newExportersForCarrierTest(),
			),
		},
	}
	carrier := newCarrierTestHarness(CarrierConfig{})
	carrier.transport = transport

	_, usedAddress, err := carrier.dialKnownAddresses(interfaces.Node{
		NodeID:    keys.NodeID{4},
		Addresses: []string{"bad:1", "good:1"},
	})
	if err != nil {
		t.Fatalf("dialKnownAddresses: %v", err)
	}
	if usedAddress != "good:1" {
		t.Fatalf("used address = %q, want good:1", usedAddress)
	}
	if len(transport.attempts) != 2 {
		t.Fatalf("attempt count = %d, want 2", len(transport.attempts))
	}
	if transport.attempts[0] != "bad:1" || transport.attempts[1] != "good:1" {
		t.Fatalf("attempts = %v, want [bad:1 good:1]", transport.attempts)
	}
}

func TestRetryUnreachablePeersSkipsClientNodes( // A
	t *testing.T,
) {
	t.Parallel()
	transport := &stubDialTransport{}
	carrier := newCarrierTestHarness(CarrierConfig{})
	carrier.transport = transport
	_ = carrier.registry.AddNode(interfaces.Node{
		NodeID:           keys.NodeID{5},
		Addresses:        []string{"client:2"},
		Role:             interfaces.NodeRoleClient,
		ConnectionStatus: interfaces.ConnectionStatusFailed,
	}, nil, nil)

	carrier.retryUnreachablePeers(context.Background())

	if len(transport.attempts) != 0 {
		t.Fatalf("dial attempts = %d, want 0", len(transport.attempts))
	}
}

func TestNodeRoleString( // A
	t *testing.T,
) {
	t.Parallel()
	if got := interfaces.NodeRoleServer.String(); got != "server" {
		t.Fatalf("server string = %q", got)
	}
	if got := interfaces.NodeRoleClient.String(); got != "client" {
		t.Fatalf("client string = %q", got)
	}
}
