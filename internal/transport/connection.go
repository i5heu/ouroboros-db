package transport

import (
	"context"
	"crypto/sha256"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"github.com/quic-go/quic-go"
)

// quicConnection wraps a quic.Conn to implement
// interfaces.Connection. TLSBindings() returns the
// static per-connection binding values derived from
// the peer's X.509 certificate. The exporter-based
// bindings (ExporterBinding, TranscriptHash) are
// derived on demand via ExportKeyingMaterial().
type quicConnection struct { // A
	conn   *quic.Conn
	nodeID keys.NodeID
}

func newQuicConnection( // A
	conn *quic.Conn,
) *quicConnection {
	return &quicConnection{conn: conn}
}

// NodeID returns the authenticated node identity.
// Zero until the auth handshake completes.
func (qc *quicConnection) NodeID() keys.NodeID { // A
	return qc.nodeID
}

// RemoteAddr returns the peer's QUIC remote address.
func (qc *quicConnection) RemoteAddr() string { // A
	return qc.conn.RemoteAddr().String()
}

// TLSBindings returns the static TLS binding values
// derived from the peer's X.509 certificate.
// ExporterBinding and TranscriptHash are left nil
// because they require proof-dependent EKM context;
// use ExportKeyingMaterial() to derive them.
func (qc *quicConnection) TLSBindings() auth.TLSBindings { // A
	cs := qc.conn.ConnectionState()
	var certPubKeyHash [sha256.Size]byte
	var x509Fingerprint [sha256.Size]byte
	if len(cs.TLS.PeerCertificates) > 0 {
		peer := cs.TLS.PeerCertificates[0]
		certPubKeyHash = sha256.Sum256(
			peer.RawSubjectPublicKeyInfo,
		)
		x509Fingerprint = sha256.Sum256(peer.Raw)
	}
	return auth.TLSBindings{
		CertPubKeyHash:  certPubKeyHash[:],
		X509Fingerprint: x509Fingerprint[:],
	}
}

// ExportKeyingMaterial derives keying material from
// the TLS session. Delegates to the underlying
// tls.ConnectionState.ExportKeyingMaterial.
func (qc *quicConnection) ExportKeyingMaterial( // A
	label string, ctx []byte, length int,
) ([]byte, error) {
	cs := qc.conn.ConnectionState()
	return cs.TLS.ExportKeyingMaterial(
		label, ctx, length,
	)
}

// OpenStream opens a new bidirectional QUIC stream.
func (qc *quicConnection) OpenStream() ( // A
	interfaces.Stream, error,
) {
	s, err := qc.conn.OpenStream()
	if err != nil {
		return nil, err
	}
	return &quicStream{stream: s}, nil
}

// AcceptStream accepts a bidirectional QUIC stream
// from the peer.
func (qc *quicConnection) AcceptStream() ( // A
	interfaces.Stream, error,
) {
	s, err := qc.conn.AcceptStream(
		context.Background(),
	)
	if err != nil {
		return nil, err
	}
	return &quicStream{stream: s}, nil
}

// SendDatagram sends a QUIC datagram (RFC 9221).
func (qc *quicConnection) SendDatagram( // A
	data []byte,
) error {
	return qc.conn.SendDatagram(data)
}

// ReceiveDatagram receives a QUIC datagram.
func (qc *quicConnection) ReceiveDatagram() ( // A
	[]byte, error,
) {
	return qc.conn.ReceiveDatagram(
		context.Background(),
	)
}

// Close closes the QUIC connection.
func (qc *quicConnection) Close() error { // A
	return qc.conn.CloseWithError(0, "closed")
}

// Compile-time interface check.
var _ interfaces.Connection = (*quicConnection)(nil)

// quicStream wraps a quic.Stream to implement
// interfaces.Stream.
type quicStream struct { // A
	stream *quic.Stream
}

func (qs *quicStream) Read(p []byte) ( // A
	int, error,
) {
	return qs.stream.Read(p)
}

func (qs *quicStream) Write(p []byte) ( // A
	int, error,
) {
	return qs.stream.Write(p)
}

func (qs *quicStream) Close() error { // A
	return qs.stream.Close()
}

// Compile-time interface check.
var _ interfaces.Stream = (*quicStream)(nil)
