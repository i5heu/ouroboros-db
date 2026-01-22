package carrier

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

// LogSubscribePayload is sent when a node wants to subscribe to another
// node's logs.
type LogSubscribePayload struct { // A
	// SubscriberNodeID identifies the node that wants to receive logs.
	SubscriberNodeID NodeID
	// Levels specifies which log levels to receive. Empty means all levels.
	Levels []string
}

// LogUnsubscribePayload is sent when a node wants to stop receiving logs.
type LogUnsubscribePayload struct { // A
	// SubscriberNodeID identifies the node that wants to stop receiving logs.
	SubscriberNodeID NodeID
}

// LogEntryPayload contains a single log entry forwarded from one node to
// another.
type LogEntryPayload struct { // A
	// SourceNodeID identifies the node that generated the log.
	SourceNodeID NodeID
	// Timestamp is the Unix timestamp in nanoseconds when the log was created.
	Timestamp int64
	// Level is the log level (debug, info, warn, error).
	Level string
	// Message is the log message text.
	Message string
	// Attributes contains additional key-value pairs from the log record.
	Attributes map[string]string
}

// DashboardAnnouncePayload is sent when a node's dashboard becomes available.
type DashboardAnnouncePayload struct { // A
	// NodeID identifies the node with the dashboard.
	NodeID NodeID
	// DashboardAddress is the HTTP address (host:port) of the dashboard.
	DashboardAddress string
}

// SerializeLogSubscribe serializes a LogSubscribePayload to bytes.
func SerializeLogSubscribe(payload LogSubscribePayload) ([]byte, error) { // A
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("encode log subscribe payload: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeLogSubscribe deserializes bytes to a LogSubscribePayload.
func DeserializeLogSubscribe(data []byte) (LogSubscribePayload, error) { // A
	var payload LogSubscribePayload
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&payload); err != nil {
		return LogSubscribePayload{}, fmt.Errorf(
			"decode log subscribe payload: %w", err)
	}
	return payload, nil
}

// SerializeLogUnsubscribe serializes a LogUnsubscribePayload to bytes.
func SerializeLogUnsubscribe(
	payload LogUnsubscribePayload,
) ([]byte, error) { // A
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("encode log unsubscribe payload: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeLogUnsubscribe deserializes bytes to a LogUnsubscribePayload.
func DeserializeLogUnsubscribe(
	data []byte,
) (LogUnsubscribePayload, error) { // A
	var payload LogUnsubscribePayload
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&payload); err != nil {
		return LogUnsubscribePayload{}, fmt.Errorf(
			"decode log unsubscribe payload: %w", err)
	}
	return payload, nil
}

// SerializeLogEntry serializes a LogEntryPayload to bytes.
func SerializeLogEntry(payload LogEntryPayload) ([]byte, error) { // A
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("encode log entry payload: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeLogEntry deserializes bytes to a LogEntryPayload.
func DeserializeLogEntry(data []byte) (LogEntryPayload, error) { // A
	var payload LogEntryPayload
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&payload); err != nil {
		return LogEntryPayload{}, fmt.Errorf("decode log entry payload: %w", err)
	}
	return payload, nil
}

// SerializeDashboardAnnounce serializes a DashboardAnnouncePayload to bytes.
func SerializeDashboardAnnounce(
	payload DashboardAnnouncePayload,
) ([]byte, error) { // A
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("encode dashboard announce payload: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeDashboardAnnounce deserializes bytes to DashboardAnnouncePayload.
func DeserializeDashboardAnnounce(
	data []byte,
) (DashboardAnnouncePayload, error) { // A
	var payload DashboardAnnouncePayload
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&payload); err != nil {
		return DashboardAnnouncePayload{}, fmt.Errorf(
			"decode dashboard announce payload: %w", err)
	}
	return payload, nil
}
