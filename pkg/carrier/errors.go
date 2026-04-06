package carrier

import "errors"

// Sentinel errors for carrier bootstrap and transport
// initialization failures.
var ( // A
	// ErrNoBootstrapAddresses is returned when the
	// configuration contains no bootstrap addresses
	// and no peers are available for initial cluster
	// discovery.
	ErrNoBootstrapAddresses = errors.New(
		"no bootstrap addresses configured",
	)

	// ErrBootstrapFailed is returned when the bootstrap
	// process fails after exhausting all retries and
	// available bootstrap addresses.
	ErrBootstrapFailed = errors.New(
		"bootstrap failed after maximum retries",
	)

	// ErrNodeIdentityRequired is returned when a
	// node operation requires identity configuration
	// but no SelfCert is provided in the configuration.
	ErrNodeIdentityRequired = errors.New(
		"node identity certificate required",
	)

	// ErrTransportNotInitialized is returned when
	// transport operations are attempted before the
	// carrier transport layer has been properly
	// initialized and started.
	ErrTransportNotInitialized = errors.New(
		"transport not initialized",
	)

	// ErrBootstrapDialAuthFailed is returned when the
	// outbound auth handshake fails during bootstrap
	// dialing of a peer address.
	ErrBootstrapDialAuthFailed = errors.New(
		"bootstrap dial auth handshake failed",
	)

	// ErrBootstrapVerifyFailed is returned when peer
	// verification (inbound auth) fails during bootstrap
	// dialing of a peer address.
	ErrBootstrapVerifyFailed = errors.New(
		"bootstrap peer verification failed",
	)

	// ErrBootstrapRegisterFailed is returned when peer
	// registration fails during bootstrap dialing,
	// e.g. due to a zero NodeID from auth.
	ErrBootstrapRegisterFailed = errors.New(
		"bootstrap peer registration failed",
	)
)
