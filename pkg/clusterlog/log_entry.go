package clusterlog

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// LogEntry is a single cluster-wide log record.
type LogEntry struct { // AC
	Timestamp time.Time         `json:"timestamp"`
	NodeID    keys.NodeID       `json:"nodeID"`
	Level     LogLevel          `json:"level"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// isExpired reports whether the entry has exceeded
// its TTL based on the current time.
func (e *LogEntry) isExpired(now time.Time) bool { // AC
	ttl, ok := ttlByLevel[e.Level]
	if !ok {
		ttl = ttlByLevel[LogLevelInfo]
	}
	return now.After(e.Timestamp.Add(ttl))
}
