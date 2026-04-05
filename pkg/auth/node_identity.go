package auth

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// NodeIdentity bundles all materials a node needs to
// establish authenticated connections with cluster
// peers. It manages the persistent ML-DSA-87 key
// pair, the ephemeral session identity (Phase 2),
// and the NodeCert bundle signed by CAs (Phase 1).
//
// Usage:
//
//	ni, _ := auth.NewNodeIdentity(key, certs, sigs)
//	// Use ni.TLSClientConfig() / TLSServerConfig()
//	// for QUIC transport, then call SignDelegation
//	// with ni.ExporterFn(conn) after TLS completes.
type NodeIdentity struct { // A
	mu      sync.RWMutex
	key     *keys.AsyncCrypt
	session *SessionIdentity
	certs   []NodeCertLike
	caSigs  [][]byte
	nodeID  keys.NodeID
}

// NewNodeIdentity creates a NodeIdentity from the
// node's persistent key, its CA-signed cert bundle,
// and the corresponding CA signatures. A fresh
// SessionIdentity is generated automatically.
func NewNodeIdentity( // A
	key *keys.AsyncCrypt,
	certs []NodeCertLike,
	caSigs [][]byte,
) (*NodeIdentity, error) {
	if len(certs) == 0 {
		return nil, fmt.Errorf(
			"at least one NodeCert is required",
		)
	}
	if len(caSigs) != len(certs) {
		return nil, fmt.Errorf(
			"cert/sig count mismatch: %d vs %d",
			len(certs), len(caSigs),
		)
	}
	nodeID := certs[0].NodeID()
	session, err := NewSessionIdentity(
		time.Duration(MaxDelegationTTL) * time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create session identity: %w", err,
		)
	}
	return &NodeIdentity{
		key:     key,
		session: session,
		certs:   certs,
		caSigs:  caSigs,
		nodeID:  nodeID,
	}, nil
}

// NodeID returns this node's identity.
func (ni *NodeIdentity) NodeID() keys.NodeID { // A
	return ni.nodeID
}

// Key returns the underlying AsyncCrypt for signing.
func (ni *NodeIdentity) Key() *keys.AsyncCrypt { // A
	return ni.key
}

// Certs returns the NodeCert bundle.
func (ni *NodeIdentity) Certs() []NodeCertLike { // A
	ni.mu.RLock()
	defer ni.mu.RUnlock()
	out := make([]NodeCertLike, len(ni.certs))
	copy(out, ni.certs)
	return out
}

// CASigs returns the CA signatures corresponding to
// each cert in the bundle.
func (ni *NodeIdentity) CASigs() [][]byte { // A
	ni.mu.RLock()
	defer ni.mu.RUnlock()
	out := make([][]byte, len(ni.caSigs))
	copy(out, ni.caSigs)
	return out
}

// Session returns the current SessionIdentity.
func (ni *NodeIdentity) Session() *SessionIdentity { // A
	ni.mu.RLock()
	defer ni.mu.RUnlock()
	return ni.session
}

// RotateSession generates a new ephemeral session
// identity. Existing connections are unaffected;
// new connections will use the new session.
func (ni *NodeIdentity) RotateSession() error { // A
	s, err := NewSessionIdentity(
		time.Duration(MaxDelegationTTL) * time.Second,
	)
	if err != nil {
		return err
	}
	ni.mu.Lock()
	ni.session = s
	ni.mu.Unlock()
	return nil
}

// TLSClientConfig returns a *tls.Config for the
// prover (dialer) side. It uses the ephemeral
// session cert and enforces PQ-hybrid key exchange
// via X25519MLKEM768. X.509 verification is skipped
// because authentication is via DelegationProof,
// not the TLS PKI chain.
func (ni *NodeIdentity) TLSClientConfig() *tls.Config { // A
	ni.mu.RLock()
	sess := ni.session
	ni.mu.RUnlock()
	return &tls.Config{
		Certificates: []tls.Certificate{
			sess.TLSCertificate,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519MLKEM768,
		},
		//nolint:gosec // Auth is via DelegationProof.
		InsecureSkipVerify: true,
		NextProtos:         []string{"ouroboros-v1"},
		MinVersion:         tls.VersionTLS13,
	}
}

// TLSServerConfig returns a *tls.Config for the
// verifier (listener) side. It requests the peer's
// X.509 cert without chain validation and enforces
// PQ-hybrid key exchange.
func (ni *NodeIdentity) TLSServerConfig() *tls.Config { // A
	ni.mu.RLock()
	sess := ni.session
	ni.mu.RUnlock()
	return &tls.Config{
		Certificates: []tls.Certificate{
			sess.TLSCertificate,
		},
		ClientAuth: tls.RequireAnyClientCert,
		CurvePreferences: []tls.CurveID{
			tls.X25519MLKEM768,
		},
		NextProtos: []string{"ouroboros-v1"},
		MinVersion: tls.VersionTLS13,
	}
}
