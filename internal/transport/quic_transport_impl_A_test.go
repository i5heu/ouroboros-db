package transport

import (
	"bytes"
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
