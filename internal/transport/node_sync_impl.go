package transport

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// nodeSyncImpl handles periodic registry sync
// between cluster peers.
type nodeSyncImpl struct { // A
	mu        sync.Mutex
	registry  NodeRegistry
	transport QuicTransport
	auth      *auth.CarrierAuth
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   bool
}

// NewNodeSync creates a new NodeSync instance.
func NewNodeSync( // A
	registry NodeRegistry,
	transport QuicTransport,
	carrierAuth *auth.CarrierAuth,
) NodeSync {
	return &nodeSyncImpl{
		registry:  registry,
		transport: transport,
		auth:      carrierAuth,
		stopCh:    make(chan struct{}),
	}
}

// StartSyncInterval begins periodic node list sync.
func (s *nodeSyncImpl) StartSyncInterval( // A
	d time.Duration,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}
	s.running = true
	s.wg.Add(1)
	go s.syncLoop(d)
}

// StopSync terminates the sync goroutine and waits
// for it to finish.
func (s *nodeSyncImpl) StopSync() { // A
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
}

// TriggerFullSync exchanges the node list with all
// connected peers.
func (s *nodeSyncImpl) TriggerFullSync() error { // A
	nodes := s.registry.GetAllNodes()
	var firstErr error
	for _, peer := range nodes {
		if err := s.ExchangeNodeList(
			peer.NodeID,
		); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ExchangeNodeList sends the local node list to a
// peer and processes the response.
func (s *nodeSyncImpl) ExchangeNodeList( // A
	peerID keys.NodeID,
) error {
	conns := s.transport.GetActiveConnections()
	var conn Connection
	for _, c := range conns {
		if c.NodeID() == peerID {
			conn = c
			break
		}
	}
	if conn == nil {
		return fmt.Errorf(
			"no connection to peer %s", peerID,
		)
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	// Send our node list
	allNodes := s.registry.GetAllNodes()
	data, err := json.Marshal(allNodes)
	if err != nil {
		return fmt.Errorf(
			"marshal node list: %w", err,
		)
	}

	syncMsg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: data,
	}
	if err := WriteMessage(
		stream, syncMsg,
	); err != nil {
		return fmt.Errorf(
			"write sync message: %w", err,
		)
	}

	return nil
}

// HandleNodeListUpdate processes a node list
// received from a peer and updates the local
// registry.
func (s *nodeSyncImpl) HandleNodeListUpdate( // A
	nodes []interfaces.NodeInfo,
) error {
	for _, info := range nodes {
		_, err := s.registry.GetNode(
			info.Peer.NodeID,
		)
		if err != nil {
			// Node not in registry; add it.
			addErr := s.registry.AddNode(
				info.Peer,
				info.CASignature,
				info.TrustScope,
			)
			if addErr != nil {
				return fmt.Errorf(
					"add synced node: %w", addErr,
				)
			}
		} else {
			// Node already known; update LastSeen.
			_ = s.registry.UpdateLastSeen(
				info.Peer.NodeID,
			)
		}
	}
	return nil
}

func (s *nodeSyncImpl) syncLoop( // A
	d time.Duration,
) {
	defer s.wg.Done()
	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			_ = s.TriggerFullSync()
		}
	}
}
