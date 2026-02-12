package transport

import (
	"github.com/quic-go/quic-go"
)

// quicStream wraps a quic-go Stream to implement
// the Stream interface.
type quicStream struct { // A
	inner *quic.Stream
}

func newQuicStream( // A
	s *quic.Stream,
) *quicStream {
	return &quicStream{inner: s}
}

func (s *quicStream) Read( // A
	p []byte,
) (int, error) {
	return s.inner.Read(p)
}

func (s *quicStream) Write( // A
	p []byte,
) (int, error) {
	return s.inner.Write(p)
}

func (s *quicStream) Close() error { // A
	return s.inner.Close()
}
