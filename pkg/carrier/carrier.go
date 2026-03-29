package carrier

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"sort"
	"sync"
	"time"

	oldproto "github.com/golang/protobuf/proto"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"github.com/quic-go/quic-go"
)

const ( // A
	transportALPN = "ouroboros-carrier/1"
	maxFrameSize  = 16 << 20
	proofTTL      = 60
)

type peerState struct { // A
	peer     interfaces.PeerNode
	conn     *quic.Conn
	authCtx  auth.AuthContext
	status   interfaces.ConnectionStatus
	connRole connectionRole
}

type connectionRole uint8 // A

const ( // A
	connectionRoleUnknown connectionRole = iota
	connectionRoleInbound
	connectionRoleOutbound
)

type tlsIdentity struct { // A
	certificate tls.Certificate
	pubKeyHash  []byte
	fingerprint []byte
}

// Carrier implements the QUIC transport layer for
// cluster communication.
type Carrier struct { // A
	logger      *slog.Logger
	localSigner *keys.AsyncCrypt
	localPeer   interfaces.PeerNode
	localCerts  []interfaces.NodeCert
	localSigs   [][]byte
	tlsID       tlsIdentity
	authStore   interfaces.CarrierAuth
	listener    *quic.Listener
	quicConfig  *quic.Config
	tlsConfig   *tls.Config

	mu         sync.RWMutex
	peers      map[keys.NodeID]*peerState
	dispatcher Dispatcher
	closed     chan struct{}
	wg         sync.WaitGroup
}

// New creates, starts, and returns a QUIC carrier.
func New( // A
	cfg Config,
) (*Carrier, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	tlsID, err := newTLSIdentity()
	if err != nil {
		return nil, err
	}
	localCerts, localSigs, err := prepareLocalIdentity(
		cfg.LocalSigner,
		cfg.LocalNodeCerts,
		cfg.LocalCASignatures,
	)
	if err != nil {
		return nil, err
	}
	localPubKey := cfg.LocalSigner.GetPublicKey()
	localNodeID, err := localPubKey.NodeID()
	if err != nil {
		return nil, fmt.Errorf("derive local node ID: %w", err)
	}
	authStore := auth.NewCarrierAuth(cfg.Logger)
	if err := addTrustedAdmins(
		authStore,
		cfg.TrustedAdminPubKeys,
		localPubKey,
	); err != nil {
		return nil, err
	}

	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{tlsID.certificate},
		NextProtos:         []string{transportALPN},
		InsecureSkipVerify: true,
		ClientAuth:         tls.RequireAnyClientCert,
		MinVersion:         tls.VersionTLS13,
	}
	quicConf := &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: 15 * time.Second,
		MaxIdleTimeout:  30 * time.Second,
	}
	listener, err := quic.ListenAddr(
		cfg.ListenAddress,
		tlsConf,
		quicConf,
	)
	if err != nil {
		return nil, fmt.Errorf("listen QUIC: %w", err)
	}
	carrier := &Carrier{
		logger:      cfg.Logger,
		localSigner: cfg.LocalSigner,
		localPeer: interfaces.PeerNode{
			NodeID:       localNodeID,
			Addresses:    []string{listener.Addr().String()},
			NodeCerts:    cloneCerts(localCerts),
			CASignatures: cloneByteSlices(localSigs),
		},
		localCerts: cloneCerts(localCerts),
		localSigs:  cloneByteSlices(localSigs),
		tlsID:      tlsID,
		authStore:  authStore,
		listener:   listener,
		quicConfig: quicConf,
		tlsConfig:  tlsConf,
		peers:      make(map[keys.NodeID]*peerState),
		dispatcher: cfg.Dispatcher,
		closed:     make(chan struct{}),
	}
	for _, peer := range cfg.BootstrapPeers {
		if err := carrier.JoinCluster(peer); err != nil {
			listener.Close()
			return nil, err
		}
	}
	carrier.wg.Add(1)
	go carrier.acceptLoop()
	return carrier, nil
}

// SetDispatcher sets the inbound message dispatcher.
func (c *Carrier) SetDispatcher( // A
	dispatcher Dispatcher,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dispatcher = dispatcher
}

// LocalPeer returns the local peer information exposed
// to other cluster nodes.
func (c *Carrier) LocalPeer() interfaces.PeerNode { // A
	c.mu.RLock()
	defer c.mu.RUnlock()
	return clonePeer(c.localPeer)
}

// ListenAddress returns the bound listen address.
func (c *Carrier) ListenAddress() string { // A
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.localPeer.Addresses) == 0 {
		return ""
	}
	return c.localPeer.Addresses[0]
}

// GetNodes returns all known peers.
func (c *Carrier) GetNodes() []interfaces.PeerNode { // A
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]interfaces.PeerNode, 0, len(c.peers))
	for _, peer := range c.peers {
		out = append(out, clonePeer(peer.peer))
	}
	return out
}

// GetNode returns a known peer by node ID.
func (c *Carrier) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.PeerNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	peer, ok := c.peers[nodeID]
	if !ok {
		return interfaces.PeerNode{}, fmt.Errorf("node not found")
	}
	return clonePeer(peer.peer), nil
}

// BroadcastReliable sends a reliable message to all
// known peers.
func (c *Carrier) BroadcastReliable( // A
	message interfaces.Message,
) ([]interfaces.PeerNode, []interfaces.PeerNode, error) {
	peers := c.GetNodes()
	success := make([]interfaces.PeerNode, 0, len(peers))
	failed := make([]interfaces.PeerNode, 0)
	var lastErr error
	for _, peer := range peers {
		if err := c.SendMessageToNodeReliable(peer.NodeID, message); err != nil {
			failed = append(failed, peer)
			lastErr = err
			continue
		}
		success = append(success, peer)
	}
	return success, failed, lastErr
}

// SendMessageToNodeReliable sends a request over a
// reliable QUIC stream and waits for the response.
func (c *Carrier) SendMessageToNodeReliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	conn, _, err := c.ensureConnection(nodeID)
	if err != nil {
		return err
	}
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer stream.Close()
	encoded, err := interfaces.MarshalMessage(message)
	if err != nil {
		return err
	}
	if err := writeFrame(stream, encoded); err != nil {
		return err
	}
	if err := stream.Close(); err != nil {
		return err
	}
	responseBytes, err := readFrame(stream, maxFrameSize)
	if err != nil {
		return err
	}
	response, err := interfaces.UnmarshalResponse(responseBytes)
	if err != nil {
		return err
	}
	if response.GetErrorText() != "" {
		return errors.New(response.GetErrorText())
	}
	return nil
}

// BroadcastUnreliable sends an unreliable datagram to
// all known peers.
func (c *Carrier) BroadcastUnreliable( // A
	message interfaces.Message,
) []interfaces.PeerNode {
	peers := c.GetNodes()
	attempted := make([]interfaces.PeerNode, 0, len(peers))
	for _, peer := range peers {
		if err := c.SendMessageToNodeUnreliable(peer.NodeID, message); err == nil {
			attempted = append(attempted, peer)
		}
	}
	return attempted
}

// SendMessageToNodeUnreliable sends a QUIC datagram.
func (c *Carrier) SendMessageToNodeUnreliable( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	conn, _, err := c.ensureConnection(nodeID)
	if err != nil {
		return err
	}
	encoded, err := interfaces.MarshalMessage(message)
	if err != nil {
		return err
	}
	return conn.SendDatagram(encoded)
}

// Broadcast sends a reliable message to all peers.
func (c *Carrier) Broadcast( // A
	message interfaces.Message,
) ([]interfaces.PeerNode, error) {
	success, _, err := c.BroadcastReliable(message)
	return success, err
}

// SendMessageToNode sends a reliable message to one
// peer.
func (c *Carrier) SendMessageToNode( // A
	nodeID keys.NodeID,
	message interfaces.Message,
) error {
	return c.SendMessageToNodeReliable(nodeID, message)
}

// JoinCluster adds a peer and attempts a connection.
func (c *Carrier) JoinCluster( // A
	clusterNode interfaces.PeerNode,
) error {
	if clusterNode.NodeID.IsZero() {
		return fmt.Errorf("peer node ID must not be zero")
	}
	if len(clusterNode.Addresses) == 0 {
		return fmt.Errorf("peer must advertise an address")
	}
	if err := c.trustPeerNode(clusterNode); err != nil {
		return err
	}
	c.mu.Lock()
	existing, ok := c.peers[clusterNode.NodeID]
	if ok {
		existing.peer = mergePeer(existing.peer, clusterNode)
	} else {
		c.peers[clusterNode.NodeID] = &peerState{
			peer:   clonePeer(clusterNode),
			status: interfaces.ConnectionStatusDisconnected,
		}
	}
	c.mu.Unlock()
	return nil
}

// LeaveCluster disconnects from a peer without
// removing its registry entry.
func (c *Carrier) LeaveCluster( // A
	clusterNode interfaces.PeerNode,
) error {
	c.mu.Lock()
	peer, ok := c.peers[clusterNode.NodeID]
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("node not found")
	}
	if peer.conn != nil {
		return peer.conn.CloseWithError(0, "leave cluster")
	}
	return nil
}

// RemoveNode removes a peer and closes its active
// connection.
func (c *Carrier) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	c.mu.Lock()
	peer, ok := c.peers[nodeID]
	if ok {
		delete(c.peers, nodeID)
	}
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("node not found")
	}
	if peer.conn != nil {
		return peer.conn.CloseWithError(0, "node removed")
	}
	return nil
}

// IsConnected reports whether the peer currently has
// an active authenticated connection.
func (c *Carrier) IsConnected( // A
	nodeID keys.NodeID,
) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	peer, ok := c.peers[nodeID]
	return ok && peer.conn != nil
}

// Close shuts down the carrier.
func (c *Carrier) Close() error { // A
	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
	}
	if c.listener != nil {
		_ = c.listener.Close()
	}
	c.mu.RLock()
	conns := make([]*quic.Conn, 0, len(c.peers))
	for _, peer := range c.peers {
		if peer.conn != nil {
			conns = append(conns, peer.conn)
		}
	}
	c.mu.RUnlock()
	for _, conn := range conns {
		_ = conn.CloseWithError(0, "carrier closed")
	}
	c.wg.Wait()
	return nil
}

func (c *Carrier) acceptLoop() { // A
	defer c.wg.Done()
	for {
		conn, err := c.listener.Accept(context.Background())
		if err != nil {
			select {
			case <-c.closed:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			c.logger.WarnContext(
				context.Background(),
				"accept failed",
				"error", err,
			)
			continue
		}
		c.wg.Add(1)
		go c.handleIncomingConn(conn)
	}
}

func (c *Carrier) handleIncomingConn( // A
	conn *quic.Conn,
) {
	defer c.wg.Done()
	peer, authCtx, err := c.authenticateConn(
		conn,
		false,
	)
	if err != nil {
		c.logger.WarnContext(
			context.Background(),
			"incoming auth failed",
			"error", err,
		)
		_ = conn.CloseWithError(0, err.Error())
		return
	}
	c.registerConn(peer, authCtx, conn, connectionRoleInbound)
	c.runPeerLoops(peer.NodeID, conn)
}

func (c *Carrier) ensureConnection( // A
	nodeID keys.NodeID,
) (*quic.Conn, auth.AuthContext, error) {
	c.mu.RLock()
	peer, ok := c.peers[nodeID]
	if ok && peer.conn != nil {
		conn := peer.conn
		authCtx := peer.authCtx
		c.mu.RUnlock()
		return conn, authCtx, nil
	}
	if !ok {
		c.mu.RUnlock()
		return nil, auth.AuthContext{}, fmt.Errorf("node not found")
	}
	peerInfo := clonePeer(peer.peer)
	c.mu.RUnlock()
	conn, authCtx, err := c.dialPeer(peerInfo)
	if err != nil {
		return nil, auth.AuthContext{}, err
	}
	return conn, authCtx, nil
}

func (c *Carrier) dialPeer( // A
	peer interfaces.PeerNode,
) (*quic.Conn, auth.AuthContext, error) {
	tlsConf := c.tlsConfig.Clone()
	tlsConf.ServerName = "ouroboros-peer"
	var lastErr error
	for _, address := range peer.Addresses {
		conn, err := quic.DialAddr(
			context.Background(),
			address,
			tlsConf,
			c.quicConfig,
		)
		if err != nil {
			lastErr = err
			continue
		}
		peerInfo, authCtx, err := c.authenticateConn(
			conn,
			true,
		)
		if err != nil {
			lastErr = err
			_ = conn.CloseWithError(0, "auth failed")
			continue
		}
		peerInfo = mergePeer(peerInfo, peer)
		c.registerConn(
			peerInfo,
			authCtx,
			conn,
			connectionRoleOutbound,
		)
		if currentConn, currentAuth, ok := c.currentConn(peerInfo.NodeID); ok &&
			currentConn != conn {
			return currentConn, currentAuth, nil
		}
		c.wg.Add(1)
		go func(nodeID keys.NodeID) {
			defer c.wg.Done()
			c.runPeerLoops(nodeID, conn)
		}(peerInfo.NodeID)
		return conn, authCtx, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no peer addresses available")
	}
	return nil, auth.AuthContext{}, lastErr
}

func (c *Carrier) registerConn( // A
	peer interfaces.PeerNode,
	authCtx auth.AuthContext,
	conn *quic.Conn,
	role connectionRole,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.peers[peer.NodeID]; ok &&
		existing.conn != nil && existing.conn != conn {
		if shouldReplaceConn(
			c.localPeer.NodeID,
			peer.NodeID,
			existing.connRole,
			role,
		) {
			oldConn := existing.conn
			existing.peer = mergePeer(existing.peer, peer)
			existing.conn = conn
			existing.authCtx = authCtx
			existing.status = interfaces.ConnectionStatusConnected
			existing.connRole = role
			_ = oldConn.CloseWithError(
				0,
				"superseded by preferred duplicate",
			)
			return
		}
		_ = conn.CloseWithError(0, "duplicate")
		return
	}
	c.peers[peer.NodeID] = &peerState{
		peer:     clonePeer(peer),
		conn:     conn,
		authCtx:  authCtx,
		status:   interfaces.ConnectionStatusConnected,
		connRole: role,
	}
}

func (c *Carrier) currentConn( // A
	nodeID keys.NodeID,
) (*quic.Conn, auth.AuthContext, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	peer, ok := c.peers[nodeID]
	if !ok || peer.conn == nil {
		return nil, auth.AuthContext{}, false
	}
	return peer.conn, peer.authCtx, true
}

func (c *Carrier) runPeerLoops( // A
	nodeID keys.NodeID,
	conn *quic.Conn,
) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.streamLoop(ctx, nodeID, conn)
		cancel()
	}()
	go func() {
		defer wg.Done()
		c.datagramLoop(ctx, nodeID, conn)
		cancel()
	}()
	select {
	case <-conn.Context().Done():
	case <-ctx.Done():
	}
	c.clearConn(nodeID, conn)
	cancel()
	wg.Wait()
}

func (c *Carrier) clearConn( // A
	nodeID keys.NodeID,
	conn *quic.Conn,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	peer, ok := c.peers[nodeID]
	if !ok || peer.conn != conn {
		return
	}
	peer.conn = nil
	peer.status = interfaces.ConnectionStatusDisconnected
	peer.authCtx = auth.AuthContext{}
	peer.connRole = connectionRoleUnknown
}

func (c *Carrier) streamLoop( // A
	ctx context.Context,
	nodeID keys.NodeID,
	conn *quic.Conn,
) {
	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go c.handleStream(nodeID, stream)
	}
}

func (c *Carrier) handleStream( // A
	nodeID keys.NodeID,
	stream *quic.Stream,
) {
	requestBytes, err := readFrame(stream, maxFrameSize)
	if err != nil {
		return
	}
	request, err := interfaces.UnmarshalMessage(requestBytes)
	if err != nil {
		_ = writeFrame(
			stream,
			mustMarshalResponse(
				interfaces.NewWireResponse(nil, err.Error(), nil),
			),
		)
		_ = stream.Close()
		return
	}
	authCtx, ok := c.authContextFor(nodeID)
	if !ok {
		_ = writeFrame(
			stream,
			mustMarshalResponse(
				interfaces.NewWireResponse(nil, "unknown peer", nil),
			),
		)
		_ = stream.Close()
		return
	}
	response, handlerErr := c.dispatch(
		context.Background(),
		request,
		authCtx,
	)
	if handlerErr != nil {
		response = interfaces.NewWireResponse(
			response.GetPayload(),
			handlerErr.Error(),
			response.GetMetadata(),
		)
		if response == nil {
			response = interfaces.NewWireResponse(
				nil,
				handlerErr.Error(),
				nil,
			)
		}
	}
	if response == nil {
		response = interfaces.NewWireResponse(nil, "", nil)
	}
	encoded, err := interfaces.MarshalResponse(response)
	if err != nil {
		encoded = mustMarshalResponse(
			interfaces.NewWireResponse(nil, err.Error(), nil),
		)
	}
	_ = writeFrame(stream, encoded)
	_ = stream.Close()
}

func (c *Carrier) datagramLoop( // A
	ctx context.Context,
	nodeID keys.NodeID,
	conn *quic.Conn,
) {
	for {
		payload, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		request, err := interfaces.UnmarshalMessage(payload)
		if err != nil {
			continue
		}
		authCtx, ok := c.authContextFor(nodeID)
		if !ok {
			continue
		}
		_, _ = c.dispatch(
			context.Background(),
			request,
			authCtx,
		)
	}
}

func (c *Carrier) dispatch( // A
	ctx context.Context,
	msg interfaces.Message,
	authCtx auth.AuthContext,
) (interfaces.Response, error) {
	c.mu.RLock()
	dispatcher := c.dispatcher
	c.mu.RUnlock()
	if dispatcher == nil {
		return interfaces.NewWireResponse(
			nil,
			"dispatcher not configured",
			nil,
		), fmt.Errorf("dispatcher not configured")
	}
	return dispatcher(ctx, msg, authCtx)
}

func (c *Carrier) authContextFor( // A
	nodeID keys.NodeID,
) (auth.AuthContext, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	peer, ok := c.peers[nodeID]
	if !ok {
		return auth.AuthContext{}, false
	}
	return peer.authCtx, true
}

func (c *Carrier) authenticateConn( // A
	conn *quic.Conn,
	initiator bool,
) (interfaces.PeerNode, auth.AuthContext, error) {
	select {
	case <-conn.HandshakeComplete():
	case <-time.After(10 * time.Second):
		return interfaces.PeerNode{}, auth.AuthContext{},
			fmt.Errorf("handshake timeout")
	}
	peerTLSValues, err := peerTLSValuesFromConn(conn)
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	localHello := &authHello{
		Certs:        mustEncodeNodeCerts(c.localCerts),
		CASignatures: cloneByteSlices(c.localSigs),
	}
	localHelloBytes, err := oldproto.Marshal(localHello)
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	var stream *quic.Stream
	if initiator {
		stream, err = conn.OpenStreamSync(context.Background())
	} else {
		stream, err = conn.AcceptStream(context.Background())
	}
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	defer stream.Close()
	var peerHelloBytes []byte
	if initiator {
		if err := writeFrame(stream, localHelloBytes); err != nil {
			return interfaces.PeerNode{}, auth.AuthContext{}, err
		}
		peerHelloBytes, err = readFrame(stream, maxFrameSize)
	} else {
		peerHelloBytes, err = readFrame(stream, maxFrameSize)
		if err == nil {
			err = writeFrame(stream, localHelloBytes)
		}
	}
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	peerHello := &authHello{}
	if err := oldproto.Unmarshal(peerHelloBytes, peerHello); err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	transcriptHash := handshakeTranscriptHash(
		localHelloBytes,
		peerHelloBytes,
		initiator,
	)
	localProofMsg, err := c.buildLocalProof(conn, transcriptHash)
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	localProofBytes, err := oldproto.Marshal(localProofMsg)
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	var peerProofBytes []byte
	if initiator {
		if err := writeFrame(stream, localProofBytes); err != nil {
			return interfaces.PeerNode{}, auth.AuthContext{}, err
		}
		peerProofBytes, err = readFrame(stream, maxFrameSize)
	} else {
		peerProofBytes, err = readFrame(stream, maxFrameSize)
		if err == nil {
			err = writeFrame(stream, localProofBytes)
		}
	}
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	peerProof := &authProof{}
	if err := oldproto.Unmarshal(peerProofBytes, peerProof); err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	peerCerts, peerNodeCerts, err := decodeNodeCerts(peerHello.Certs)
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	proof := auth.NewDelegationProof(
		peerProof.TLSCertPubKeyHash,
		peerProof.TLSExporterBinding,
		peerProof.TLSTranscriptHash,
		peerProof.X509Fingerprint,
		peerProof.NodeCertBundleHash,
		peerProof.NotBefore,
		peerProof.NotAfter,
	)
	exporterBinding, err := exporterBindingFromConn(conn, proof)
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	authCtx, err := c.authStore.VerifyPeerCert(
		peerCerts,
		cloneByteSlices(peerHello.CASignatures),
		proof,
		append([]byte(nil), peerProof.DelegationSig...),
		peerTLSValues.pubKeyHash,
		exporterBinding,
		peerTLSValues.fingerprint,
		transcriptHash,
	)
	if err != nil {
		return interfaces.PeerNode{}, auth.AuthContext{}, err
	}
	peer := interfaces.PeerNode{
		NodeID:       authCtx.NodeID,
		Addresses:    []string{conn.RemoteAddr().String()},
		NodeCerts:    peerNodeCerts,
		CASignatures: cloneByteSlices(peerHello.CASignatures),
	}
	return peer, authCtx, nil
}

func (c *Carrier) buildLocalProof( // A
	conn *quic.Conn,
	transcriptHash []byte,
) (*authProof, error) {
	bundleBytes, err := auth.CanonicalNodeCertBundle(c.localCerts)
	if err != nil {
		return nil, err
	}
	bundleHash := sha256.Sum256(bundleBytes)
	now := time.Now().Unix()
	proofNoExporter := auth.NewDelegationProof(
		c.tlsID.pubKeyHash,
		nil,
		transcriptHash,
		c.tlsID.fingerprint,
		bundleHash[:],
		now-5,
		now+proofTTL,
	)
	exporterBinding, err := exporterBindingFromConn(conn, proofNoExporter)
	if err != nil {
		return nil, err
	}
	proof := auth.NewDelegationProof(
		c.tlsID.pubKeyHash,
		exporterBinding,
		transcriptHash,
		c.tlsID.fingerprint,
		bundleHash[:],
		proofNoExporter.NotBefore(),
		proofNoExporter.NotAfter(),
	)
	canonical, err := auth.CanonicalDelegationProof(proof)
	if err != nil {
		return nil, err
	}
	signature, err := c.localSigner.Sign(
		auth.DomainSeparate(auth.CTXNodeDelegationV1, canonical),
	)
	if err != nil {
		return nil, err
	}
	return &authProof{
		TLSCertPubKeyHash:  proof.TLSCertPubKeyHash(),
		TLSExporterBinding: proof.TLSExporterBinding(),
		TLSTranscriptHash:  proof.TLSTranscriptHash(),
		X509Fingerprint:    proof.X509Fingerprint(),
		NodeCertBundleHash: proof.NodeCertBundleHash(),
		NotBefore:          proof.NotBefore(),
		NotAfter:           proof.NotAfter(),
		DelegationSig:      signature,
	}, nil
}

func prepareLocalIdentity( // A
	signer *keys.AsyncCrypt,
	providedCerts []interfaces.NodeCert,
	providedSigs [][]byte,
) ([]interfaces.NodeCert, [][]byte, error) {
	if len(providedCerts) > 0 {
		return cloneCerts(providedCerts), cloneByteSlices(providedSigs), nil
	}
	pubKey := signer.GetPublicKey()
	combined, err := marshalCombinedPubKey(pubKey)
	if err != nil {
		return nil, nil, err
	}
	adminCA, err := auth.NewAdminCA(combined)
	if err != nil {
		return nil, nil, err
	}
	serial := make([]byte, 16)
	nonce := make([]byte, 16)
	if _, err := rand.Read(serial); err != nil {
		return nil, nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	now := time.Now().Unix()
	cert, err := auth.NewNodeCert(
		pubKey,
		adminCA.Hash(),
		now-60,
		now+24*60*60,
		serial,
		nonce,
	)
	if err != nil {
		return nil, nil, err
	}
	canonical, err := auth.CanonicalNodeCert(cert)
	if err != nil {
		return nil, nil, err
	}
	signature, err := signer.Sign(
		auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical),
	)
	if err != nil {
		return nil, nil, err
	}
	return []interfaces.NodeCert{cert}, [][]byte{signature}, nil
}

func addTrustedAdmins( // A
	store interfaces.CarrierAuth,
	trusted [][]byte,
	localPub keys.PublicKey,
) error {
	localCombined, err := marshalCombinedPubKey(localPub)
	if err != nil {
		return err
	}
	trusted = append(trusted, localCombined)
	seen := make(map[string]struct{})
	for _, pubKey := range trusted {
		adminCA, err := auth.NewAdminCA(pubKey)
		if err != nil {
			return err
		}
		if _, ok := seen[adminCA.Hash()]; ok {
			continue
		}
		seen[adminCA.Hash()] = struct{}{}
		if err := store.AddAdminPubKey(pubKey); err != nil &&
			!errors.Is(err, auth.ErrCAAlreadyExists) {
			return err
		}
	}
	return nil
}

func (c *Carrier) trustPeerNode( // A
	peer interfaces.PeerNode,
) error {
	for _, cert := range peer.NodeCerts {
		combined, err := marshalCombinedPubKey(cert.NodePubKey())
		if err != nil {
			return err
		}
		if err := c.authStore.AddAdminPubKey(combined); err != nil &&
			!errors.Is(err, auth.ErrCAAlreadyExists) {
			return err
		}
	}
	return nil
}

func mustEncodeNodeCerts( // A
	certs []interfaces.NodeCert,
) []*authNodeCert {
	encoded := make([]*authNodeCert, 0, len(certs))
	for _, cert := range certs {
		msg, err := encodeNodeCert(cert)
		if err != nil {
			panic(err)
		}
		encoded = append(encoded, msg)
	}
	return encoded
}

func encodeNodeCert( // A
	cert interfaces.NodeCert,
) (*authNodeCert, error) {
	pubKey := cert.NodePubKey()
	kem, err := pubKey.MarshalBinaryKEM()
	if err != nil {
		return nil, err
	}
	sign, err := pubKey.MarshalBinarySign()
	if err != nil {
		return nil, err
	}
	return &authNodeCert{
		CertVersion:    uint32(cert.CertVersion()),
		NodePubKeyKEM:  kem,
		NodePubKeySign: sign,
		IssuerCAHash:   cert.IssuerCAHash(),
		ValidFrom:      cert.ValidFrom(),
		ValidUntil:     cert.ValidUntil(),
		Serial:         cert.Serial(),
		CertNonce:      cert.CertNonce(),
	}, nil
}

func decodeNodeCerts( // A
	encoded []*authNodeCert,
) ([]auth.NodeCertLike, []interfaces.NodeCert, error) {
	decoded := make([]auth.NodeCertLike, 0, len(encoded))
	decodedWire := make([]interfaces.NodeCert, 0, len(encoded))
	for _, msg := range encoded {
		pubKey, err := keys.NewPublicKeyFromBinary(
			msg.NodePubKeyKEM,
			msg.NodePubKeySign,
		)
		if err != nil {
			return nil, nil, err
		}
		cert, err := auth.NewNodeCert(
			*pubKey,
			msg.IssuerCAHash,
			msg.ValidFrom,
			msg.ValidUntil,
			msg.Serial,
			msg.CertNonce,
		)
		if err != nil {
			return nil, nil, err
		}
		decoded = append(decoded, cert)
		decodedWire = append(decodedWire, cert)
	}
	return decoded, decodedWire, nil
}

func marshalCombinedPubKey( // A
	pubKey keys.PublicKey,
) ([]byte, error) {
	kem, err := pubKey.MarshalBinaryKEM()
	if err != nil {
		return nil, err
	}
	sign, err := pubKey.MarshalBinarySign()
	if err != nil {
		return nil, err
	}
	combined := make([]byte, len(kem)+len(sign))
	copy(combined, kem)
	copy(combined[len(kem):], sign)
	return combined, nil
}

func writeFrame( // A
	w io.Writer,
	payload []byte,
) error {
	if len(payload) > maxFrameSize {
		return fmt.Errorf("frame too large")
	}
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, uint32(len(payload)))
	if _, err := w.Write(head); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFrame( // A
	r io.Reader,
	limit int,
) ([]byte, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(head))
	if size < 0 || size > limit {
		return nil, fmt.Errorf("invalid frame size %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func mustMarshalResponse( // A
	response interfaces.Response,
) []byte {
	encoded, err := interfaces.MarshalResponse(response)
	if err != nil {
		panic(err)
	}
	return encoded
}

func handshakeTranscriptHash( // A
	localHello []byte,
	peerHello []byte,
	initiator bool,
) []byte {
	hasher := sha256.New()
	writeTranscriptPart := func(direction byte, payload []byte) {
		hasher.Write([]byte{direction})
		lenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBytes, uint32(len(payload)))
		hasher.Write(lenBytes)
		hasher.Write(payload)
	}
	if initiator {
		writeTranscriptPart('o', localHello)
		writeTranscriptPart('i', peerHello)
	} else {
		writeTranscriptPart('o', peerHello)
		writeTranscriptPart('i', localHello)
	}
	return hasher.Sum(nil)
}

func exporterBindingFromConn( // A
	conn *quic.Conn,
	proof auth.DelegationProofLike,
) ([]byte, error) {
	ctx, err := auth.CanonicalDelegationProofForExporter(proof)
	if err != nil {
		return nil, err
	}
	state := conn.ConnectionState().TLS
	return state.ExportKeyingMaterial(
		auth.ExporterLabel,
		ctx,
		auth.TLSExporterBindingSize,
	)
}

func peerTLSValuesFromConn( // A
	conn *quic.Conn,
) (tlsIdentity, error) {
	state := conn.ConnectionState().TLS
	if len(state.PeerCertificates) == 0 {
		return tlsIdentity{}, fmt.Errorf("peer certificate missing")
	}
	leaf := state.PeerCertificates[0]
	pubKeyHash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	fingerprint := sha256.Sum256(leaf.Raw)
	return tlsIdentity{
		pubKeyHash:  pubKeyHash[:],
		fingerprint: fingerprint[:],
	}, nil
}

func newTLSIdentity() (tlsIdentity, error) { // A
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tlsIdentity{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return tlsIdentity{}, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "ouroboros-carrier",
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(10 * time.Minute),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		priv.Public(),
		priv,
	)
	if err != nil {
		return tlsIdentity{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return tlsIdentity{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tlsIdentity{}, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tlsIdentity{}, err
	}
	certificate.Leaf = leaf
	pubKeyHash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	fingerprint := sha256.Sum256(leaf.Raw)
	return tlsIdentity{
		certificate: certificate,
		pubKeyHash:  pubKeyHash[:],
		fingerprint: fingerprint[:],
	}, nil
}

func cloneCerts( // A
	certs []interfaces.NodeCert,
) []interfaces.NodeCert {
	if certs == nil {
		return nil
	}
	out := make([]interfaces.NodeCert, len(certs))
	copy(out, certs)
	return out
}

func cloneByteSlices( // A
	slices [][]byte,
) [][]byte {
	if slices == nil {
		return nil
	}
	out := make([][]byte, len(slices))
	for i := range slices {
		out[i] = append([]byte(nil), slices[i]...)
	}
	return out
}

func clonePeer(peer interfaces.PeerNode) interfaces.PeerNode { // A
	return interfaces.PeerNode{
		NodeID:       peer.NodeID,
		Addresses:    append([]string(nil), peer.Addresses...),
		NodeCerts:    cloneCerts(peer.NodeCerts),
		CASignatures: cloneByteSlices(peer.CASignatures),
	}
}

func shouldReplaceConn( // A
	localNodeID keys.NodeID,
	peerNodeID keys.NodeID,
	existingRole connectionRole,
	incomingRole connectionRole,
) bool {
	preferred := preferredRole(localNodeID, peerNodeID)
	if incomingRole != preferred {
		return false
	}
	return existingRole != preferred
}

func preferredRole( // A
	localNodeID keys.NodeID,
	peerNodeID keys.NodeID,
) connectionRole {
	if bytes.Compare(localNodeID[:], peerNodeID[:]) < 0 {
		return connectionRoleOutbound
	}
	return connectionRoleInbound
}

func mergePeer( // A
	left interfaces.PeerNode,
	right interfaces.PeerNode,
) interfaces.PeerNode {
	merged := clonePeer(left)
	if merged.NodeID.IsZero() {
		merged.NodeID = right.NodeID
	}
	addressSet := make(map[string]struct{})
	for _, address := range merged.Addresses {
		addressSet[address] = struct{}{}
	}
	for _, address := range right.Addresses {
		if _, ok := addressSet[address]; ok {
			continue
		}
		merged.Addresses = append(merged.Addresses, address)
		addressSet[address] = struct{}{}
	}
	if len(merged.NodeCerts) == 0 {
		merged.NodeCerts = cloneCerts(right.NodeCerts)
	}
	if len(merged.CASignatures) == 0 {
		merged.CASignatures = cloneByteSlices(right.CASignatures)
	}
	sort.Strings(merged.Addresses)
	return merged
}
