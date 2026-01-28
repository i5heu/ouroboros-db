package harness

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestPlaywrightE2E is the main entry point for running Playwright E2E tests.
// It starts a multi-node OuroborosDB cluster, then runs the Playwright test
// suite against it.
//
// Run with: go test ./e2e/harness -v -run TestPlaywrightE2E -timeout 180s
func TestPlaywrightE2E(t *testing.T) { // A
	if testing.Short() {
		t.Skip("skipping E2E tests in short mode")
	}

	// Check if npm/npx is available
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not found, skipping E2E tests")
	}

	// Setup logger
	logLevel := slog.LevelInfo
	if os.Getenv("E2E_DEBUG") != "" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Start the test cluster with 3 nodes
	logger.InfoContext(ctx, "starting test cluster")
	cluster, err := NewTestCluster(ctx, 3, logger)
	if err != nil {
		t.Fatalf("failed to create test cluster: %v", err)
	}
	defer cluster.Cleanup(ctx)

	dashboardURL := cluster.PrimaryDashboardURL()
	logger.InfoContext(ctx, "cluster started",
		"dashboardURL", dashboardURL,
		"nodeCount", len(cluster.Nodes))

	// Log all node dashboard URLs for debugging
	for i, node := range cluster.Nodes {
		logger.InfoContext(ctx, "node dashboard",
			"node", i,
			"url", node.DashboardURL,
			"nodeID", string(node.NodeID)[:16]+"...")
	}

	// Find the e2e directory (relative to this test file)
	e2eDir := findE2EDir(t)

	// Check if node_modules exists, if not suggest running npm install
	nodeModulesPath := filepath.Join(e2eDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); os.IsNotExist(err) {
		t.Fatalf(
			"node_modules not found in %s. "+
				"Run 'cd e2e && npm install && npx playwright install chromium' first",
			e2eDir,
		)
	}

	// Run Playwright tests
	logger.InfoContext(ctx, "running playwright tests", "dir", e2eDir)

	cmd := exec.CommandContext(ctx, "npx", "playwright", "test",
		"--project=chromium",
		"--reporter=list",
	)
	cmd.Dir = e2eDir
	cmd.Env = append(os.Environ(), "DASHBOARD_URL="+dashboardURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Check if it's just a test failure vs a setup error
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("playwright tests failed with exit code %d", exitErr.ExitCode())
		}
		t.Fatalf("failed to run playwright: %v", err)
	}

	logger.InfoContext(ctx, "playwright tests passed")
}

// TestClusterFormation tests that the cluster can be formed and nodes can
// communicate. This is a simpler test that doesn't require Playwright.
func TestClusterFormation(t *testing.T) { // A
	if testing.Short() {
		t.Skip("skipping cluster test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use different ports than TestPlaywrightE2E to avoid conflicts
	opts := ClusterOptions{
		BaseCarrierPort:   14342, // Different from 14242
		BaseDashboardPort: 18520, // Different from 18420
	}

	// Start a 3-node cluster
	cluster, err := NewTestClusterWithOptions(ctx, 3, logger, opts)
	if err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}
	defer cluster.Cleanup(ctx)

	// Verify all nodes see each other
	for i, node := range cluster.Nodes {
		nodes, err := node.Carrier.GetNodes(ctx)
		if err != nil {
			t.Errorf("node %d: failed to get nodes: %v", i, err)
			continue
		}
		if len(nodes) != 3 {
			t.Errorf("node %d: expected 3 nodes, got %d", i, len(nodes))
		}
	}

	// Verify all dashboards are accessible
	for i, node := range cluster.Nodes {
		if !isDashboardReady(node.DashboardURL) {
			t.Errorf("node %d: dashboard not ready at %s", i, node.DashboardURL)
		}
	}

	t.Logf("cluster formed successfully with %d nodes", len(cluster.Nodes))
	t.Logf("primary dashboard URL: %s", cluster.PrimaryDashboardURL())
}

// findE2EDir locates the e2e directory relative to the test file.
func findE2EDir(t *testing.T) string { // A
	t.Helper()

	// Try common locations
	candidates := []string{
		".",                     // If running from e2e/harness
		"..",                    // If running from e2e/harness, go up to e2e
		"../../e2e",             // If running from project root
		filepath.Join("..", ""), // Just the parent
	}

	for _, candidate := range candidates {
		packageJSON := filepath.Join(candidate, "package.json")
		if _, err := os.Stat(packageJSON); err == nil {
			absPath, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			return absPath
		}
	}

	// Fallback: try to find it from the working directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working directory: %v", err)
	}

	// Walk up looking for e2e/package.json
	dir := wd
	for i := 0; i < 5; i++ {
		e2eDir := filepath.Join(dir, "e2e")
		if _, err := os.Stat(filepath.Join(e2eDir, "package.json")); err == nil {
			return e2eDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("cannot find e2e directory from %s", wd)
	return ""
}
