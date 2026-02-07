package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

const keyFileName = "node.key"

// Node holds the cryptographic identity of a cluster node.
type Node struct { // H
	crypt  *crypt.Crypt
	nodeID keys.NodeID
}

// New creates or loads a Node from the given data directory.
// On first run a new key pair is generated and saved to
// <dataDir>/node.key. On subsequent runs the existing key
// file is loaded.
func New(dataDir string) (*Node, error) { // AC
	keyPath := filepath.Join(dataDir, keyFileName)

	c, err := loadOrCreateCrypt(keyPath)
	if err != nil {
		return nil, fmt.Errorf(
			"node crypto init: %w",
			err,
		)
	}

	return &Node{
		crypt:  c,
		nodeID: c.NodeID,
	}, nil
}

// ID returns the node's unique identifier derived from
// its public key material.
func (n *Node) ID() keys.NodeID { // H
	return n.nodeID
}

// Crypt returns the underlying crypto instance for
// encryption, decryption, and signing operations.
func (n *Node) Crypt() *crypt.Crypt { // H
	return n.crypt
}

func loadOrCreateCrypt( // AC
	keyPath string,
) (*crypt.Crypt, error) {
	_, err := os.Stat(keyPath)
	switch {
	case err == nil:
		c, loadErr := crypt.NewFromFile(keyPath)
		if loadErr != nil {
			return nil, fmt.Errorf(
				"load key file %q: %w",
				keyPath,
				loadErr,
			)
		}
		return c, nil

	case os.IsNotExist(err):
		c, genErr := newCrypt()
		if genErr != nil {
			return nil, fmt.Errorf(
				"generate keys: %w",
				genErr,
			)
		}
		if saveErr := c.Keys.SaveToFile(keyPath); saveErr != nil {
			return nil, fmt.Errorf(
				"save key file %q: %w",
				keyPath,
				saveErr,
			)
		}
		return c, nil

	default:
		return nil, fmt.Errorf(
			"stat key file %q: %w",
			keyPath,
			err,
		)
	}
}

// newCrypt wraps crypt.New with panic recovery because the
// upstream constructor panics on key-generation failure.
func newCrypt() (c *crypt.Crypt, err error) { // AC
	defer func() {
		if r := recover(); r != nil {
			c = nil
			err = fmt.Errorf(
				"crypt.New panicked: %v",
				r,
			)
		}
	}()

	c = crypt.New()
	return c, nil
}

// Json returns the JSON representation of the node.
func (n Node) Json() (string, error) { // AC
	b, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
