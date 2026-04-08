package interfaces

import (
	"encoding/json"
	"os"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

type QuicTransport interface { // A
	Dial(node Node) (Connection, error)
	Accept() (Connection, error)
	Close() error
	GetActiveConnections() []Connection
}

type Connection interface { // A
	NodeID() keys.NodeID
	RemoteAddr() string
	TLSBindings() TLSBindings
	ExportKeyingMaterial(
		label string, ctx []byte, length int,
	) ([]byte, error)
	OpenStream() (Stream, error)
	AcceptStream() (Stream, error)
	SendDatagram(data []byte) error
	ReceiveDatagram() ([]byte, error)
	Close() error
}

type Stream interface { // A
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
}

type BootstrapConfig struct { // A
	BootstrapAddresses []string
}

func (b *BootstrapConfig) LoadFromFile(path string) error { // A
	data, err := os.ReadFile(path) //#nosec G304 // safe: path provided by caller for config loading
	if err != nil {
		return err
	}
	var cfg struct {
		BootstrapAddresses []string `json:"bootstrapAddresses"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	b.BootstrapAddresses = cfg.BootstrapAddresses
	return nil
}

func (b *BootstrapConfig) SaveToFile(path string) error { // A
	cfg := struct {
		BootstrapAddresses []string `json:"bootstrapAddresses"`
	}{
		BootstrapAddresses: b.BootstrapAddresses,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

type NodeRegistry interface { // A
	AddNode(node Node, certs []NodeCert, signatures [][]byte) error
	RemoveNode(nodeID keys.NodeID) error
	GetNode(nodeID keys.NodeID) (Node, error)
	GetAllNodes() []Node
	GetAdminNodes() []Node
	GetUserNodes() []Node
}

type NodeSync interface { // A
	StartSyncInterval(duration time.Duration)
	StopSync()
	TriggerFullSync() error
	ExchangeNodeList(peer keys.NodeID) error
	HandleNodeListUpdate(nodes []NodeInfo) error
}

type BlockDistributionRecord struct { // A
	BlockHash          hash.Hash
	WALKeys            [][]byte
	SliceConfirmations map[string]map[hash.Hash]bool
	Status             string
	Reason             string
	CreatedAt          int64
	CompletedAt        int64
}

type BlockMetadata struct { // A
	BlockHash         hash.Hash
	Created           int64
	Distributed       bool
	DistributedAt     int64
	SliceCount        int
	NodeConfirmations []string
}
