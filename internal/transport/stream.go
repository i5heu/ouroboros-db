// Package transport implements the QUIC-based
// cluster transport layer for OuroborosDB.
package transport

// Stream wraps a reliable QUIC stream for ordered,
// bidirectional byte transfer.
type Stream interface { // A
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
}
