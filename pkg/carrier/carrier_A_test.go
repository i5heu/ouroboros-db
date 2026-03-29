package carrier

import (
	"context"
	"crypto/rand"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func testLogger() *slog.Logger { // A
	return slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	)
}

type recorder struct { // A
	mu       sync.Mutex
	messages []*interfaces.WireMessage
	auths    []auth.AuthContext
	ch       chan struct{}
}

func newRecorder() *recorder { // A
	return &recorder{ch: make(chan struct{}, 16)}
}

func (r *recorder) dispatch( // A
	_ context.Context,
	msg interfaces.Message,
	authCtx auth.AuthContext,
) (interfaces.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wm, ok := msg.(*interfaces.WireMessage)
	if ok {
		r.messages = append(r.messages, wm)
	}
	r.auths = append(r.auths, authCtx)
	select {
	case r.ch <- struct{}{}:
	default:
	}
	return interfaces.NewWireResponse(
		msg.GetPayload(),
		"",
		map[string]string{"nodeID": authCtx.NodeID.String()},
	), nil
}

func (r *recorder) waitForCount( // A
	t *testing.T,
	want int,
) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		r.mu.Lock()
		count := len(r.messages)
		r.mu.Unlock()
		if count >= want {
			return
		}
		select {
		case <-r.ch:
		case <-timeout:
			t.Fatalf("timeout waiting for %d messages", want)
		}
	}
}

func (r *recorder) count() int { // A
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

func waitForPeerRole( // A
	t *testing.T,
	carrier *Carrier,
	nodeID keys.NodeID,
	want connectionRole,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		carrier.mu.RLock()
		peer, ok := carrier.peers[nodeID]
		connected := ok && peer.conn != nil
		role := connectionRoleUnknown
		if ok {
			role = peer.connRole
		}
		carrier.mu.RUnlock()
		if connected && role == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for peer role %d", want)
}

func (r *recorder) last() (*interfaces.WireMessage, auth.AuthContext) { // A
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.messages[len(r.messages)-1], r.auths[len(r.auths)-1]
}

func mustSigner(t *testing.T) *keys.AsyncCrypt { // A
	t.Helper()
	signer, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("keys.NewAsyncCrypt: %v", err)
	}
	return signer
}

func trustedAdminBytes( // A
	t *testing.T,
	signer *keys.AsyncCrypt,
) []byte {
	t.Helper()
	pubKey := signer.GetPublicKey()
	kem, err := pubKey.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("marshal KEM: %v", err)
	}
	sign, err := pubKey.MarshalBinarySign()
	if err != nil {
		t.Fatalf("marshal sign: %v", err)
	}
	combined := make([]byte, len(kem)+len(sign))
	copy(combined, kem)
	copy(combined[len(kem):], sign)
	return combined
}

func issueNodeCert( // A
	t *testing.T,
	adminSigner *keys.AsyncCrypt,
	nodeSigner *keys.AsyncCrypt,
) (interfaces.NodeCert, []byte) {
	t.Helper()
	adminPub := adminSigner.GetPublicKey()
	adminBytes := trustedAdminBytes(t, adminSigner)
	adminCA, err := auth.NewAdminCA(adminBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	serial := make([]byte, 16)
	nonce := make([]byte, 16)
	if _, err := rand.Read(serial); err != nil {
		t.Fatalf("serial rand: %v", err)
	}
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("nonce rand: %v", err)
	}
	now := time.Now().Unix()
	cert, err := auth.NewNodeCert(
		nodeSigner.GetPublicKey(),
		adminCA.Hash(),
		now-60,
		now+3600,
		serial,
		nonce,
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	_ = adminPub
	canonical, err := auth.CanonicalNodeCert(cert)
	if err != nil {
		t.Fatalf("CanonicalNodeCert: %v", err)
	}
	sig, err := adminSigner.Sign(
		auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical),
	)
	if err != nil {
		t.Fatalf("admin sign: %v", err)
	}
	return cert, sig
}

func TestCarrierReliableAndUnreliableMessaging( // A
	t *testing.T,
) {
	t.Parallel()
	logger := testLogger()
	adminSigner := mustSigner(t)
	signerA := mustSigner(t)
	signerB := mustSigner(t)
	adminBytes := trustedAdminBytes(t, adminSigner)
	certA, sigA := issueNodeCert(t, adminSigner, signerA)
	certB, sigB := issueNodeCert(t, adminSigner, signerB)

	recorderA := newRecorder()
	carrierA, err := New(Config{
		Logger:              logger,
		ListenAddress:       "127.0.0.1:0",
		LocalSigner:         signerA,
		LocalNodeCerts:      []interfaces.NodeCert{certA},
		LocalCASignatures:   [][]byte{sigA},
		TrustedAdminPubKeys: [][]byte{adminBytes},
		Dispatcher:          recorderA.dispatch,
	})
	if err != nil {
		t.Fatalf("New carrierA: %v", err)
	}
	defer carrierA.Close()

	recorderB := newRecorder()
	carrierB, err := New(Config{
		Logger:              logger,
		ListenAddress:       "127.0.0.1:0",
		LocalSigner:         signerB,
		LocalNodeCerts:      []interfaces.NodeCert{certB},
		LocalCASignatures:   [][]byte{sigB},
		TrustedAdminPubKeys: [][]byte{adminBytes},
		Dispatcher:          recorderB.dispatch,
	})
	if err != nil {
		t.Fatalf("New carrierB: %v", err)
	}
	defer carrierB.Close()

	if err := carrierB.JoinCluster(carrierA.LocalPeer()); err != nil {
		t.Fatalf("JoinCluster B->A: %v", err)
	}

	msg := interfaces.NewWireMessage(
		interfaces.MessageTypeHeartbeat,
		[]byte("reliable"),
	)
	if err := carrierB.SendMessageToNodeReliable(
		carrierA.LocalPeer().NodeID,
		msg,
	); err != nil {
		t.Fatalf("SendMessageToNodeReliable: %v", err)
	}
	recorderA.waitForCount(t, 1)
	received, authCtx := recorderA.last()
	if got := string(received.GetPayload()); got != "reliable" {
		t.Fatalf("payload: got %q", got)
	}
	if authCtx.NodeID != carrierB.LocalPeer().NodeID {
		t.Fatalf("auth node ID mismatch: got %s", authCtx.NodeID)
	}

	before := recorderA.count()
	datagram := interfaces.NewWireMessage(
		interfaces.MessageTypeLogPush,
		[]byte("datagram"),
	)
	if err := carrierB.SendMessageToNodeUnreliable(
		carrierA.LocalPeer().NodeID,
		datagram,
	); err != nil {
		t.Fatalf("SendMessageToNodeUnreliable: %v", err)
	}
	recorderA.waitForCount(t, before+1)
	received, _ = recorderA.last()
	if got := string(received.GetPayload()); got != "datagram" {
		t.Fatalf("datagram payload: got %q", got)
	}
	if !carrierA.IsConnected(carrierB.LocalPeer().NodeID) {
		t.Fatal("carrierA should report connected peer")
	}
	if !carrierB.IsConnected(carrierA.LocalPeer().NodeID) {
		t.Fatal("carrierB should report connected peer")
	}
}

func TestCarrierSimultaneousDialUsesDeterministicWinner( // A
	t *testing.T,
) {
	t.Parallel()
	logger := testLogger()
	adminSigner := mustSigner(t)
	signerA := mustSigner(t)
	signerB := mustSigner(t)
	adminBytes := trustedAdminBytes(t, adminSigner)
	certA, sigA := issueNodeCert(t, adminSigner, signerA)
	certB, sigB := issueNodeCert(t, adminSigner, signerB)

	recorderA := newRecorder()
	carrierA, err := New(Config{
		Logger:              logger,
		ListenAddress:       "127.0.0.1:0",
		LocalSigner:         signerA,
		LocalNodeCerts:      []interfaces.NodeCert{certA},
		LocalCASignatures:   [][]byte{sigA},
		TrustedAdminPubKeys: [][]byte{adminBytes},
		Dispatcher:          recorderA.dispatch,
	})
	if err != nil {
		t.Fatalf("New carrierA: %v", err)
	}
	defer carrierA.Close()

	recorderB := newRecorder()
	carrierB, err := New(Config{
		Logger:              logger,
		ListenAddress:       "127.0.0.1:0",
		LocalSigner:         signerB,
		LocalNodeCerts:      []interfaces.NodeCert{certB},
		LocalCASignatures:   [][]byte{sigB},
		TrustedAdminPubKeys: [][]byte{adminBytes},
		Dispatcher:          recorderB.dispatch,
	})
	if err != nil {
		t.Fatalf("New carrierB: %v", err)
	}
	defer carrierB.Close()

	if err := carrierA.JoinCluster(carrierB.LocalPeer()); err != nil {
		t.Fatalf("JoinCluster A->B: %v", err)
	}
	if err := carrierB.JoinCluster(carrierA.LocalPeer()); err != nil {
		t.Fatalf("JoinCluster B->A: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, err := carrierA.ensureConnection(carrierB.LocalPeer().NodeID)
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		_, _, err := carrierB.ensureConnection(carrierA.LocalPeer().NodeID)
		errCh <- err
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("ensureConnection: %v", err)
		}
	}

	peerA := carrierA.LocalPeer()
	peerB := carrierB.LocalPeer()
	waitForPeerRole(
		t,
		carrierA,
		peerB.NodeID,
		preferredRole(peerA.NodeID, peerB.NodeID),
	)
	waitForPeerRole(
		t,
		carrierB,
		peerA.NodeID,
		preferredRole(peerB.NodeID, peerA.NodeID),
	)

	msgA := interfaces.NewWireMessage(
		interfaces.MessageTypeHeartbeat,
		[]byte("from-a"),
	)
	if err := carrierA.SendMessageToNodeReliable(peerB.NodeID, msgA); err != nil {
		t.Fatalf("SendMessageToNodeReliable A->B: %v", err)
	}
	recorderB.waitForCount(t, 1)

	msgB := interfaces.NewWireMessage(
		interfaces.MessageTypeHeartbeat,
		[]byte("from-b"),
	)
	if err := carrierB.SendMessageToNodeReliable(peerA.NodeID, msgB); err != nil {
		t.Fatalf("SendMessageToNodeReliable B->A: %v", err)
	}
	recorderA.waitForCount(t, 1)
}
