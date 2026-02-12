package transport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func TestBootstrapConfigSaveLoad( // A
	t *testing.T,
) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.json")

	cfg := &BootstrapConfig{
		BootstrapNodes: []interfaces.PeerNode{
			{
				NodeID: keys.NodeID{1},
				Addresses: []string{
					"127.0.0.1:9000",
				},
			},
			{
				NodeID: keys.NodeID{2},
				Addresses: []string{
					"192.168.1.1:9001",
				},
			},
		},
	}

	if err := cfg.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	loaded := &BootstrapConfig{}
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if len(loaded.BootstrapNodes) != 2 {
		t.Fatalf(
			"expected 2 nodes, got %d",
			len(loaded.BootstrapNodes),
		)
	}

	if loaded.BootstrapNodes[0].NodeID !=
		cfg.BootstrapNodes[0].NodeID {
		t.Errorf("NodeID mismatch")
	}

	if loaded.BootstrapNodes[0].Addresses[0] !=
		"127.0.0.1:9000" {
		t.Errorf("address mismatch")
	}
}

func TestBootstrapConfigLoadNotFound( // A
	t *testing.T,
) {
	t.Parallel()
	cfg := &BootstrapConfig{}
	err := cfg.LoadFromFile("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestBootstrapConfigLoadInvalid( // A
	t *testing.T,
) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(
		path,
		[]byte("not json"),
		0o600,
	)

	cfg := &BootstrapConfig{}
	err := cfg.LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestBootstrapEmptyConfig(t *testing.T) { // A
	t.Parallel()
	bs := NewBootStrapper(
		&BootstrapConfig{},
		nil,
		nil,
	)
	err := bs.Bootstrap()
	if err != nil {
		t.Fatalf("expected nil for empty config: %v",
			err)
	}
}

func TestBootstrapNilConfig(t *testing.T) { // A
	t.Parallel()
	bs := NewBootStrapper(nil, nil, nil)
	err := bs.Bootstrap()
	if err != nil {
		t.Fatalf("expected nil for nil config: %v",
			err)
	}
}
