package carrier

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"
)

type receivedLogEntry struct { // A
	sender NodeID
	entry  LogEntryPayload
}

func TestCluster_LogSubscribeAllNodes_ReceivesAllLogs(t *testing.T) { // A
	network := &pipeNetwork{
		listeners:  make(map[string]*pipeListener),
		identities: make(map[string]nodeInfo),
		transports: make(map[*pipeTransport]string),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger := slog.New(
		slog.NewTextHandler(
			os.Stderr,
			&slog.HandlerOptions{Level: slog.LevelWarn},
		),
	)

	numNodes := 4
	subscribeSeen := make(chan NodeID, 32)
	carriers := make([]*DefaultCarrier, 0, numNodes)
	lbs := make([]*LogBroadcaster, 0, numNodes)

	for i := 0; i < numNodes; i++ {
		id, err := NewNodeIdentity()
		if err != nil {
			t.Fatalf("NewNodeIdentity failed: %v", err)
		}
		nodeID, err := NodeIDFromPublicKey(&id.PublicKey)
		if err != nil {
			t.Fatalf("NodeIDFromPublicKey failed: %v", err)
		}
		addr := "node-" + strconv.Itoa(i) + ":4242"

		transport := newPipeTransport(network)
		transport.RegisterNode(addr, nodeID, &id.PublicKey)

		c, err := NewDefaultCarrier(Config{
			LocalNode: Node{
				NodeID:    nodeID,
				Addresses: []string{addr},
				PublicKey: &id.PublicKey,
			},
			NodeIdentity: id,
			Logger:       logger,
			Transport:    transport,
		})
		if err != nil {
			t.Fatalf("NewDefaultCarrier failed: %v", err)
		}
		if err := c.Start(ctx); err != nil {
			t.Fatalf("start carrier %d: %v", i, err)
		}
		defer func() { _ = c.Stop(ctx) }()

		inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})
		lb := NewLogBroadcaster(LogBroadcasterConfig{
			Carrier:     c,
			LocalNodeID: nodeID,
			Inner:       inner,
		})
		lb.Start()
		defer lb.Stop()

		localLB := lb
		if i == 0 {
			c.RegisterHandler(MessageTypeLogSubscribe, localLB.HandleLogSubscribe)
		} else {
			c.RegisterHandler(
				MessageTypeLogSubscribe,
				func(
					ctx context.Context,
					senderID NodeID,
					msg Message,
				) (*Message, error) { // A
					resp, err := localLB.HandleLogSubscribe(
						ctx,
						senderID,
						msg,
					)
					select {
					case subscribeSeen <- senderID:
					default:
					}
					return resp, err
				},
			)
		}
		c.RegisterHandler(MessageTypeLogUnsubscribe, lb.HandleLogUnsubscribe)

		carriers = append(carriers, c)
		lbs = append(lbs, lb)
	}

	// Fully connect the cluster membership.
	for i := 0; i < numNodes; i++ {
		for j := 0; j < numNodes; j++ {
			if i == j {
				continue
			}
			if err := carriers[i].AddNode(ctx, carriers[j].LocalNode()); err != nil {
				t.Fatalf("AddNode %d->%d: %v", i, j, err)
			}
		}
	}

	subscriberIdx := 0
	subscriberID := carriers[subscriberIdx].LocalNode().NodeID
	heartbeatCh := make(chan NodeID, 32)

	recvCh := make(chan receivedLogEntry, 128)
	carriers[subscriberIdx].RegisterHandler(
		MessageTypeHeartbeat,
		func(
			ctx context.Context,
			senderID NodeID,
			msg Message,
		) (*Message, error) { // A
			select {
			case heartbeatCh <- senderID:
			default:
			}
			return nil, nil
		},
	)

	carriers[subscriberIdx].RegisterHandler(
		MessageTypeLogEntry,
		func(
			ctx context.Context,
			senderID NodeID,
			msg Message,
		) (*Message, error) { // A
			entry, err := DeserializeLogEntry(msg.Payload)
			if err != nil {
				return nil, err
			}
			select {
			case recvCh <- receivedLogEntry{sender: senderID, entry: entry}:
			default:
			}
			return nil, nil
		},
	)

	// Ensure each remote node establishes an outgoing connection to the
	// subscriber so the subscriber has an incoming connection to read from.
	for i := 0; i < numNodes; i++ {
		if i == subscriberIdx {
			continue
		}
		msg := Message{Type: MessageTypeHeartbeat, Payload: []byte("ping")}
		if err := carriers[i].SendMessageToNode(ctx, subscriberID, msg); err != nil {
			t.Fatalf("remote heartbeat %d->subscriber: %v", i, err)
		}
	}

	connected := make(map[NodeID]bool)
	deadlineConn := time.Now().Add(2 * time.Second)
	for len(connected) < numNodes-1 && time.Now().Before(deadlineConn) {
		select {
		case senderID := <-heartbeatCh:
			if senderID != subscriberID {
				connected[senderID] = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if len(connected) != numNodes-1 {
		t.Fatalf(
			"expected %d nodes to connect to subscriber, got %d",
			numNodes-1,
			len(connected),
		)
	}

	// Subscriber subscribes to logs from every other node.
	for i := 0; i < numNodes; i++ {
		if i == subscriberIdx {
			continue
		}
		payload, err := SerializeLogSubscribe(LogSubscribePayload{
			SubscriberNodeID: subscriberID,
			Levels:           nil,
		})
		if err != nil {
			t.Fatalf("SerializeLogSubscribe: %v", err)
		}
		msg := Message{Type: MessageTypeLogSubscribe, Payload: payload}
		targetID := carriers[i].LocalNode().NodeID
		if err := carriers[subscriberIdx].SendMessageToNode(
			ctx,
			targetID,
			msg,
		); err != nil {
			t.Fatalf("subscribe to %s: %v", targetID, err)
		}
	}

	seen := 0
	deadlineSeen := time.Now().Add(2 * time.Second)
	for seen < numNodes-1 && time.Now().Before(deadlineSeen) {
		select {
		case <-subscribeSeen:
			seen++
		case <-time.After(50 * time.Millisecond):
		}
	}
	if seen != numNodes-1 {
		t.Fatalf("expected %d subscribe deliveries, got %d", numNodes-1, seen)
	}

	// Give subscribe messages a moment to be processed.
	time.Sleep(50 * time.Millisecond)

	// Each non-subscriber node emits one log.
	for i := 0; i < numNodes; i++ {
		if i == subscriberIdx {
			continue
		}
		nodeMsg := "hello from node " + strconv.Itoa(i)
		slog.New(lbs[i]).InfoContext(ctx, nodeMsg)
	}

	// Subscriber should receive one log entry from each other node.
	deadline := time.Now().Add(3 * time.Second)
	gotFrom := make(map[NodeID]bool)
	for len(gotFrom) < numNodes-1 {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		select {
		case got := <-recvCh:
			if got.entry.SourceNodeID != got.sender {
				t.Fatalf(
					"source mismatch: payload=%s sender=%s",
					got.entry.SourceNodeID,
					got.sender,
				)
			}
			if got.sender == subscriberID {
				t.Fatalf("unexpected self log from %s", got.sender)
			}
			gotFrom[got.sender] = true
		case <-time.After(remaining):
			// fallthrough to timeout check
		}
	}

	if len(gotFrom) != numNodes-1 {
		t.Fatalf("expected %d senders, got %d", numNodes-1, len(gotFrom))
	}
}
