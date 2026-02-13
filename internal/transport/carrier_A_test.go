package transport

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

var (
	testCAOnce sync.Once
	testCA     *keys.AsyncCrypt
	testCAErr  error
)

func testCASigner(t *testing.T) *keys.AsyncCrypt { // A
	t.Helper()
	testCAOnce.Do(func() {
		testCA, testCAErr = keys.NewAsyncCrypt()
	})
	if testCAErr != nil {
		t.Fatalf("create test CA signer: %v", testCAErr)
	}
	return testCA
}

func randomSerial16(t *testing.T) [16]byte { // A
	t.Helper()
	var serial [16]byte
	if _, err := rand.Read(serial[:]); err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	return serial
}

func testRandomNonce32(t *testing.T) [32]byte { // A
	t.Helper()
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatalf("generate nonce: %v", err)
	}
	return nonce
}

func buildCarrierAuthMaterial( // A
	t *testing.T,
) (*auth.CarrierAuth, *auth.NodeCert, []byte, *keys.AsyncCrypt) {
	t.Helper()
	caSigner := testCASigner(t)
	caPubVal := caSigner.GetPublicKey()
	caPub := &caPubVal

	carrierAuth := auth.NewCarrierAuth(auth.CarrierAuthConfig{})
	if err := carrierAuth.AddAdminPubKey(caPub); err != nil {
		t.Fatalf("AddAdminPubKey: %v", err)
	}

	adminCA, err := auth.NewAdminCA(caPub)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	nodeSigner, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("create node signer: %v", err)
	}
	nodePubVal := nodeSigner.GetPublicKey()
	nodePub := &nodePubVal

	now := time.Now().UTC()
	cert, err := auth.NewNodeCert(auth.NodeCertParams{
		NodePubKey:   nodePub,
		IssuerCAHash: adminCA.Hash(),
		ValidFrom:    now.Add(-time.Hour),
		ValidUntil:   now.Add(time.Hour),
		Serial:       randomSerial16(t),
		RoleClaims:   auth.ScopeAdmin,
		CertNonce:    testRandomNonce32(t),
	})
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	caSig, err := auth.SignNodeCert(caSigner, cert)
	if err != nil {
		t.Fatalf("SignNodeCert: %v", err)
	}

	carrierAuth.RefreshRevocationState(adminCA.Hash())

	return carrierAuth, cert, caSig, nodeSigner
}

func testLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))
}

func newTestCarrier( // A
	t *testing.T,
	nodeID keys.NodeID,
) *Carrier {
	t.Helper()
	carrierAuth, cert, caSig, nodeSigner := buildCarrierAuthMaterial(t)
	c, err := NewCarrier(CarrierConfig{
		ListenAddr:       "127.0.0.1:0",
		CarrierAuth:      carrierAuth,
		LocalNodeID:      nodeID,
		LocalCert:        cert,
		LocalCASignature: caSig,
		LocalKeys:        nodeSigner,
		Logger:           testLogger(),
	})
	if err != nil {
		t.Fatalf("NewCarrier: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestCarrierCreateAndClose( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	nodes := c.GetNodes()
	if len(nodes) != 0 {
		t.Fatalf(
			"expected 0 nodes, got %d",
			len(nodes),
		)
	}
}

func TestCarrierNilAuth(t *testing.T) { // A
	t.Parallel()
	_, err := NewCarrier(CarrierConfig{
		ListenAddr: "127.0.0.1:0",
		Logger:     testLogger(),
	})
	if err == nil {
		t.Fatal("expected error for nil auth")
	}
}

func TestCarrierNilLogger(t *testing.T) { // A
	t.Parallel()
	carrierAuth, _, _, _ := buildCarrierAuthMaterial(t)
	_, err := NewCarrier(CarrierConfig{
		ListenAddr:  "127.0.0.1:0",
		CarrierAuth: carrierAuth,
	})
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
}

func TestCarrierSendMessageReliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	addrA := cA.ListenAddr()

	// Register A as a peer of B and establish
	// connection.
	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{addrA},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	// Send a reliable message from B to A.
	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("hello"),
	}
	err = cB.SendMessageToNode(peerA.NodeID, msg)
	if err != nil {
		t.Fatalf("SendMessageToNode: %v", err)
	}
}

func TestCarrierBroadcastReliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	// Connect B to A.
	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("broadcast"),
	}
	success, failed, err := cB.BroadcastReliable(msg)
	if err != nil {
		t.Fatalf("BroadcastReliable: %v", err)
	}
	if len(success) != 1 {
		t.Errorf(
			"expected 1 success, got %d",
			len(success),
		)
	}
	if len(failed) != 0 {
		t.Errorf(
			"expected 0 failed, got %d",
			len(failed),
		)
	}
}

func TestCarrierBroadcastUnreliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("dgram"),
	}
	attempted := cB.BroadcastUnreliable(msg)
	if len(attempted) != 1 {
		t.Errorf(
			"expected 1 attempted, got %d",
			len(attempted),
		)
	}
}

func TestCarrierJoinAndLeave(t *testing.T) { // A
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}

	// Join
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	nodes := cB.GetNodes()
	if len(nodes) != 1 {
		t.Fatalf(
			"expected 1 node after join, got %d",
			len(nodes),
		)
	}

	if !cB.IsConnected(peerA.NodeID) {
		t.Error("expected connected after join")
	}

	// Leave
	err = cB.LeaveCluster(peerA)
	if err != nil {
		t.Fatalf("LeaveCluster: %v", err)
	}

	nodes = cB.GetNodes()
	if len(nodes) != 0 {
		t.Fatalf(
			"expected 0 nodes after leave, got %d",
			len(nodes),
		)
	}
}

func TestCarrierRemoveNode(t *testing.T) { // A
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	err := cB.RemoveNode(peerA.NodeID)
	if err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}

	if cB.IsConnected(peerA.NodeID) {
		t.Error(
			"expected disconnected after remove",
		)
	}
}

func TestCarrierGetNodeNotFound( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	_, err := c.GetNode(keys.NodeID{99})
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestCarrierIsConnectedUnknown( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	if c.IsConnected(keys.NodeID{99}) {
		t.Error(
			"expected false for unknown node",
		)
	}
}

func TestCarrierImplementsInterface( // A
	t *testing.T,
) {
	t.Parallel()
	// Compile-time check that Carrier implements
	// interfaces.Carrier.
	var _ interfaces.Carrier = (*Carrier)(nil)
}

// TestCarrierSetMessageReceiver verifies that
// SetMessageReceiver installs the callback.
func TestCarrierSetMessageReceiver( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	called := false
	recv := MessageReceiver(
		func(
			_ interfaces.Message,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (interfaces.Response, error) {
			called = true
			return interfaces.Response{}, nil
		},
	)
	c.SetMessageReceiver(recv)

	if c.receiver.Load() == nil {
		t.Fatal("receiver not set")
	}
	_ = called
}

// TestCarrierInboundMessageDispatch verifies the
// full request/response cycle: sender opens a
// stream, writes a message, and reads the response
// from the receiver on the other carrier.
func TestCarrierInboundMessageDispatch( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	// Set up a receiver on A that echoes the
	// payload back.
	recv := MessageReceiver(
		func(
			msg interfaces.Message,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (interfaces.Response, error) {
			return interfaces.Response{
				Payload: msg.Payload,
				Metadata: map[string]string{
					"echo": "true",
				},
			}, nil
		},
	)
	cA.SetMessageReceiver(recv)

	// Connect B to A.
	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	// Give the accept loop time to spawn the
	// connection handler.
	time.Sleep(100 * time.Millisecond)

	// B sends a message to A and reads the response
	// by opening a stream directly.
	conn := cB.transport.GetConnection(peerA.NodeID)
	if conn == nil {
		t.Fatal("no connection to peer A")
	}

	stream, err := conn.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("ping"),
	}
	err = WriteMessage(stream, msg)
	if err != nil {
		t.Fatalf("write message: %v", err)
	}

	resp, err := ReadResponse(stream)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if string(resp.Payload) != "ping" {
		t.Fatalf(
			"payload: got %q, want %q",
			resp.Payload, "ping",
		)
	}
	if resp.Metadata["echo"] != "true" {
		t.Fatalf(
			"metadata: got %q, want %q",
			resp.Metadata["echo"], "true",
		)
	}
}

// TestCarrierInboundNoReceiver verifies that a
// message sent when no receiver is set returns an
// error response.
func TestCarrierInboundNoReceiver( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	// No receiver set on A.

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	conn := cB.transport.GetConnection(peerA.NodeID)
	if conn == nil {
		t.Fatal("no connection to peer A")
	}

	stream, err := conn.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("test"),
	}
	err = WriteMessage(stream, msg)
	if err != nil {
		t.Fatalf("write message: %v", err)
	}

	resp, err := ReadResponse(stream)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error response")
	}
}

// TestCarrierRegistry verifies that Registry()
// returns a non-nil NodeRegistry.
func TestCarrierRegistry(t *testing.T) { // A
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	reg := c.Registry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

// TestCarrierBroadcastWrapper verifies the Broadcast
// convenience method returns success and no error.
func TestCarrierBroadcastWrapper( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("wrap"),
	}
	success, err := cB.Broadcast(msg)
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if len(success) != 1 {
		t.Errorf(
			"expected 1 success, got %d",
			len(success),
		)
	}
}

// TestCarrierSendMessageToNodeUnreliable verifies
// the unreliable single-node send path.
func TestCarrierSendMessageToNodeUnreliable( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	_ = cB.JoinCluster(peerA, nil)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("dgram-single"),
	}
	err := cB.SendMessageToNodeUnreliable(
		peerA.NodeID, msg,
	)
	if err != nil {
		t.Fatalf(
			"SendMessageToNodeUnreliable: %v",
			err,
		)
	}
}

// TestCarrierSendUnreliableNoConn verifies that
// sending to an unconnected node returns an error.
func TestCarrierSendUnreliableNoConn( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("fail"),
	}
	err := c.SendMessageToNodeUnreliable(
		keys.NodeID{99}, msg,
	)
	if err == nil {
		t.Fatal(
			"expected error for unconnected node",
		)
	}
}

// TestCarrierGetNodeSuccess verifies GetNode returns
// the peer after JoinCluster.
func TestCarrierGetNodeSuccess( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	err := cB.JoinCluster(peerA, nil)
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}

	got, err := cB.GetNode(peerA.NodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.NodeID != peerA.NodeID {
		t.Fatalf(
			"NodeID = %v, want %v",
			got.NodeID, peerA.NodeID,
		)
	}
}

// TestCarrierJoinClusterDialError verifies that
// JoinCluster returns an error for a bad address.
func TestCarrierJoinClusterDialError( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	badPeer := interfaces.PeerNode{
		NodeID:    keys.NodeID{99},
		Addresses: []string{"bad-address"},
	}
	err := c.JoinCluster(badPeer, nil)
	if err == nil {
		t.Fatal(
			"expected error for unreachable peer",
		)
	}
}

// TestCarrierEnsureConnectionUnknownNode verifies
// that sending to an unregistered node returns an
// error.
func TestCarrierEnsureConnectionUnknownNode( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("x"),
	}
	err := c.SendMessageToNodeReliable(
		keys.NodeID{99}, msg,
	)
	if err == nil {
		t.Fatal(
			"expected error for unknown node",
		)
	}
}

type fakeAuthStream struct { // A
	r        *bytes.Reader
	w        bytes.Buffer
	isClosed bool
	writeErr error
	closeErr error
	readErr  error
}

func newFakeAuthStream( // A
	input []byte,
) *fakeAuthStream {
	return &fakeAuthStream{r: bytes.NewReader(input)}
}

func (s *fakeAuthStream) Read(p []byte) (int, error) { // A
	if s.readErr != nil {
		return 0, s.readErr
	}
	n, err := s.r.Read(p)
	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}
	return n, err
}

func (s *fakeAuthStream) Write(p []byte) (int, error) { // A
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return s.w.Write(p)
}

func (s *fakeAuthStream) Close() error { // A
	s.isClosed = true
	return s.closeErr
}

type fakeAuthConn struct { // A
	nodeID      keys.NodeID
	stream      Stream
	certs       [][]byte
	localCert   []byte
	exporter    []byte
	exporterErr error
	acceptErr   error
	acceptCalls int
	closed      bool
}

func testExporterBinding() []byte { // A
	return bytes.Repeat([]byte{0xAB}, 32)
}

func mustTestCertDER(t *testing.T) []byte { // A
	t.Helper()
	cert, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("generated cert has no leaf")
	}
	out := make([]byte, len(cert.Certificate[0]))
	copy(out, cert.Certificate[0])
	return out
}

func (c *fakeAuthConn) NodeID() keys.NodeID { // A
	return c.nodeID
}

func (c *fakeAuthConn) OpenStream() (Stream, error) { // A
	return nil, errors.New("not implemented")
}

func (c *fakeAuthConn) AcceptStream() (Stream, error) { // A
	c.acceptCalls++
	if c.acceptErr != nil {
		return nil, c.acceptErr
	}
	if c.stream == nil {
		return nil, io.EOF
	}
	stream := c.stream
	c.stream = nil
	return stream, nil
}

func (c *fakeAuthConn) SendDatagram(_ []byte) error { // A
	return errors.New("not implemented")
}

func (c *fakeAuthConn) ReceiveDatagram() ([]byte, error) { // A
	return nil, errors.New("not implemented")
}

func (c *fakeAuthConn) Close() error { // A
	c.closed = true
	return nil
}

func (c *fakeAuthConn) PeerCertificatesDER() [][]byte { // A
	return c.certs
}

func (c *fakeAuthConn) LocalCertificateDER() []byte { // A
	out := make([]byte, len(c.localCert))
	copy(out, c.localCert)
	return out
}

func (c *fakeAuthConn) ExportKeyingMaterial( // A
	_ string,
	_ []byte,
	length int,
) ([]byte, error) {
	if c.exporterErr != nil {
		return nil, c.exporterErr
	}
	if len(c.exporter) == length {
		out := make([]byte, len(c.exporter))
		copy(out, c.exporter)
		return out, nil
	}
	out := make([]byte, length)
	copy(out, c.exporter)
	return out, nil
}

func buildSerializedMessage( // A
	t *testing.T,
	msg interfaces.Message,
) []byte {
	t.Helper()
	data, err := SerializeMessage(msg)
	if err != nil {
		t.Fatalf("SerializeMessage: %v", err)
	}
	return data
}

func TestCarrierReceiveAuthHandshakeRejectsWrongMessageType( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("not-auth"),
	}
	stream := newFakeAuthStream(buildSerializedMessage(t, msg))
	peerCert := mustTestCertDER(t)
	conn := &fakeAuthConn{
		stream:    stream,
		certs:     [][]byte{peerCert},
		localCert: peerCert,
		exporter:  testExporterBinding(),
	}

	_, _, err := c.receiveAuthHandshake(conn)
	if err == nil || !strings.Contains(err.Error(), "expected auth handshake") {
		t.Fatalf("err = %v, want expected auth handshake", err)
	}
}

func TestCarrierReceiveAuthHandshakeRejectsMalformedAuthPayload( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeAuthHandshake,
		Payload: []byte{0x00, 0x01, 0x02},
	}
	stream := newFakeAuthStream(buildSerializedMessage(t, msg))
	peerCert := mustTestCertDER(t)
	conn := &fakeAuthConn{
		stream:    stream,
		certs:     [][]byte{peerCert},
		localCert: peerCert,
		exporter:  testExporterBinding(),
	}

	_, _, err := c.receiveAuthHandshake(conn)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("err = %v, want authentication failed error", err)
	}
}

func TestCarrierReceiveAuthHandshakeRejectsInvalidDelegationProof( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})
	localCert := c.transport.tlsCert.Certificate[0]
	buildConn := &fakeAuthConn{
		localCert: localCert,
		exporter:  testExporterBinding(),
	}

	payload, err := c.buildLocalAuthPayload(buildConn)
	if err != nil {
		t.Fatalf("buildLocalAuthPayload: %v", err)
	}
	fields, err := decodeAuthHandshakePayload(payload)
	if err != nil {
		t.Fatalf("decodeAuthHandshakePayload: %v", err)
	}
	fields.delegationProof = []byte{0x01, 0x02, 0x03}
	badPayload, err := encodeAuthHandshakePayload(fields)
	if err != nil {
		t.Fatalf("encodeAuthHandshakePayload: %v", err)
	}

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeAuthHandshake,
		Payload: badPayload,
	}
	stream := newFakeAuthStream(buildSerializedMessage(t, msg))
	peerCert := mustTestCertDER(t)
	conn := &fakeAuthConn{
		stream:    stream,
		certs:     [][]byte{peerCert},
		localCert: peerCert,
		exporter:  testExporterBinding(),
	}

	_, _, err = c.receiveAuthHandshake(conn)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("err = %v, want authentication failed error", err)
	}
}

func TestCarrierVerifyAuthPayloadRejectsMissingTLSBindingData( // A
	t *testing.T,
) {
	t.Parallel()
	c := newTestCarrier(t, keys.NodeID{1})
	localCert := c.transport.tlsCert.Certificate[0]
	buildConn := &fakeAuthConn{
		localCert: localCert,
		exporter:  testExporterBinding(),
	}

	payload, err := c.buildLocalAuthPayload(buildConn)
	if err != nil {
		t.Fatalf("buildLocalAuthPayload: %v", err)
	}
	fields, err := decodeAuthHandshakePayload(payload)
	if err != nil {
		t.Fatalf("decodeAuthHandshakePayload: %v", err)
	}
	proof, err := auth.UnmarshalDelegationProof(fields.delegationProof)
	if err != nil {
		t.Fatalf("UnmarshalDelegationProof: %v", err)
	}

	_, _, err = c.verifyAuthPayload(
		payload,
		proof.X509Fingerprint(),
		proof.TLSCertPubKeyHash(),
		[32]byte{},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "TLS exporter") {
		t.Fatalf("err = %v, want TLS exporter error", err)
	}

	_, _, err = c.verifyAuthPayload(
		payload,
		[32]byte{},
		proof.TLSCertPubKeyHash(),
		proof.TLSExporterBinding(),
		[]byte("has-transcript"),
	)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("err = %v, want authentication failed", err)
	}
}

func TestCarrierJoinClusterFailsWhenPeerRejectsAuthHandshake( // A
	t *testing.T,
) {
	t.Parallel()
	cA := newTestCarrier(t, keys.NodeID{1})
	cB := newTestCarrier(t, keys.NodeID{2})

	if err := cA.auth.RevokeAdminCA(cB.localCert.IssuerCAHash()); err != nil {
		t.Fatalf("RevokeAdminCA: %v", err)
	}

	peerA := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{cA.ListenAddr()},
	}
	err := cB.JoinCluster(peerA, nil)
	if err == nil {
		t.Fatal("expected JoinCluster auth rejection error")
	}

	if cB.IsConnected(peerA.NodeID) {
		t.Fatal("peer must not be connected after auth rejection")
	}
}
