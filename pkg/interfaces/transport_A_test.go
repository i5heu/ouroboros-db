package interfaces

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapConfigSaveAndLoad(t *testing.T) { // A
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bootstrap.json")

	cfg := &BootstrapConfig{
		BootstrapAddresses: []string{
			"node1.example.com:7465",
			"192.168.1.100:7465",
		},
	}

	if err := cfg.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	loaded := &BootstrapConfig{}
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if len(loaded.BootstrapAddresses) != len(cfg.BootstrapAddresses) {
		t.Errorf("got %d addresses, want %d",
			len(loaded.BootstrapAddresses), len(cfg.BootstrapAddresses))
	}
	for i, addr := range cfg.BootstrapAddresses {
		if loaded.BootstrapAddresses[i] != addr {
			t.Errorf("address[%d] = %q, want %q",
				i, loaded.BootstrapAddresses[i], addr)
		}
	}
}

func TestBootstrapConfigLoadFromFileNotFound(t *testing.T) { // A
	cfg := &BootstrapConfig{}
	err := cfg.LoadFromFile("/nonexistent/path/bootstrap.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestBootstrapConfigLoadFromFileInvalidJSON(t *testing.T) { // A
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := &BootstrapConfig{}
	err := cfg.LoadFromFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestBootstrapConfigSaveToReadOnlyDir(t *testing.T) { // A
	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(roDir, 0o555); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	cfg := &BootstrapConfig{
		BootstrapAddresses: []string{"node1:7465"},
	}
	path := filepath.Join(roDir, "bootstrap.json")
	err := cfg.SaveToFile(path)
	if err == nil {
		t.Error("expected error for read-only directory")
	}
}

func TestBootstrapConfigEmpty(t *testing.T) { // A
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.json")

	cfg := &BootstrapConfig{}
	if err := cfg.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	loaded := &BootstrapConfig{}
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if len(loaded.BootstrapAddresses) != 0 {
		t.Errorf("got %d addresses, want 0", len(loaded.BootstrapAddresses))
	}
}
