package dashboard

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/i5heu/ouroboros-db/internal/carrier"
)

//go:embed static/*
var staticFiles embed.FS

// Dashboard provides a debug web interface for OuroborosDB clusters.
// It allows viewing cluster state, log streaming, and optionally uploading data.
//
// WARNING: This is an UNSECURE debug tool. Do not use in production.
type Dashboard struct { // A
	config Config
	server *http.Server
	mux    *http.ServeMux

	// actualPort stores the port we're actually listening on
	actualPort atomic.Uint32

	// hub manages WebSocket connections for log streaming
	hub *LogStreamHub

	// uploadTracker tracks upload flow progress
	uploadTracker *UploadFlowTracker

	// stopCh signals shutdown
	stopCh chan struct{}
	// doneCh signals shutdown complete
	doneCh chan struct{}
}

// New creates a new Dashboard instance.
// The dashboard must be started with Start() before it will serve requests.
func New(cfg Config) (*Dashboard, error) { // A
	if cfg.Carrier == nil {
		return nil, errors.New("dashboard: carrier is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	d := &Dashboard{
		config:        cfg,
		mux:           http.NewServeMux(),
		hub:           NewLogStreamHub(),
		uploadTracker: NewUploadFlowTracker(),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}

	d.setupRoutes()

	return d, nil
}

// setupRoutes configures the HTTP routes for the dashboard.
func (d *Dashboard) setupRoutes() { // A
	// Static files (HTML, CSS, JS)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		d.config.Logger.Error("failed to create static fs", "error", err)
	} else {
		d.mux.Handle("/", http.FileServer(http.FS(staticFS)))
	}

	// API routes
	d.mux.HandleFunc("/api/nodes", d.handleGetNodes)
	d.mux.HandleFunc("/api/nodes/", d.handleNodeRoutes)
	d.mux.HandleFunc("/api/vertices", d.handleGetVertices)
	d.mux.HandleFunc("/api/vertices/", d.handleGetVertex)
	d.mux.HandleFunc("/api/blocks", d.handleGetBlocks)
	d.mux.HandleFunc("/api/blocks/", d.handleBlockRoutes)
	d.mux.HandleFunc("/api/distribution", d.handleGetDistribution)

	// Upload routes (only if enabled)
	d.mux.HandleFunc("/api/upload", d.handleUpload)
	d.mux.HandleFunc("/api/upload/", d.handleUploadRoutes)

	// WebSocket for log streaming
	d.mux.HandleFunc("/ws/logs", d.handleWebSocket)
}

// Start begins serving the dashboard on an available port.
// It announces the dashboard address to the cluster via carrier.
func (d *Dashboard) Start(ctx context.Context) error { // A
	if !d.config.Enabled {
		return nil
	}

	// Find an available port
	port, listener, err := d.findAvailablePort()
	if err != nil {
		return fmt.Errorf("dashboard: find port: %w", err)
	}

	d.actualPort.Store(uint32(port))

	d.server = &http.Server{
		Handler:           d.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the hub for WebSocket connections
	d.hub.Start()

	// Start HTTP server
	go func() {
		d.config.Logger.Info("dashboard started",
			"address", d.Address(),
			"uploadEnabled", d.config.AllowUpload)

		if err := d.server.Serve(listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			d.config.Logger.Error("dashboard server error", "error", err)
		}
		close(d.doneCh)
	}()

	// Announce dashboard to cluster
	d.announceDashboard(ctx)

	// Register handler for incoming log entries
	if d.config.Carrier != nil {
		d.config.Carrier.RegisterHandler(
			carrier.MessageTypeLogEntry,
			d.handleIncomingLogEntry,
		)
	}

	return nil
}

// Stop gracefully shuts down the dashboard.
func (d *Dashboard) Stop(ctx context.Context) error { // A
	if d.server == nil {
		return nil
	}

	close(d.stopCh)

	// Stop the WebSocket hub
	d.hub.Stop()

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := d.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("dashboard: shutdown: %w", err)
	}

	<-d.doneCh
	return nil
}

// Address returns the address the dashboard is listening on.
// Returns empty string if not started.
func (d *Dashboard) Address() string { // A
	port := d.actualPort.Load()
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

// Port returns the port the dashboard is listening on.
// Returns 0 if not started.
func (d *Dashboard) Port() uint16 { // A
	return uint16(d.actualPort.Load())
}

// findAvailablePort tries to find an available port, starting with the
// preferred port if configured.
func (d *Dashboard) findAvailablePort() (uint16, net.Listener, error) { // A
	// Try preferred port first
	preferredPort := d.config.PreferredPort
	if preferredPort == 0 {
		preferredPort = DefaultPort
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", preferredPort))
	if err == nil {
		return preferredPort, listener, nil
	}

	// Preferred port unavailable, let OS assign one
	listener, err = net.Listen("tcp", ":0")
	if err != nil {
		return 0, nil, fmt.Errorf("listen on any port: %w", err)
	}

	addr := listener.Addr().(*net.TCPAddr)
	return uint16(addr.Port), listener, nil
}

// announceDashboard broadcasts the dashboard address to the cluster.
func (d *Dashboard) announceDashboard(ctx context.Context) { // A
	if d.config.Carrier == nil {
		return
	}

	// Get local node ID from carrier
	nodes, err := d.config.Carrier.GetNodes(ctx)
	if err != nil || len(nodes) == 0 {
		return
	}

	// Find local node (first node is typically self)
	var localNodeID carrier.NodeID
	for _, n := range nodes {
		localNodeID = n.NodeID
		break
	}

	payload := carrier.DashboardAnnouncePayload{
		NodeID:           localNodeID,
		DashboardAddress: d.Address(),
	}

	data, err := carrier.SerializeDashboardAnnounce(payload)
	if err != nil {
		d.config.Logger.Warn("failed to serialize dashboard announce",
			"error", err)
		return
	}

	msg := carrier.Message{
		Type:    carrier.MessageTypeDashboardAnnounce,
		Payload: data,
	}

	_, err = d.config.Carrier.Broadcast(ctx, msg)
	if err != nil {
		d.config.Logger.Warn("failed to announce dashboard", "error", err)
	}
}

// handleIncomingLogEntry processes log entries received from other nodes
// and forwards them to connected WebSocket clients.
func (d *Dashboard) handleIncomingLogEntry(
	ctx context.Context,
	senderID carrier.NodeID,
	msg carrier.Message,
) (*carrier.Message, error) { // A
	entry, err := carrier.DeserializeLogEntry(msg.Payload)
	if err != nil {
		return nil, err
	}

	// Forward to WebSocket hub
	d.hub.Broadcast(LogStreamMessage{
		SourceNodeID: string(entry.SourceNodeID),
		Timestamp:    entry.Timestamp,
		Level:        entry.Level,
		Message:      entry.Message,
		Attributes:   entry.Attributes,
	})

	return nil, nil
}
