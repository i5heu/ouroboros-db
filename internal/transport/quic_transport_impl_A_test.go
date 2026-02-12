package transport

import (
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func TestQuicTransportDialAccept( // A
	t *testing.T,
) {
	t.Parallel()
	authA := auth.NewCarrierAuth()
	authB := auth.NewCarrierAuth()

	tA, err := NewQuicTransport(
		"127.0.0.1:0",
		authA,
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("transport A: %v", err)
	}
	defer tA.Close()

	tB, err := NewQuicTransport(
		"127.0.0.1:0",
		authB,
		keys.NodeID{2},
	)
	if err != nil {
		t.Fatalf("transport B: %v", err)
	}
	defer tB.Close()

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
	defer connB.Close()

	// Wait for accept
	select {
	case connA := <-acceptDone:
		defer connA.Close()
	case err := <-acceptErr:
		t.Fatalf("accept: %v", err)
	}
}

func TestQuicTransportStreamRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()
	authA := auth.NewCarrierAuth()
	authB := auth.NewCarrierAuth()

	tA, err := NewQuicTransport(
		"127.0.0.1:0",
		authA,
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("transport A: %v", err)
	}
	defer tA.Close()

	tB, err := NewQuicTransport(
		"127.0.0.1:0",
		authB,
		keys.NodeID{2},
	)
	if err != nil {
		t.Fatalf("transport B: %v", err)
	}
	defer tB.Close()

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
	defer connB.Close()

	connA := <-acceptDone
	defer connA.Close()

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
	authA := auth.NewCarrierAuth()
	authB := auth.NewCarrierAuth()

	tA, err := NewQuicTransport(
		"127.0.0.1:0",
		authA,
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("transport A: %v", err)
	}
	defer tA.Close()

	tB, err := NewQuicTransport(
		"127.0.0.1:0",
		authB,
		keys.NodeID{2},
	)
	if err != nil {
		t.Fatalf("transport B: %v", err)
	}
	defer tB.Close()

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
	defer connB.Close()

	connA := <-acceptDone
	defer connA.Close()

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
		auth.NewCarrierAuth(),
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
		auth.NewCarrierAuth(),
		keys.NodeID{1},
	)
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	defer tr.Close()

	peer := interfaces.PeerNode{
		NodeID: keys.NodeID{2},
	}
	_, err = tr.Dial(peer)
	if err == nil {
		t.Fatal("expected error for no addresses")
	}
}
