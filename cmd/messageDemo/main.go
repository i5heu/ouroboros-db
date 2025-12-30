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

// Log key constants.
const (
	logKeyError   = "error"
	logKeyNodeID  = "nodeId"
	logKeyAddress = "address"
)

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

	setupSignalHandler(cancel)

	c, nodeID, localAddr, err := setupCarrier(ctx, logger, *port, *peer)
	if err != nil {
		logger.ErrorContext(ctx, "setup failed", logKeyError, err)
		os.Exit(1)
	}
	defer stopCarrier(ctx, logger, c)

	if *peer != "" {
		bootstrapPeer(ctx, logger, c, *peer)
	}

	printWelcome(*port, *peer, localAddr)
	runInputLoop(ctx, logger, c, nodeID)
}

func setupSignalHandler(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()
}

func setupCarrier(
	ctx context.Context,
	logger *slog.Logger,
	port int,
	peer string,
) (*carrier.DefaultCarrier, carrier.NodeID, string, error) {
	nodeIdentity, err := carrier.NewNodeIdentity()
	if err != nil {
		return nil, "", "", fmt.Errorf("create node identity: %w", err)
	}

	nodeID, err := carrier.NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		return nil, "", "", fmt.Errorf("create node ID: %w", err)
	}

	localAddr := fmt.Sprintf("localhost:%d", port)
	logger.InfoContext(ctx, "starting message demo",
		logKeyNodeID, string(nodeID)[:8]+"...",
		logKeyAddress, localAddr)

	transport := newTCPTransport(logger, nodeID)

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

	if peer != "" {
		cfg.BootstrapAddresses = []string{peer}
	}

	c, err := carrier.NewDefaultCarrier(cfg)
	if err != nil {
		return nil, "", "", fmt.Errorf("create carrier: %w", err)
	}

	c.RegisterDefaultHandlers()
	c.RegisterHandler(MessageTypeChat, makeChatHandler())

	if err := c.Start(ctx); err != nil {
		return nil, "", "", fmt.Errorf("start carrier: %w", err)
	}

	return c, nodeID, localAddr, nil
}

func makeChatHandler() carrier.MessageHandler {
	return func(
		_ context.Context,
		senderID carrier.NodeID,
		msg carrier.Message,
	) (*carrier.Message, error) {
		fmt.Printf("\n[%s...]: %s\n> ", string(senderID)[:8], string(msg.Payload))
		return nil, nil
	}
}

func stopCarrier(
	ctx context.Context,
	logger *slog.Logger,
	c *carrier.DefaultCarrier,
) {
	if stopErr := c.Stop(context.Background()); stopErr != nil {
		logger.WarnContext(ctx, "error stopping carrier", logKeyError, stopErr)
	}
}

func bootstrapPeer(
	ctx context.Context,
	logger *slog.Logger,
	c *carrier.DefaultCarrier,
	peer string,
) {
	logger.InfoContext(ctx, "connecting to peer", logKeyAddress, peer)
	if err := c.Bootstrap(ctx); err != nil {
		logger.WarnContext(ctx,
			"bootstrap failed (peer may not be running yet)",
			logKeyError,
			err,
		)
	} else {
		logger.InfoContext(ctx, "connected to peer")
	}
}

func printWelcome(port int, peer, localAddr string) {
	fmt.Println("\n=== Message Demo ===")
	fmt.Println("Type a message and press Enter to send.")
	fmt.Println("Press Ctrl+C to quit.")
	if peer == "" {
		fmt.Printf(
			"Run another instance with: "+
				"go run ./cmd/messageDemo -port %d -peer %s\n",
			port+1,
			localAddr,
		)
	}
	fmt.Println()
}

func runInputLoop(
	ctx context.Context,
	logger *slog.Logger,
	c *carrier.DefaultCarrier,
	nodeID carrier.NodeID,
) {
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

	fmt.Print("> ")
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-inputCh:
			if !ok {
				return
			}
			handleInput(ctx, logger, c, nodeID, line)
		}
	}
}

func handleInput(
	ctx context.Context,
	logger *slog.Logger,
	c *carrier.DefaultCarrier,
	nodeID carrier.NodeID,
	line string,
) {
	if line == "" {
		fmt.Print("> ")
		return
	}

	switch line {
	case "/nodes":
		handleNodesCommand(ctx, c, nodeID)
	case "/help":
		handleHelpCommand()
	default:
		handleChatMessage(ctx, logger, c, line)
	}
}

func handleNodesCommand(
	ctx context.Context,
	c *carrier.DefaultCarrier,
	nodeID carrier.NodeID,
) {
	nodes, err := c.GetNodes(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
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
}

func handleHelpCommand() {
	fmt.Println("Commands:")
	fmt.Println("  /nodes  - List known nodes")
	fmt.Println("  /help   - Show this help")
	fmt.Println("  <text>  - Send message to all peers")
	fmt.Print("> ")
}

func handleChatMessage(
	ctx context.Context,
	logger *slog.Logger,
	c *carrier.DefaultCarrier,
	line string,
) {
	msg := carrier.Message{
		Type:    MessageTypeChat,
		Payload: []byte(line),
	}

	result, err := c.Broadcast(ctx, msg)
	if err != nil {
		logger.ErrorContext(ctx, "broadcast failed", logKeyError, err)
		fmt.Print("> ")
		return
	}

	if len(result.SuccessNodes) == 0 && len(result.FailedNodes) == 0 {
		fmt.Println("(no peers connected)")
	} else if len(result.FailedNodes) > 0 {
		fmt.Printf("(sent to %d, failed %d)\n",
			len(result.SuccessNodes), len(result.FailedNodes))
	}
	fmt.Print("> ")
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
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf(
				"failed to send node ID: %w (close err: %v)",
				err, closeErr)
		}
		return nil, fmt.Errorf("failed to send node ID: %w", err)
	}

	// Receive remote node ID
	remoteID, err := tc.receiveNodeID()
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf(
				"failed to receive node ID: %w (close err: %v)",
				err, closeErr)
		}
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
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf(
				"failed to receive node ID: %w (close err: %v)",
				err, closeErr)
		}
		return nil, fmt.Errorf("failed to receive node ID: %w", err)
	}
	tc.remoteID = remoteID

	// Send our node ID
	if err := tc.sendNodeID(l.localID); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf(
				"failed to send node ID: %w (close err: %v)",
				err, closeErr)
		}
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
	//nolint:gosec // length is bounded by nodeID size
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
	//nolint:gosec // length is bounded by message size
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
	_ context.Context,
	_ *carrier.EncryptedMessage,
) error {
	return errors.New("encrypted send not implemented in demo")
}

func (c *tcpConnection) Receive(_ context.Context) (carrier.Message, error) {
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
	_ context.Context,
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
