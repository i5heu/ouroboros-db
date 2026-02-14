package interfaces

import (
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
	return nil
}

func (b *BootstrapConfig) SaveToFile(path string) error { // A
	return nil
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
