package carrier

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/quic-go/quic-go"
)

// QUICTransport implements the Transport interface using QUIC protocol.
// It provides reliable, multiplexed, encrypted communication between nodes.
// Each connection maintains a persistent QUIC connection with the ability
// to open multiple streams for concurrent messaging.
type QUICTransport struct { // A
	logger    *slog.Logger
	localID   NodeID
	tlsConfig *tls.Config
	quicConf  *quic.Config

	mu       sync.Mutex
	listener *quic.Listener
	closed   bool
}

// NewQUICTransport creates a new QUIC transport for the given local node ID.
func NewQUICTransport(
	logger *slog.Logger,
	localID NodeID,
	qConfig QUICConfig,
) (*QUICTransport, error) { // A
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("generate TLS config: %w", err)
	}

	quicConf := &quic.Config{
		MaxIdleTimeout: time.Duration(
			qConfig.MaxIdleTimeout,
		) * time.Millisecond,
		KeepAlivePeriod: time.Duration(
			qConfig.KeepAlivePeriod,
		) * time.Millisecond,
		MaxIncomingStreams: qConfig.MaxIncomingStreams,
	}

	return &QUICTransport{
		logger:    logger,
		localID:   localID,
		tlsConfig: tlsConfig,
		quicConf:  quicConf,
	}, nil
}

// Connect establishes a persistent QUIC connection to a remote node.
// The returned Connection can be used to send multiple messages via streams.
func (t *QUICTransport) Connect(
	ctx context.Context,
	address string,
) (Connection, error) { // A
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, errors.New("transport is closed")
	}
	t.mu.Unlock()

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"ouroboros-quic"},
	}

	conn, err := quic.DialAddr(ctx, address, clientTLS, t.quicConf)
	if err != nil {
		return nil, fmt.Errorf("quic dial: %w", err)
	}

	qc := &quicConn{
		conn:     conn,
		localID:  t.localID,
		logger:   t.logger,
		isClient: true,
	}

	// Perform handshake on the control stream
	if err := qc.performClientHandshake(ctx); err != nil {
		_ = conn.CloseWithError(1, "handshake failed")
		return nil, fmt.Errorf("handshake: %w", err)
	}

	t.logger.Debug("quic connection established",
		logKeyAddress, address,
		"remoteNodeId", truncateNodeID(qc.remoteID))

	return qc, nil
}

// Listen starts accepting incoming QUIC connections on the specified address.
func (t *QUICTransport) Listen(
	ctx context.Context,
	address string,
) (Listener, error) { // A
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, errors.New("transport is closed")
	}

	listener, err := quic.ListenAddr(address, t.tlsConfig, t.quicConf)
	if err != nil {
		return nil, fmt.Errorf("quic listen: %w", err)
	}

	t.listener = listener

	return &quicListener{
		listener: listener,
		localID:  t.localID,
		logger:   t.logger,
	}, nil
}

// Close shuts down the transport and releases all resources.
func (t *QUICTransport) Close() error { // A
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

// quicListener implements the Listener interface for QUIC connections.
type quicListener struct { // A
	listener *quic.Listener
	localID  NodeID
	logger   *slog.Logger
}

// Accept waits for and returns the next incoming connection.
func (l *quicListener) Accept(ctx context.Context) (Connection, error) { // A
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}

	qc := &quicConn{
		conn:     conn,
		localID:  l.localID,
		logger:   l.logger,
		isClient: false,
	}

	// Perform server-side handshake
	if err := qc.performServerHandshake(ctx); err != nil {
		_ = conn.CloseWithError(1, "handshake failed")
		return nil, fmt.Errorf("handshake: %w", err)
	}

	l.logger.Debug("quic connection accepted",
		logKeyAddress, conn.RemoteAddr().String(),
		"remoteNodeId", truncateNodeID(qc.remoteID))

	return qc, nil
}

// Addr returns the listener's network address.
func (l *quicListener) Addr() string { // A
	return l.listener.Addr().String()
}

// Close stops the listener from accepting new connections.
func (l *quicListener) Close() error { // A
	return l.listener.Close()
}

// quicConn implements the Connection interface for a persistent QUIC
// connection.
// It uses QUIC streams for multiplexed messaging - each Send/Receive uses a
// new stream, allowing concurrent operations on a single connection.
type quicConn struct { // A
	conn     *quic.Conn
	localID  NodeID
	remoteID NodeID
	logger   *slog.Logger
	isClient bool

	mu     sync.Mutex
	closed bool
}

// Wire protocol constants.
const (
	maxNodeIDLength  = 256      // Maximum node ID length in bytes
	maxPayloadLength = 16 << 20 // 16 MB max payload
	handshakeTimeout = 10 * time.Second
	streamTimeout    = 30 * time.Second
)

// performClientHandshake initiates the handshake from the client side.
func (c *quicConn) performClientHandshake(ctx context.Context) error { // A
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open control stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	if err := stream.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}

	// Client sends ID first
	if err := writeNodeID(stream, c.localID); err != nil {
		return fmt.Errorf("send local ID: %w", err)
	}

	// Receive server's ID
	remoteID, err := readNodeID(stream)
	if err != nil {
		return fmt.Errorf("receive remote ID: %w", err)
	}
	c.remoteID = remoteID

	return nil
}

// performServerHandshake handles the handshake from the server side.
func (c *quicConn) performServerHandshake(ctx context.Context) error { // A
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return fmt.Errorf("accept control stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	if err := stream.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}

	// Server receives client's ID first
	remoteID, err := readNodeID(stream)
	if err != nil {
		return fmt.Errorf("receive remote ID: %w", err)
	}
	c.remoteID = remoteID

	// Send our ID back
	if err := writeNodeID(stream, c.localID); err != nil {
		return fmt.Errorf("send local ID: %w", err)
	}

	return nil
}

// Send transmits a message over the connection using a new QUIC stream.
// Wire format: [1 byte type][4 bytes payload length][payload]
func (c *quicConn) Send(ctx context.Context, msg Message) error { // A
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("connection closed")
	}
	c.mu.Unlock()

	if len(msg.Payload) > maxPayloadLength {
		return fmt.Errorf(
			"payload too large: %d > %d",
			len(msg.Payload),
			maxPayloadLength,
		)
	}

	// Open a new stream for this message
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	if err := stream.SetWriteDeadline(time.Now().Add(streamTimeout)); err != nil {
		return err
	}

	// Header: [1 byte type][4 bytes payload length]
	header := make([]byte, 5)
	header[0] = byte(msg.Type)
	binary.BigEndian.PutUint32(header[1:], uint32(len(msg.Payload)))

	if _, err := stream.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if len(msg.Payload) > 0 {
		if _, err := stream.Write(msg.Payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}

	return nil
}

// SendEncrypted transmits a pre-encrypted message over the connection.
func (c *quicConn) SendEncrypted(
	ctx context.Context,
	enc *EncryptedMessage,
) error { // A
	return errors.New("use Send; QUIC provides transport encryption")
}

// Receive waits for and returns the next message from an incoming stream.
func (c *quicConn) Receive(ctx context.Context) (Message, error) { // A
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return Message{}, errors.New("connection closed")
	}
	c.mu.Unlock()

	// Accept an incoming stream
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return Message{}, fmt.Errorf("accept stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	if err := stream.SetReadDeadline(time.Now().Add(streamTimeout)); err != nil {
		return Message{}, err
	}

	// Read header
	header := make([]byte, 5)
	if _, err := io.ReadFull(stream, header); err != nil {
		return Message{}, fmt.Errorf("read header: %w", err)
	}

	msgType := MessageType(header[0])
	payloadLen := binary.BigEndian.Uint32(header[1:])

	if payloadLen > maxPayloadLength {
		return Message{}, fmt.Errorf("payload too large: %d", payloadLen)
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(stream, payload); err != nil {
			return Message{}, fmt.Errorf("read payload: %w", err)
		}
	}

	return Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

// ReceiveEncrypted waits for and returns the next encrypted message.
func (c *quicConn) ReceiveEncrypted(
	ctx context.Context,
) (*EncryptedMessage, error) { // A
	return nil, errors.New("use Receive; QUIC provides transport encryption")
}

// Close terminates the connection.
func (c *quicConn) Close() error { // A
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	return c.conn.CloseWithError(0, "connection closed")
}

// RemoteNodeID returns the ID of the connected remote node.
func (c *quicConn) RemoteNodeID() NodeID { // A
	return c.remoteID
}

// RemotePublicKey returns the public key of the connected node.
func (c *quicConn) RemotePublicKey() *keys.PublicKey { // A
	return nil
}

// IsClosed returns whether the connection has been closed.
func (c *quicConn) IsClosed() bool { // A
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// writeNodeID writes a node ID to a stream.
func writeNodeID(w io.Writer, id NodeID) error { // A
	idBytes := []byte(id)
	if len(idBytes) > maxNodeIDLength {
		return errors.New("node ID too long")
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(idBytes)))

	if _, err := w.Write(lenBuf); err != nil {
		return err
	}
	_, err := w.Write(idBytes)
	return err
}

// readNodeID reads a node ID from a stream.
func readNodeID(r io.Reader) (NodeID, error) { // A
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return "", err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	if length > maxNodeIDLength {
		return "", errors.New("node ID too long")
	}

	idBytes := make([]byte, length)
	if _, err := io.ReadFull(r, idBytes); err != nil {
		return "", err
	}
	return NodeID(idBytes), nil
}

// generateTLSConfig creates a TLS configuration with a self-signed certificate.
func generateTLSConfig() (*tls.Config, error) { // A
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"OuroborosDB"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		&key.PublicKey,
		key,
	)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"ouroboros-quic"},
	}, nil
}

// truncateNodeID returns a truncated version of a node ID for logging.
func truncateNodeID(id NodeID) string { // A
	s := string(id)
	if len(s) > 8 {
		return s[:8] + "..."
	}
	return s
}

// Ensure interfaces are satisfied at compile time.
var (
	_ Transport  = (*QUICTransport)(nil)
	_ Listener   = (*quicListener)(nil)
	_ Connection = (*quicConn)(nil)
)
