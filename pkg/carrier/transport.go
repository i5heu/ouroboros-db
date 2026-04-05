package carrier

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"github.com/quic-go/quic-go"
)

// quicTransport wraps quic-go's Transport to
// implement interfaces.QuicTransport. It owns the
// UDP socket and optional listener.
type quicTransport struct { // A
	mu        sync.RWMutex
	transport *quic.Transport
	listener  *quic.Listener
	tlsClient *tls.Config
	tlsServer *tls.Config
	quicCfg   *quic.Config
	conns     map[keys.NodeID]*quicConnection
}

// newQuicTransport creates a QUIC transport bound
// to the given UDP address. If listenAddr is empty
// the transport uses ":0" (random port). The TLS
// configs are used for outbound dials (client) and
// inbound accepts (server).
func newQuicTransport( // A
	listenAddr string,
	ni *auth.NodeIdentity,
) (*quicTransport, error) {
	if listenAddr == "" {
		listenAddr = ":0"
	}
	udpAddr, err := net.ResolveUDPAddr(
		"udp", listenAddr,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve listen address: %w", err,
		)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf(
			"bind UDP: %w", err,
		)
	}
	qcfg := &quic.Config{
		EnableDatagrams: true,
	}
	tp := &quic.Transport{Conn: udpConn}
	qt := &quicTransport{
		transport: tp,
		tlsClient: ni.TLSClientConfig(),
		tlsServer: ni.TLSServerConfig(),
		quicCfg:   qcfg,
		conns:     make(map[keys.NodeID]*quicConnection),
	}
	return qt, nil
}

// startListener creates the QUIC listener on the
// underlying transport. Must be called before
// Accept().
func (qt *quicTransport) startListener() error { // A
	qt.mu.Lock()
	defer qt.mu.Unlock()
	if qt.listener != nil {
		return nil
	}
	l, err := qt.transport.Listen(
		qt.tlsServer, qt.quicCfg,
	)
	if err != nil {
		return fmt.Errorf("QUIC listen: %w", err)
	}
	qt.listener = l
	return nil
}

// Dial connects to a remote node using the first
// available address.
func (qt *quicTransport) Dial( // A
	node interfaces.Node,
) (interfaces.Connection, error) {
	if len(node.Addresses) == 0 {
		return nil, fmt.Errorf(
			"node has no addresses",
		)
	}
	addr, err := net.ResolveUDPAddr(
		"udp", node.Addresses[0],
	)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve %s: %w",
			node.Addresses[0], err,
		)
	}
	conn, err := qt.transport.Dial(
		context.Background(),
		addr,
		qt.tlsClient,
		qt.quicCfg,
	)
	if err != nil {
		return nil, fmt.Errorf("QUIC dial: %w", err)
	}
	qc := newQuicConnection(conn)
	qt.mu.Lock()
	qt.conns[node.NodeID] = qc
	qt.mu.Unlock()
	return qc, nil
}

// Accept blocks until a new inbound connection
// arrives. startListener must be called first.
func (qt *quicTransport) Accept() ( // A
	interfaces.Connection, error,
) {
	qt.mu.RLock()
	l := qt.listener
	qt.mu.RUnlock()
	if l == nil {
		return nil, fmt.Errorf(
			"listener not started",
		)
	}
	conn, err := l.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	return newQuicConnection(conn), nil
}

// Close shuts down the listener and transport.
func (qt *quicTransport) Close() error { // A
	qt.mu.Lock()
	defer qt.mu.Unlock()
	for _, c := range qt.conns {
		_ = c.Close()
	}
	qt.conns = make(map[keys.NodeID]*quicConnection)
	if qt.listener != nil {
		_ = qt.listener.Close()
	}
	return qt.transport.Close()
}

// GetActiveConnections returns all tracked
// connections.
func (qt *quicTransport) GetActiveConnections() []interfaces.Connection { // A
	qt.mu.RLock()
	defer qt.mu.RUnlock()
	out := make(
		[]interfaces.Connection, 0, len(qt.conns),
	)
	for _, c := range qt.conns {
		out = append(out, c)
	}
	return out
}

// Compile-time interface check.
var _ interfaces.QuicTransport = (*quicTransport)(nil)

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
