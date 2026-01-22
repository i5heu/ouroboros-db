package dashboard

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
)

// LogStreamMessage represents a log message sent to WebSocket clients.
type LogStreamMessage struct { // A
	Type         string            `json:"type"`
	SourceNodeID string            `json:"sourceNodeId"`
	Timestamp    int64             `json:"timestamp"`
	Level        string            `json:"level"`
	Message      string            `json:"message"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// Client represents a connected WebSocket client.
type Client struct { // A
	conn   *websocket.Conn
	sendCh chan []byte
}

// LogStreamHub manages WebSocket connections for log streaming.
// It uses a channel-based design with a single goroutine owning the clients
// map.
type LogStreamHub struct { // A
	// Channels for the hub goroutine
	registerCh   chan *Client
	unregisterCh chan *Client
	broadcastCh  chan LogStreamMessage
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewLogStreamHub creates a new LogStreamHub.
func NewLogStreamHub() *LogStreamHub { // A
	return &LogStreamHub{
		registerCh:   make(chan *Client, 16),
		unregisterCh: make(chan *Client, 16),
		broadcastCh:  make(chan LogStreamMessage, 256),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start begins the hub's event loop.
func (h *LogStreamHub) Start() { // A
	go h.run()
}

// Stop shuts down the hub.
func (h *LogStreamHub) Stop() { // A
	close(h.stopCh)
	<-h.doneCh
}

// Broadcast sends a log message to all connected clients.
func (h *LogStreamHub) Broadcast(msg LogStreamMessage) { // A
	msg.Type = "log"
	select {
	case h.broadcastCh <- msg:
	default:
		// Channel full, drop message
	}
}

// run is the main event loop - single owner of clients map.
func (h *LogStreamHub) run() { // A
	defer close(h.doneCh)

	clients := make(map[*Client]struct{})

	for {
		select {
		case client := <-h.registerCh:
			clients[client] = struct{}{}

		case client := <-h.unregisterCh:
			if _, ok := clients[client]; ok {
				delete(clients, client)
				close(client.sendCh)
			}

		case msg := <-h.broadcastCh:
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			for client := range clients {
				select {
				case client.sendCh <- data:
				default:
					// Client too slow, skip this message for them
				}
			}

		case <-h.stopCh:
			// Close all client connections
			for client := range clients {
				close(client.sendCh)
			}
			return
		}
	}
}

// handleWebSocket handles WebSocket connections for log streaming.
func (d *Dashboard) handleWebSocket(
	w http.ResponseWriter,
	r *http.Request,
) { // A
	websocket.Handler(func(conn *websocket.Conn) {
		d.serveWebSocket(conn)
	}).ServeHTTP(w, r)
}

// serveWebSocket handles a single WebSocket connection.
func (d *Dashboard) serveWebSocket(conn *websocket.Conn) { // A
	client := &Client{
		conn:   conn,
		sendCh: make(chan []byte, 64),
	}

	// Register client
	d.hub.registerCh <- client

	// Ensure cleanup on exit
	defer func() {
		d.hub.unregisterCh <- client
		_ = conn.Close()
	}()

	// Start writer goroutine
	go d.wsWriter(client)

	// Reader loop (to detect disconnection)
	d.wsReader(client)
}

// wsWriter sends messages from the send channel to the WebSocket.
func (d *Dashboard) wsWriter(client *Client) { // A
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-client.sendCh:
			if !ok {
				// Channel closed, connection is being closed
				return
			}
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := client.conn.Write(data); err != nil {
				return
			}

		case <-ticker.C:
			// Send ping/keepalive
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := client.conn.Write([]byte(`{"type":"ping"}`)); err != nil {
				return
			}
		}
	}
}

// wsReader reads from the WebSocket to detect disconnection.
func (d *Dashboard) wsReader(client *Client) { // A
	_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	for {
		var msg []byte
		err := websocket.Message.Receive(client.conn, &msg)
		if err != nil {
			return
		}
		// Reset read deadline on any message (including pongs)
		_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	}
}
