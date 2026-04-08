package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
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
	ni *certpkg.NodeIdentity,
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
	node *interfaces.Node,
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

func (qt *quicTransport) listenAddress() string { // A
	qt.mu.RLock()
	defer qt.mu.RUnlock()
	if qt.transport == nil || qt.transport.Conn == nil {
		return ""
	}
	return qt.transport.Conn.LocalAddr().String()
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
