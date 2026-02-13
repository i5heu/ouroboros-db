package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"reflect"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"github.com/quic-go/quic-go"
)

const (
	alpnProtocol     = "ouroboros-db/1"
	handshakeTimeout = 10 * time.Second
	idleTimeout      = 30 * time.Second
	certValidityDays = 365
	attemptWindow    = 10 * time.Second
	maxAttempts      = 30
)

var errStrictTLSProfile = fmt.Errorf( // A
	"strict TLS profile required: tls1.3 + x25519mlkem768",
)

type attemptCounter struct { // A
	windowStart time.Time
	count       int
}

// quicTransportImpl is the concrete QUIC transport.
type quicTransportImpl struct { // A
	mu             sync.RWMutex
	listener       *quic.Listener
	connections    map[keys.NodeID]*quicConnection
	carrierAuth    *auth.CarrierAuth
	localNodeID    keys.NodeID
	tlsCert        tls.Certificate
	listenAddr     string
	ctx            context.Context
	cancel         context.CancelFunc
	rateMu         sync.Mutex
	dialAttempts   map[string]attemptCounter
	acceptAttempts map[string]attemptCounter
}

// NewQuicTransport creates a new QUIC transport
// bound to the given listen address.
func NewQuicTransport( // A
	listenAddr string,
	carrierAuth *auth.CarrierAuth,
	localNodeID keys.NodeID,
) (QuicTransport, error) {
	cert, err := generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf(
			"generate TLS cert: %w", err,
		)
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	t := &quicTransportImpl{
		connections: make(
			map[keys.NodeID]*quicConnection,
		),
		carrierAuth: carrierAuth,
		localNodeID: localNodeID,
		tlsCert:     cert,
		listenAddr:  listenAddr,
		ctx:         ctx,
		cancel:      cancel,
		dialAttempts: make(
			map[string]attemptCounter,
		),
		acceptAttempts: make(
			map[string]attemptCounter,
		),
	}

	listener, err := quic.ListenAddr(
		listenAddr,
		t.serverTLSConfig(),
		t.quicConfig(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf(
			"listen %s: %w", listenAddr, err,
		)
	}
	t.listener = listener

	return t, nil
}

// ListenAddr returns the actual address the
// transport is listening on.
func (t *quicTransportImpl) ListenAddr() string { // A
	return t.listener.Addr().String()
}

func (t *quicTransportImpl) Dial( // A
	peer interfaces.PeerNode,
) (Connection, error) {
	if len(peer.Addresses) == 0 {
		return nil, fmt.Errorf(
			"peer %s has no addresses",
			peer.NodeID,
		)
	}

	if !t.allowDialAttempt(peer.Addresses[0]) {
		return nil, fmt.Errorf(
			"dial throttled for %s",
			peer.Addresses[0],
		)
	}

	cert, err := generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf(
			"generate dial TLS cert: %w",
			err,
		)
	}

	conn, err := quic.DialAddr(
		t.ctx,
		peer.Addresses[0],
		t.clientTLSConfig(cert),
		t.quicConfig(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"dial %s: %w",
			peer.Addresses[0],
			err,
		)
	}
	if err := verifyStrictTLSProfile(conn); err != nil {
		_ = conn.CloseWithError(
			0x101,
			err.Error(),
		)
		return nil, fmt.Errorf(
			"dial strict TLS profile: %w",
			err,
		)
	}

	qc := newQuicConnection(
		conn,
		peer.NodeID,
		cert.Certificate[0],
	)

	t.mu.Lock()
	t.connections[peer.NodeID] = qc
	t.mu.Unlock()

	return qc, nil
}

func (t *quicTransportImpl) Accept() ( // A
	Connection,
	error,
) {
	conn, err := t.listener.Accept(t.ctx)
	if err != nil {
		return nil, fmt.Errorf("accept: %w", err)
	}
	if err := verifyStrictTLSProfile(conn); err != nil {
		_ = conn.CloseWithError(
			0x101,
			err.Error(),
		)
		return nil, fmt.Errorf(
			"accept strict TLS profile: %w",
			err,
		)
	}

	remoteAddr := conn.RemoteAddr().String()
	if !t.allowAcceptAttempt(remoteAddr) {
		_ = conn.CloseWithError(
			0x100,
			"rate limited",
		)
		return nil, fmt.Errorf(
			"accept throttled for %s",
			remoteAddr,
		)
	}

	peerNodeID := extractNodeIDFromConn(conn)
	qc := newQuicConnection(
		conn,
		peerNodeID,
		t.tlsCert.Certificate[0],
	)

	t.mu.Lock()
	t.connections[peerNodeID] = qc
	t.mu.Unlock()

	return qc, nil
}

func (t *quicTransportImpl) Close() error { // A
	t.cancel()

	t.mu.Lock()
	conns := make(
		[]*quicConnection,
		0,
		len(t.connections),
	)
	for _, c := range t.connections {
		conns = append(conns, c)
	}
	t.connections = make(
		map[keys.NodeID]*quicConnection,
	)
	t.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}

	return t.listener.Close()
}

func (t *quicTransportImpl) GetActiveConnections() []Connection { // A
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make(
		[]Connection,
		0,
		len(t.connections),
	)
	for _, c := range t.connections {
		out = append(out, c)
	}
	return out
}

// GetConnection returns an existing connection to
// the given node, or nil if none exists.
func (t *quicTransportImpl) GetConnection( // A
	nodeID keys.NodeID,
) Connection {
	t.mu.RLock()
	defer t.mu.RUnlock()

	c, ok := t.connections[nodeID]
	if !ok {
		return nil
	}
	return c
}

// RemoveConnection removes a connection from the
// internal map.
func (t *quicTransportImpl) RemoveConnection( // A
	nodeID keys.NodeID,
) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.connections, nodeID)
}

func (t *quicTransportImpl) serverTLSConfig() *tls.Config { // A
	return &tls.Config{
		Certificates: []tls.Certificate{t.tlsCert},
		ClientAuth:   tls.RequireAnyClientCert,
		NextProtos:   []string{alpnProtocol},
		MinVersion:   tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519MLKEM768,
		},
		VerifyConnection: verifyStrictTLSConnState,
	}
}

func (t *quicTransportImpl) clientTLSConfig( // A
	cert tls.Certificate,
) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{
			cert,
		},
		// #nosec G402 -- peer identity is verified by CarrierAuth
		// after handshake using NodeCert and trust scopes.
		InsecureSkipVerify: true,
		NextProtos:         []string{alpnProtocol},
		MinVersion:         tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519MLKEM768,
		},
		VerifyConnection: verifyStrictTLSConnState,
	}
}

func (t *quicTransportImpl) quicConfig() *quic.Config { // A
	return &quic.Config{
		EnableDatagrams:      true,
		HandshakeIdleTimeout: handshakeTimeout,
		MaxIdleTimeout:       idleTimeout,
	}
}

// extractNodeIDFromConn derives a NodeID from the
// peer certificate's public key via SHA-256. Trust
// is established at the application layer via
// CarrierAuth, not X.509 chain validation.
func extractNodeIDFromConn( // A
	conn *quic.Conn,
) keys.NodeID {
	state := conn.ConnectionState()
	peerCerts := state.TLS.PeerCertificates
	if len(peerCerts) == 0 {
		return keys.NodeID{}
	}
	h := sha256.Sum256(
		peerCerts[0].RawSubjectPublicKeyInfo,
	)
	var id keys.NodeID
	copy(id[:], h[:])
	return id
}

// generateSelfSignedCert creates a self-signed
// TLS certificate for QUIC transport. Trust is
// established at the application layer via
// CarrierAuth, not X.509 chain validation.
func generateSelfSignedCert() ( // A
	tls.Certificate,
	error,
) {
	pub, priv, err := ed25519.GenerateKey(
		rand.Reader,
	)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf(
			"generate key: %w", err,
		)
	}

	serialNumber, err := rand.Int(
		rand.Reader,
		new(big.Int).Lsh(big.NewInt(1), 128),
	)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf(
			"generate serial: %w", err,
		)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"ouroboros-db"},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(
			certValidityDays * 24 * time.Hour,
		),
		KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader,
		tmpl,
		tmpl,
		pub,
		priv,
	)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf(
			"create cert: %w", err,
		)
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}, nil
}

func verifyStrictTLSProfile(conn *quic.Conn) error { // A
	state := conn.ConnectionState().TLS
	return verifyStrictTLSConnState(state)
}

func verifyStrictTLSConnState( // A
	state tls.ConnectionState,
) error {
	if state.Version != tls.VersionTLS13 {
		return errStrictTLSProfile
	}
	curveID, ok, err := getNegotiatedCurveID(state)
	if err != nil {
		return errStrictTLSProfile
	}
	if !ok || curveID != tls.X25519MLKEM768 {
		return errStrictTLSProfile
	}
	if state.NegotiatedProtocol != alpnProtocol {
		return fmt.Errorf("invalid ALPN protocol")
	}
	return nil
}

func getNegotiatedCurveID( // A
	state tls.ConnectionState,
) (tls.CurveID, bool, error) {
	v := reflect.ValueOf(state)
	field := v.FieldByName("CurveID")
	if !field.IsValid() {
		return 0, false, nil
	}
	if !field.CanUint() {
		return 0, false, fmt.Errorf(
			"invalid curve field type",
		)
	}
	raw := field.Uint()
	if raw > uint64(^uint16(0)) {
		return 0, false, fmt.Errorf(
			"curve id overflow",
		)
	}
	return tls.CurveID(uint16(raw)), true, nil
}

func (t *quicTransportImpl) allowDialAttempt( // A
	addr string,
) bool {
	return t.allowAttempt(
		t.dialAttempts,
		addr,
	)
}

func (t *quicTransportImpl) allowAcceptAttempt( // A
	remoteAddr string,
) bool {
	return t.allowAttempt(
		t.acceptAttempts,
		remoteAddr,
	)
}

func (t *quicTransportImpl) allowAttempt( // A
	attempts map[string]attemptCounter,
	key string,
) bool {
	now := time.Now().UTC()
	t.rateMu.Lock()
	defer t.rateMu.Unlock()

	counter := attempts[key]
	if counter.windowStart.IsZero() ||
		now.Sub(counter.windowStart) >= attemptWindow {
		attempts[key] = attemptCounter{
			windowStart: now,
			count:       1,
		}
		return true
	}

	if counter.count >= maxAttempts {
		return false
	}

	counter.count++
	attempts[key] = counter
	return true
}
