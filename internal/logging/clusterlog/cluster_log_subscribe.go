package clusterlog

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// SubscribeLog registers subscriberNodeID to receive
// log pushes whenever sourceNodeID produces a log
// entry on this ClusterLog instance.
func (cl *ClusterLog) SubscribeLog( // AC
	_ context.Context,
	sourceNodeID keys.NodeID,
	subscriberNodeID keys.NodeID,
) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.subscribers[sourceNodeID] == nil {
		cl.subscribers[sourceNodeID] = make(map[keys.NodeID]struct{})
	}
	cl.subscribers[sourceNodeID][subscriberNodeID] = struct{}{}
}

// SubscribeLogAll registers subscriberNodeID to
// receive log pushes from all source nodes.
func (cl *ClusterLog) SubscribeLogAll( // AC
	ctx context.Context,
	subscriberNodeID keys.NodeID,
) {
	nodes := cl.carrier.GetNodes()
	cl.mu.Lock()
	defer cl.mu.Unlock()

	for _, n := range nodes {
		if cl.subscribers[n.NodeID] == nil {
			cl.subscribers[n.NodeID] = make(map[keys.NodeID]struct{})
		}
		cl.subscribers[n.NodeID][subscriberNodeID] = struct{}{}
	}
}

// UnsubscribeLog removes the subscriber for a
// specific source node.
func (cl *ClusterLog) UnsubscribeLog( // AC
	_ context.Context,
	sourceNodeID keys.NodeID,
	subscriberNodeID keys.NodeID,
) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	subs := cl.subscribers[sourceNodeID]
	if subs != nil {
		delete(subs, subscriberNodeID)
		if len(subs) == 0 {
			delete(cl.subscribers, sourceNodeID)
		}
	}
}

// UnsubscribeLogAll removes the subscriber from all
// source nodes.
func (cl *ClusterLog) UnsubscribeLogAll( // AC
	_ context.Context,
	subscriberNodeID keys.NodeID,
) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	for src, subs := range cl.subscribers {
		delete(subs, subscriberNodeID)
		if len(subs) == 0 {
			delete(cl.subscribers, src)
		}
	}
}

// SendLog sends historical log entries from
// sourceNodeID since the given time to targetNodeID
// via the Carrier.
func (cl *ClusterLog) SendLog( // AC
	ctx context.Context,
	targetNodeID keys.NodeID,
	sourceNodeID keys.NodeID,
	since time.Time,
) error {
	entries := cl.Query(sourceNodeID, since)
	if len(entries) == 0 {
		return nil
	}

	payload, err := json.Marshal(entries)
	if err != nil {
		cl.logger.Log(
			ctx, slog.LevelError,
			"failed to marshal log entries",
			keyTarget, targetNodeID.String(),
			keySource, sourceNodeID.String(),
		)
		return err
	}

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeLogSendResponse,
		Payload: payload,
	}
	return cl.carrier.SendMessageToNode(
		targetNodeID, msg,
	)
}

// push delivers entries to a set of subscriber node
// IDs via the Carrier. It skips the local node to
// avoid self-delivery.
func (cl *ClusterLog) push( // AC
	ctx context.Context,
	targets []keys.NodeID,
	entries []LogEntry,
) {
	if len(targets) == 0 || len(entries) == 0 {
		return
	}

	payload, err := json.Marshal(entries)
	if err != nil {
		cl.logger.Log(
			ctx, slog.LevelError,
			"failed to marshal log push",
			keyEntries, len(entries),
		)
		return
	}

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeLogPush,
		Payload: payload,
	}

	for _, target := range targets {
		if target == cl.selfID {
			continue
		}
		if err := cl.carrier.SendMessageToNode(
			target, msg,
		); err != nil {
			cl.logger.Log(
				ctx, slog.LevelWarn,
				"log push failed",
				keyTarget, target.String(),
			)
		}
	}
}

// getSubscribers returns the subscriber node IDs for
// the given source. Must be called with cl.mu held.
func (cl *ClusterLog) getSubscribers( // AC
	sourceNodeID keys.NodeID,
) []keys.NodeID {
	subs := cl.subscribers[sourceNodeID]
	if len(subs) == 0 {
		return nil
	}
	out := make([]keys.NodeID, 0, len(subs))
	for id := range subs {
		out = append(out, id)
	}
	return out
}
