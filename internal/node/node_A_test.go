package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

func TestNewCreatesKeyFile( // A
	t *testing.T,
) {
	t.Parallel()

	dir := t.TempDir()
	n, err := New(dir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	keyPath := filepath.Join(dir, keyFileName)
	info, statErr := os.Stat(keyPath)
	if statErr != nil {
		t.Fatalf(
			"key file not created: %v",
			statErr,
		)
	}
	if info.Size() == 0 {
		t.Fatal("key file is empty")
	}
	if n.ID().IsZero() {
		t.Fatal("NodeID is zero after creation")
	}
}

func TestNewLoadsExistingKey( // A
	t *testing.T,
) {
	t.Parallel()

	dir := t.TempDir()
	n1, err := New(dir)
	if err != nil {
		t.Fatalf("first New() error: %v", err)
	}

	n2, err := New(dir)
	if err != nil {
		t.Fatalf("second New() error: %v", err)
	}

	if n1.ID() != n2.ID() {
		t.Fatalf(
			"NodeID mismatch: %s != %s",
			n1.ID(),
			n2.ID(),
		)
	}
}

func TestNewInvalidPath( // A
	t *testing.T,
) {
	t.Parallel()

	_, err := New("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestNewCorruptKeyFile( // A
	t *testing.T,
) {
	t.Parallel()

	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFileName)

	err := os.WriteFile(
		keyPath,
		[]byte("not valid key data"),
		0o600,
	)
	if err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, loadErr := New(dir)
	if loadErr == nil {
		t.Fatal(
			"expected error for corrupt key file",
		)
	}
}

func TestNodeIDNotZero( // A
	t *testing.T,
) {
	t.Parallel()

	dir := t.TempDir()
	n, err := New(dir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if n.ID().IsZero() {
		t.Fatal("NodeID must not be zero")
	}
}

func TestNodeCryptNotNil( // A
	t *testing.T,
) {
	t.Parallel()

	dir := t.TempDir()
	n, err := New(dir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if n.Crypt() == nil {
		t.Fatal("Crypt() must not be nil")
	}

	if n.Crypt().NodeID != n.ID() {
		t.Fatalf(
			"Crypt().NodeID %s != ID() %s",
			n.Crypt().NodeID,
			n.ID(),
		)
	}
}

func TestKeyFilePermissions( // A
	t *testing.T,
) {
	t.Parallel()

	dir := t.TempDir()
	_, err := New(dir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	keyPath := filepath.Join(dir, keyFileName)
	info, statErr := os.Stat(keyPath)
	if statErr != nil {
		t.Fatalf("stat key file: %v", statErr)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Fatalf(
			"key file perms = %o, want 0600",
			perm,
		)
	}
}

func TestRapidNodeIdentity( // A
	t *testing.T,
) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		n1, err := New(dir)
		if err != nil {
			rt.Fatalf("New() error: %v", err)
		}

		if n1.ID().IsZero() {
			rt.Fatal("NodeID must not be zero")
		}

		// Reload from same directory must yield
		// the same identity.
		n2, err := New(dir)
		if err != nil {
			rt.Fatalf(
				"reload New() error: %v",
				err,
			)
		}

		if n1.ID() != n2.ID() {
			rt.Fatalf(
				"reload mismatch: %s != %s",
				n1.ID(),
				n2.ID(),
			)
		}
	})
}

func TestRapidUniqueNodeIDs( // A
	t *testing.T,
) {
	t.Parallel()

	seen := make(map[[32]byte]bool)

	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		n, err := New(dir)
		if err != nil {
			rt.Fatalf("New() error: %v", err)
		}

		id := n.ID()
		if seen[id] {
			rt.Fatalf("duplicate NodeID: %s", id)
		}
		seen[id] = true
	})
}

func TestRapidJson( // A
	t *testing.T,
) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		n, err := New(dir)
		if err != nil {
			rt.Fatalf("New() error: %v", err)
		}

		json1, err := n.Json()
		if err != nil {
			rt.Fatalf("Json() error: %v", err)
		}

		json2, err := n.Json()
		if err != nil {
			rt.Fatalf("Json() second call error: %v", err)
		}

		if json1 != json2 {
			rt.Fatalf(
				"Json() inconsistent: %s != %s",
				json1,
				json2,
			)
		}

		var result map[string]interface{}
		if unmarshalErr := json.Unmarshal(
			[]byte(json1),
			&result,
		); unmarshalErr != nil {
			rt.Fatalf(
				"Json() output is not valid JSON: %v",
				unmarshalErr,
			)
		}
	})
}
