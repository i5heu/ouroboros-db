package carrier

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// connPool manages persistent connections to remote nodes.
// It provides connection reuse and automatic reconnection.
type connPool struct { // A
	transport Transport
	localID   NodeID
	log       interface {
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
	mu         sync.Mutex
	connecting bool
}

// newConnPool creates a new connection pool.
func newConnPool(transport Transport, localID NodeID, log interface { // A
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}) *connPool {
	return &connPool{
		transport: transport,
		localID:   localID,
		log:       log,
		conns:     make(map[NodeID]*pooledConn),
	}
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
	p.mu.RLock()
	pc, exists := p.conns[node.NodeID]
	p.mu.RUnlock()

	if exists && pc.conn != nil && !isConnClosed(pc.conn) {
		pc.mu.Lock()
		pc.lastUsed = time.Now()
		pc.mu.Unlock()
		return pc.conn, nil
	}

	// Slow path: need to establish connection
	p.mu.Lock()
	// Double-check after acquiring write lock
	pc, exists = p.conns[node.NodeID]
	if exists && pc.conn != nil && !isConnClosed(pc.conn) {
		p.mu.Unlock()
		pc.mu.Lock()
		pc.lastUsed = time.Now()
		pc.mu.Unlock()
		return pc.conn, nil
	}

	// Create or update pooled connection entry
	if !exists {
		pc = &pooledConn{
			nodeID:    node.NodeID,
			addresses: node.Addresses,
		}
		p.conns[node.NodeID] = pc
	}
	p.mu.Unlock()

	// Establish connection (outside lock to avoid blocking)
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Another goroutine may have connected while we waited
	if pc.conn != nil && !isConnClosed(pc.conn) {
		pc.lastUsed = time.Now()
		return pc.conn, nil
	}

	// Mark as connecting to prevent concurrent connection attempts
	if pc.connecting {
		// Wait for other goroutine to finish connecting
		pc.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		pc.mu.Lock()
		if pc.conn != nil && !isConnClosed(pc.conn) {
			return pc.conn, nil
		}
	}
	pc.connecting = true
	defer func() { pc.connecting = false }()

	// Try each address
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

		// Verify we connected to the right node
		if conn.RemoteNodeID() != node.NodeID {
			conn.Close()
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

		p.log.Debug("connection established",
			logKeyNodeID, string(node.NodeID),
			logKeyAddress, addr)

		return conn, nil
	}

	return nil, fmt.Errorf("failed to connect to node %s: %w", node.NodeID, lastErr)
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
			// Keep existing connection, close new one
			// (prefer connections we initiated)
			conn.Close()
			return
		}
	}

	p.conns[remoteID] = &pooledConn{
		conn:     conn,
		nodeID:   remoteID,
		lastUsed: time.Now(),
	}

	p.log.Debug("incoming connection added to pool",
		logKeyNodeID, string(remoteID))
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
		pc.conn.Close()
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
			pc.conn.Close()
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
