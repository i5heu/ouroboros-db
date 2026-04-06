package interfaces

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// Carrier is the QUIC-based cluster transport with dual-mode
// delivery. It maintains persistent connections to all known
// cluster nodes and provides both reliable (QUIC streams) and
// unreliable (RFC 9221 datagrams Extension to QUIC) message delivery.
//
// # Transport
//
// All node-to-node communication uses QUIC (RFC 9000) with
// multiplexed streams. Reliable delivery uses standard QUIC
// streams (TCP-style ordering). Unreliable delivery uses RFC
// 9221 QUIC Datagrams (UDP-style, fire-and-forget).
//
// # Connections
//
// The Carrier maintains persistent connections to every known
// node in the cluster. When a node is removed via RemoveNode
// the connection is immediately torn down.
//
// # Bootstrap
//
// Initial shared material is minimal: trusted AdminCA public
// keys plus a list of bootstrap node addresses. Bootstrap
// addresses are connection seeds only—they are NOT trusted
// identities and grant NO special privileges. During startup
// the Carrier dials these addresses and authenticates each
// connection normally through CarrierAuth. No cluster-wide
// shared encryption state is assumed.
//
// # Node Sharing
//
// Periodic full sync via NodeSync ensures all nodes share all
// admin/user-signed nodes. Full NodeCert bundles and CA
// signatures are exchanged.
//
// # Trust
//
// Every node-to-node connection is independently authenticated
// and encrypted. Transport confidentiality and integrity are
// per-connection (authenticated encryption from QUIC/TLS),
// not cluster-wide. Authentication is delegated to CarrierAuth
// (pkg/auth). Dynamic CA addition/removal and node
// addition/removal are supported at runtime.
//
// # Security Notes
//
// Authentication is performed during connection setup over
// reliable QUIC streams. QUIC datagrams only piggyback on the
// already authenticated and encrypted TLS connection.
//
// This means datagrams do not bypass peer authentication, but
// they also inherit the lifecycle of the underlying connection.
// Long-lived connections therefore need explicit freshness and
// revocation handling above the initial handshake. Implementors
// should re-verify or tear down connections after delegation TTL
// expiry and on revocation events.
//
// Datagram delivery is best-effort and unordered. State-changing
// traffic should either stay on reliable streams or add its own
// replay, deduplication, ordering, and rate-limiting controls.
type Carrier interface { // A
	// GetNodes returns all currently known peer nodes
	// in the cluster registry.
	GetNodes() []PeerNode

	// GetNode returns the peer node identified by nodeID,
	// or an error if the node is not in the registry.
	GetNode(nodeID keys.NodeID) (PeerNode, error)

	// GetNodeConnection returns a NodeConnection wrapping
	// the peer and its active Connection. Returns an error
	// if the node is unknown. The returned NodeConnection
	// may have a nil Conn if the peer is not currently
	// connected.
	GetNodeConnection(
		nodeID keys.NodeID,
	) (NodeConnection, error)

	// BroadcastReliable sends a message to every known
	// node over reliable QUIC streams. It returns two
	// lists: nodes that acknowledged the send and nodes
	// where delivery failed. A non-nil err indicates a
	// systemic failure (e.g. empty registry), not per-
	// node failures.
	BroadcastReliable(
		message Message,
	) (success, failed []PeerNode, err error)

	// SendMessageToNodeReliable sends a message to a
	// single node over a reliable QUIC stream. Returns
	// an error if the node is unknown, not connected, or
	// the stream write fails.
	SendMessageToNodeReliable(
		nodeID keys.NodeID,
		message Message,
	) error

	// BroadcastUnreliable sends a message to every known
	// node using RFC 9221 QUIC Datagrams (best-effort,
	// no acknowledgment). Returns the list of nodes that
	// were attempted. Individual datagram loss is not
	// reported.
	BroadcastUnreliable(
		message Message,
	) (attempted []PeerNode)

	// SendMessageToNodeUnreliable sends a message to a
	// single node over a QUIC Datagram. Returns an error
	// only if the node is unknown or not connected.
	SendMessageToNodeUnreliable(
		nodeID keys.NodeID,
		message Message,
	) error

	// Broadcast sends a message to every known node using
	// the default (reliable) delivery mode. Returns the
	// nodes that succeeded and a non-nil error on systemic
	// failure.
	Broadcast(
		message Message,
	) (success []PeerNode, err error)

	// SendMessageToNode sends a message to a single node
	// using the default (reliable) delivery mode. Returns
	// an error if the node is unknown, disconnected, or
	// delivery fails.
	SendMessageToNode(
		nodeID keys.NodeID,
		message Message,
	) error

	// OpenPeerChannel dials a peer and authenticates the
	// transport connection using the provided NodeCert. The
	// peer is admitted to the registry only after CarrierAuth
	// verification succeeds.
	OpenPeerChannel(
		clusterNode PeerNode,
		cert NodeCert,
	) error

	// LeaveCluster gracefully disconnects from the given
	// peer. The node may remain in the registry for a
	// future reconnection attempt.
	LeaveCluster(clusterNode PeerNode) error

	// RemoveNode removes a node from the cluster registry
	// and immediately tears down its active connection.
	// Future sends to this node will fail until it is
	// re-added.
	RemoveNode(nodeID keys.NodeID) error

	// IsConnected reports whether an active, authenticated
	// transport connection exists to the given node.
	IsConnected(nodeID keys.NodeID) bool

	// StartListener starts accepting inbound node-to-node
	// QUIC connections on the configured ListenAddress.
	// It blocks until the context is cancelled or a fatal
	// accept error occurs. Returns an error immediately if
	// ListenAddress is empty or the transport is not
	// initialized.
	StartListener(ctx context.Context) error
}
