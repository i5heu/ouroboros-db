package carrier

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

type stubCarrierAuth struct { // A
	authCtx auth.AuthContext
	err     error
}

func (s *stubCarrierAuth) VerifyPeerCert( // A
	_ interfaces.PeerHandshake,
) (interfaces.AuthContext, error) {
	if s.err != nil {
		return interfaces.AuthContext{}, s.err
	}
	return s.authCtx, nil
}

func (s *stubCarrierAuth) AddAdminPubKey([]byte) error { // A
	return nil
}

func (s *stubCarrierAuth) AddUserPubKey( // A
	[]byte,
	[]byte,
	string,
) error {
	return nil
}

func (s *stubCarrierAuth) RemoveAdminPubKey(string) error { // A
	return nil
}

func (s *stubCarrierAuth) RemoveUserPubKey(string) error { // A
	return nil
}

func (s *stubCarrierAuth) RevokeAdminCA(string) error { // A
	return nil
}

func (s *stubCarrierAuth) RevokeUserCA(string) error { // A
	return nil
}

func (s *stubCarrierAuth) RevokeNode(keys.NodeID) error { // A
	return nil
}

func (s *stubCarrierAuth) SetRevocationHook( // A
	auth.RevocationHook,
) {
}

type scriptedConn struct { // A
	mu                   sync.Mutex
	tlsBindings          interfaces.TLSBindings
	exporter             map[string][]byte
	openStream           interfaces.Stream
	acceptStreams        []interfaces.Stream
	closed               chan struct{}
	closeOnce            sync.Once
	openStreamCalls      int
	acceptStreamCalls    int
	sendDatagramCalls    int
	receiveDatagramCalls int
	closeCalls           int
}

func newScriptedConn( // A
	tlsBindings interfaces.TLSBindings,
	exporter map[string][]byte,
	acceptStreams ...interfaces.Stream,
) *scriptedConn {
	return &scriptedConn{
		tlsBindings: tlsBindings,
		exporter:    exporter,
		closed:      make(chan struct{}),
		acceptStreams: append(
			[]interfaces.Stream(nil),
			acceptStreams...,
		),
	}
}

func (c *scriptedConn) NodeID() keys.NodeID { // A
	return keys.NodeID{}
}

func (c *scriptedConn) RemoteAddr() string { // A
	return "scripted-remote"
}

func (c *scriptedConn) TLSBindings() interfaces.TLSBindings { // A
	return c.tlsBindings
}

func (c *scriptedConn) ExportKeyingMaterial( // A
	label string,
	_ []byte,
	_ int,
) ([]byte, error) {
	data, ok := c.exporter[label]
	if !ok {
		return nil, errors.New("missing exporter")
	}
	return append([]byte(nil), data...), nil
}

func (c *scriptedConn) OpenStream() (interfaces.Stream, error) { // A
	c.mu.Lock()
	c.openStreamCalls++
	stream := c.openStream
	c.mu.Unlock()
	if stream == nil {
		return nil, errors.New("open stream not configured")
	}
	return stream, nil
}

func (c *scriptedConn) AcceptStream() (interfaces.Stream, error) { // A
	c.mu.Lock()
	c.acceptStreamCalls++
	if len(c.acceptStreams) > 0 {
		stream := c.acceptStreams[0]
		c.acceptStreams = c.acceptStreams[1:]
		c.mu.Unlock()
		return stream, nil
	}
	closed := c.closed
	c.mu.Unlock()
	<-closed
	return nil, errors.New("closed")
}

func (c *scriptedConn) SendDatagram([]byte) error { // A
	c.mu.Lock()
	c.sendDatagramCalls++
	c.mu.Unlock()
	return nil
}

func (c *scriptedConn) ReceiveDatagram() ([]byte, error) { // A
	c.mu.Lock()
	c.receiveDatagramCalls++
	closed := c.closed
	c.mu.Unlock()
	<-closed
	return nil, errors.New("closed")
}

func (c *scriptedConn) Close() error { // A
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closeCalls++
		c.mu.Unlock()
		close(c.closed)
	})
	return nil
}

func (c *scriptedConn) counts( // A
) (int, int, int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.openStreamCalls,
		c.acceptStreamCalls,
		c.sendDatagramCalls,
		c.receiveDatagramCalls,
		c.closeCalls
}

func newCarrierTestHarness( // A
	conf CarrierConfig,
) *carrierImpl {
	return &carrierImpl{
		logger: slog.New(
			slog.NewTextHandler(io.Discard, nil),
		),
		config:      conf,
		registry:    newNodeRegistry(),
		connections: make(map[keys.NodeID]interfaces.Connection),
		peerScopes:  make(map[keys.NodeID]auth.TrustScope),
	}
}

func newNodeIdentityForCarrierTest( // A
	t *testing.T,
) (*auth.NodeIdentity, auth.NodeCertLike) {
	t.Helper()
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminPub := adminAC.GetPublicKey()
	adminBytes, err := auth.MarshalPubKeyBytes(&adminPub)
	if err != nil {
		t.Fatalf("marshal admin pubkey: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminBytes)
	if err != nil {
		t.Fatalf("new admin ca: %v", err)
	}
	nodeAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("node key: %v", err)
	}
	nodePub := nodeAC.GetPublicKey()
	now := time.Now().Unix()
	cert, err := auth.NewNodeCert(
		nodePub,
		adminCA.Hash(),
		now-60,
		now+3600,
		[]byte("serial"),
		[]byte("nonce"),
	)
	if err != nil {
		t.Fatalf("new node cert: %v", err)
	}
	ni, err := auth.NewNodeIdentity(
		nodeAC,
		[]auth.NodeCertLike{cert},
		[][]byte{[]byte("ca-sig")},
		nil,
	)
	if err != nil {
		t.Fatalf("new node identity: %v", err)
	}
	return ni, cert
}

func newTLSBindingsForCarrierTest() interfaces.TLSBindings { // A
	return interfaces.TLSBindings{
		CertPubKeyHash:  bytesOf(0x11, auth.TLSCertPubKeyHashSize),
		ExporterBinding: bytesOf(0x22, auth.TLSExporterBindingSize),
		X509Fingerprint: bytesOf(0x33, auth.X509FingerprintSize),
		TranscriptHash:  bytesOf(0x44, auth.TLSTranscriptHashSize),
	}
}

func newExportersForCarrierTest() map[string][]byte { // A
	return map[string][]byte{
		auth.TranscriptBindingLabel: bytesOf(
			0x44,
			auth.TLSTranscriptHashSize,
		),
		auth.ExporterLabel: bytesOf(
			0x55,
			auth.TLSExporterBindingSize,
		),
	}
}

func bytesOf(fill byte, size int) []byte { // A
	out := make([]byte, size)
	for i := range out {
		out[i] = fill
	}
	return out
}

func newHandshakeStreamForCarrierTest( // A
	t *testing.T,
	ni *auth.NodeIdentity,
	conn interfaces.Connection,
) interfaces.Stream {
	t.Helper()
	proof, sig, err := auth.SignDelegation(
		ni.Key(),
		ni.Certs(),
		ni.Session(),
		func(
			label string,
			ctx []byte,
			length int,
		) ([]byte, error) {
			return conn.ExportKeyingMaterial(
				label,
				ctx,
				length,
			)
		},
	)
	if err != nil {
		t.Fatalf("sign delegation: %v", err)
	}
	stream := newTestStream(nil)
	err = writeAuthHandshake(
		stream,
		ni.Certs(),
		ni.CASigs(),
		ni.Authorities(),
		proof,
		sig,
	)
	if err != nil {
		t.Fatalf("write auth handshake: %v", err)
	}
	return newTestStream(stream.writes.Bytes())
}

func waitForCarrierCondition( // A
	t *testing.T,
	condition func() bool,
	message string,
) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(message)
}

func TestConnectionForNodeRejectsUnknownPeer( // A
	t *testing.T,
) {
	carrier := newCarrierTestHarness(CarrierConfig{})
	_, err := carrier.connectionForNode(keys.NodeID{})
	if err == nil {
		t.Fatal("expected unknown peer lookup to fail")
	}
	if !strings.Contains(err.Error(), "node not connected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendMessageToNodeUnreliableRejectsUnknownPeer( // A
	t *testing.T,
) {
	carrier := newCarrierTestHarness(CarrierConfig{})
	err := carrier.SendMessageToNodeUnreliable(
		keys.NodeID{},
		interfaces.Message{Type: interfaces.MessageTypeHeartbeat},
	)
	if err == nil {
		t.Fatal("expected unreliable send to fail")
	}
	if !strings.Contains(err.Error(), "node not connected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDialAndAuthUsesReliableStream( // A
	t *testing.T,
) {
	ni, _ := newNodeIdentityForCarrierTest(t)
	bindings := newTLSBindingsForCarrierTest()
	exporters := newExportersForCarrierTest()
	writeStream := newTestStream(nil)
	conn := newScriptedConn(bindings, exporters)
	conn.openStream = writeStream
	carrier := newCarrierTestHarness(CarrierConfig{})

	err := carrier.dialAndAuth(conn, ni)
	if err != nil {
		t.Fatalf("dialAndAuth: %v", err)
	}
	openCalls, _, dgramCalls, _, _ := conn.counts()
	if openCalls != 1 {
		t.Fatalf("open stream calls = %d, want 1", openCalls)
	}
	if dgramCalls != 0 {
		t.Fatalf("send datagram calls = %d, want 0", dgramCalls)
	}
	if writeStream.writes.Len() == 0 {
		t.Fatal("expected auth handshake bytes on stream")
	}
}

func TestHandleIncomingConnAuthFailureSkipsDatagrams( // A
	t *testing.T,
) {
	ni, cert := newNodeIdentityForCarrierTest(t)
	bindings := newTLSBindingsForCarrierTest()
	exporters := newExportersForCarrierTest()
	conn := newScriptedConn(bindings, exporters)
	conn.acceptStreams = []interfaces.Stream{
		newHandshakeStreamForCarrierTest(t, ni, conn),
	}
	carrier := newCarrierTestHarness(CarrierConfig{
		Auth: &stubCarrierAuth{err: errors.New("denied")},
	})

	carrier.handleIncomingConn(context.Background(), conn)

	_, acceptCalls, dgramCalls, recvCalls, closeCalls := conn.counts()
	if acceptCalls != 1 {
		t.Fatalf("accept stream calls = %d, want 1", acceptCalls)
	}
	if dgramCalls != 0 {
		t.Fatalf("send datagram calls = %d, want 0", dgramCalls)
	}
	if recvCalls != 0 {
		t.Fatalf("receive datagram calls = %d, want 0", recvCalls)
	}
	if closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", closeCalls)
	}
	if carrier.IsConnected(cert.NodeID()) {
		t.Fatal("peer should not be registered after auth failure")
	}
}

func TestHandleIncomingConnStartsLoopsAfterAuth( // A
	t *testing.T,
) {
	ni, cert := newNodeIdentityForCarrierTest(t)
	bindings := newTLSBindingsForCarrierTest()
	exporters := newExportersForCarrierTest()
	responseStream := newTestStream(nil)
	conn := newScriptedConn(bindings, exporters)
	conn.acceptStreams = []interfaces.Stream{
		newHandshakeStreamForCarrierTest(t, ni, conn),
	}
	conn.openStream = responseStream
	carrier := newCarrierTestHarness(CarrierConfig{
		Auth: &stubCarrierAuth{authCtx: auth.AuthContext{
			NodeID:         cert.NodeID(),
			EffectiveScope: auth.ScopeAdmin,
		}},
		NodeIdentity: ni,
	})

	carrier.handleIncomingConn(context.Background(), conn)

	waitForCarrierCondition(t, func() bool {
		openCalls, acceptCalls, _, recvCalls, _ := conn.counts()
		return openCalls >= 1 && acceptCalls >= 2 && recvCalls >= 1
	}, "expected post-auth stream and datagram loops to start")

	if !carrier.IsConnected(cert.NodeID()) {
		t.Fatal("peer should be connected after auth success")
	}
	if responseStream.writes.Len() == 0 {
		t.Fatal("expected local auth response on stream")
	}
	_ = conn.Close()
}
