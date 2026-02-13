package transport

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"reflect"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func deferCloseNoError( // A
	t *testing.T,
	closeFn func() error,
) {
	t.Helper()
	if err := closeFn(); err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestQuicTransportDialAccept( // A
	t *testing.T,
) {
	t.Parallel()
	authA := auth.NewCarrierAuth(auth.CarrierAuthConfig{})
	authB := auth.NewCarrierAuth(auth.CarrierAuthConfig{})

	tA, err := NewQuicTransport(
		"127.0.0.1:0",
		authA,
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("transport A: %v", err)
	}
	defer deferCloseNoError(t, tA.Close)

	tB, err := NewQuicTransport(
		"127.0.0.1:0",
		authB,
		keys.NodeID{2},
	)
	if err != nil {
		t.Fatalf("transport B: %v", err)
	}
	defer deferCloseNoError(t, tB.Close)

	addrA := tA.(*quicTransportImpl).ListenAddr()

	// Accept in background
	acceptDone := make(chan Connection, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := tA.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		acceptDone <- conn
	}()

	// Dial from B to A
	peer := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{addrA},
	}
	connB, err := tB.Dial(peer)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer deferCloseNoError(t, connB.Close)

	// Wait for accept
	select {
	case connA := <-acceptDone:
		defer deferCloseNoError(t, connA.Close)
	case err := <-acceptErr:
		t.Fatalf("accept: %v", err)
	}
}

func TestQuicTransportStreamRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()
	authA := auth.NewCarrierAuth(auth.CarrierAuthConfig{})
	authB := auth.NewCarrierAuth(auth.CarrierAuthConfig{})

	tA, err := NewQuicTransport(
		"127.0.0.1:0",
		authA,
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("transport A: %v", err)
	}
	defer deferCloseNoError(t, tA.Close)

	tB, err := NewQuicTransport(
		"127.0.0.1:0",
		authB,
		keys.NodeID{2},
	)
	if err != nil {
		t.Fatalf("transport B: %v", err)
	}
	defer deferCloseNoError(t, tB.Close)

	addrA := tA.(*quicTransportImpl).ListenAddr()

	acceptDone := make(chan Connection, 1)
	go func() {
		conn, _ := tA.Accept()
		acceptDone <- conn
	}()

	peer := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{addrA},
	}
	connB, err := tB.Dial(peer)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer deferCloseNoError(t, connB.Close)

	connA := <-acceptDone
	defer deferCloseNoError(t, connA.Close)

	// Open stream from B, accept on A
	streamDone := make(chan Stream, 1)
	go func() {
		s, _ := connA.AcceptStream()
		streamDone <- s
	}()

	sB, err := connB.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	msg := []byte("hello ouroboros")
	_, err = sB.Write(msg)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = sB.Close()

	sA := <-streamDone
	buf := make([]byte, 256)
	n, _ := sA.Read(buf)
	if string(buf[:n]) != string(msg) {
		t.Errorf(
			"got %q, want %q",
			string(buf[:n]),
			string(msg),
		)
	}
}

func TestQuicTransportDatagramRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()
	authA := auth.NewCarrierAuth(auth.CarrierAuthConfig{})
	authB := auth.NewCarrierAuth(auth.CarrierAuthConfig{})

	tA, err := NewQuicTransport(
		"127.0.0.1:0",
		authA,
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("transport A: %v", err)
	}
	defer deferCloseNoError(t, tA.Close)

	tB, err := NewQuicTransport(
		"127.0.0.1:0",
		authB,
		keys.NodeID{2},
	)
	if err != nil {
		t.Fatalf("transport B: %v", err)
	}
	defer deferCloseNoError(t, tB.Close)

	addrA := tA.(*quicTransportImpl).ListenAddr()

	acceptDone := make(chan Connection, 1)
	go func() {
		conn, _ := tA.Accept()
		acceptDone <- conn
	}()

	peer := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{addrA},
	}
	connB, err := tB.Dial(peer)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer deferCloseNoError(t, connB.Close)

	connA := <-acceptDone
	defer deferCloseNoError(t, connA.Close)

	// Send datagram from B, receive on A
	msg := []byte("datagram test")
	err = connB.SendDatagram(msg)
	if err != nil {
		t.Fatalf("send datagram: %v", err)
	}

	data, err := connA.ReceiveDatagram()
	if err != nil {
		t.Fatalf("receive datagram: %v", err)
	}
	if string(data) != string(msg) {
		t.Errorf(
			"got %q, want %q",
			string(data),
			string(msg),
		)
	}
}

func TestQuicTransportClose(t *testing.T) { // A
	t.Parallel()
	tr, err := NewQuicTransport(
		"127.0.0.1:0",
		auth.NewCarrierAuth(auth.CarrierAuthConfig{}),
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}

	conns := tr.GetActiveConnections()
	if len(conns) != 0 {
		t.Errorf(
			"expected 0 connections, got %d",
			len(conns),
		)
	}

	err = tr.Close()
	if err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestQuicTransportDialNoAddress( // A
	t *testing.T,
) {
	t.Parallel()
	tr, err := NewQuicTransport(
		"127.0.0.1:0",
		auth.NewCarrierAuth(auth.CarrierAuthConfig{}),
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	defer deferCloseNoError(t, tr.Close)

	peer := interfaces.PeerNode{
		NodeID: keys.NodeID{2},
	}
	_, err = tr.Dial(peer)
	if err == nil {
		t.Fatal("expected error for no addresses")
	}
}

func TestQuicTransportDialUsesEphemeralClientCert( // A
	t *testing.T,
) {
	t.Parallel()
	authA := auth.NewCarrierAuth(auth.CarrierAuthConfig{})
	authB := auth.NewCarrierAuth(auth.CarrierAuthConfig{})

	tA, err := NewQuicTransport(
		"127.0.0.1:0",
		authA,
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("transport A: %v", err)
	}
	defer deferCloseNoError(t, tA.Close)

	tB, err := NewQuicTransport(
		"127.0.0.1:0",
		authB,
		keys.NodeID{2},
	)
	if err != nil {
		t.Fatalf("transport B: %v", err)
	}
	defer deferCloseNoError(t, tB.Close)

	addrA := tA.(*quicTransportImpl).ListenAddr()
	peer := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{addrA},
	}

	acceptDone := make(chan Connection, 2)
	go func() {
		conn, _ := tA.Accept()
		acceptDone <- conn
	}()
	firstConn, err := tB.Dial(peer)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer deferCloseNoError(t, firstConn.Close)
	serverFirst := <-acceptDone
	defer deferCloseNoError(t, serverFirst.Close)

	go func() {
		conn, _ := tA.Accept()
		acceptDone <- conn
	}()
	secondConn, err := tB.Dial(peer)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	defer deferCloseNoError(t, secondConn.Close)
	serverSecond := <-acceptDone
	defer deferCloseNoError(t, serverSecond.Close)

	firstDER := firstConn.LocalCertificateDER()
	secondDER := secondConn.LocalCertificateDER()
	if len(firstDER) == 0 || len(secondDER) == 0 {
		t.Fatal("expected non-empty local cert DER")
	}
	if bytes.Equal(firstDER, secondDER) {
		t.Fatal("expected per-dial ephemeral client certificates")
	}
}

func TestQuicTransportAllowAttemptRateLimit( // A
	t *testing.T,
) {
	t.Parallel()
	tpt := &quicTransportImpl{
		dialAttempts: make(map[string]attemptCounter),
	}
	addr := "127.0.0.1:9999"

	for i := 0; i < maxAttempts; i++ {
		if !tpt.allowDialAttempt(addr) {
			t.Fatalf("attempt %d unexpectedly throttled", i+1)
		}
	}

	if tpt.allowDialAttempt(addr) {
		t.Fatal("expected dial attempt to be throttled")
	}
}

func TestGenerateSelfSignedCertUsesEd25519( // A
	t *testing.T,
) {
	t.Parallel()

	cert, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate leaf")
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if leaf.PublicKeyAlgorithm != x509.Ed25519 {
		t.Fatalf(
			"public key algorithm = %v, want %v",
			leaf.PublicKeyAlgorithm,
			x509.Ed25519,
		)
	}
}

func TestQuicTransportNegotiatesStrictTLSProfile( // A
	t *testing.T,
) {
	t.Parallel()

	tA := mustNewQuicTransport(t, keys.NodeID{1})
	defer deferCloseNoError(t, tA.Close)
	tB := mustNewQuicTransport(t, keys.NodeID{2})
	defer deferCloseNoError(t, tB.Close)

	connA, connB := mustConnectTransports(t, tA, tB)
	defer deferCloseNoError(t, connA.Close)
	defer deferCloseNoError(t, connB.Close)

	qcA := connA.(*quicConnection)
	stateA := qcA.inner.ConnectionState().TLS
	assertStrictTLSState(t, "server", stateA)

	qcB := connB.(*quicConnection)
	stateB := qcB.inner.ConnectionState().TLS
	assertStrictTLSState(t, "client", stateB)
}

func mustNewQuicTransport( // A
	t *testing.T,
	nodeID keys.NodeID,
) QuicTransport {
	t.Helper()

	tpt, err := NewQuicTransport(
		"127.0.0.1:0",
		auth.NewCarrierAuth(auth.CarrierAuthConfig{}),
		nodeID,
	)
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	return tpt
}

type acceptResult struct { // A
	conn Connection
	err  error
}

func mustConnectTransports( // A
	t *testing.T,
	tA QuicTransport,
	tB QuicTransport,
) (Connection, Connection) {
	t.Helper()

	addrA := tA.(*quicTransportImpl).ListenAddr()
	acceptResultCh := make(chan acceptResult, 1)
	go func() {
		conn, err := tA.Accept()
		acceptResultCh <- acceptResult{conn: conn, err: err}
	}()

	peer := interfaces.PeerNode{
		NodeID:    keys.NodeID{1},
		Addresses: []string{addrA},
	}
	connB, err := tB.Dial(peer)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	result := <-acceptResultCh
	if result.err != nil {
		t.Fatalf("accept: %v", result.err)
	}

	return result.conn, connB
}

func assertStrictTLSState( // A
	t *testing.T,
	side string,
	state tls.ConnectionState,
) {
	t.Helper()

	if state.Version != tls.VersionTLS13 {
		t.Fatalf("%s TLS version = %x, want TLS1.3", side, state.Version)
	}
	curve, ok, err := curveIDFromStateForTest(state)
	if err != nil {
		t.Fatalf("%s curve read: %v", side, err)
	}
	if !ok || curve != uint64(tls.X25519MLKEM768) {
		t.Fatalf(
			"%s curve = %v, want %v",
			side,
			curve,
			tls.X25519MLKEM768,
		)
	}
}

func curveIDFromStateForTest( // A
	state tls.ConnectionState,
) (uint64, bool, error) {
	v := reflect.ValueOf(state)
	field := v.FieldByName("CurveID")
	if !field.IsValid() {
		return 0, false, nil
	}
	if !field.CanUint() {
		return 0, false, fmt.Errorf("invalid curve field type")
	}
	return field.Uint(), true, nil
}
