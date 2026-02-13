package transport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// BootstrapConfig holds the static bootstrap node
// list for initial cluster join.
type BootstrapConfig struct { // A
	BootstrapNodes []interfaces.PeerNode `json:"bootstrapNodes"`
}

// LoadFromFile reads the bootstrap config from a
// JSON file at the given path.
func (c *BootstrapConfig) LoadFromFile( // A
	path string,
) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}

	// sanitize and reject obvious path-traversal attempts
	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") {
		return fmt.Errorf("invalid path: contains '..'")
	}

	// ensure the target exists and is a regular file
	info, err := os.Stat(clean)
	if err != nil {
		return fmt.Errorf("stat bootstrap config %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("bootstrap config %q is a directory", path)
	}

	data, err := os.ReadFile(clean)
	if err != nil {
		return fmt.Errorf(
			"read bootstrap config %q: %w",
			path,
			err,
		)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf(
			"parse bootstrap config: %w", err,
		)
	}
	return nil
}

// SaveToFile writes the bootstrap config to a JSON
// file at the given path.
func (c *BootstrapConfig) SaveToFile( // A
	path string,
) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf(
			"marshal bootstrap config: %w", err,
		)
	}
	if err := os.WriteFile(
		path,
		data,
		0o600,
	); err != nil {
		return fmt.Errorf(
			"write bootstrap config %q: %w",
			path,
			err,
		)
	}
	return nil
}

// BootStrapper performs initial node bootstrapping
// against known bootstrap peers.
type BootStrapper struct { // A
	config    *BootstrapConfig
	transport QuicTransport
	registry  NodeRegistry
}

// NewBootStrapper creates a new BootStrapper.
func NewBootStrapper( // A
	config *BootstrapConfig,
	transport QuicTransport,
	registry NodeRegistry,
) *BootStrapper {
	return &BootStrapper{
		config:    config,
		transport: transport,
		registry:  registry,
	}
}

// BootstrapNode connects to a bootstrap node and
// registers it in the node registry.
func (b *BootStrapper) BootstrapNode( // A
	peer interfaces.PeerNode,
) error {
	conn, err := b.transport.Dial(peer)
	if err != nil {
		return fmt.Errorf(
			"dial bootstrap node %s: %w",
			peer.NodeID,
			err,
		)
	}

	// Register the bootstrap node in the registry.
	err = b.registry.AddNode(
		peer,
		nil,
		0,
	)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"register bootstrap node: %w", err,
		)
	}

	err = b.registry.UpdateConnectionStatus(
		peer.NodeID,
		interfaces.ConnectionStatusConnected,
	)
	if err != nil {
		return fmt.Errorf(
			"update connection status: %w", err,
		)
	}

	return nil
}

// Bootstrap connects to all configured bootstrap
// nodes. It returns the first error encountered but
// continues attempting remaining nodes.
func (b *BootStrapper) Bootstrap() error { // A
	if b.config == nil ||
		len(b.config.BootstrapNodes) == 0 {
		return nil
	}

	var firstErr error
	for _, peer := range b.config.BootstrapNodes {
		if err := b.BootstrapNode(peer); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
