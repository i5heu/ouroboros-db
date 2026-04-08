package ouroboros

import (
	"testing"
)

func TestNew(t *testing.T) { // A
	t.Parallel()

	dir := t.TempDir()
	db, err := New(&Config{
		Paths: []string{dir},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil OuroborosDB")
	}
}

func TestNewNoPaths(t *testing.T) { // A
	t.Parallel()

	_, err := New(&Config{})
	if err == nil {
		t.Fatal("expected error for empty paths")
	}
}

func TestNewWithLogger(t *testing.T) { // A
	t.Parallel()

	dir := t.TempDir()
	logger := defaultLogger()
	db, err := New(&Config{
		Paths:  []string{dir},
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if db.log != logger {
		t.Error("logger not set correctly")
	}
}

func TestNodeID(t *testing.T) { // A
	t.Parallel()

	dir := t.TempDir()
	db, err := New(&Config{
		Paths: []string{dir},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	id := db.NodeID()
	if id.IsZero() {
		t.Fatal("NodeID should not be zero")
	}
}

func TestGetVersion(t *testing.T) { // A
	t.Parallel()

	v := GetVersion()
	if v != CurrentDbVersion {
		t.Fatalf(
			"GetVersion = %q, want %q",
			v, CurrentDbVersion,
		)
	}
}

func TestNodeIDDeterministic(t *testing.T) { // A
	t.Parallel()

	dir := t.TempDir()
	db1, err := New(&Config{
		Paths: []string{dir},
	})
	if err != nil {
		t.Fatalf("New 1: %v", err)
	}

	db2, err := New(&Config{
		Paths: []string{dir},
	})
	if err != nil {
		t.Fatalf("New 2: %v", err)
	}

	if db1.NodeID() != db2.NodeID() {
		t.Fatal("NodeID should be deterministic")
	}
}

func TestConfigPrimaryPathUsesStorage(t *testing.T) { // A
	t.Parallel()

	dir := t.TempDir()
	path, err := (&Config{
		Storage: StorageConfig{
			Paths: []string{dir},
		},
	}).PrimaryPath()
	if err != nil {
		t.Fatalf("PrimaryPath: %v", err)
	}
	if path != dir {
		t.Fatalf("PrimaryPath = %q, want %q", path, dir)
	}
}

func TestNewNormalizesStorageConfig(t *testing.T) { // A
	t.Parallel()

	dir := t.TempDir()
	db, err := New(&Config{
		Paths: []string{dir},
		Network: NetworkConfig{
			ListenAddress: ":0",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(db.config.Storage.Paths) != 1 {
		t.Fatalf(
			"normalized storage paths = %d, want 1",
			len(db.config.Storage.Paths),
		)
	}
	if db.config.Storage.Paths[0] != dir {
		t.Fatalf(
			"storage path = %q, want %q",
			db.config.Storage.Paths[0],
			dir,
		)
	}
	if db.config.EffectiveListenAddress() != ":0" {
		t.Fatalf(
			"listen address = %q, want %q",
			db.config.EffectiveListenAddress(),
			":0",
		)
	}
}
