package carrier

import (
	"context"
	"log/slog"
	"time"
)

// LogBroadcaster intercepts log records and forwards them to subscribed nodes.
// It implements slog.Handler to wrap an existing logger and broadcast logs
// via the carrier to all subscribers.
//
// LogBroadcaster uses a channel-based design with a single owner goroutine
// managing the subscribers map, avoiding the need for mutexes.
type LogBroadcaster struct { // A
	carrier     Carrier
	localNodeID NodeID
	inner       slog.Handler

	// preAttrs stores attributes added via WithAttrs for inclusion in broadcasts
	preAttrs []slog.Attr
	// group stores the current group name for attribute prefixing
	group string

	// Channel-based command pattern - single goroutine owns subscribers
	subscribeCh   chan subscribeCmd
	unsubscribeCh chan NodeID
	logCh         chan LogEntryPayload
	stopCh        chan struct{}
	doneCh        chan struct{}
}

// subscribeCmd is sent to the manager goroutine to add a subscriber.
type subscribeCmd struct { // A
	nodeID NodeID
	levels map[string]bool
}

// subscriberConfig holds configuration for a single subscriber.
type subscriberConfig struct { // A
	nodeID NodeID
	levels map[string]bool // Empty map means all levels
}

// LogBroadcasterConfig configures a LogBroadcaster.
type LogBroadcasterConfig struct { // A
	Carrier     Carrier
	LocalNodeID NodeID
	Inner       slog.Handler
}

// NewLogBroadcaster creates a new LogBroadcaster.
// The broadcaster must be started with Start() before it will forward logs.
func NewLogBroadcaster(cfg LogBroadcasterConfig) *LogBroadcaster { // A
	return &LogBroadcaster{
		carrier:       cfg.Carrier,
		localNodeID:   cfg.LocalNodeID,
		inner:         cfg.Inner,
		subscribeCh:   make(chan subscribeCmd, 16),
		unsubscribeCh: make(chan NodeID, 16),
		logCh:         make(chan LogEntryPayload, 256),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start begins the broadcast loop. Call this before using the broadcaster.
func (lb *LogBroadcaster) Start() { // A
	go lb.run()
}

// Stop gracefully shuts down the broadcaster.
func (lb *LogBroadcaster) Stop() { // A
	close(lb.stopCh)
	<-lb.doneCh
}

// Subscribe adds a node as a subscriber to this node's logs.
// Levels specifies which log levels to forward; empty slice means all levels.
func (lb *LogBroadcaster) Subscribe(nodeID NodeID, levels []string) { // A
	levelMap := make(map[string]bool, len(levels))
	for _, l := range levels {
		levelMap[l] = true
	}
	lb.subscribeCh <- subscribeCmd{nodeID: nodeID, levels: levelMap}
}

// Unsubscribe removes a node from the subscribers list.
func (lb *LogBroadcaster) Unsubscribe(nodeID NodeID) { // A
	lb.unsubscribeCh <- nodeID
}

// run is the main loop that manages subscribers and broadcasts logs.
// It is the single owner of the subscribers map - no mutex needed.
func (lb *LogBroadcaster) run() { // A
	defer close(lb.doneCh)

	subscribers := make(map[NodeID]*subscriberConfig)

	for {
		select {
		case cmd := <-lb.subscribeCh:
			subscribers[cmd.nodeID] = &subscriberConfig{
				nodeID: cmd.nodeID,
				levels: cmd.levels,
			}

		case nodeID := <-lb.unsubscribeCh:
			delete(subscribers, nodeID)

		case entry := <-lb.logCh:
			lb.broadcastToSubscribers(subscribers, entry)

		case <-lb.stopCh:
			return
		}
	}
}

// broadcastToSubscribers sends a log entry to all matching subscribers.
func (lb *LogBroadcaster) broadcastToSubscribers(
	subscribers map[NodeID]*subscriberConfig,
	entry LogEntryPayload,
) { // A
	for _, sub := range subscribers {
		// Check if subscriber wants this level
		if len(sub.levels) > 0 && !sub.levels[entry.Level] {
			continue
		}

		// Send in goroutine to not block the main loop
		nodeID := sub.nodeID
		go lb.sendToSubscriber(nodeID, entry)
	}
}

// sendToSubscriber sends a log entry to a single subscriber node.
func (lb *LogBroadcaster) sendToSubscriber(
	nodeID NodeID,
	entry LogEntryPayload,
) { // A
	payload, err := SerializeLogEntry(entry)
	if err != nil {
		return // Silently drop on serialization error
	}

	msg := Message{
		Type:    MessageTypeLogEntry,
		Payload: payload,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fire and forget - don't block on slow subscribers
	_ = lb.carrier.SendMessageToNode(ctx, nodeID, msg)
}

// Enabled implements slog.Handler.
func (lb *LogBroadcaster) Enabled(ctx context.Context, level slog.Level) bool { // A
	return lb.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler. It forwards the record to the inner handler
// and queues it for broadcast to subscribers.
func (lb *LogBroadcaster) Handle(
	ctx context.Context,
	record slog.Record,
) error { // A
	// Always forward to inner handler first
	err := lb.inner.Handle(ctx, record)

	// Queue for broadcast (non-blocking)
	entry := lb.recordToEntry(record)
	select {
	case lb.logCh <- entry:
		// Queued successfully
	default:
		// Channel full, drop the broadcast (but inner handler still got it)
	}

	return err
}

// WithAttrs implements slog.Handler.
func (lb *LogBroadcaster) WithAttrs(attrs []slog.Attr) slog.Handler { // A
	// Combine existing preAttrs with new attrs
	newAttrs := make([]slog.Attr, len(lb.preAttrs)+len(attrs))
	copy(newAttrs, lb.preAttrs)
	copy(newAttrs[len(lb.preAttrs):], attrs)

	return &LogBroadcaster{
		carrier:       lb.carrier,
		localNodeID:   lb.localNodeID,
		inner:         lb.inner.WithAttrs(attrs),
		preAttrs:      newAttrs,
		group:         lb.group,
		subscribeCh:   lb.subscribeCh,
		unsubscribeCh: lb.unsubscribeCh,
		logCh:         lb.logCh,
		stopCh:        lb.stopCh,
		doneCh:        lb.doneCh,
	}
}

// WithGroup implements slog.Handler.
func (lb *LogBroadcaster) WithGroup(name string) slog.Handler { // A
	newGroup := name
	if lb.group != "" {
		newGroup = lb.group + "." + name
	}

	return &LogBroadcaster{
		carrier:       lb.carrier,
		localNodeID:   lb.localNodeID,
		inner:         lb.inner.WithGroup(name),
		preAttrs:      lb.preAttrs,
		group:         newGroup,
		subscribeCh:   lb.subscribeCh,
		unsubscribeCh: lb.unsubscribeCh,
		logCh:         lb.logCh,
		stopCh:        lb.stopCh,
		doneCh:        lb.doneCh,
	}
}

// recordToEntry converts an slog.Record to a LogEntryPayload.
func (lb *LogBroadcaster) recordToEntry(record slog.Record) LogEntryPayload { // A
	attrs := make(map[string]string)

	// Include pre-configured attributes from WithAttrs
	for _, a := range lb.preAttrs {
		key := a.Key
		if lb.group != "" {
			key = lb.group + "." + key
		}
		attrs[key] = a.Value.String()
	}

	// Include attributes from the record itself
	record.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if lb.group != "" {
			key = lb.group + "." + key
		}
		attrs[key] = a.Value.String()
		return true
	})

	return LogEntryPayload{
		SourceNodeID: lb.localNodeID,
		Timestamp:    record.Time.UnixNano(),
		Level:        record.Level.String(),
		Message:      record.Message,
		Attributes:   attrs,
	}
}

// levelToString converts slog.Level to a lowercase string.
func levelToString(level slog.Level) string { // A
	switch level {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		return "info"
	}
}

// HandleLogSubscribe processes an incoming log subscription request.
// This should be registered as a handler on the carrier.
func (lb *LogBroadcaster) HandleLogSubscribe(
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) { // A
	payload, err := DeserializeLogSubscribe(msg.Payload)
	if err != nil {
		return nil, err
	}

	lb.Subscribe(payload.SubscriberNodeID, payload.Levels)
	return nil, nil
}

// HandleLogUnsubscribe processes an incoming log unsubscription request.
// This should be registered as a handler on the carrier.
func (lb *LogBroadcaster) HandleLogUnsubscribe(
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) { // A
	payload, err := DeserializeLogUnsubscribe(msg.Payload)
	if err != nil {
		return nil, err
	}

	lb.Unsubscribe(payload.SubscriberNodeID)
	return nil, nil
}

// Ensure LogBroadcaster implements slog.Handler.
var _ slog.Handler = (*LogBroadcaster)(nil)
