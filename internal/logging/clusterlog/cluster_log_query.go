package clusterlog

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// Tail returns the most recent limit log entries.
func (cl *ClusterLog) Tail( // AC
	limit int,
) []LogEntry {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	n := len(cl.entries)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]LogEntry, limit)
	copy(out, cl.entries[n-limit:])
	return out
}

// Query returns entries from a specific node since
// the given time.
func (cl *ClusterLog) Query( // AC
	nodeID keys.NodeID,
	since time.Time,
) []LogEntry {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var out []LogEntry
	for i := range cl.entries {
		e := &cl.entries[i]
		if e.NodeID == nodeID &&
			!e.Timestamp.Before(since) {
			out = append(out, *e)
		}
	}
	return out
}

// QueryAll returns all entries since the given time.
func (cl *ClusterLog) QueryAll( // AC
	since time.Time,
) []LogEntry {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var out []LogEntry
	for i := range cl.entries {
		e := &cl.entries[i]
		if !e.Timestamp.Before(since) {
			out = append(out, *e)
		}
	}
	return out
}

// QueryByLevel returns entries matching the given
// level since the specified time.
func (cl *ClusterLog) QueryByLevel( // AC
	level LogLevel,
	since time.Time,
) []LogEntry {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var out []LogEntry
	for i := range cl.entries {
		e := &cl.entries[i]
		if e.Level == level &&
			!e.Timestamp.Before(since) {
			out = append(out, *e)
		}
	}
	return out
}
