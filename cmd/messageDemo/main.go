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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

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

func main() { // H
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

func setupSignalHandler(cancel context.CancelFunc) { // H
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
) (*carrier.DefaultCarrier, carrier.NodeID, string, error) { // H
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

	// Use QUIC transport
	transport, err := carrier.NewQUICTransport(
		logger,
		nodeID,
		carrier.DefaultQUICConfig(),
	)
	if err != nil {
		return nil, "", "", fmt.Errorf("create transport: %w", err)
	}

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

func makeChatHandler() carrier.MessageHandler { // H
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
) { // H
	if stopErr := c.Stop(context.Background()); stopErr != nil {
		logger.WarnContext(ctx, "error stopping carrier", logKeyError, stopErr)
	}
}

func bootstrapPeer(
	ctx context.Context,
	logger *slog.Logger,
	c *carrier.DefaultCarrier,
	peer string,
) { // H
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

func printWelcome(port int, peer, localAddr string) { // H
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
) { // H
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
) { // H
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
) { // H
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

func handleHelpCommand() { // H
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
) { // H
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
