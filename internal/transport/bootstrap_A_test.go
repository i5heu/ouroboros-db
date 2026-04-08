package transport

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"pgregory.net/rapid"
)

// bootstrapDialTransport is a test transport that
// records dial attempts and returns pre-configured
// connections or errors by address.
type bootstrapDialTransport struct { // A
	mu         sync.Mutex
	attempts   []string
	connByAddr map[string]interfaces.Connection
	errByAddr  map[string]error
}

func newBootstrapDialTransport() *bootstrapDialTransport { // A
	return &bootstrapDialTransport{
		connByAddr: make(map[string]interfaces.Connection),
		errByAddr:  make(map[string]error),
	}
}

func (t *bootstrapDialTransport) Dial( // A
	node *interfaces.Node,
) (interfaces.Connection, error) {
	if len(node.Addresses) == 0 {
		return nil, errors.New("no addresses")
	}
	addr := node.Addresses[0]
	t.mu.Lock()
	t.attempts = append(t.attempts, addr)
	conn := t.connByAddr[addr]
	err := t.errByAddr[addr]
	t.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if conn != nil {
		return conn, nil
	}
	return nil, errors.New("no conn configured")
}

func (t *bootstrapDialTransport) attemptCount() int { // A
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.attempts)
}

func (t *bootstrapDialTransport) Accept() ( // A
	interfaces.Connection, error,
) {
	return nil, errors.New("not implemented")
}

func (t *bootstrapDialTransport) Close() error { // A
	return nil
}

func (t *bootstrapDialTransport) GetActiveConnections() []interfaces.Connection { // A
	return nil
}

// makeBootstrapConn builds a scriptedConn pre-loaded
// with an inbound auth handshake stream for peerNI.
// The connection is automatically closed at test end.
func makeBootstrapConn( // A
	t *testing.T,
	peerNI *auth.NodeIdentity,
) *scriptedConn {
	t.Helper()
	bindings := newTLSBindingsForCarrierTest()
	exporters := newExportersForCarrierTest()
	conn := newScriptedConn(bindings, exporters)
	// dialAndAuth opens a stream to write our auth;
	// provide a discard sink.
	conn.openStream = newTestStream(nil)
	// awaitPeerAuth accepts a stream with the peer's
	// auth message pre-loaded.
	conn.acceptStreams = []interfaces.Stream{
		newHandshakeStreamForCarrierTest(t, peerNI, conn),
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// newBootstrapCarrier constructs a carrierImpl wired
// with a stubCarrierAuth that approves peerNI, using
// the same self identity for both SelfCert and
// NodeIdentity to avoid ID mismatches in assertions.
func newBootstrapCarrier( // A
	t *testing.T,
	peerNI *auth.NodeIdentity,
	selfNI *auth.NodeIdentity,
	transport interfaces.QuicTransport,
	maxRetries int,
	retryInterval time.Duration,
) *carrierImpl {
	t.Helper()
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:     selfNI.Certs()[0],
		NodeIdentity: selfNI,
		Auth: &stubCarrierAuth{
			authCtx: auth.AuthContext{
				NodeID:         peerNI.Certs()[0].NodeID(),
				EffectiveScope: auth.ScopeAdmin,
			},
		},
		BootstrapRetryInterval: retryInterval,
		BootstrapMaxRetries:    maxRetries,
	})
	c.transport = transport
	return c
}

func TestBootstrapNoAddresses( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	c := newCarrierTestHarness(
		CarrierConfig{SelfCert: selfNI.Certs()[0]},
	)
	// runBootstrapLoop must return immediately.
	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal(
			"runBootstrapLoop did not return with no addresses",
		)
	}
	if c.isOffline() {
		t.Fatal("node should remain online with no addresses")
	}
}

func TestBootstrapFirstAttemptSuccess( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	conn := makeBootstrapConn(t, peerNI)
	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn
	c := newBootstrapCarrier(
		t, peerNI, selfNI, tr, 5, 10*time.Millisecond,
	)
	c.config.BootstrapAddresses = []string{"peer:1"}

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	if c.isOffline() {
		t.Fatal("node should be online after successful bootstrap")
	}
	if !c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("expected peer to be connected after bootstrap")
	}
	if tr.attemptCount() != 1 {
		t.Fatalf(
			"dial attempts = %d, want 1",
			tr.attemptCount(),
		)
	}
}

func TestBootstrapRetryThenSuccess( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)

	// First 2 dials fail; 3rd succeeds.
	var dialCount atomic.Int32
	conn := makeBootstrapConn(t, peerNI)
	tr := &countingDialTransport{
		failUntil: 2,
		dialCount: &dialCount,
		conn:      conn,
	}
	c := newBootstrapCarrier(
		t, peerNI, selfNI, tr, 5, 10*time.Millisecond,
	)
	c.config.BootstrapAddresses = []string{"peer:1"}

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	if c.isOffline() {
		t.Fatal("node should be online after eventual success")
	}
	if !c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("expected peer connected after retry success")
	}
	// 2 failures + 1 success = exactly 3 dials.
	if dialCount.Load() != 3 {
		t.Fatalf("dial count = %d, want 3", dialCount.Load())
	}
}

func TestBootstrapDefaultExhaustedFiveTotalAttempts( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	tr := newBootstrapDialTransport()
	tr.errByAddr["peer:1"] = errors.New("refused")
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		BootstrapRetryInterval: 1 * time.Millisecond,
		// Use the default constant explicitly so the
		// test breaks if someone changes the default.
		BootstrapMaxRetries: defaultBootstrapMaxRetries,
		BootstrapAddresses:  []string{"peer:1"},
	})
	c.transport = tr

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return after exhaustion")
	}

	if !c.isOffline() {
		t.Fatal("node should be offline after bootstrap exhaustion")
	}
	// 1 initial + defaultBootstrapMaxRetries retries =
	// 5 total attempts (matches "5 tries" requirement).
	wantAttempts := 1 + defaultBootstrapMaxRetries
	if tr.attemptCount() != wantAttempts {
		t.Fatalf(
			"dial attempts = %d, want %d",
			tr.attemptCount(),
			wantAttempts,
		)
	}
}

func TestBootstrapExhaustedGoesOffline( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	tr := newBootstrapDialTransport()
	tr.errByAddr["peer:1"] = errors.New("refused")
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		BootstrapRetryInterval: 50 * time.Millisecond, // Short interval for testing
		BootstrapMaxRetries:    2,                     // 2 retries (3 total attempts)
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return after exhaustion")
	}

	if !c.isOffline() {
		t.Fatal("node should be offline after bootstrap exhaustion")
	}
	// 1 initial + 2 retries = 3 total attempts.
	if tr.attemptCount() != 3 {
		t.Fatalf(
			"dial attempts = %d, want 3",
			tr.attemptCount(),
		)
	}
}

func TestBootstrapContextCancelDoesNotGoOffline( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	tr := newBootstrapDialTransport()
	tr.errByAddr["peer:1"] = errors.New("refused")
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		BootstrapRetryInterval: 10 * time.Second,
		BootstrapMaxRetries:    100,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(ctx)
		close(done)
	}()

	// Wait for the first attempt to fire, then cancel.
	waitForCarrierCondition(t, func() bool {
		return tr.attemptCount() >= 1
	}, "first bootstrap attempt never fired")
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal(
			"runBootstrapLoop did not stop on context cancel",
		)
	}
	// Cancellation is not exhaustion; node must stay online.
	if c.isOffline() {
		t.Fatal(
			"context cancel must not put node offline",
		)
	}
}

// TestBootstrapAuthDeniedCountsAsFailure verifies that
// a peer whose cert is rejected by CarrierAuth is never
// registered, and repeated denials exhaust retries.
// Each attempt sends a valid handshake so readAuthHandshake
// succeeds — the denial comes from VerifyPeerCert.
func TestBootstrapAuthDeniedCountsAsFailure( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	// Three retry attempts, each with a valid auth stream.
	// VerifyPeerCert always returns an error.
	var conns [3]*scriptedConn
	for i := range conns {
		c := newScriptedConn(
			newTLSBindingsForCarrierTest(),
			newExportersForCarrierTest(),
		)
		c.openStream = newTestStream(nil)
		c.acceptStreams = []interfaces.Stream{
			newHandshakeStreamForCarrierTest(t, peerNI, c),
		}
		conns[i] = c
		t.Cleanup(func() { _ = c.Close() })
	}
	var dialIdx atomic.Int32
	tr := &funcDialTransport{dialFn: func(
		node interfaces.Node,
	) (interfaces.Connection, error) {
		i := int(dialIdx.Add(1)) - 1
		if i >= len(conns) {
			return nil, errors.New("no more conns")
		}
		return conns[i], nil
	}}
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		Auth:                   &stubCarrierAuth{err: errors.New("denied")},
		BootstrapRetryInterval: 1 * time.Millisecond,
		BootstrapMaxRetries:    2,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	if !c.isOffline() {
		t.Fatal("auth denials should exhaust bootstrap")
	}
	// Peer must never be registered despite valid handshake.
	if c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("denied peer must not be registered")
	}
	// Each denied conn must have been closed.
	for i, conn := range conns {
		_, _, _, _, closeCalls := conn.counts()
		if closeCalls != 1 {
			t.Fatalf(
				"conn[%d] close calls = %d, want 1",
				i, closeCalls,
			)
		}
	}
}

// TestBootstrapPartialSuccessIsEnough verifies that
// one reachable bootstrap address out of many is
// sufficient for the node to come online.
func TestBootstrapPartialSuccessIsEnough( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	conn := makeBootstrapConn(t, peerNI)
	tr := newBootstrapDialTransport()
	tr.errByAddr["bad:1"] = errors.New("refused")
	tr.errByAddr["bad:2"] = errors.New("refused")
	tr.connByAddr["good:1"] = conn
	c := newBootstrapCarrier(
		t, peerNI, selfNI, tr, 0, time.Millisecond,
	)
	c.config.BootstrapAddresses = []string{
		"bad:1", "bad:2", "good:1",
	}

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	if c.isOffline() {
		t.Fatal(
			"one reachable address should be enough to come online",
		)
	}
	if !c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("expected the reachable peer to be connected")
	}
	if tr.attemptCount() != 3 {
		t.Fatalf(
			"dial attempts = %d, want 3 (one per address)",
			tr.attemptCount(),
		)
	}
}

func TestReconnectResetsOfflineAndConnects( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	conn := makeBootstrapConn(t, peerNI)
	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn
	c := newBootstrapCarrier(
		t, peerNI, selfNI, tr, 1, 10*time.Millisecond,
	)
	c.config.BootstrapAddresses = []string{"peer:1"}

	c.setOffline()
	if !c.isOffline() {
		t.Fatal("precondition: node should be offline")
	}

	err := c.Reconnect()
	if err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	if c.isOffline() {
		t.Fatal("node should be online after Reconnect")
	}
	if !c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("expected peer connected after Reconnect")
	}
}

func TestReconnectNoAddressesReturnsError( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	c := newCarrierTestHarness(
		CarrierConfig{SelfCert: selfNI.Certs()[0]},
	)

	err := c.Reconnect()
	if err == nil {
		t.Fatal("expected error with no bootstrap addresses")
	}
	if !errors.Is(err, ErrNoBootstrapAddresses) {
		t.Fatalf(
			"error = %q, want ErrNoBootstrapAddresses",
			err.Error(),
		)
	}
}

func TestReconnectExhaustedReturnsError( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	tr := newBootstrapDialTransport()
	tr.errByAddr["peer:1"] = errors.New("refused")
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		BootstrapRetryInterval: 1 * time.Millisecond,
		BootstrapMaxRetries:    1,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr
	c.setOffline()

	err := c.Reconnect()
	if err == nil {
		t.Fatal("expected error when Reconnect exhausts retries")
	}
	if !errors.Is(err, ErrBootstrapFailed) {
		t.Fatalf(
			"error = %q, want ErrBootstrapFailed",
			err.Error(),
		)
	}
	if !c.isOffline() {
		t.Fatal("node should be offline after failed Reconnect")
	}
}

func TestDialBootstrapAddrNoIdentityReturnsError( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	c := newCarrierTestHarness(
		CarrierConfig{SelfCert: selfNI.Certs()[0]},
	)

	err := c.dialBootstrapAddr("peer:1")
	if err == nil {
		t.Fatal("expected error with nil NodeIdentity")
	}
	if !errors.Is(err, ErrNodeIdentityRequired) {
		t.Fatalf(
			"error = %q, want ErrNodeIdentityRequired",
			err.Error(),
		)
	}
}

func TestDialBootstrapAddrNoTransportReturnsError( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:     selfNI.Certs()[0],
		NodeIdentity: selfNI,
	})
	// transport is nil by default in test harness.

	err := c.dialBootstrapAddr("peer:1")
	if err == nil {
		t.Fatal("expected error with nil transport")
	}
	if !errors.Is(err, ErrTransportNotInitialized) {
		t.Fatalf(
			"error = %q, want ErrTransportNotInitialized",
			err.Error(),
		)
	}
}

// TestDialBootstrapAddrDuplicateConnectionDiscarded
// verifies that when a bootstrap dial completes auth
// but reveals a NodeID already connected, the new
// connection is closed and the existing one is kept.
// The duplicate check must happen after auth because
// NodeID is unknown before the handshake completes.
func TestDialBootstrapAddrDuplicateConnectionDiscarded( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	peerID := peerNI.Certs()[0].NodeID()

	existingConn := newScriptedConn(
		newTLSBindingsForCarrierTest(),
		newExportersForCarrierTest(),
	)
	t.Cleanup(func() { _ = existingConn.Close() })

	dialConn := makeBootstrapConn(t, peerNI)
	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = dialConn

	c := newBootstrapCarrier(
		t, peerNI, selfNI, tr, 0, time.Millisecond,
	)
	// Pre-register the peer as already connected.
	c.connections[peerID] = existingConn

	err := c.dialBootstrapAddr("peer:1")
	if err != nil {
		t.Fatalf("dialBootstrapAddr: %v", err)
	}
	// dialConn was closed because peer was already present.
	_, _, _, _, closeCalls := dialConn.counts()
	if closeCalls != 1 {
		t.Fatalf(
			"duplicate conn close calls = %d, want 1",
			closeCalls,
		)
	}
	// The pre-existing connection must be untouched.
	if c.connections[peerID] != existingConn {
		t.Fatal("existing connection must not be replaced")
	}
}

// countingDialTransport fails the first `failUntil`
// dials then returns the configured conn.
type countingDialTransport struct { // A
	failUntil int
	dialCount *atomic.Int32
	conn      *scriptedConn
}

func (t *countingDialTransport) Dial( // A
	_ *interfaces.Node,
) (interfaces.Connection, error) {
	n := int(t.dialCount.Add(1))
	if n <= t.failUntil {
		return nil, errors.New("transient failure")
	}
	return t.conn, nil
}

func (t *countingDialTransport) Accept() ( // A
	interfaces.Connection, error,
) {
	return nil, errors.New("not implemented")
}

func (t *countingDialTransport) Close() error { // A
	return nil
}

func (t *countingDialTransport) GetActiveConnections() []interfaces.Connection { // A
	return nil
}

// funcDialTransport is a transport whose Dial behaviour
// is supplied by a test-provided function.
type funcDialTransport struct { // A
	dialFn func(interfaces.Node) (interfaces.Connection, error)
}

func (t *funcDialTransport) Dial( // A
	node *interfaces.Node,
) (interfaces.Connection, error) {
	return t.dialFn(*node)
}

func (t *funcDialTransport) Accept() ( // A
	interfaces.Connection, error,
) {
	return nil, errors.New("not implemented")
}

func (t *funcDialTransport) Close() error { // A
	return nil
}

func (t *funcDialTransport) GetActiveConnections() []interfaces.Connection { // A
	return nil
}

// TestBootstrapTriggeredByEnsureBackgroundLoops verifies
// that the bootstrap loop is started as part of the
// normal background loop initialisation, not just when
// called directly. This guards against a refactor that
// removes the go c.runBootstrapLoop(ctx) call from
// ensureBackgroundLoops.
func TestBootstrapTriggeredByEnsureBackgroundLoops( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	conn := makeBootstrapConn(t, peerNI)
	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn
	c := newBootstrapCarrier(
		t, peerNI, selfNI, tr, 0, time.Millisecond,
	)
	c.config.BootstrapAddresses = []string{"peer:1"}

	// Trigger via the same path production uses.
	c.ensureBackgroundLoops()

	waitForCarrierCondition(t, func() bool {
		return c.IsConnected(peerNI.Certs()[0].NodeID())
	}, "bootstrap peer was not connected via ensureBackgroundLoops")

	if c.isOffline() {
		t.Fatal(
			"node should be online after background bootstrap",
		)
	}
}

// TestBootstrapMaxRetriesZeroFallsBackToDefault verifies
// that setting BootstrapMaxRetries to 0 uses the default
// constant (4), resulting in 5 total attempts.
func TestBootstrapMaxRetriesZeroFallsBackToDefault( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	tr := newBootstrapDialTransport()
	tr.errByAddr["peer:1"] = errors.New("refused")
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		BootstrapRetryInterval: 1 * time.Millisecond,
		BootstrapMaxRetries:    0,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	if !c.isOffline() {
		t.Fatal("node should be offline after exhaustion")
	}
	wantAttempts := 1 + defaultBootstrapMaxRetries
	if tr.attemptCount() != wantAttempts {
		t.Fatalf(
			"dial attempts = %d, want %d (1 + default %d)",
			tr.attemptCount(),
			wantAttempts,
			defaultBootstrapMaxRetries,
		)
	}
}

// TestBootstrapExhaustedConnectionsClosed verifies that
// connections returned by the transport during failed
// bootstrap attempts are properly closed.
func TestBootstrapExhaustedConnectionsClosed( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	peerNI, _ := newNodeIdentityForCarrierTest(t)

	var conns [3]*scriptedConn
	for i := range conns {
		c := newScriptedConn(
			newTLSBindingsForCarrierTest(),
			newExportersForCarrierTest(),
		)
		c.openStream = newTestStream(nil)
		c.acceptStreams = []interfaces.Stream{
			newHandshakeStreamForCarrierTest(t, peerNI, c),
		}
		conns[i] = c
		t.Cleanup(func() { _ = c.Close() })
	}

	var dialIdx atomic.Int32
	tr := &funcDialTransport{dialFn: func(
		node interfaces.Node,
	) (interfaces.Connection, error) {
		i := int(dialIdx.Add(1)) - 1
		if i >= len(conns) {
			return nil, errors.New("no more conns")
		}
		return conns[i], nil
	}}
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		Auth:                   &stubCarrierAuth{err: errors.New("denied")},
		BootstrapRetryInterval: 1 * time.Millisecond,
		BootstrapMaxRetries:    2,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	if !c.isOffline() {
		t.Fatal("node should be offline after exhaustion")
	}
	for i, conn := range conns {
		_, _, _, _, closeCalls := conn.counts()
		if closeCalls != 1 {
			t.Fatalf(
				"conn[%d] close calls = %d, want 1",
				i, closeCalls,
			)
		}
	}
}

// TestBootstrapDialAndAuthFailsClosesConn verifies that
// when dialAndAuth (outbound auth) fails, the connection
// is closed and not registered.
func TestBootstrapDialAndAuthFailsClosesConn( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)

	conn := newScriptedConn(
		newTLSBindingsForCarrierTest(),
		newExportersForCarrierTest(),
	)
	t.Cleanup(func() { _ = conn.Close() })
	conn.openStream = nil

	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:     selfNI.Certs()[0],
		NodeIdentity: selfNI,
		Auth: &stubCarrierAuth{
			authCtx: auth.AuthContext{
				NodeID:         peerNI.Certs()[0].NodeID(),
				EffectiveScope: auth.ScopeAdmin,
			},
		},
		BootstrapRetryInterval: 1 * time.Millisecond,
		BootstrapMaxRetries:    0,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	err := c.dialBootstrapAddr("peer:1")
	if err == nil {
		t.Fatal("expected error when dialAndAuth fails")
	}
	if !errors.Is(err, ErrBootstrapDialAuthFailed) {
		t.Fatalf(
			"error = %q, want ErrBootstrapDialAuthFailed",
			err.Error(),
		)
	}
	_, _, _, _, closeCalls := conn.counts()
	if closeCalls != 1 {
		t.Fatalf(
			"conn close calls = %d, want 1",
			closeCalls,
		)
	}
	if c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("peer must not be registered on auth failure")
	}
}

// TestBootstrapRegisterPeerFailsClosesConn verifies that
// when registerPeer fails (e.g. zero NodeID from auth),
// the connection is closed and not added to the map.
func TestBootstrapRegisterPeerFailsClosesConn( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)

	conn := newScriptedConn(
		newTLSBindingsForCarrierTest(),
		newExportersForCarrierTest(),
	)
	t.Cleanup(func() { _ = conn.Close() })
	conn.openStream = newTestStream(nil)
	conn.acceptStreams = []interfaces.Stream{
		newHandshakeStreamForCarrierTest(t, peerNI, conn),
	}

	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:     selfNI.Certs()[0],
		NodeIdentity: selfNI,
		Auth: &stubCarrierAuth{
			authCtx: auth.AuthContext{
				NodeID:         keys.NodeID{},
				EffectiveScope: auth.ScopeAdmin,
			},
		},
		BootstrapRetryInterval: 1 * time.Millisecond,
		BootstrapMaxRetries:    0,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	err := c.dialBootstrapAddr("peer:1")
	if err == nil {
		t.Fatal("expected error when registerPeer fails")
	}
	if !errors.Is(err, ErrBootstrapRegisterFailed) {
		t.Fatalf(
			"error = %q, want ErrBootstrapRegisterFailed",
			err.Error(),
		)
	}
	_, _, _, _, closeCalls := conn.counts()
	if closeCalls != 1 {
		t.Fatalf(
			"conn close calls = %d, want 1",
			closeCalls,
		)
	}
}

// TestBootstrapAwaitPeerAuthFailsClosesConn verifies that
// when awaitPeerAuth fails (e.g. nil Auth), the
// connection is closed and the peer is not registered.
func TestBootstrapAwaitPeerAuthFailsClosesConn( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)

	conn := newScriptedConn(
		newTLSBindingsForCarrierTest(),
		newExportersForCarrierTest(),
	)
	t.Cleanup(func() { _ = conn.Close() })
	conn.openStream = newTestStream(nil)
	conn.acceptStreams = []interfaces.Stream{
		newHandshakeStreamForCarrierTest(t, peerNI, conn),
	}

	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		Auth:                   nil,
		BootstrapRetryInterval: 1 * time.Millisecond,
		BootstrapMaxRetries:    0,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	err := c.dialBootstrapAddr("peer:1")
	if err == nil {
		t.Fatal("expected error when awaitPeerAuth fails")
	}
	if !errors.Is(err, ErrBootstrapVerifyFailed) {
		t.Fatalf(
			"error = %q, want ErrBootstrapVerifyFailed",
			err.Error(),
		)
	}
	_, _, _, _, closeCalls := conn.counts()
	if closeCalls != 1 {
		t.Fatalf(
			"conn close calls = %d, want 1",
			closeCalls,
		)
	}
	if c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("peer must not be registered on verify failure")
	}
}

// TestReconnectOnAlreadyOnlineNode verifies that
// Reconnect can be called when the node is already
// online, re-attempting bootstrap and succeeding.
func TestReconnectOnAlreadyOnlineNode( // A
	t *testing.T,
) {
	t.Parallel()
	peerNI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	conn := makeBootstrapConn(t, peerNI)
	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn
	c := newBootstrapCarrier(
		t, peerNI, selfNI, tr, 5, 10*time.Millisecond,
	)
	c.config.BootstrapAddresses = []string{"peer:1"}

	err := c.Reconnect()
	if err != nil {
		t.Fatalf("Reconnect on online node: %v", err)
	}
	if c.isOffline() {
		t.Fatal("node should stay online after Reconnect")
	}
	if !c.IsConnected(peerNI.Certs()[0].NodeID()) {
		t.Fatal("expected peer connected after Reconnect")
	}
}

// TestBootstrapMultiplePeersAllRegistered verifies that
// when multiple bootstrap addresses each reach a
// different peer, all peers are registered.
func TestBootstrapMultiplePeersAllRegistered( // A
	t *testing.T,
) {
	t.Parallel()
	peer1NI, _ := newNodeIdentityForCarrierTest(t)
	peer2NI, _ := newNodeIdentityForCarrierTest(t)
	selfNI, _ := newNodeIdentityForCarrierTest(t)

	conn1 := makeBootstrapConn(t, peer1NI)
	conn2 := makeBootstrapConn(t, peer2NI)
	tr := newBootstrapDialTransport()
	tr.connByAddr["peer:1"] = conn1
	tr.connByAddr["peer:2"] = conn2

	var callIdx atomic.Int32
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:     selfNI.Certs()[0],
		NodeIdentity: selfNI,
		Auth: &multiPeerStubAuth{
			peers: map[int]auth.AuthContext{
				0: {
					NodeID:         peer1NI.Certs()[0].NodeID(),
					EffectiveScope: auth.ScopeAdmin,
				},
				1: {
					NodeID:         peer2NI.Certs()[0].NodeID(),
					EffectiveScope: auth.ScopeAdmin,
				},
			},
			callIdx: &callIdx,
		},
		BootstrapRetryInterval: 10 * time.Millisecond,
		BootstrapMaxRetries:    0,
		BootstrapAddresses:     []string{"peer:1", "peer:2"},
	})
	c.transport = tr

	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	if c.isOffline() {
		t.Fatal("node should be online with two peers")
	}
	if !c.IsConnected(peer1NI.Certs()[0].NodeID()) {
		t.Fatal("peer1 should be connected")
	}
	if !c.IsConnected(peer2NI.Certs()[0].NodeID()) {
		t.Fatal("peer2 should be connected")
	}
	if tr.attemptCount() != 2 {
		t.Fatalf(
			"dial attempts = %d, want 2",
			tr.attemptCount(),
		)
	}
}

// multiPeerStubAuth is a CarrierAuth that returns different
// auth contexts on each call, allowing tests with multiple
// distinct peers.
type multiPeerStubAuth struct { // A
	peers   map[int]auth.AuthContext
	callIdx *atomic.Int32
}

func (m *multiPeerStubAuth) VerifyPeerCert( // A
	_ *interfaces.PeerHandshake,
) (interfaces.AuthContext, error) {
	idx := int(m.callIdx.Add(1)) - 1
	ctx, ok := m.peers[idx]
	if !ok {
		return interfaces.AuthContext{}, errors.New("unknown peer")
	}
	return interfaces.AuthContext{NodeID: ctx.NodeID, EffectiveScope: ctx.EffectiveScope}, nil
}

func (m *multiPeerStubAuth) AddAdminPubKey([]byte) error { // A
	return nil
}

func (m *multiPeerStubAuth) AddUserPubKey( // A
	[]byte, []byte, string,
) error {
	return nil
}

func (m *multiPeerStubAuth) RemoveAdminPubKey(string) error { // A
	return nil
}

func (m *multiPeerStubAuth) RemoveUserPubKey(string) error { // A
	return nil
}

func (m *multiPeerStubAuth) RevokeAdminCA(string) error { // A
	return nil
}

func (m *multiPeerStubAuth) RevokeUserCA(string) error { // A
	return nil
}

func (m *multiPeerStubAuth) RevokeNode(keys.NodeID) error { // A
	return nil
}

func (m *multiPeerStubAuth) SetRevocationHook( // A
	auth.RevocationHook,
) {
}

// TestBootstrapRetryIntervalRespected verifies that the
// retry interval is actually waited between attempts by
// checking elapsed wall time.
func TestBootstrapRetryIntervalRespected( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	tr := newBootstrapDialTransport()
	tr.errByAddr["peer:1"] = errors.New("refused")
	interval := 50 * time.Millisecond
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		BootstrapRetryInterval: interval,
		BootstrapMaxRetries:    2,
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	start := time.Now()
	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}
	elapsed := time.Since(start)
	wantMin := time.Duration(2) * interval
	if elapsed < wantMin {
		t.Fatalf(
			"elapsed = %v, want at least %v (2 retries × %v interval)",
			elapsed,
			wantMin,
			interval,
		)
	}
}

// TestBootstrapRetryIntervalZeroFallsBackToDefault verifies
// that when BootstrapRetryInterval is 0 the bootstrapRetryInterval()
// fallback uses defaultBootstrapRetryInterval. Use BootstrapMaxRetries:1,
// all dials fail, measure elapsed time and assert elapsed >= defaultBootstrapRetryInterval.
func TestBootstrapRetryIntervalZeroFallsBackToDefault( // A
	t *testing.T,
) {
	t.Parallel()
	selfNI, _ := newNodeIdentityForCarrierTest(t)
	tr := newBootstrapDialTransport()
	tr.errByAddr["peer:1"] = errors.New("refused")
	c := newCarrierTestHarness(CarrierConfig{
		SelfCert:               selfNI.Certs()[0],
		NodeIdentity:           selfNI,
		BootstrapRetryInterval: 0, // Zero should fall back to default
		BootstrapMaxRetries:    1, // 1 retry (2 total attempts)
		BootstrapAddresses:     []string{"peer:1"},
	})
	c.transport = tr

	start := time.Now()
	done := make(chan struct{})
	go func() {
		c.runBootstrapLoop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runBootstrapLoop did not return")
	}

	elapsed := time.Since(start)
	// Should wait at least the default interval for the 1 retry
	if elapsed < defaultBootstrapRetryInterval {
		t.Fatalf(
			"elapsed = %v, want at least %v (default interval)",
			elapsed,
			defaultBootstrapRetryInterval,
		)
	}
}

// TestBootstrapAttemptCountProperty is a property-based
// test verifying that for any positive maxRetries, total
// dial attempts always equal maxRetries + 1 when all
// dials fail.
func TestBootstrapAttemptCountProperty( // A
	t *testing.T,
) {
	rapid.Check(t, func(rt *rapid.T) {
		maxRetries := rapid.IntRange(1, 10).Draw(
			rt, "maxRetries",
		)
		selfNI, _ := newNodeIdentityForCarrierTest(t)
		tr := newBootstrapDialTransport()
		tr.errByAddr["peer:1"] = errors.New("refused")
		c := newCarrierTestHarness(CarrierConfig{
			SelfCert:               selfNI.Certs()[0],
			NodeIdentity:           selfNI,
			BootstrapRetryInterval: time.Millisecond,
			BootstrapMaxRetries:    maxRetries,
			BootstrapAddresses:     []string{"peer:1"},
		})
		c.transport = tr

		done := make(chan struct{})
		go func() {
			c.runBootstrapLoop(context.Background())
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("runBootstrapLoop did not return")
		}

		want := maxRetries + 1
		got := tr.attemptCount()
		if got != want {
			rt.Fatalf(
				"maxRetries=%d: attempts=%d, want %d",
				maxRetries, got, want,
			)
		}
	})
}

// Compile-time interface checks.
var (
	_ interfaces.QuicTransport = (*countingDialTransport)(nil)
	_ interfaces.QuicTransport = (*bootstrapDialTransport)(nil)
	_ interfaces.QuicTransport = (*funcDialTransport)(nil)
	_ interfaces.CarrierAuth   = (*multiPeerStubAuth)(nil)
)
