// Package harness provides a test harness for running Playwright E2E tests
// against a multi-node OuroborosDB cluster with dashboards enabled.
package harness

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-db/internal/carrier"
	"github.com/i5heu/ouroboros-db/pkg/dashboard"
)

// NodeConfig holds configuration for a single test node.
type NodeConfig struct { // A
	DataDir       string
	ListenAddr    string
	DashboardPort uint16
	BootstrapAddr string
}

// TestNode represents a running OuroborosDB node for E2E testing.
type TestNode struct { // A
	Config       NodeConfig
	NodeID       carrier.NodeID
	Carrier      *carrier.DefaultCarrier
	Dashboard    *dashboard.Dashboard
	Identity     *carrier.NodeIdentity
	DashboardURL string

	stopOnce sync.Once
	logger   *slog.Logger
}

// TestCluster manages a cluster of test nodes for E2E testing.
type TestCluster struct { // A
	Nodes   []*TestNode
	TempDir string
	logger  *slog.Logger
}

// ClusterOptions configures test cluster creation.
type ClusterOptions struct { // A
	BaseCarrierPort   int
	BaseDashboardPort int
}

// DefaultClusterOptions returns sensible defaults for test cluster ports.
func DefaultClusterOptions() ClusterOptions { // A
	return ClusterOptions{
		BaseCarrierPort:   14242,
		BaseDashboardPort: 18420,
	}
}

// NewTestCluster creates a new test cluster with the specified number of nodes.
// Each node gets its own data directory, carrier port, and dashboard port.
func NewTestCluster(
	ctx context.Context,
	numNodes int,
	logger *slog.Logger,
) (*TestCluster, error) { // A
	return NewTestClusterWithOptions(ctx, numNodes, logger, DefaultClusterOptions())
}

// NewTestClusterWithOptions creates a cluster with custom port configuration.
func NewTestClusterWithOptions(
	ctx context.Context,
	numNodes int,
	logger *slog.Logger,
	opts ClusterOptions,
) (*TestCluster, error) { // A
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	// Create temp directory for all node data
	tempDir, err := os.MkdirTemp("", "ouroboros-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	cluster := &TestCluster{
		Nodes:   make([]*TestNode, 0, numNodes),
		TempDir: tempDir,
		logger:  logger,
	}

	// Create and start each node
	baseCarrierPort := opts.BaseCarrierPort
	baseDashboardPort := opts.BaseDashboardPort

	for i := 0; i < numNodes; i++ {
		dataDir := filepath.Join(tempDir, fmt.Sprintf("node-%d", i))
		if err := os.MkdirAll(dataDir, 0o750); err != nil {
			cluster.Cleanup(ctx)
			return nil, fmt.Errorf("create node data dir: %w", err)
		}

		cfg := NodeConfig{
			DataDir:       dataDir,
			ListenAddr:    fmt.Sprintf("127.0.0.1:%d", baseCarrierPort+i),
			DashboardPort: uint16(baseDashboardPort + i),
		}

		// Nodes after the first bootstrap to the first node
		if i > 0 {
			cfg.BootstrapAddr = fmt.Sprintf("127.0.0.1:%d", baseCarrierPort)
		}

		node, err := newTestNode(ctx, cfg, logger)
		if err != nil {
			cluster.Cleanup(ctx)
			return nil, fmt.Errorf("create node %d: %w", i, err)
		}

		cluster.Nodes = append(cluster.Nodes, node)
	}

	// Wait for all nodes to connect and form a cluster
	if err := cluster.waitForCluster(ctx, 10*time.Second); err != nil {
		cluster.Cleanup(ctx)
		return nil, fmt.Errorf("cluster formation: %w", err)
	}

	return cluster, nil
}

// newTestNode creates and starts a single test node.
func newTestNode(
	ctx context.Context,
	cfg NodeConfig,
	logger *slog.Logger,
) (*TestNode, error) { // A
	// Create node identity
	identity, err := carrier.NewNodeIdentity()
	if err != nil {
		return nil, fmt.Errorf("create identity: %w", err)
	}

	nodeID, err := carrier.NodeIDFromPublicKey(&identity.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("derive node ID: %w", err)
	}

	// Create QUIC transport
	transport, err := carrier.NewQUICTransport(
		logger,
		nodeID,
		carrier.DefaultQUICConfig(),
	)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}

	// Build local node info
	localNode := carrier.Node{
		NodeID:    nodeID,
		Addresses: []string{cfg.ListenAddr},
		PublicKey: &identity.PublicKey,
	}

	// Prepare bootstrap addresses
	var bootstrapAddrs []string
	if cfg.BootstrapAddr != "" {
		bootstrapAddrs = []string{cfg.BootstrapAddr}
	}

	// Create the carrier
	carrierCfg := carrier.Config{
		LocalNode:          localNode,
		NodeIdentity:       identity,
		Logger:             logger,
		Transport:          transport,
		BootstrapAddresses: bootstrapAddrs,
	}

	carr, err := carrier.NewDefaultCarrier(carrierCfg)
	if err != nil {
		return nil, fmt.Errorf("create carrier: %w", err)
	}

	// Register default message handlers
	carr.RegisterDefaultHandlers()

	// Setup LogBroadcaster for dashboard log streaming
	logBroadcaster := carrier.NewLogBroadcaster(carrier.LogBroadcasterConfig{
		Carrier:     carr,
		LocalNodeID: nodeID,
		Inner:       logger.Handler(),
	})
	logBroadcaster.Start()

	// Create logger that broadcasts to subscribers
	broadcastLogger := slog.New(logBroadcaster)

	// Register log subscription handlers
	carr.RegisterHandler(
		carrier.MessageTypeLogSubscribe,
		logBroadcaster.HandleLogSubscribe,
	)
	carr.RegisterHandler(
		carrier.MessageTypeLogUnsubscribe,
		logBroadcaster.HandleLogUnsubscribe,
	)

	// Start the carrier
	if err := carr.Start(ctx); err != nil {
		logBroadcaster.Stop()
		return nil, fmt.Errorf("start carrier: %w", err)
	}

	// Bootstrap to cluster if address provided
	if cfg.BootstrapAddr != "" {
		if err := carr.Bootstrap(ctx); err != nil {
			// Log but don't fail - peer may not be ready yet
			logger.WarnContext(ctx, "bootstrap failed, will retry",
				"error", err, "bootstrapAddr", cfg.BootstrapAddr)
		}
	}

	// Create and start dashboard
	dash, err := dashboard.New(dashboard.Config{
		Enabled:        true,
		AllowUpload:    true,
		PreferredPort:  cfg.DashboardPort,
		Carrier:        carr,
		LogBroadcaster: logBroadcaster,
		Logger:         broadcastLogger,
	})
	if err != nil {
		_ = carr.Stop(ctx)
		logBroadcaster.Stop()
		return nil, fmt.Errorf("create dashboard: %w", err)
	}

	if err := dash.Start(ctx); err != nil {
		_ = carr.Stop(ctx)
		logBroadcaster.Stop()
		return nil, fmt.Errorf("start dashboard: %w", err)
	}

	node := &TestNode{
		Config:       cfg,
		NodeID:       nodeID,
		Carrier:      carr,
		Dashboard:    dash,
		Identity:     identity,
		DashboardURL: dash.Address(),
		logger:       broadcastLogger,
	}

	return node, nil
}

// Stop gracefully stops the test node.
func (n *TestNode) Stop(ctx context.Context) error { // A
	var firstErr error
	n.stopOnce.Do(func() {
		if n.Dashboard != nil {
			if err := n.Dashboard.Stop(ctx); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("stop dashboard: %w", err)
			}
		}
		if n.Carrier != nil {
			if err := n.Carrier.Stop(ctx); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("stop carrier: %w", err)
			}
		}
	})
	return firstErr
}

// waitForCluster waits for all nodes to see each other.
func (c *TestCluster) waitForCluster(
	ctx context.Context,
	timeout time.Duration,
) error { // A
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		allConnected := true

		for _, node := range c.Nodes {
			nodes, err := node.Carrier.GetNodes(ctx)
			if err != nil {
				allConnected = false
				break
			}
			// Each node should see all other nodes
			if len(nodes) < len(c.Nodes) {
				allConnected = false
				break
			}
		}

		if allConnected {
			// Verify dashboards are accessible
			for _, node := range c.Nodes {
				if !isDashboardReady(node.DashboardURL) {
					allConnected = false
					break
				}
			}
		}

		if allConnected {
			c.logger.InfoContext(ctx, "cluster formed",
				"nodeCount", len(c.Nodes))
			return nil
		}

		// Retry bootstrapping for non-primary nodes
		for i, node := range c.Nodes {
			if i > 0 && node.Config.BootstrapAddr != "" {
				_ = node.Carrier.Bootstrap(ctx)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for cluster formation")
}

// isDashboardReady checks if a dashboard is responding to HTTP requests.
func isDashboardReady(url string) bool { // A
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/api/nodes")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// PrimaryDashboardURL returns the URL of the first node's dashboard.
func (c *TestCluster) PrimaryDashboardURL() string { // A
	if len(c.Nodes) == 0 {
		return ""
	}
	return c.Nodes[0].DashboardURL
}

// Cleanup stops all nodes and removes temporary directories.
func (c *TestCluster) Cleanup(ctx context.Context) { // A
	c.logger.InfoContext(ctx, "cleaning up test cluster")

	// Stop all nodes
	for _, node := range c.Nodes {
		if err := node.Stop(ctx); err != nil {
			c.logger.WarnContext(ctx, "error stopping node",
				"nodeID", string(node.NodeID)[:16], "error", err)
		}
	}

	// Remove temp directory
	if c.TempDir != "" {
		if err := os.RemoveAll(c.TempDir); err != nil {
			c.logger.WarnContext(ctx, "error removing temp dir",
				"path", c.TempDir, "error", err)
		}
	}
}
