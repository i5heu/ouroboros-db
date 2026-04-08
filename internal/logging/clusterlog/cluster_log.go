// Package clusterlog provides a small, in-memory, cluster-scoped log.
//
// It supports:
// - Recording log entries (Info/Warn/Debug/Err or Log with an explicit level)
// - Emitting structured output via slog.Logger
// - Keeping an in-memory history with TTL-based retention
// - Querying recent history (Tail, Query, QueryAll, QueryByLevel)
// - Push delivery to subscribers using an interfaces.Carrier transport
//
// Each ClusterLog instance is associated with a single node identity
// (keys.NodeID). When the local node records a log entry, the entry is stored
// in memory and pushed to any subscribers registered for that source node.
//
// New starts a background cleanup goroutine that periodically removes expired
// entries. Call Stop to terminate the cleanup goroutine when shutting down.
//
// Example:
//
//	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
//	carrier := myCarrier{} // implements interfaces.Carrier
//	selfID := keys.NodeID{1}
//
//	cl := clusterlog.New(logger, carrier, selfID)
//	defer cl.Stop()
//
//	ctx := context.Background()
//	cl.SubscribeLog(ctx, selfID, keys.NodeID{2})
//	cl.Info(ctx, "hello", map[string]string{"k": "v"})
//
//	entries := cl.Tail(10)
//	_ = entries
package clusterlog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

const cleanupInterval = 1 * time.Hour

// ClusterLog stores log entries in memory, outputs
// them to slog, and pushes them to subscriber nodes
// via a Carrier transport.
type ClusterLog struct { // AC
	logger  *slog.Logger
	carrier interfaces.Carrier
	selfID  keys.NodeID

	mu          sync.RWMutex
	entries     []LogEntry
	subscribers map[keys.NodeID]map[keys.NodeID]struct{}

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a ClusterLog and starts the background
// TTL cleanup goroutine.
func New( // H
	logger *slog.Logger,
	carrier interfaces.Carrier,
	selfID keys.NodeID,
) *ClusterLog {
	cl := &ClusterLog{
		logger:      logger,
		carrier:     carrier,
		selfID:      selfID,
		entries:     make([]LogEntry, 0),
		subscribers: make(map[keys.NodeID]map[keys.NodeID]struct{}),
		stopCh:      make(chan struct{}),
	}
	cl.wg.Add(1)
	go cl.cleanupLoop()
	return cl
}

// Stop terminates the background cleanup goroutine
// and waits for it to finish.
func (cl *ClusterLog) Stop() { // H
	close(cl.stopCh)
	cl.wg.Wait()
}

// record stores a log entry in memory and pushes it
// to subscribers. Callers handle slog output so each
// level can format its own attributes.
func (cl *ClusterLog) record( // AC
	ctx context.Context,
	entry *LogEntry,
) {
	cl.mu.Lock()
	cl.entries = append(cl.entries, *entry)
	subs := cl.getSubscribers(cl.selfID)
	cl.mu.Unlock()

	cl.push(ctx, subs, []LogEntry{*entry})
}

// baseAttrs returns the common slog attributes for
// every log call.
func (cl *ClusterLog) baseAttrs( // AC
	level LogLevel,
	fields map[string]string,
) []any {
	attrs := []any{
		keyNodeID, cl.selfID.String(),
		keyLevel, level.String(),
	}
	if len(fields) > 0 {
		attrs = append(attrs, keyFields, fields)
	}
	return attrs
}

// Log records a log entry at the given level. It
// writes to slog, stores in memory, and pushes to
// all subscribers of this node.
func (cl *ClusterLog) Log( // AC
	ctx context.Context,
	level LogLevel,
	msg string,
	fields map[string]string,
) {
	entry := LogEntry{
		Timestamp: time.Now(),
		NodeID:    cl.selfID,
		Level:     level,
		Message:   msg,
		Fields:    fields,
	}

	cl.logger.Log(
		ctx, level.ToSlogLevel(), "log entry",
		append(cl.baseAttrs(level, fields), keyMessage, msg)...,
	)

	cl.record(ctx, &entry)
}

// Info logs a message at Info level.
func (cl *ClusterLog) Info( // H
	ctx context.Context,
	msg string,
	fields map[string]string,
) {
	entry := LogEntry{
		Timestamp: time.Now(),
		NodeID:    cl.selfID,
		Level:     LogLevelInfo,
		Message:   msg,
		Fields:    fields,
	}

	cl.logger.Log(
		ctx, slog.LevelInfo, "log entry",
		append(cl.baseAttrs(LogLevelInfo, fields), keyMessage, msg)...,
	)

	cl.record(ctx, &entry)
}

// Warn logs a message at Warn level.
func (cl *ClusterLog) Warn( // H
	ctx context.Context,
	msg string,
	fields map[string]string,
) {
	entry := LogEntry{
		Timestamp: time.Now(),
		NodeID:    cl.selfID,
		Level:     LogLevelWarn,
		Message:   msg,
		Fields:    fields,
	}

	cl.logger.Log(
		ctx, slog.LevelWarn, "log entry",
		append(cl.baseAttrs(LogLevelWarn, fields), keyMessage, msg)...,
	)

	cl.record(ctx, &entry)
}

// Debug logs a message at Debug level.
func (cl *ClusterLog) Debug( // H
	ctx context.Context,
	msg string,
	fields map[string]string,
) {
	entry := LogEntry{
		Timestamp: time.Now(),
		NodeID:    cl.selfID,
		Level:     LogLevelDebug,
		Message:   msg,
		Fields:    fields,
	}

	cl.logger.Log(
		ctx, slog.LevelDebug, "log entry",
		append(cl.baseAttrs(LogLevelDebug, fields), keyMessage, msg)...,
	)

	cl.record(ctx, &entry)
}

// Err logs a message at Error level. The error is
// included as a native slog attribute. The fields
// parameter is optional and may be omitted.
func (cl *ClusterLog) Err( // AC
	ctx context.Context,
	msg string,
	err error,
	fields ...map[string]string,
) {
	var f map[string]string
	if len(fields) > 0 && fields[0] != nil {
		f = fields[0]
	}
	if err != nil {
		if f == nil {
			f = make(map[string]string)
		}
		f[keyError] = err.Error()
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		NodeID:    cl.selfID,
		Level:     LogLevelError,
		Message:   msg,
		Fields:    f,
	}

	attrs := cl.baseAttrs(LogLevelError, f)
	if err != nil {
		attrs = append(attrs, keyError, err)
	}
	cl.logger.Log(
		ctx, slog.LevelError, "log entry",
		append(attrs, keyMessage, msg)...,
	)

	cl.record(ctx, &entry)
}

// cleanupLoop periodically removes expired entries.
func (cl *ClusterLog) cleanupLoop() { // AC
	defer cl.wg.Done()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cl.stopCh:
			return
		case <-ticker.C:
			cl.cleanup()
		}
	}
}

// cleanup removes entries whose TTL has expired.
func (cl *ClusterLog) cleanup() { // AC
	now := time.Now()
	cl.mu.Lock()
	defer cl.mu.Unlock()

	kept := cl.entries[:0]
	for i := range cl.entries {
		if !cl.entries[i].IsExpired(now) {
			kept = append(kept, cl.entries[i])
		}
	}
	cl.entries = kept
}
