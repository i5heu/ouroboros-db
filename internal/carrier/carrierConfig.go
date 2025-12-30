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
//
// Example usage with bootstrap addresses:
//
//	carrier, err := NewDefaultCarrier(Config{
//		LocalNode:    myNode,
//		NodeIdentity: identity,
//		Logger:       logger,
//		Transport:    transport,
//		// Bootstrap from seed nodes
//		BootstrapAddresses: []string{
//			"seed1.cluster.example.com:4242",
//			"seed2.cluster.example.com", // Uses DefaultPort
//			"192.168.1.100:4242",
//		},
//		DefaultPort: 4242, // Optional, defaults to 4242
//	})
//	if err != nil {
//		return err
//	}
//
//	// Join cluster using configured addresses
//	err = carrier.Bootstrap(ctx)
//
//	// Or bootstrap from specific addresses at runtime
//	err = carrier.BootstrapFromAddresses(ctx, []string{"other-node:4242"})
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
	// BootstrapAddresses is a list of addresses or domains used to discover
	// and join an existing cluster. These can be in the format "host:port"
	// or just "host" (in which case the default port will be used).
	// If empty, the node starts as a standalone cluster.
	BootstrapAddresses []string
	// DefaultPort is the default port used when a bootstrap address does not
	// specify a port. Defaults to 4242 if not set.
	DefaultPort uint16
}
