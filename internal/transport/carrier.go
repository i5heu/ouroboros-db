package transport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

const (
	tlsExporterLabel      = "EXPORTER-Ouroboros-Auth-V1"
	tlsExporterBindingLen = 32
)

const (
	keyPeerNodeId = "peerNodeId"
	keyFailReason = "failReason"
)

// MessageReceiver is the callback invoked when an
// inbound message arrives from an authenticated peer.
// The Carrier writes the returned Response back on
// the same stream.
type MessageReceiver func( // A
	msg interfaces.Message,
	peer keys.NodeID,
	scope auth.TrustScope,
) (interfaces.Response, error)

// CarrierConfig holds configuration for creating a
// Carrier instance.
type CarrierConfig struct { // A
	ListenAddr       string
	BootstrapConfig  *BootstrapConfig
	CarrierAuth      *auth.CarrierAuth
	LocalNodeID      keys.NodeID
	LocalCert        *auth.NodeCert
	LocalCASignature []byte
	LocalKeys        *keys.AsyncCrypt
	// LOGGER: Using slog directly because Carrier
	// is a dependency of ClusterLog. Using
	// pkg/clusterlog here would create a
	// subscription loop.
	Logger *slog.Logger
}

// Carrier is the concrete QUIC-based cluster
// transport that implements interfaces.Carrier.
type Carrier struct { // A
	transport        *quicTransportImpl
	registry         NodeRegistry
	nodeSync         NodeSync
	bootstrapper     *BootStrapper
	auth             *auth.CarrierAuth
	localCert        *auth.NodeCert
	localCASignature []byte
	localKeys        *keys.AsyncCrypt
	receiver         atomic.Pointer[MessageReceiver]
	// LOGGER: Using slog directly because Carrier
	// is a dependency of ClusterLog. Using
	// pkg/clusterlog here would create a
	// subscription loop.
	logger *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCarrier creates and starts a new Carrier.
func NewCarrier( // A
	cfg CarrierConfig,
) (*Carrier, error) {
	if cfg.CarrierAuth == nil {
		return nil, fmt.Errorf(
			"carrier auth must not be nil",
		)
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf(
			"logger must not be nil",
		)
	}

	qt, err := NewQuicTransport(
		cfg.ListenAddr,
		cfg.CarrierAuth,
		cfg.LocalNodeID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create transport: %w", err,
		)
	}

	impl := qt.(*quicTransportImpl)
	registry := NewNodeRegistry()
	ns := NewNodeSync(
		registry,
		qt,
		cfg.CarrierAuth,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	c := &Carrier{
		transport:        impl,
		registry:         registry,
		nodeSync:         ns,
		bootstrapper:     nil,
		auth:             cfg.CarrierAuth,
		localCert:        cfg.LocalCert,
		localCASignature: cfg.LocalCASignature,
		localKeys:        cfg.LocalKeys,
		logger:           cfg.Logger,
		ctx:              ctx,
		cancel:           cancel,
	}

	bs, bsErr := NewBootStrapper(
		cfg.BootstrapConfig,
		qt,
		registry,
		c.authenticateBootstrapConn,
	)
	if bsErr != nil {
		_ = qt.Close()
		cancel()
		return nil, fmt.Errorf(
			"create bootstrapper: %w", bsErr,
		)
	}
	c.bootstrapper = bs

	c.wg.Add(1)
	go c.acceptLoop()

	return c, nil
}

// Close gracefully shuts down the Carrier.
func (c *Carrier) Close() error { // A
	c.cancel()
	c.nodeSync.StopSync()
	err := c.transport.Close()
	c.wg.Wait()
	return err
}

// ListenAddr returns the address the Carrier is
// listening on.
func (c *Carrier) ListenAddr() string { // A
	return c.transport.ListenAddr()
}

// Registry returns the node registry for external
// inspection.
func (c *Carrier) Registry() NodeRegistry { // A
	return c.registry
}

func (c *Carrier) GetNodes() []interfaces.PeerNode { // A
	return c.registry.GetAllNodes()
}

func (c *Carrier) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.PeerNode, error) {
	info, err := c.registry.GetNode(nodeID)
	if err != nil {
		return interfaces.PeerNode{}, err
	}
	return info.Peer, nil
}

func (c *Carrier) BroadcastReliable( // A
	message interfaces.Message,
) (
	success []interfaces.PeerNode,
	failed []interfaces.PeerNode,
	err error,
) {
	nodes := c.registry.GetAllNodes()
	for _, peer := range nodes {
		sendErr := c.SendMessageToNodeReliable(
			peer.NodeID,
			message,
		)
		if sendErr != nil {
			failed = append(failed, peer)
		} else {
			success = append(success, peer)
		}
	}
	return success, failed, nil
}

func (c *Carrier) SendMessageToNodeReliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	conn, err := c.ensureConnection(nodeID)
	if err != nil {
		return err
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	return WriteMessage(stream, message)
}

func (c *Carrier) BroadcastUnreliable( // A
	message interfaces.Message,
) (attempted []interfaces.PeerNode) {
	data, err := SerializeMessage(message)
	if err != nil {
		return nil
	}

	nodes := c.registry.GetAllNodes()
	for _, peer := range nodes {
		conn := c.transport.GetConnection(
			peer.NodeID,
		)
		if conn == nil {
			continue
		}
		_ = conn.SendDatagram(data)
		attempted = append(attempted, peer)
	}
	return attempted
}

func (c *Carrier) SendMessageToNodeUnreliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	data, err := SerializeMessage(message)
	if err != nil {
		return fmt.Errorf(
			"serialize message: %w", err,
		)
	}

	conn := c.transport.GetConnection(nodeID)
	if conn == nil {
		return fmt.Errorf(
			"no connection to %s", nodeID,
		)
	}
	return conn.SendDatagram(data)
}

func (c *Carrier) Broadcast( // A
	message interfaces.Message,
) (success []interfaces.PeerNode, err error) {
	s, _, err := c.BroadcastReliable(message)
	return s, err
}

func (c *Carrier) SendMessageToNode( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	return c.SendMessageToNodeReliable(
		nodeID,
		message,
	)
}

func (c *Carrier) JoinCluster( // PAP
	clusterNode interfaces.PeerNode,
	cert *auth.NodeCert,
) error {
	conn, err := c.transport.Dial(clusterNode)
	if err != nil {
		return fmt.Errorf(
			"dial cluster node: %w", err,
		)
	}

	if err := c.sendAuthHandshake(conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"send auth handshake: %w", err,
		)
	}

	nodeID, scope, err := c.receiveAuthHandshake(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"receive peer auth handshake: %w", err,
		)
	}

	if !clusterNode.NodeID.IsZero() &&
		nodeID != clusterNode.NodeID {
		_ = conn.Close()
		return fmt.Errorf(
			"peer node ID mismatch: got %s, expected %s",
			nodeID,
			clusterNode.NodeID,
		)
	}

	peer := interfaces.PeerNode{
		NodeID:    nodeID,
		Addresses: clusterNode.Addresses,
	}
	err = c.registry.AddNode(peer, nil, scope)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf(
			"register cluster node: %w", err,
		)
	}

	return c.registry.UpdateConnectionStatus(
		nodeID,
		interfaces.ConnectionStatusConnected,
	)
}

func (c *Carrier) LeaveCluster( // A
	clusterNode interfaces.PeerNode,
) error {
	conn := c.transport.GetConnection(
		clusterNode.NodeID,
	)
	if conn != nil {
		_ = conn.Close()
	}
	c.transport.RemoveConnection(
		clusterNode.NodeID,
	)
	return c.registry.RemoveNode(
		clusterNode.NodeID,
	)
}

func (c *Carrier) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	conn := c.transport.GetConnection(nodeID)
	if conn != nil {
		_ = conn.Close()
	}
	c.transport.RemoveConnection(nodeID)
	return c.registry.RemoveNode(nodeID)
}

func (c *Carrier) IsConnected( // A
	nodeID keys.NodeID,
) bool {
	info, err := c.registry.GetNode(nodeID)
	if err != nil {
		return false
	}
	return info.ConnectionStatus ==
		interfaces.ConnectionStatusConnected
}

// SetMessageReceiver sets the callback invoked for
// each inbound message from authenticated peers.
// Must be called before connections arrive to avoid
// missing messages. Thread-safe.
func (c *Carrier) SetMessageReceiver( // A
	recv MessageReceiver,
) {
	c.receiver.Store(&recv)
}

func (c *Carrier) acceptLoop() { // A
	defer c.wg.Done()
	for {
		conn, err := c.transport.Accept()
		if err != nil {
			select {
			case <-c.ctx.Done():
				return
			default:
				continue
			}
		}

		c.wg.Add(1)
		go c.authenticateAndHandle(conn)
	}
}

// authenticateAndHandle reads the auth handshake from
// the accepted connection, verifies the peer, registers
// it, and then handles subsequent streams.
func (c *Carrier) authenticateAndHandle( // PAP
	conn Connection,
) {
	defer c.wg.Done()

	nodeID, scope, err := c.receiveAuthHandshake(
		conn,
	)
	if err != nil {
		_ = conn.Close()
		return
	}

	if err := c.sendAuthHandshake(conn); err != nil {
		c.logger.WarnContext(
			c.ctx,
			"failed to send auth handshake back",
			keyPeerNodeId, nodeID.String(),
			keyFailReason, err.Error(),
		)
		_ = conn.Close()
		return
	}

	if qc, ok := conn.(*quicConnection); ok {
		qc.setNodeID(nodeID)
	}

	peer := interfaces.PeerNode{
		NodeID: nodeID,
	}
	if err := c.registry.AddNode(
		peer, nil, scope,
	); err != nil {
		c.logger.WarnContext(
			c.ctx,
			"failed to register accepted peer",
			keyPeerNodeId, nodeID.String(),
			keyFailReason, err.Error(),
		)
		_ = conn.Close()
		return
	}
	_ = c.registry.UpdateConnectionStatus(
		nodeID,
		interfaces.ConnectionStatusConnected,
	)

	c.transport.mu.Lock()
	c.transport.connections[nodeID] = conn.(*quicConnection)
	c.transport.mu.Unlock()

	c.wg.Add(1)
	go c.handleConnection(conn)
}

// receiveAuthHandshake reads an auth handshake message
// from the peer, extracts TLS binding info, and verifies
// the peer certificate through the 6-step pipeline.
func (c *Carrier) receiveAuthHandshake( // PAP
	conn Connection,
) (keys.NodeID, auth.TrustScope, error) {
	stream, err := conn.AcceptStream()
	if err != nil {
		return keys.NodeID{}, 0, fmt.Errorf(
			"accept auth stream: %w", err,
		)
	}
	defer func() { _ = stream.Close() }()

	msg, err := ReadMessage(stream)
	if err != nil {
		return keys.NodeID{}, 0, fmt.Errorf(
			"read auth message: %w", err,
		)
	}

	if msg.Type != interfaces.MessageTypeAuthHandshake {
		return keys.NodeID{}, 0, fmt.Errorf(
			"expected auth handshake, got %d",
			msg.Type,
		)
	}

	x509FP := extractX509Fingerprint(conn)
	certPubKeyHash, err := extractPeerCertPublicKeyHash(conn)
	if err != nil {
		return keys.NodeID{}, 0, errors.New(
			"authentication failed",
		)
	}
	exporterBinding, err := extractTLSExporterBinding(conn)
	if err != nil {
		return keys.NodeID{}, 0, errors.New(
			"authentication failed",
		)
	}
	transcriptHash := exporterBinding[:]

	nodeID, scope, err := c.verifyAuthPayload(
		msg.Payload,
		x509FP,
		certPubKeyHash,
		exporterBinding,
		transcriptHash,
	)
	if err != nil {
		c.logger.WarnContext(
			c.ctx,
			"auth handshake rejected",
			keyFailReason, err.Error(),
		)
		return keys.NodeID{}, 0, errors.New(
			"authentication failed",
		)
	}

	resp := interfaces.Response{
		Payload: []byte("auth-ok"),
	}
	_ = WriteResponse(stream, resp)

	return nodeID, scope, nil
}

// verifyAuthPayload deserializes and verifies the
// auth handshake payload. The payload format is:
// [NodeCert fields] + [CA sig] + [DelegationProof] +
// [DelegationSig] + [SessionPubKey].
// For now this is a placeholder that delegates to
// VerifyPeerCert once full serialization is wired.
func (c *Carrier) verifyAuthPayload( // PAP
	payload []byte,
	x509FP [32]byte,
	tlsCertPubKeyHash [32]byte,
	tlsExporterBinding [32]byte,
	transcriptHash []byte,
) (keys.NodeID, auth.TrustScope, error) {
	if c.auth == nil {
		return keys.NodeID{}, 0, errors.New(
			"carrier auth must not be nil",
		)
	}

	fields, err := decodeAuthHandshakePayload(payload)
	if err != nil {
		return keys.NodeID{}, 0, fmt.Errorf(
			"decode auth payload: %w", err,
		)
	}

	cert, err := auth.UnmarshalNodeCert(fields.nodeCert)
	if err != nil {
		return keys.NodeID{}, 0, fmt.Errorf(
			"unmarshal node cert: %w", err,
		)
	}

	proof, err := auth.UnmarshalDelegationProof(fields.delegationProof)
	if err != nil {
		return keys.NodeID{}, 0, fmt.Errorf(
			"unmarshal delegation proof: %w", err,
		)
	}

	return c.auth.VerifyPeerCert(auth.VerifyPeerCertParams{
		PeerCert:           cert,
		CASignature:        fields.caSignature,
		DelegationProof:    proof,
		DelegationSig:      fields.delegationSig,
		TLSCertPubKeyHash:  tlsCertPubKeyHash,
		TLSExporterBinding: tlsExporterBinding,
		TLSX509Fingerprint: x509FP,
		TLSTranscriptHash:  transcriptHash,
	})
}

func (c *Carrier) authenticateBootstrapConn( // PAP
	conn Connection,
) (keys.NodeID, auth.TrustScope, error) {
	if err := c.sendAuthHandshake(conn); err != nil {
		return keys.NodeID{}, 0, fmt.Errorf(
			"send auth handshake: %w", err,
		)
	}
	return c.receiveAuthHandshake(conn)
}

// sendAuthHandshake sends the local node's auth
// material to the peer over a new stream.
func (c *Carrier) sendAuthHandshake( // PAP
	conn Connection,
) error {
	if err := c.validateLocalAuthMaterial(); err != nil {
		return err
	}

	payload, err := c.buildLocalAuthPayload(conn)
	if err != nil {
		return err
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf(
			"open auth stream: %w", err,
		)
	}
	defer func() { _ = stream.Close() }()

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeAuthHandshake,
		Payload: payload,
	}
	if err := WriteMessage(stream, msg); err != nil {
		return fmt.Errorf(
			"write auth message: %w", err,
		)
	}

	_, err = ReadResponse(stream)
	return err
}

func (c *Carrier) validateLocalAuthMaterial() error { // A
	if c.localCert == nil {
		return errors.New("local cert must not be nil")
	}
	if c.localKeys == nil {
		return errors.New("local keys must not be nil")
	}
	if len(c.localCASignature) == 0 {
		return errors.New("local CA signature must not be empty")
	}
	if len(c.transport.tlsCert.Certificate) == 0 {
		return errors.New("local TLS certificate is missing")
	}
	return nil
}

func (c *Carrier) buildLocalAuthPayload( // A
	conn Connection,
) ([]byte, error) {
	proof, err := c.buildLocalDelegationProof(conn)
	if err != nil {
		return nil, err
	}

	delegationSig, err := auth.SignDelegationProof(
		c.localKeys,
		proof,
	)
	if err != nil {
		return nil, fmt.Errorf("sign delegation proof: %w", err)
	}

	certBytes, err := auth.MarshalNodeCert(c.localCert)
	if err != nil {
		return nil, fmt.Errorf("marshal node cert: %w", err)
	}
	proofBytes, err := auth.MarshalDelegationProof(proof)
	if err != nil {
		return nil, fmt.Errorf("marshal delegation proof: %w", err)
	}

	payload, err := encodeAuthHandshakePayload(authHandshakeFields{
		nodeCert:        certBytes,
		caSignature:     c.localCASignature,
		delegationProof: proofBytes,
		delegationSig:   delegationSig,
	})
	if err != nil {
		return nil, fmt.Errorf("encode auth payload: %w", err)
	}
	return payload, nil
}

func (c *Carrier) buildLocalDelegationProof( // PAP
	conn Connection,
) (*auth.DelegationProof, error) {
	certHash, err := c.localCert.Hash()
	if err != nil {
		return nil, fmt.Errorf("hash local cert: %w", err)
	}

	localCertDER := conn.LocalCertificateDER()
	if len(localCertDER) == 0 {
		return nil, errors.New("local TLS certificate is missing")
	}
	x509FP := sha256.Sum256(localCertDER)

	certPubKeyHash, err := hashCertSubjectPublicKeyInfo(localCertDER)
	if err != nil {
		return nil, fmt.Errorf(
			"hash local cert SPKI: %w",
			err,
		)
	}

	tlsExporterBinding, err := extractTLSExporterBinding(conn)
	if err != nil {
		return nil, fmt.Errorf(
			"derive TLS exporter binding: %w",
			err,
		)
	}

	handshakeNonce, err := randomNonce32()
	if err != nil {
		return nil, fmt.Errorf("generate handshake nonce: %w", err)
	}
	now := time.Now().UTC()
	proof, err := auth.NewDelegationProof(auth.DelegationProofParams{
		TLSCertPubKeyHash:  certPubKeyHash,
		TLSExporterBinding: tlsExporterBinding,
		X509Fingerprint:    x509FP,
		NodeCertHash:       certHash,
		NotBefore:          now.Add(-time.Minute),
		NotAfter:           now.Add(2 * time.Minute),
		HandshakeNonce:     handshakeNonce,
	})
	if err != nil {
		return nil, fmt.Errorf("build delegation proof: %w", err)
	}
	return proof, nil
}

type authHandshakeFields struct { // A
	nodeCert        []byte
	caSignature     []byte
	delegationProof []byte
	delegationSig   []byte
}

func encodeAuthHandshakePayload( // A
	fields authHandshakeFields,
) ([]byte, error) {
	parts := [][]byte{
		fields.nodeCert,
		fields.caSignature,
		fields.delegationProof,
		fields.delegationSig,
	}
	total := 0
	for _, part := range parts {
		total += 4 + len(part)
	}
	buf := make([]byte, 0, total)
	for _, part := range parts {
		if len(part) > int(^uint32(0)) {
			return nil, errors.New(
				"auth payload part too large",
			)
		}
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(
			lenBuf[:],
			uint32(len(part)), //#nosec G115
		)
		buf = append(buf, lenBuf[:]...)
		buf = append(buf, part...)
	}
	return buf, nil
}

func decodeAuthHandshakePayload( // A
	payload []byte,
) (authHandshakeFields, error) {
	offset := 0
	certBytes, err := readAuthPayloadField(payload, &offset)
	if err != nil {
		return authHandshakeFields{}, err
	}
	caSig, err := readAuthPayloadField(payload, &offset)
	if err != nil {
		return authHandshakeFields{}, err
	}
	proofBytes, err := readAuthPayloadField(payload, &offset)
	if err != nil {
		return authHandshakeFields{}, err
	}
	delegSig, err := readAuthPayloadField(payload, &offset)
	if err != nil {
		return authHandshakeFields{}, err
	}

	if offset != len(payload) {
		return authHandshakeFields{}, errors.New(
			"auth payload has trailing bytes",
		)
	}

	if len(certBytes) == 0 || len(caSig) == 0 ||
		len(proofBytes) == 0 || len(delegSig) == 0 {
		return authHandshakeFields{}, errors.New(
			"auth payload fields must not be empty",
		)
	}

	return authHandshakeFields{
		nodeCert:        certBytes,
		caSignature:     caSig,
		delegationProof: proofBytes,
		delegationSig:   delegSig,
	}, nil
}

func readAuthPayloadField( // A
	payload []byte,
	offset *int,
) ([]byte, error) {
	if len(payload[*offset:]) < 4 {
		return nil, errors.New("missing field length")
	}
	fieldLen := int(binary.BigEndian.Uint32(
		payload[*offset : *offset+4],
	))
	*offset += 4
	if fieldLen < 0 || len(payload[*offset:]) < fieldLen {
		return nil, errors.New("invalid field length")
	}
	field := make([]byte, fieldLen)
	copy(field, payload[*offset:*offset+fieldLen])
	*offset += fieldLen
	return field, nil
}

func randomNonce32() ([32]byte, error) { // A
	var nonce [32]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return [32]byte{}, err
	}
	return nonce, nil
}

// extractX509Fingerprint computes SHA-256 of the first
// peer certificate's raw DER bytes.
func extractX509Fingerprint( // A
	conn Connection,
) [32]byte {
	certs := conn.PeerCertificatesDER()
	if len(certs) == 0 {
		return [32]byte{}
	}
	return sha256.Sum256(certs[0])
}

// computeTranscriptHash derives a transcript binding
// hash from the peer's TLS certificate material.
func computeTranscriptHash( // A
	conn Connection,
) []byte {
	binding, err := extractTLSExporterBinding(conn)
	if err != nil {
		return nil
	}
	out := make([]byte, len(binding))
	copy(out, binding[:])
	return out
}

func extractPeerCertPublicKeyHash( // A
	conn Connection,
) ([32]byte, error) {
	certs := conn.PeerCertificatesDER()
	if len(certs) == 0 {
		return [32]byte{}, errors.New(
			"peer certificate is missing",
		)
	}
	return hashCertSubjectPublicKeyInfo(certs[0])
}

func hashCertSubjectPublicKeyInfo( // A
	certDER []byte,
) ([32]byte, error) {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return [32]byte{}, fmt.Errorf(
			"parse certificate: %w",
			err,
		)
	}
	return sha256.Sum256(cert.RawSubjectPublicKeyInfo), nil
}

func extractTLSExporterBinding( // PAP
	conn Connection,
) ([32]byte, error) {
	exporter, err := conn.ExportKeyingMaterial(
		tlsExporterLabel,
		nil,
		tlsExporterBindingLen,
	)
	if err != nil {
		return [32]byte{}, err
	}
	if len(exporter) != tlsExporterBindingLen {
		return [32]byte{}, errors.New(
			"invalid exporter binding length",
		)
	}
	var out [32]byte
	copy(out[:], exporter)
	return out, nil
}

// handleConnection processes inbound streams from a
// single peer connection. Each stream carries one
// request/response exchange.
func (c *Carrier) handleConnection( // A
	conn Connection,
) {
	defer c.wg.Done()
	for {
		stream, err := conn.AcceptStream()
		if err != nil {
			select {
			case <-c.ctx.Done():
				return
			default:
				// Connection closed or error;
				// stop processing this peer.
				return
			}
		}

		c.wg.Add(1)
		go c.handleStream(conn, stream)
	}
}

// handleStream reads a single message from the
// stream, dispatches it to the MessageReceiver,
// and writes the response back.
func (c *Carrier) handleStream( // A
	conn Connection,
	stream Stream,
) {
	defer c.wg.Done()
	defer func() { _ = stream.Close() }()

	msg, err := ReadMessage(stream)
	if err != nil {
		return
	}

	_ = c.registry.UpdateLastSeen(conn.NodeID())

	resp := c.dispatchMessage(msg, conn.NodeID())

	_ = WriteResponse(stream, resp)
}

// dispatchMessage invokes the MessageReceiver or
// returns an error response if no receiver is set.
func (c *Carrier) dispatchMessage( // A
	msg interfaces.Message,
	peer keys.NodeID,
) interfaces.Response {
	recv := c.receiver.Load()
	if recv == nil {
		return interfaces.Response{
			Error: fmt.Errorf(
				"no message receiver configured",
			),
		}
	}

	info, err := c.registry.GetNode(peer)
	if err != nil {
		return interfaces.Response{
			Error: fmt.Errorf(
				"unknown peer %s", peer,
			),
		}
	}

	resp, err := (*recv)(
		msg, peer, info.TrustScope,
	)
	if err != nil {
		return interfaces.Response{
			Error: err,
		}
	}
	return resp
}

// ensureConnection returns an existing connection
// or dials a new one.
func (c *Carrier) ensureConnection( // A
	nodeID keys.NodeID,
) (Connection, error) {
	existing := c.transport.GetConnection(nodeID)
	if existing != nil {
		return existing, nil
	}

	info, err := c.registry.GetNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf(
			"node %s not in registry", nodeID,
		)
	}

	conn, err := c.transport.Dial(info.Peer)
	if err != nil {
		return nil, fmt.Errorf(
			"dial %s: %w", nodeID, err,
		)
	}

	_ = c.registry.UpdateConnectionStatus(
		nodeID,
		interfaces.ConnectionStatusConnected,
	)

	return conn, nil
}
