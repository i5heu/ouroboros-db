package carrier

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ConnCallback is called when a new outgoing connection is established.
// The callback can be used to start a receive loop for the connection.
type ConnCallback func(nodeID NodeID, conn Connection) // A

// connPool manages persistent connections to remote nodes.
// It provides connection reuse and automatic reconnection.
type connPool struct { // A
	transport     Transport
	localID       NodeID
	onNewOutgoing ConnCallback
	log           interface {
		Debug(msg string, args ...any)
		Warn(msg string, args ...any)
	}

	mu    sync.RWMutex
	conns map[NodeID]*pooledConn
}

// pooledConn wraps a Connection with metadata for pool management.
type pooledConn struct { // A
	conn       Connection
	nodeID     NodeID
	addresses  []string
	lastUsed   time.Time
	incoming   bool
	mu         sync.Mutex
	connecting bool
}

// newConnPool creates a new connection pool.
func newConnPool(transport Transport, localID NodeID, log interface { // A
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
},
) *connPool {
	return &connPool{
		transport:     transport,
		localID:       localID,
		log:           log,
		onNewOutgoing: nil,
		conns:         make(map[NodeID]*pooledConn),
	}
}

// SetOnNewOutgoing sets the callback for new outgoing connections.
// This allows the carrier to start receive loops for outgoing connections.
func (p *connPool) SetOnNewOutgoing(cb ConnCallback) { // A
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onNewOutgoing = cb
}

// SetLogger updates the logger used by the connection pool.
func (p *connPool) SetLogger(log interface {
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.log = log
}

// getOrConnect returns an existing connection to the node or establishes a new
// one. The returned connection is persistent and should NOT be closed by the
// caller.
func (p *connPool) getOrConnect(
	ctx context.Context,
	node Node,
) (Connection, error) { // A
	if node.NodeID == p.localID {
		return nil, fmt.Errorf("cannot connect to self")
	}

	// Fast path: check for existing connection
	if conn, ok := p.checkExistingConnection(node.NodeID); ok {
		return conn, nil
	}

	// Slow path: obtain pooled connection structure
	pc, conn := p.getOrCreatePooledConn(node)
	if conn != nil {
		return conn, nil
	}

	// Establish connection (outside lock to avoid blocking)
	return p.connectToNode(ctx, pc, node)
}

func (p *connPool) checkExistingConnection(nodeID NodeID) (Connection, bool) {
	p.mu.RLock()
	pc, exists := p.conns[nodeID]
	p.mu.RUnlock()

	// Use any valid connection, including incoming ones.
	// QUIC connections are bidirectional, so incoming connections
	// can be used for sending messages as well.
	if exists && pc.conn != nil && !isConnClosed(pc.conn) {
		pc.mu.Lock()
		pc.lastUsed = time.Now()
		pc.mu.Unlock()
		return pc.conn, true
	}
	return nil, false
}

func (p *connPool) getOrCreatePooledConn(
	node Node,
) (*pooledConn, Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	// Use any valid connection, including incoming ones.
	pc, exists := p.conns[node.NodeID]
	if exists && pc.conn != nil && !isConnClosed(pc.conn) {
		pc.mu.Lock()
		pc.lastUsed = time.Now()
		pc.mu.Unlock()
		return nil, pc.conn
	}

	// Create or update pooled connection entry
	if !exists {
		pc = &pooledConn{
			nodeID:    node.NodeID,
			addresses: node.Addresses,
		}
		p.conns[node.NodeID] = pc
	}

	return pc, nil
}

func (p *connPool) connectToNode(
	ctx context.Context,
	pc *pooledConn,
	node Node,
) (Connection, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Another goroutine may have connected while we waited.
	// Use any valid connection, including incoming ones.
	if pc.conn != nil && !isConnClosed(pc.conn) {
		pc.lastUsed = time.Now()
		return pc.conn, nil
	}

	// Mark as connecting to prevent concurrent connection attempts
	if pc.connecting {
		if conn := p.waitForConnection(pc); conn != nil {
			return conn, nil
		}
	}
	pc.connecting = true
	defer func() { pc.connecting = false }()

	return p.dialAddresses(ctx, pc, node)
}

func (p *connPool) waitForConnection(pc *pooledConn) Connection {
	// Wait for other goroutine to finish connecting
	pc.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	pc.mu.Lock()
	if pc.conn != nil && !isConnClosed(pc.conn) {
		return pc.conn
	}
	return nil
}

func (p *connPool) dialAddresses(
	ctx context.Context,
	pc *pooledConn,
	node Node,
) (Connection, error) {
	var lastErr error
	for _, addr := range node.Addresses {
		conn, err := p.transport.Connect(ctx, addr)
		if err != nil {
			lastErr = err
			p.log.Debug("connection attempt failed",
				logKeyNodeID, string(node.NodeID),
				logKeyAddress, addr,
				logKeyError, err.Error())
			continue
		}

		if pc.conn != nil && !isConnClosed(pc.conn) {
			_ = pc.conn.Close()
		}

		// Verify we connected to the right node
		if conn.RemoteNodeID() != node.NodeID {
			_ = conn.Close()
			lastErr = fmt.Errorf(
				"node ID mismatch: expected %s, got %s",
				node.NodeID,
				conn.RemoteNodeID(),
			)
			continue
		}

		pc.conn = conn
		pc.addresses = node.Addresses
		pc.lastUsed = time.Now()
		pc.incoming = false

		p.log.Debug("connection established",
			logKeyNodeID, string(node.NodeID),
			logKeyAddress, addr)

		// Notify callback about new outgoing connection (for receive loop)
		p.mu.RLock()
		cb := p.onNewOutgoing
		p.mu.RUnlock()
		if cb != nil {
			cb(node.NodeID, conn)
		}

		return conn, nil
	}

	return nil, fmt.Errorf(
		"failed to connect to node %s: %w",
		node.NodeID,
		lastErr,
	)
}

// addIncoming adds an incoming connection to the pool.
// This is called when a remote node connects to us.
func (p *connPool) addIncoming(conn Connection) { // A
	remoteID := conn.RemoteNodeID()
	if remoteID == "" || remoteID == p.localID {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if we already have a connection
	if existing, ok := p.conns[remoteID]; ok {
		if existing.conn != nil && !isConnClosed(existing.conn) {
			if !existing.incoming {
				_ = conn.Close()
				return
			}
			return
		}
	}

	p.conns[remoteID] = &pooledConn{
		conn:     conn,
		nodeID:   remoteID,
		lastUsed: time.Now(),
		incoming: true,
	}

	p.log.Debug("incoming connection added to pool",
		logKeyNodeID, string(remoteID))
}

// addOutgoing adds an outgoing connection to the pool that was created
// externally (e.g., during bootstrap). This allows reuse of the connection
// for subsequent requests.
// Note: This does NOT trigger the onNewOutgoing callback because bootstrap
// connections may need synchronous request-response before starting a
// receive loop. Use StartReceiveLoops() after bootstrap to enable
// bidirectional communication.
func (p *connPool) addOutgoing(nodeID NodeID, conn Connection) { // A
	if nodeID == "" || nodeID == p.localID {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if we already have a connection
	if existing, ok := p.conns[nodeID]; ok {
		if existing.conn != nil && !isConnClosed(existing.conn) {
			// Keep existing connection, close new one
			_ = conn.Close()
			return
		}
	}

	p.conns[nodeID] = &pooledConn{
		conn:     conn,
		nodeID:   nodeID,
		lastUsed: time.Now(),
	}

	p.log.Debug("outgoing connection added to pool",
		logKeyNodeID, string(nodeID))
}

// StartReceiveLoops starts receive loops for all existing outgoing connections
// in the pool. This should be called after bootstrap is complete.
func (p *connPool) StartReceiveLoops() { // A
	p.mu.RLock()
	cb := p.onNewOutgoing
	var outgoingConns []struct {
		nodeID NodeID
		conn   Connection
	}
	for nodeID, pc := range p.conns {
		if pc.conn != nil && !pc.incoming && !isConnClosed(pc.conn) {
			outgoingConns = append(outgoingConns, struct {
				nodeID NodeID
				conn   Connection
			}{nodeID, pc.conn})
		}
	}
	p.mu.RUnlock()

	if cb == nil {
		return
	}

	for _, oc := range outgoingConns {
		cb(oc.nodeID, oc.conn)
	}
}

// remove removes a connection from the pool and closes it.
func (p *connPool) remove(nodeID NodeID) { // A
	p.mu.Lock()
	pc, exists := p.conns[nodeID]
	if exists {
		delete(p.conns, nodeID)
	}
	p.mu.Unlock()

	if exists && pc.conn != nil {
		_ = pc.conn.Close()
	}
}

func (p *connPool) removeIfMatch(nodeID NodeID, conn Connection) { // A
	p.mu.Lock()
	pc, exists := p.conns[nodeID]
	if exists && pc.conn == conn {
		delete(p.conns, nodeID)
	}
	p.mu.Unlock()

	if exists && pc.conn == conn && pc.conn != nil {
		_ = pc.conn.Close()
	}
}

// closeAll closes all connections in the pool.
func (p *connPool) closeAll() { // A
	p.mu.Lock()
	conns := make([]*pooledConn, 0, len(p.conns))
	for _, pc := range p.conns {
		conns = append(conns, pc)
	}
	p.conns = make(map[NodeID]*pooledConn)
	p.mu.Unlock()

	for _, pc := range conns {
		if pc.conn != nil {
			_ = pc.conn.Close()
		}
	}
}

// isConnClosed checks if a connection is closed.
// This is a helper that checks the IsClosed method if available.
func isConnClosed(conn Connection) bool { // A
	if closer, ok := conn.(interface{ IsClosed() bool }); ok {
		return closer.IsClosed()
	}
	return false
}
