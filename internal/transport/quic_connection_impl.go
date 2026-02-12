package transport

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/quic-go/quic-go"
)

// quicConnection wraps a quic-go Conn to implement
// the Connection interface.
type quicConnection struct { // A
	inner  *quic.Conn
	nodeID keys.NodeID
}

func newQuicConnection( // A
	conn *quic.Conn,
	nodeID keys.NodeID,
) *quicConnection {
	return &quicConnection{
		inner:  conn,
		nodeID: nodeID,
	}
}

func (c *quicConnection) NodeID() keys.NodeID { // A
	return c.nodeID
}

func (c *quicConnection) OpenStream() ( // A
	Stream,
	error,
) {
	s, err := c.inner.OpenStream()
	if err != nil {
		return nil, err
	}
	return newQuicStream(s), nil
}

func (c *quicConnection) AcceptStream() ( // A
	Stream,
	error,
) {
	s, err := c.inner.AcceptStream(
		context.Background(),
	)
	if err != nil {
		return nil, err
	}
	return newQuicStream(s), nil
}

func (c *quicConnection) SendDatagram( // A
	data []byte,
) error {
	return c.inner.SendDatagram(data)
}

func (c *quicConnection) ReceiveDatagram() ( // A
	[]byte,
	error,
) {
	return c.inner.ReceiveDatagram(
		context.Background(),
	)
}

func (c *quicConnection) Close() error { // A
	return c.inner.CloseWithError(
		0,
		"graceful close",
	)
}
