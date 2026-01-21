package carrier

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

type nodeInfo struct {
	nodeID NodeID
	pubKey *keys.PublicKey
}

type pipeNetwork struct {
	mu         sync.RWMutex
	listeners  map[string]*pipeListener
	identities map[string]nodeInfo
	transports map[*pipeTransport]string // maps transport instance to its bound address
}

// pipeTransport is a Transport implementation that uses Go channels
// to simulate a network for testing multiple carriers in memory.
type pipeTransport struct {
	network *pipeNetwork
}

func newPipeTransport(network *pipeNetwork) *pipeTransport {
	return &pipeTransport{
		network: network,
	}
}

func (t *pipeTransport) RegisterNode(address string, nodeID NodeID, pubKey *keys.PublicKey) {
	t.network.mu.Lock()
	defer t.network.mu.Unlock()
	t.network.identities[address] = nodeInfo{nodeID: nodeID, pubKey: pubKey}
}

func (t *pipeTransport) Connect(ctx context.Context, address string) (Connection, error) {
	t.network.mu.RLock()
	listener, ok := t.network.listeners[address]
	serverInfo := t.network.identities[address]

	// Find our own address to identify ourself to the server
	var clientAddr string
	for transport, addr := range t.network.transports {
		if transport == t {
			clientAddr = addr
			break
		}
	}
	clientInfo := t.network.identities[clientAddr]
	t.network.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("connection refused: no listener at %s", address)
	}

	toServer := make(chan Message, 100)
	toClient := make(chan Message, 100)
	serverClose := make(chan struct{})
	clientClose := make(chan struct{})

	clientConn := &pipeConnection{
		address:      address,
		remoteNodeID: serverInfo.nodeID,
		remotePubKey: serverInfo.pubKey,
		sendChan:     toServer,
		receiveChan:  toClient,
		closeChan:    clientClose,
		peerClose:    serverClose,
	}

	serverConn := &pipeConnection{
		address:      clientAddr,
		remoteNodeID: clientInfo.nodeID,
		remotePubKey: clientInfo.pubKey,
		sendChan:     toClient,
		receiveChan:  toServer,
		closeChan:    serverClose,
		peerClose:    clientClose,
	}

	select {
	case listener.acceptChan <- serverConn:
		return clientConn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Second):
		return nil, fmt.Errorf("timeout waiting for listener to accept")
	}
}

func (t *pipeTransport) Listen(ctx context.Context, address string) (Listener, error) {
	t.network.mu.Lock()
	defer t.network.mu.Unlock()

	if _, ok := t.network.listeners[address]; ok {
		return nil, fmt.Errorf("address already in use: %s", address)
	}

	t.network.transports[t] = address

	l := &pipeListener{
		addr:       address,
		acceptChan: make(chan Connection, 10),
		closeChan:  make(chan struct{}),
		network:    t.network,
	}
	t.network.listeners[address] = l
	return l, nil
}

func (t *pipeTransport) Close() error {
	// Individual carriers closing their transport should not affect others
	return nil
}

type pipeListener struct {
	addr       string
	acceptChan chan Connection
	closeChan  chan struct{}
	network    *pipeNetwork
}

func (l *pipeListener) Accept(ctx context.Context) (Connection, error) {
	select {
	case conn := <-l.acceptChan:
		return conn, nil
	case <-l.closeChan:
		return nil, fmt.Errorf("listener closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *pipeListener) Addr() string { return l.addr }

func (l *pipeListener) Close() error {
	l.network.mu.Lock()
	delete(l.network.listeners, l.addr)
	l.network.mu.Unlock()

	select {
	case <-l.closeChan:
		// already closed
	default:
		close(l.closeChan)
	}
	return nil
}

type pipeConnection struct {
	address      string
	remoteNodeID NodeID
	remotePubKey *keys.PublicKey
	sendChan     chan Message
	receiveChan  chan Message
	closeChan    chan struct{}
	peerClose    chan struct{}
	mu           sync.Mutex
	closed       bool
}

func (c *pipeConnection) Send(ctx context.Context, msg Message) error {
	select {
	case c.sendChan <- msg:
		return nil
	case <-c.closeChan:
		return fmt.Errorf("connection closed")
	case <-c.peerClose:
		return fmt.Errorf("peer closed connection")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *pipeConnection) SendEncrypted(ctx context.Context, enc *EncryptedMessage) error {
	return fmt.Errorf("SendEncrypted not implemented")
}

func (c *pipeConnection) Receive(ctx context.Context) (Message, error) {
	select {
	case msg := <-c.receiveChan:
		return msg, nil
	case <-c.closeChan:
		return Message{}, fmt.Errorf("connection closed")
	case <-c.peerClose:
		return Message{}, fmt.Errorf("peer closed connection")
	case <-ctx.Done():
		return Message{}, ctx.Err()
	}
}

func (c *pipeConnection) ReceiveEncrypted(ctx context.Context) (*EncryptedMessage, error) {
	return nil, fmt.Errorf("ReceiveEncrypted not implemented")
}

func (c *pipeConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		// Don't close immediately to let pending messages be received
		go func() {
			time.Sleep(100 * time.Millisecond)
			close(c.closeChan)
		}()
	}
	return nil
}

func (c *pipeConnection) RemoteNodeID() NodeID             { return c.remoteNodeID }
func (c *pipeConnection) RemotePublicKey() *keys.PublicKey { return c.remotePubKey }

func TestCarrierIntegration(t *testing.T) {
	network := &pipeNetwork{
		listeners:  make(map[string]*pipeListener),
		identities: make(map[string]nodeInfo),
		transports: make(map[*pipeTransport]string),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	numCarriers := 3
	carriers := make([]*DefaultCarrier, numCarriers)
	transports := make([]*pipeTransport, numCarriers)
	receivedMessages := make(map[NodeID]chan Message)

	for i := 0; i < numCarriers; i++ {
		id, _ := NewNodeIdentity()
		nodeID, _ := NodeIDFromPublicKey(&id.PublicKey)
		addr := fmt.Sprintf("node-%d:4242", i)

		transports[i] = newPipeTransport(network)
		transports[i].RegisterNode(addr, nodeID, &id.PublicKey)

		carriers[i], _ = NewDefaultCarrier(Config{
			LocalNode: Node{
				NodeID:    nodeID,
				Addresses: []string{addr},
			},
			NodeIdentity: id,
			Logger:       logger,
			Transport:    transports[i],
		})

		receivedChan := make(chan Message, 100)
		receivedMessages[nodeID] = receivedChan
		carriers[i].RegisterHandler(MessageTypeHeartbeat, func(ctx context.Context, senderID NodeID, msg Message) (*Message, error) {
			select {
			case receivedChan <- msg:
			default:
			}
			return &Message{Type: MessageTypeHeartbeat, Payload: []byte("pong")}, nil
		})

		if err := carriers[i].Start(ctx); err != nil {
			t.Fatalf("Failed to start carrier %d: %v", i, err)
		}
		defer carriers[i].Stop(ctx)
	}

	for i := 0; i < numCarriers; i++ {
		for j := 0; j < numCarriers; j++ {
			if i == j {
				continue
			}
			carriers[i].AddNode(ctx, carriers[j].LocalNode())
		}
	}

	t.Run("P2P messaging", func(t *testing.T) {
		msg := Message{Type: MessageTypeHeartbeat, Payload: []byte("ping from 0")}
		senderID := carriers[0].LocalNode().NodeID
		targetID := carriers[1].LocalNode().NodeID

		t.Logf("Node 0 (%s) sending P2P to Node 1 (%s)...", senderID, targetID)
		if err := carriers[0].SendMessageToNode(ctx, targetID, msg); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		select {
		case received := <-receivedMessages[targetID]:
			if string(received.Payload) != "ping from 0" {
				t.Errorf("Received wrong payload: %s", string(received.Payload))
			} else {
				t.Logf("Node 1 received P2P correctly")
			}
		case <-time.After(3 * time.Second):
			t.Error("Timed out waiting for message at Node 1")
		}
	})

	t.Run("Broadcast", func(t *testing.T) {
		broadcastMsg := Message{Type: MessageTypeHeartbeat, Payload: []byte("broadcast from 2")}
		senderID := carriers[2].LocalNode().NodeID

		t.Logf("Node 2 (%s) broadcasting to all...", senderID)
		result, err := carriers[2].Broadcast(ctx, broadcastMsg)
		if err != nil {
			t.Fatalf("Broadcast failed: %v", err)
		}
		t.Logf("Broadcast result: %d success, %d failed", len(result.SuccessNodes), len(result.FailedNodes))

		for i := 0; i < numCarriers; i++ {
			if i == 2 {
				continue
			}
			nodeID := carriers[i].LocalNode().NodeID
			select {
			case received := <-receivedMessages[nodeID]:
				if string(received.Payload) != "broadcast from 2" {
					t.Errorf("Node %d received wrong broadcast payload: %s", i, string(received.Payload))
				} else {
					t.Logf("Node %d received broadcast correctly", i)
				}
			case <-time.After(3 * time.Second):
				t.Errorf("Timed out waiting for broadcast at Node %d (%s)", i, nodeID)
			}
		}
	})
}
