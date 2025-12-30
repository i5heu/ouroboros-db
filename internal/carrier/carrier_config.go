package carrier

import "log/slog"

// QUICConfig holds configuration for the QUIC transport.
type QUICConfig struct { // A
	// MaxIdleTimeout is the maximum time a connection can be idle.
	MaxIdleTimeout int64 // milliseconds
	// KeepAlivePeriod is the period for sending keep-alive packets.
	KeepAlivePeriod int64 // milliseconds
	// MaxIncomingStreams is the maximum number of concurrent incoming streams.
	MaxIncomingStreams int64
}

// DefaultQUICConfig returns sensible default QUIC configuration.
func DefaultQUICConfig() QUICConfig { // A
	return QUICConfig{
		MaxIdleTimeout:     30000, // 30 seconds
		KeepAlivePeriod:    10000, // 10 seconds
		MaxIncomingStreams: 100,
	}
}

// Config holds configuration for the DefaultCarrier.
type Config struct { // A
	// LocalNode is this node's identity information.
	LocalNode Node
	// NodeIdentity is the cryptographic identity for this node.
	NodeIdentity *NodeIdentity
	// Logger is the structured logger for the carrier.
	Logger *slog.Logger
	// Transport is the network transport implementation (QUIC by default).
	Transport Transport
	// BootStrapper handles node initialization.
	BootStrapper BootStrapper
	// QUICConfig holds QUIC-specific settings.
	QUICConfig QUICConfig
}
