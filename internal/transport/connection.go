package transport

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// Connection represents a single QUIC connection
// to a remote peer node. It provides both reliable
// streams and unreliable datagrams.
type Connection interface { // A
	NodeID() keys.NodeID
	OpenStream() (Stream, error)
	AcceptStream() (Stream, error)
	SendDatagram(data []byte) error
	ReceiveDatagram() ([]byte, error)
	Close() error
	// PeerCertificatesDER returns the raw DER-encoded
	// X.509 certificates presented by the peer during
	// the TLS handshake.
	PeerCertificatesDER() [][]byte
}
