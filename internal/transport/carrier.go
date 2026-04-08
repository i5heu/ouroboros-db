package transport

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// CarrierConfig holds the configuration needed to
// construct a Carrier. It is consumed by New().
//
// # Bootstrap Addresses
//
// BootstrapAddresses is a list of "<host>:<port>"
// endpoints used only for initial cluster discovery.
// These addresses are NOT trusted identities and grant
// NO special privileges - they are connection seeds.
// Each dialed peer is authenticated normally through
// CarrierAuth before being admitted to the registry.
//
// # Self Identity
//
// SelfCert is this node's identity certificate. The
// NodeID is derived automatically from the cert:
//
//	NodeID = SHA-256(SelfCert.NodePubKey())
//
// Callers should retrieve it via SelfCert.NodeID()
// rather than computing it independently.
//
// # Logger
//
// If Logger is nil, the implementation should create a
// default structured logger writing to stderr.
type CarrierConfig struct { // A
	// BootstrapAddresses is a list of "<host>:<port>"
	// endpoints for initial cluster discovery.
	BootstrapAddresses []string

	// SelfCert is this node's identity certificate.
	// NodeID is derived from it via SelfCert.NodeID().
	SelfCert interfaces.NodeCert

	// ListenAddress is the local "<host>:<port>" address
	// the QUIC transport binds to. If empty the Carrier
	// will not accept inbound connections (dial-only).
	ListenAddress string

	// Logger is an optional structured logger. If nil a
	// default stderr logger is used.
	Logger *slog.Logger

	// Auth is the CarrierAuth used to verify inbound
	// peers. If nil, all inbound connections are
	// rejected.
	Auth interfaces.CarrierAuth

	// NodeIdentity holds the node's persistent key,
	// ephemeral session identity, cert bundle, and
	// CA signatures. Required for dialing peers
	// (prover side). If nil, the carrier cannot
	// initiate outbound authenticated connections.
	NodeIdentity *certpkg.NodeIdentity

	// NodeRole controls whether this node behaves as a
	// server or client peer for heartbeat scheduling.
	NodeRole interfaces.NodeRole

	// HeartbeatInterval controls how often the carrier
	// emits heartbeat messages to eligible peers.
	HeartbeatInterval time.Duration

	// HeartbeatTimeout is the maximum age of the last
	// inbound heartbeat or other message before a peer
	// is considered unreachable.
	HeartbeatTimeout time.Duration

	// ReconnectInterval controls how often unreachable
	// peers are retried across all known addresses.
	ReconnectInterval time.Duration

	// BootstrapRetryInterval controls how long to
	// wait between bootstrap connection attempts.
	BootstrapRetryInterval time.Duration

	// BootstrapMaxRetries is the maximum number of
	// bootstrap retry rounds before going offline.
	BootstrapMaxRetries int

	// BroadcastMaxRetries controls how many times a
	// failed peer send is retried during broadcast.
	BroadcastMaxRetries int

	// BroadcastRetryInterval is the delay between
	// broadcast send retries for a failed peer.
	BroadcastRetryInterval time.Duration
}

// RuntimeCarrier extends the public carrier contract
// with runtime wiring helpers needed by commands.
type RuntimeCarrier interface { // A
	interfaces.Carrier
	SetController(controller interfaces.ClusterController)
	ListenAddress() string
	// Reconnect resets the offline state and triggers
	// a new round of bootstrap connection attempts.
	Reconnect() error
}

const logKeyMessageType = "messageType" // A

// Compile-time interface compliance check.
var (
	_ interfaces.Carrier = (*carrierImpl)(nil)
	_ RuntimeCarrier     = (*carrierImpl)(nil)
)

// carrierImpl implements interfaces.Carrier. It owns the
// QUIC transport, node registry, and connection state.
type carrierImpl struct { // A
	mu          sync.RWMutex
	logger      *slog.Logger
	config      CarrierConfig
	registry    *nodeRegistry
	transport   interfaces.QuicTransport
	connections map[keys.NodeID]interfaces.Connection
	controller  interfaces.ClusterController
	peerScopes  map[keys.NodeID]auth.TrustScope

	backgroundOnce   sync.Once
	backgroundCtx    context.Context
	backgroundCancel context.CancelFunc

	// nodeStatus is 0 (online) or 1 (offline).
	// Transitions atomically; use setOffline/setOnline.
	nodeStatus int32
}

const ( // A
	defaultHeartbeatInterval = 5 * time.Second
	defaultHeartbeatTimeout  = 15 * time.Second
	defaultReconnectInterval = 5 * time.Minute
	// defaultBootstrapMaxRetries is the number of retry
	// rounds after the initial attempt, giving 5 total
	// connection attempts before the node goes offline.
	defaultBootstrapRetryInterval = 5 * time.Second
	defaultBootstrapMaxRetries    = 4
	// defaultBroadcastMaxRetries is how many times a
	// failed send is retried per peer before giving up.
	defaultBroadcastMaxRetries = 3
	// defaultBroadcastRetryInterval is the backoff
	// between broadcast send retries.
	defaultBroadcastRetryInterval = 250 * time.Millisecond
)

const ( // A
	nodeStatusOnline  int32 = 0
	nodeStatusOffline int32 = 1
)

// New creates a new Carrier from the given configuration.
func New( //nolint:cyclop // A: constructor validates multiple required config fields
	conf *CarrierConfig,
) (*carrierImpl, error) {
	if conf.SelfCert == nil {
		return nil, fmt.Errorf("SelfCert must not be nil")
	}
	if conf.HeartbeatInterval <= 0 {
		conf.HeartbeatInterval = defaultHeartbeatInterval
	}
	if conf.HeartbeatTimeout <= 0 {
		conf.HeartbeatTimeout = defaultHeartbeatTimeout
	}
	if conf.HeartbeatTimeout < conf.HeartbeatInterval {
		conf.HeartbeatTimeout = conf.HeartbeatInterval
	}
	if conf.ReconnectInterval <= 0 {
		conf.ReconnectInterval = defaultReconnectInterval
	}
	if conf.BootstrapRetryInterval <= 0 {
		conf.BootstrapRetryInterval = defaultBootstrapRetryInterval
	}
	if conf.BootstrapMaxRetries <= 0 {
		conf.BootstrapMaxRetries = defaultBootstrapMaxRetries
	}
	if conf.BroadcastMaxRetries <= 0 {
		conf.BroadcastMaxRetries = defaultBroadcastMaxRetries
	}
	if conf.BroadcastRetryInterval <= 0 {
		conf.BroadcastRetryInterval = defaultBroadcastRetryInterval
	}
	if conf.Logger == nil {
		h := slog.NewTextHandler(
			os.Stderr,
			&slog.HandlerOptions{Level: slog.LevelInfo},
		)
		conf.Logger = slog.New(h)
	}
	c := &carrierImpl{
		logger:      conf.Logger,
		config:      *conf,
		registry:    newNodeRegistry(),
		connections: make(map[keys.NodeID]interfaces.Connection),
		peerScopes:  make(map[keys.NodeID]auth.TrustScope),
	}
	if conf.NodeIdentity != nil {
		qt, err := newQuicTransport(
			conf.ListenAddress,
			conf.NodeIdentity,
		)
		if err != nil {
			return nil, fmt.Errorf("init transport: %w", err)
		}
		c.transport = qt
	}
	return c, nil
}

// ListenAddress returns the bound local QUIC address.
func (c *carrierImpl) ListenAddress() string { // A
	c.mu.RLock()
	tp := c.transport
	c.mu.RUnlock()
	qt, ok := tp.(*quicTransport)
	if !ok {
		return c.config.ListenAddress
	}
	return qt.listenAddress()
}

func (c *carrierImpl) ensureBackgroundLoops() { // A
	c.backgroundOnce.Do(func() {
		ctx, cancel := context.WithCancel( //nolint:gosec // G118: cancel stored in c.backgroundCancel
			context.Background(),
		)
		c.mu.Lock()
		c.backgroundCtx = ctx
		c.backgroundCancel = cancel
		c.mu.Unlock()
		go c.runHeartbeatLoop(ctx)
		go c.runReconnectLoop(ctx)
		go c.runBootstrapLoop(ctx)
	})
}

// isOffline reports whether the node has entered
// offline mode after bootstrap exhaustion.
func (c *carrierImpl) isOffline() bool { // A
	return atomic.LoadInt32(&c.nodeStatus) ==
		nodeStatusOffline
}

// setOffline transitions the node to offline mode.
func (c *carrierImpl) setOffline() { // A
	atomic.StoreInt32(&c.nodeStatus, nodeStatusOffline)
}

// setOnline transitions the node back to online mode.
func (c *carrierImpl) setOnline() { // A
	atomic.StoreInt32(&c.nodeStatus, nodeStatusOnline)
}

// Reconnect resets the offline state and triggers a
// new round of bootstrap connection attempts. It
// returns nil once at least one bootstrap peer has
// been reached, or an error if all retries are
// exhausted again.
func (c *carrierImpl) Reconnect() error { // A
	c.setOnline()
	addrs := c.config.BootstrapAddresses
	if len(addrs) == 0 {
		return ErrNoBootstrapAddresses
	}
	c.mu.RLock()
	ctx := c.backgroundCtx
	c.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if c.attemptBootstrap(ctx, addrs) {
		return nil
	}
	return ErrBootstrapFailed
}

func compactAddresses(addresses []string) []string { // A
	out := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		out = append(out, address)
	}
	return out
}
