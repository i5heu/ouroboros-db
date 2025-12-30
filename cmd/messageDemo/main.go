// Package main provides a demo application for testing inter-node messaging.
// Run two instances on different ports to chat with yourself:
//
//	Terminal 1: go run ./cmd/messageDemo -port 4242
//	Terminal 2: go run ./cmd/messageDemo -port 4243 -peer localhost:4242
//
// Then type messages in either terminal to send them to the other.
package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/carrier"
)

// MessageTypeChat is a custom message type for chat messages in this demo.
const MessageTypeChat carrier.MessageType = 100

func main() {
	port := flag.Int("port", 4242, "Port to listen on")
	peer := flag.String(
		"peer",
		"",
		"Peer address to connect to (e.g., localhost:4243)",
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Create node identity
	nodeIdentity, err := carrier.NewNodeIdentity()
	if err != nil {
		logger.Error("failed to create node identity", "error", err)
		os.Exit(1)
	}

	nodeID, err := carrier.NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		logger.Error("failed to create node ID", "error", err)
		os.Exit(1)
	}

	localAddr := fmt.Sprintf("localhost:%d", *port)
	logger.Info("starting message demo",
		"nodeID", string(nodeID)[:8]+"...",
		"address", localAddr)

	// Create TCP transport
	transport := newTCPTransport(logger, nodeID)

	// Create carrier config
	cfg := carrier.Config{
		LocalNode: carrier.Node{
			NodeID:    nodeID,
			Addresses: []string{localAddr},
			PublicKey: &nodeIdentity.PublicKey,
		},
		NodeIdentity: nodeIdentity,
		Logger:       logger,
		Transport:    transport,
	}

	if *peer != "" {
		cfg.BootstrapAddresses = []string{*peer}
	}

	// Create carrier
	c, err := carrier.NewDefaultCarrier(cfg)
	if err != nil {
		logger.Error("failed to create carrier", "error", err)
		os.Exit(1)
	}

	// Register default handlers for node sync
	c.RegisterDefaultHandlers()

	// Register chat message handler
	c.RegisterHandler(MessageTypeChat, func(
		ctx context.Context,
		senderID carrier.NodeID,
		msg carrier.Message,
	) (*carrier.Message, error) {
		fmt.Printf("\n[%s...]: %s\n> ", string(senderID)[:8], string(msg.Payload))
		return nil, nil
	})

	// Start listening
	if err := c.Start(ctx); err != nil {
		logger.Error("failed to start carrier", "error", err)
		os.Exit(1)
	}
	defer func() {
		if stopErr := c.Stop(context.Background()); stopErr != nil {
			logger.Warn("error stopping carrier", "error", stopErr)
		}
	}()

	// Bootstrap if peer specified
	if *peer != "" {
		logger.Info("connecting to peer", "address", *peer)
		if err := c.Bootstrap(ctx); err != nil {
			logger.Warn(
				"bootstrap failed (peer may not be running yet)",
				"error",
				err,
			)
		} else {
			logger.Info("connected to peer")
		}
	}

	fmt.Println("\n=== Message Demo ===")
	fmt.Println("Type a message and press Enter to send.")
	fmt.Println("Press Ctrl+C to quit.")
	if *peer == "" {
		fmt.Printf(
			"Run another instance with: go run ./cmd/messageDemo -port %d -peer %s\n",
			*port+1,
			localAddr,
		)
	}
	fmt.Println()

	// Read input in a goroutine so we can handle cancellation
	inputCh := make(chan string)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				close(inputCh)
				return
			}
			inputCh <- strings.TrimSpace(line)
		}
	}()

	// Process input
	fmt.Print("> ")
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-inputCh:
			if !ok {
				return
			}

			if line == "" {
				fmt.Print("> ")
				continue
			}

			// Handle special commands
			if line == "/nodes" {
				nodes, nodesErr := c.GetNodes(ctx)
				if nodesErr != nil {
					fmt.Printf("Error: %v\n", nodesErr)
				} else {
					fmt.Printf("Known nodes (%d):\n", len(nodes))
					for _, node := range nodes {
						marker := ""
						if node.NodeID == nodeID {
							marker = " (self)"
						}
						fmt.Printf("  - %s... @ %v%s\n",
							string(node.NodeID)[:8], node.Addresses, marker)
					}
				}
				fmt.Print("> ")
				continue
			}

			if line == "/help" {
				fmt.Println("Commands:")
				fmt.Println("  /nodes  - List known nodes")
				fmt.Println("  /help   - Show this help")
				fmt.Println("  <text>  - Send message to all peers")
				fmt.Print("> ")
				continue
			}

			msg := carrier.Message{
				Type:    MessageTypeChat,
				Payload: []byte(line),
			}

			result, err := c.Broadcast(ctx, msg)
			if err != nil {
				logger.Error("broadcast failed", "error", err)
				fmt.Print("> ")
				continue
			}

			if len(result.SuccessNodes) == 0 && len(result.FailedNodes) == 0 {
				fmt.Println("(no peers connected)")
			} else if len(result.FailedNodes) > 0 {
				fmt.Printf("(sent to %d, failed %d)\n",
					len(result.SuccessNodes), len(result.FailedNodes))
			}
			fmt.Print("> ")
		}
	}
}

// --- Simple TCP Transport Implementation ---

type tcpTransport struct {
	logger   *slog.Logger
	localID  carrier.NodeID
	mu       sync.Mutex
	listener net.Listener
}

func newTCPTransport(
	logger *slog.Logger,
	localID carrier.NodeID,
) *tcpTransport {
	return &tcpTransport{
		logger:  logger,
		localID: localID,
	}
}

func (t *tcpTransport) Connect(
	ctx context.Context,
	address string,
) (carrier.Connection, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	tc := &tcpConnection{
		conn:    conn,
		localID: t.localID,
	}

	// Send our node ID
	if err := tc.sendNodeID(t.localID); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send node ID: %w", err)
	}

	// Receive remote node ID
	remoteID, err := tc.receiveNodeID()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to receive node ID: %w", err)
	}
	tc.remoteID = remoteID

	return tc, nil
}

func (t *tcpTransport) Listen(
	ctx context.Context,
	address string,
) (carrier.Listener, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	t.mu.Lock()
	t.listener = ln
	t.mu.Unlock()

	return &tcpListener{
		listener: ln,
		localID:  t.localID,
	}, nil
}

func (t *tcpTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

type tcpListener struct {
	listener net.Listener
	localID  carrier.NodeID
}

func (l *tcpListener) Accept(ctx context.Context) (carrier.Connection, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}

	tc := &tcpConnection{
		conn:    conn,
		localID: l.localID,
	}

	// Receive remote node ID first
	remoteID, err := tc.receiveNodeID()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to receive node ID: %w", err)
	}
	tc.remoteID = remoteID

	// Send our node ID
	if err := tc.sendNodeID(l.localID); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send node ID: %w", err)
	}

	return tc, nil
}

func (l *tcpListener) Addr() string {
	return l.listener.Addr().String()
}

func (l *tcpListener) Close() error {
	return l.listener.Close()
}

type tcpConnection struct {
	conn     net.Conn
	localID  carrier.NodeID
	remoteID carrier.NodeID
	mu       sync.Mutex
}

func (c *tcpConnection) sendNodeID(id carrier.NodeID) error {
	idBytes := []byte(id)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(idBytes)))

	if _, err := c.conn.Write(lenBuf); err != nil {
		return err
	}
	_, err := c.conn.Write(idBytes)
	return err
}

func (c *tcpConnection) receiveNodeID() (carrier.NodeID, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return "", err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	if length > 1024 { // Sanity check
		return "", errors.New("node ID too long")
	}

	idBytes := make([]byte, length)
	if _, err := io.ReadFull(c.conn, idBytes); err != nil {
		return "", err
	}
	return carrier.NodeID(idBytes), nil
}

func (c *tcpConnection) Send(ctx context.Context, msg carrier.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Message format: [1 byte type][4 bytes payload length][payload]
	header := make([]byte, 5)
	header[0] = byte(msg.Type)
	binary.BigEndian.PutUint32(header[1:], uint32(len(msg.Payload)))

	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if len(msg.Payload) > 0 {
		if _, err := c.conn.Write(msg.Payload); err != nil {
			return err
		}
	}
	return nil
}

func (c *tcpConnection) SendEncrypted(
	ctx context.Context,
	enc *carrier.EncryptedMessage,
) error {
	return errors.New("encrypted send not implemented in demo")
}

func (c *tcpConnection) Receive(ctx context.Context) (carrier.Message, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return carrier.Message{}, err
	}

	msgType := carrier.MessageType(header[0])
	payloadLen := binary.BigEndian.Uint32(header[1:])

	if payloadLen > 1024*1024 { // 1MB sanity check
		return carrier.Message{}, errors.New("payload too large")
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(c.conn, payload); err != nil {
			return carrier.Message{}, err
		}
	}

	return carrier.Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

func (c *tcpConnection) ReceiveEncrypted(
	ctx context.Context,
) (*carrier.EncryptedMessage, error) {
	return nil, errors.New("encrypted receive not implemented in demo")
}

func (c *tcpConnection) Close() error {
	return c.conn.Close()
}

func (c *tcpConnection) RemoteNodeID() carrier.NodeID {
	return c.remoteID
}

func (c *tcpConnection) RemotePublicKey() *keys.PublicKey {
	return nil // Not implemented in demo
}
