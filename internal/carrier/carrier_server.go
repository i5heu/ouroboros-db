package carrier

import (
	"context"
	"errors"
	"fmt"
)

// Start begins listening for incoming connections on the local node's address.
// It returns immediately; connections are handled in background goroutines.
func (c *DefaultCarrier) Start(ctx context.Context) error { // A
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()

	if c.running {
		return errors.New("carrier is already running")
	}

	if len(c.localNode.Addresses) == 0 {
		return errors.New("local node has no addresses to listen on")
	}

	// Use the first address for listening
	listenAddr := c.localNode.Addresses[0]

	listener, err := c.transport.Listen(ctx, listenAddr)
	if err != nil {
		return fmt.Errorf("failed to start listener on %s: %w", listenAddr, err)
	}

	c.listener = listener
	c.running = true
	c.stopCh = make(chan struct{})

	c.log.InfoContext(ctx, "carrier started listening",
		logKeyAddress, listenAddr)

	// Start the accept loop in a goroutine
	c.wg.Add(1)
	go c.acceptLoop(ctx)

	return nil
}

// Stop gracefully shuts down the carrier, closing all connections and
// stopping the listener.
func (c *DefaultCarrier) Stop(ctx context.Context) error { // A
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()

	if !c.running {
		return nil // Already stopped
	}

	c.log.InfoContext(ctx, "stopping carrier")

	// Signal the accept loop to stop
	close(c.stopCh)

	// Close the listener to unblock Accept()
	if c.listener != nil {
		if err := c.listener.Close(); err != nil {
			c.log.WarnContext(ctx, "error closing listener",
				logKeyError, err.Error())
		}
	}

	// Wait for all goroutines to finish
	c.wg.Wait()

	// Close the transport
	if err := c.transport.Close(); err != nil {
		c.log.WarnContext(ctx, "error closing transport",
			logKeyError, err.Error())
	}

	c.running = false
	c.listener = nil

	c.log.InfoContext(ctx, "carrier stopped")
	return nil
}

// acceptLoop continuously accepts incoming connections.
func (c *DefaultCarrier) acceptLoop(ctx context.Context) { // A
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		conn, err := c.listener.Accept(ctx)
		if err != nil {
			// Check if we're shutting down
			select {
			case <-c.stopCh:
				return
			default:
			}

			c.log.WarnContext(ctx, "error accepting connection",
				logKeyError, err.Error())
			continue
		}

		// Handle the connection in a new goroutine
		c.wg.Add(1)
		go c.handleConnection(ctx, conn)
	}
}

// handleConnection processes messages from an incoming connection.
func (c *DefaultCarrier) handleConnection( // A
	ctx context.Context,
	conn Connection,
) {
	defer c.wg.Done()
	defer func() {
		if err := conn.Close(); err != nil {
			c.log.DebugContext(ctx, "error closing connection",
				logKeyError, err.Error())
		}
	}()

	remoteID := conn.RemoteNodeID()
	c.log.DebugContext(ctx, "accepted connection",
		logKeyNodeID, string(remoteID))

	// Process messages until connection closes or carrier stops
	for {
		select {
		case <-c.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		if !c.processNextMessage(ctx, conn, remoteID) {
			return
		}
	}
}

// processNextMessage receives and handles a single message from a connection.
// Returns false if the connection should be closed.
func (c *DefaultCarrier) processNextMessage( // A
	ctx context.Context,
	conn Connection,
	remoteID NodeID,
) bool {
	msg, err := conn.Receive(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			c.log.DebugContext(ctx, "error receiving message",
				logKeyNodeID, string(remoteID),
				logKeyError, err.Error())
		}
		return false
	}

	response, err := c.dispatchMessage(ctx, remoteID, msg)
	if err != nil {
		c.log.WarnContext(ctx, "error handling message",
			logKeyNodeID, string(remoteID),
			logKeyMessageType, msg.Type.String(),
			logKeyError, err.Error())
		return true // continue processing despite handler error
	}

	if response != nil {
		if err := conn.Send(ctx, *response); err != nil {
			c.log.WarnContext(ctx, "error sending response",
				logKeyNodeID, string(remoteID),
				logKeyError, err.Error())
		}
	}
	return true
}

// dispatchMessage calls registered handlers for a message type.
func (c *DefaultCarrier) dispatchMessage( // A
	ctx context.Context,
	senderID NodeID,
	msg Message,
) (*Message, error) {
	c.handlersMu.RLock()
	handlers := c.handlers[msg.Type]
	c.handlersMu.RUnlock()

	if len(handlers) == 0 {
		c.log.DebugContext(ctx, "no handlers for message type",
			logKeyMessageType, msg.Type.String(),
			logKeyNodeID, string(senderID))
		return nil, nil
	}

	var lastResponse *Message
	for _, handler := range handlers {
		response, err := handler(ctx, senderID, msg)
		if err != nil {
			return nil, err
		}
		if response != nil {
			lastResponse = response
		}
	}

	return lastResponse, nil
}

// RegisterHandler registers a handler for a specific message type.
// Multiple handlers can be registered for the same message type; they will
// be called in order of registration.
func (c *DefaultCarrier) RegisterHandler( // A
	msgType MessageType,
	handler MessageHandler,
) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()

	c.handlers[msgType] = append(c.handlers[msgType], handler)
	c.log.DebugContext(context.Background(), "registered message handler",
		logKeyMessageType, msgType.String())
}

// IsRunning returns whether the carrier is currently running and accepting
// connections.
func (c *DefaultCarrier) IsRunning() bool { // A
	c.listenerMu.Lock()
	defer c.listenerMu.Unlock()
	return c.running
}
