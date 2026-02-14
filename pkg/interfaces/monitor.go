package interfaces

import (
	"context"
	"log/slog"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

type LogLevel int // A

const ( // A
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

type LogEntry struct { // A
	Timestamp time.Time
	NodeID    keys.NodeID
	Level     LogLevel
	Message   string
	Fields    map[string]string
}

type ClusterLog interface { // A
	New(logger *slog.Logger, carrier Carrier, selfID keys.NodeID) *ClusterLog
	Stop()
	Log(ctx context.Context, level LogLevel, msg string, fields map[string]string)
	Info(ctx context.Context, msg string, fields map[string]string)
	Warn(ctx context.Context, msg string, fields map[string]string)
	Debug(ctx context.Context, msg string, fields map[string]string)
	Err(ctx context.Context, msg string, fields map[string]string)
	Tail(limit int) []LogEntry
	Query(nodeID keys.NodeID, since time.Time) []LogEntry
	QueryAll(since time.Time) []LogEntry
	QueryByLevel(level LogLevel, since time.Time) []LogEntry
	SubscribeLog(
		ctx context.Context,
		sourceNodeID keys.NodeID,
		subscriberNodeID keys.NodeID,
	)
	SubscribeLogAll(ctx context.Context, subscriberNodeID keys.NodeID)
	UnsubscribeLog(
		ctx context.Context,
		sourceNodeID keys.NodeID,
		subscriberNodeID keys.NodeID,
	)
	UnsubscribeLogAll(ctx context.Context, subscriberNodeID keys.NodeID)
	SendLog(
		ctx context.Context,
		targetNodeID keys.NodeID,
		sourceNodeID keys.NodeID,
		since time.Time,
	) error
}

type DataStatus int // A

const ( // A
	DataStatusPresent DataStatus = iota
	DataStatusMissing
	DataStatusSyncing
	DataStatusPendingDelete
	DataStatusCorrupt
)

type NodeDataStatus struct { // A
	NodeID   string
	DataHash hash.Hash
	Status   DataStatus
	Detail   string
}

type DataState interface { // A
	GetNodesForVertex(vertexHash hash.Hash) ([]NodeDataStatus, error)
	GetNodesForSealedChunk(chunkHash hash.Hash) ([]NodeDataStatus, error)
	GetNodesForBlock(blockHash hash.Hash) ([]NodeDataStatus, error)
	GetNodesForBlockSlice(sliceHash hash.Hash) ([]NodeDataStatus, error)
	GetNodeInventory(nodeID string) ([]NodeDataStatus, error)
}

type NodeStats struct { // A
	NodeID           string
	Updated          int64
	VertexCount      uint64
	BlockCount       uint64
	BlockSliceCount  uint64
	SealedChunkCount uint64
	KeyEntryCount    uint64
}

type ClusterMonitor interface { // A
	MonitorNodeHealth()
	CollectClusterLogs()
	GetDataState() DataState
	CollectNodeStats() []NodeStats
	GetNodeStats(nodeID string) (NodeStats, error)
}

type NodeAvailabilityTracker interface { // A
	TrackAvailability()
}

type DataReBalancer interface { // A
	BalanceData()
}

type ReplicationMonitoring interface { // A
	MonitorReplications()
}

type SyncIndexTree interface { // A
	Sync()
}

type BackupManager interface { // A
	BackupData()
}

type DeletionWAL interface { // A
	LogDeletion(h hash.Hash) error
	ProcessDeletions() error
}
