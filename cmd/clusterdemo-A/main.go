// GENERATED, NO HUMAN REVIEW, DO NOT RUN ON PRODUCTION SYSTEMS
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/cluster"
	"github.com/i5heu/ouroboros-db/internal/node"
	"github.com/i5heu/ouroboros-db/internal/transport"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

const (
	eventPrefix  = "OB_EVT "
	logKeyPeerID = "peerId"
	logKeyText   = "text"
	logKeyError  = "error"
)

type demoEvent struct { // A
	Event       string `json:"event"`
	NodeName    string `json:"nodeName"`
	NodeID      string `json:"nodeId,omitempty"`
	ListenAddr  string `json:"listenAddr,omitempty"`
	PeerID      string `json:"peerId,omitempty"`
	PeerAddr    string `json:"peerAddr,omitempty"`
	Text        string `json:"text,omitempty"`
	Success     int    `json:"success,omitempty"`
	Failed      int    `json:"failed,omitempty"`
	Error       string `json:"error,omitempty"`
	InputClosed bool   `json:"inputClosed,omitempty"`
}

type cliConfig struct { // A
	nodeName      string
	listenAddr    string
	dataDir       string
	enableStdIn   bool
	sendOnPlainIn bool
	joinPeers     peerSpecs
}

type peerSpecs []string

func (p *peerSpecs) String() string { // A
	return strings.Join(*p, ",")
}

func (p *peerSpecs) Set(value string) error { // A
	*p = append(*p, value)
	return nil
}

type eventWriter struct { // A
	nodeName string
	mu       sync.Mutex
}

func (w *eventWriter) emit(evt demoEvent) { // A
	evt.NodeName = w.nodeName
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = os.Stdout.WriteString(eventPrefix)
	_, _ = os.Stdout.Write(payload)
	_, _ = os.Stdout.WriteString("\n")
}

func main() { // A
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "clusterdemo: %v\n", err)
		os.Exit(1)
	}
}

func run() error { // A
	cfg := parseFlags()
	if strings.TrimSpace(cfg.nodeName) == "" {
		return errors.New("--node-name is required")
	}

	dir, cleanup, err := ensureDataDir(cfg.dataDir)
	if err != nil {
		return err
	}
	defer cleanup()

	localNode, err := node.New(dir)
	if err != nil {
		return fmt.Errorf("init node: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelInfo},
	))

	carrier, err := transport.NewCarrier(transport.CarrierConfig{
		ListenAddr:  cfg.listenAddr,
		CarrierAuth: auth.NewCarrierAuth(),
		LocalNodeID: localNode.ID(),
		Logger:      logger,
	})
	if err != nil {
		return fmt.Errorf("create carrier: %w", err)
	}
	defer func() {
		_ = carrier.Close()
	}()

	controller, err := cluster.NewClusterController(carrier, logger)
	if err != nil {
		return fmt.Errorf("create cluster controller: %w", err)
	}

	events := &eventWriter{nodeName: cfg.nodeName}
	registerRoutes(controller, events, logger)
	carrier.SetMessageReceiver(func(
		msg interfaces.Message,
		peer keys.NodeID,
		scope auth.TrustScope,
	) (interfaces.Response, error) {
		return controller.HandleIncomingMessage(msg, peer, scope)
	})

	events.emit(demoEvent{
		Event:      "ready",
		NodeID:     encodeNodeID(localNode.ID()),
		ListenAddr: carrier.ListenAddr(),
	})

	if err := joinFromFlags(carrier, cfg.joinPeers, events); err != nil {
		return err
	}

	sigCtx, stopSignal := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignal()

	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()

	lines, errs := startLineReader(os.Stdin, cfg.enableStdIn)
	return runLoop(
		ctx,
		lines,
		errs,
		cfg,
		localNode.ID(),
		carrier,
		events,
		cancel,
	)
}

func runLoop( // A
	ctx context.Context,
	lines <-chan string,
	errs <-chan error,
	cfg cliConfig,
	localNodeID keys.NodeID,
	carrier interfaces.Carrier,
	events *eventWriter,
	cancel context.CancelFunc,
) error {
	for {
		select {
		case <-ctx.Done():
			events.emit(demoEvent{Event: "stopped"})
			return nil
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err == nil {
				events.emit(demoEvent{
					Event:       "input_closed",
					InputClosed: true,
				})
				errs = nil
				continue
			}
			return err
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			err := handleLine(
				line,
				cfg.sendOnPlainIn,
				localNodeID,
				carrier,
				events,
				cancel,
			)
			if err != nil {
				events.emit(demoEvent{
					Event: "error",
					Error: err.Error(),
				})
			}
		}
	}
}

func parseFlags() cliConfig { // A
	var cfg cliConfig

	flag.StringVar(
		&cfg.nodeName,
		"node-name",
		"",
		"logical node name",
	)
	flag.StringVar(
		&cfg.listenAddr,
		"listen",
		"127.0.0.1:0",
		"listen address",
	)
	flag.StringVar(
		&cfg.dataDir,
		"data-dir",
		"",
		"node data dir (optional)",
	)
	flag.BoolVar(
		&cfg.enableStdIn,
		"stdin",
		true,
		"read commands from stdin",
	)
	flag.BoolVar(
		&cfg.sendOnPlainIn,
		"send-on-stdin",
		false,
		"send plain stdin lines as messages",
	)
	flag.Var(
		&cfg.joinPeers,
		"peer",
		"peer in format nodeID@host:port",
	)

	flag.Parse()
	return cfg
}

func ensureDataDir(dataDir string) ( // A
	string,
	func(),
	error,
) {
	if strings.TrimSpace(dataDir) != "" {
		if err := os.MkdirAll(dataDir, 0o750); err != nil {
			return "", nil, fmt.Errorf(
				"create data dir %q: %w",
				dataDir,
				err,
			)
		}
		return dataDir, func() {}, nil
	}

	tmpDir, err := os.MkdirTemp("", "clusterdemo-node-*")
	if err != nil {
		return "", nil, fmt.Errorf(
			"create temp data dir: %w",
			err,
		)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return tmpDir, cleanup, nil
}

func registerRoutes( // A
	controller interfaces.ClusterController,
	events *eventWriter,
	logger *slog.Logger,
) {
	err := controller.RegisterHandler(
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{
			auth.ScopeAdmin,
			auth.ScopeUser,
		},
		func(
			ctx context.Context,
			msg interfaces.Message,
			peer keys.NodeID,
			_ auth.TrustScope,
		) (interfaces.Response, error) {
			payload := interfaces.UserMessagePayload{}
			if err := json.Unmarshal(
				msg.Payload,
				&payload,
			); err != nil {
				return interfaces.Response{}, fmt.Errorf(
					"decode user message: %w",
					err,
				)
			}

			events.emit(demoEvent{
				Event:  "received",
				PeerID: encodeNodeID(peer),
				Text:   payload.Text,
			})
			logger.InfoContext(
				ctx,
				"received user message",
				logKeyPeerID,
				encodeNodeID(peer),
				logKeyText,
				payload.Text,
			)

			return interfaces.Response{
				Metadata: map[string]string{
					"status": "ok",
				},
			}, nil
		},
	)
	if err != nil {
		logger.ErrorContext(
			context.Background(),
			"register user message handler",
			logKeyError,
			err.Error(),
		)
	}
}

func startLineReader( // A
	reader *os.File,
	enabled bool,
) (<-chan string, <-chan error) {
	if !enabled {
		return nil, nil
	}

	lines := make(chan string, 8)
	errs := make(chan error, 1)

	go func() {
		defer close(lines)
		defer close(errs)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			lines <- line
		}
		errs <- scanner.Err()
	}()

	return lines, errs
}

func handleLine( // A
	line string,
	sendOnPlainInput bool,
	localNodeID keys.NodeID,
	carrier interfaces.Carrier,
	events *eventWriter,
	cancel context.CancelFunc,
) error {
	if strings.HasPrefix(line, "/join ") {
		peerID, addr, err := parseJoinCommand(line)
		if err != nil {
			return err
		}

		err = carrier.JoinCluster(
			interfaces.PeerNode{
				NodeID:    peerID,
				Addresses: []string{addr},
			},
			nil,
		)
		if err != nil {
			return fmt.Errorf("join: %w", err)
		}

		events.emit(demoEvent{
			Event:    "joined",
			PeerID:   encodeNodeID(peerID),
			PeerAddr: addr,
		})
		return nil
	}

	if strings.HasPrefix(line, "/send ") {
		text := strings.TrimSpace(
			strings.TrimPrefix(line, "/send "),
		)
		return sendUserMessage(localNodeID, text, carrier, events)
	}

	if line == "/quit" {
		cancel()
		return nil
	}

	if sendOnPlainInput {
		return sendUserMessage(localNodeID, line, carrier, events)
	}

	return nil
}

func parseJoinCommand(line string) ( // A
	keys.NodeID,
	string,
	error,
) {
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return keys.NodeID{}, "", fmt.Errorf(
			"join syntax: /join <nodeID> <addr>",
		)
	}

	peerID, err := decodeNodeID(parts[1])
	if err != nil {
		return keys.NodeID{}, "", err
	}
	return peerID, parts[2], nil
}

func sendUserMessage( // A
	localNodeID keys.NodeID,
	text string,
	carrier interfaces.Carrier,
	events *eventWriter,
) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	body, err := json.Marshal(interfaces.UserMessagePayload{
		From: encodeNodeID(localNodeID),
		Text: text,
	})
	if err != nil {
		return fmt.Errorf("encode user message: %w", err)
	}

	nodes := carrier.GetNodes()
	success := 0
	failed := 0
	for _, peer := range nodes {
		err := carrier.SendMessageToNodeReliable(
			peer.NodeID,
			interfaces.Message{
				Type:    interfaces.MessageTypeUserMessage,
				Payload: body,
			},
		)
		if err != nil {
			failed++
			continue
		}
		success++
	}

	events.emit(demoEvent{
		Event:   "sent",
		Text:    text,
		Success: success,
		Failed:  failed,
	})
	return nil
}

func joinFromFlags( // A
	carrier interfaces.Carrier,
	specs peerSpecs,
	events *eventWriter,
) error {
	for _, spec := range specs {
		peerID, addr, err := parsePeerSpec(spec)
		if err != nil {
			return err
		}
		err = carrier.JoinCluster(
			interfaces.PeerNode{
				NodeID:    peerID,
				Addresses: []string{addr},
			},
			nil,
		)
		if err != nil {
			return fmt.Errorf(
				"join peer %s (%s): %w",
				spec,
				encodeNodeID(peerID),
				err,
			)
		}
		events.emit(demoEvent{
			Event:    "joined",
			PeerID:   encodeNodeID(peerID),
			PeerAddr: addr,
		})
	}
	return nil
}

func parsePeerSpec(spec string) ( // A
	keys.NodeID,
	string,
	error,
) {
	parts := strings.Split(spec, "@")
	if len(parts) != 2 {
		return keys.NodeID{}, "", fmt.Errorf(
			"peer syntax: <nodeID>@<addr>",
		)
	}

	nodeID, err := decodeNodeID(parts[0])
	if err != nil {
		return keys.NodeID{}, "", err
	}
	addr := strings.TrimSpace(parts[1])
	if addr == "" {
		return keys.NodeID{}, "", errors.New(
			"peer address is empty",
		)
	}
	return nodeID, addr, nil
}

func encodeNodeID(id keys.NodeID) string { // A
	return base64.StdEncoding.EncodeToString(id[:])
}

func decodeNodeID(value string) (keys.NodeID, error) { // A
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"decode node id: %w",
			err,
		)
	}
	if len(raw) != len(keys.NodeID{}) {
		return keys.NodeID{}, fmt.Errorf(
			"invalid node id length: %d",
			len(raw),
		)
	}
	var out keys.NodeID
	copy(out[:], raw)
	return out, nil
}
