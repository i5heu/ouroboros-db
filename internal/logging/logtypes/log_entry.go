package logtypes

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

type LogEntry struct { // A
	Timestamp time.Time         `json:"timestamp"`
	NodeID    keys.NodeID       `json:"nodeID"`
	Level     LogLevel          `json:"level"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}
