package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/i5heu/ouroboros-db/internal/carrier"
	"github.com/i5heu/ouroboros-db/pkg/dashboard"
)

func main() { // A
	// Parse command line flags
	cfg := parseFlags()

	// Setup logger
	logLevel := slog.LevelInfo
	if cfg.debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	logger.InfoContext(context.Background(), "starting ouroboros daemon",
		"listenAddr", cfg.listenAddr,
		"dataPath", cfg.dataPath,
		"dashboardEnabled", cfg.dashboardEnabled,
		"uploadEnabled", cfg.uploadEnabled)

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.InfoContext(ctx, "received shutdown signal", "signal", sig.String())
		cancel()
	}()

	// Run the daemon
	if err := run(ctx, cfg, logger); err != nil {
		logger.ErrorContext(context.Background(), "daemon error", "error", err)
		os.Exit(1)
	}
}

// daemonConfig holds the parsed command line configuration.
type daemonConfig struct { // A
	dataPath         string
	bootstrapAddr    string
	listenAddr       string
	dashboardEnabled bool
	dashboardPort    uint
	uploadEnabled    bool
	debug            bool
}

// parseFlags parses command line flags and returns the configuration.
func parseFlags() daemonConfig { // A
	cfg := daemonConfig{}

	flag.StringVar(&cfg.dataPath, "data", "./data",
		"Path to data directory")
	flag.StringVar(&cfg.bootstrapAddr, "bootstrap", "",
		"Bootstrap node address (host:port)")
	flag.StringVar(&cfg.listenAddr, "listen", ":4242",
		"Address to listen on for cluster communication")

	// Dashboard flags (intentionally ugly names to indicate UNSECURE)
	flag.BoolVar(&cfg.dashboardEnabled, "UNSECURE-dashboard", false,
		"Enable debug dashboard (INSECURE - do not use in production)")
	flag.UintVar(&cfg.dashboardPort, "UNSECURE-dashboard-port", 8420,
		"Port for debug dashboard")
	flag.BoolVar(&cfg.uploadEnabled, "UNSECURE-upload-via-dashboard", false,
		"Allow data uploads via dashboard (INSECURE)")

	flag.BoolVar(&cfg.debug, "debug", false,
		"Enable debug logging")

	flag.Parse()

	return cfg
}

// run is the main daemon logic, separated for testability.
func run(
	ctx context.Context,
	cfg daemonConfig,
	logger *slog.Logger,
) error { // A
	// Ensure data directory exists
	if err := os.MkdirAll(cfg.dataPath, 0o750); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	// Load or create node identity
	keyPath := filepath.Join(cfg.dataPath, "node.key")
	nodeIdentity, err := loadOrCreateIdentity(keyPath, logger)
	if err != nil {
		return fmt.Errorf("setup node identity: %w", err)
	}

	// Derive node ID from public key
	nodeID, err := carrier.NodeIDFromPublicKey(&nodeIdentity.PublicKey)
	if err != nil {
		return fmt.Errorf("derive node ID: %w", err)
	}

	logger.InfoContext(ctx, "node identity loaded",
		"nodeId", string(nodeID)[:16]+"...",
		"keyPath", keyPath)

	// Create QUIC transport
	transport, err := carrier.NewQUICTransport(
		logger,
		nodeID,
		carrier.DefaultQUICConfig(),
	)
	if err != nil {
		return fmt.Errorf("create QUIC transport: %w", err)
	}

	// Build local node info
	localNode := carrier.Node{
		NodeID:    nodeID,
		Addresses: []string{cfg.listenAddr},
		PublicKey: &nodeIdentity.PublicKey,
	}

	// Prepare bootstrap addresses
	var bootstrapAddrs []string
	if cfg.bootstrapAddr != "" {
		bootstrapAddrs = []string{cfg.bootstrapAddr}
	}

	// Create the carrier
	carrierCfg := carrier.Config{
		LocalNode:          localNode,
		NodeIdentity:       nodeIdentity,
		Logger:             logger,
		Transport:          transport,
		BootstrapAddresses: bootstrapAddrs,
	}

	carr, err := carrier.NewDefaultCarrier(carrierCfg)
	if err != nil {
		return fmt.Errorf("create carrier: %w", err)
	}

	// Register default message handlers
	carr.RegisterDefaultHandlers()

	// Setup LogBroadcaster if dashboard is enabled
	var logBroadcaster *carrier.LogBroadcaster
	if cfg.dashboardEnabled {
		logBroadcaster = carrier.NewLogBroadcaster(carrier.LogBroadcasterConfig{
			Carrier:     carr,
			LocalNodeID: nodeID,
			Inner:       logger.Handler(),
		})
		logBroadcaster.Start()
		defer logBroadcaster.Stop()

		// Replace logger with one that broadcasts
		logger = slog.New(logBroadcaster)

		// Register log subscription handlers
		carr.RegisterHandler(
			carrier.MessageTypeLogSubscribe,
			logBroadcaster.HandleLogSubscribe,
		)
		carr.RegisterHandler(
			carrier.MessageTypeLogUnsubscribe,
			logBroadcaster.HandleLogUnsubscribe,
		)
	}

	// Start the carrier (begins accepting connections)
	if err := carr.Start(ctx); err != nil {
		return fmt.Errorf("start carrier: %w", err)
	}
	defer func() {
		if stopErr := carr.Stop(context.Background()); stopErr != nil {
			logger.WarnContext(context.Background(), "error stopping carrier", "error", stopErr)
		}
	}()

	// Bootstrap to cluster if address provided
	if cfg.bootstrapAddr != "" {
		logger.Info("bootstrapping to cluster", "address", cfg.bootstrapAddr)
		if err := carr.Bootstrap(ctx); err != nil {
			logger.WarnContext(context.Background(), "bootstrap failed (peer may not be running yet)",
				"error", err)
		} else {
			logger.Info("bootstrap successful")
		}
	}

	// Start dashboard if enabled
	var dash *dashboard.Dashboard
	if cfg.dashboardEnabled {
		dash, err = dashboard.New(dashboard.Config{
			Enabled:        true,
			AllowUpload:    cfg.uploadEnabled,
			PreferredPort:  uint16(cfg.dashboardPort),
			Carrier:        carr,
			LogBroadcaster: logBroadcaster,
			Logger:         logger,
		})
		if err != nil {
			return fmt.Errorf("create dashboard: %w", err)
		}

		if err := dash.Start(ctx); err != nil {
			return fmt.Errorf("start dashboard: %w", err)
		}
		defer func() { _ = dash.Stop(ctx) }()

		logger.Info("dashboard available", "address", dash.Address())
	}

	logger.Info("daemon started",
		"nodeId", string(nodeID)[:16]+"...",
		"listenAddr", cfg.listenAddr)

	// Wait for shutdown
	<-ctx.Done()

	logger.Info("daemon shutting down")
	return nil
}

// loadOrCreateIdentity loads an existing node identity from disk or creates a
// new one if it doesn't exist.
func loadOrCreateIdentity(
	keyPath string,
	logger *slog.Logger,
) (*carrier.NodeIdentity, error) { // A
	// Check if key file exists
	if _, err := os.Stat(keyPath); err == nil {
		// Key exists, load it
		identity, err := carrier.NewNodeIdentityFromFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("load identity from %s: %w", keyPath, err)
		}
		logger.DebugContext(
			context.Background(),
			"loaded existing node identity",
			"path",
			keyPath,
		)
		return identity, nil
	}

	// Create new identity
	identity, err := carrier.NewNodeIdentity()
	if err != nil {
		return nil, fmt.Errorf("create new identity: %w", err)
	}

	// Save to disk
	if err := identity.SaveToFile(keyPath); err != nil {
		return nil, fmt.Errorf("save identity to %s: %w", keyPath, err)
	}

	logger.Info("created new node identity", "path", keyPath)
	return identity, nil
}
