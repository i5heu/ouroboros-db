package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// UploadFlow represents the state of an upload operation as it progresses
// through the storage pipeline.
type UploadFlow struct { // A
	ID                string              `json:"id"`
	Status            string              `json:"status"`
	StartedAt         int64               `json:"startedAt"`
	CompletedAt       int64               `json:"completedAt,omitempty"`
	Stages            []UploadFlowStage   `json:"stages"`
	VertexHash        string              `json:"vertexHash,omitempty"`
	BlockHash         string              `json:"blockHash,omitempty"`
	SliceDistribution []SliceDistribution `json:"sliceDistribution,omitempty"`
	Error             string              `json:"error,omitempty"`
}

// UploadFlowStage represents a single stage in the upload pipeline.
type UploadFlowStage struct { // A
	Name        string `json:"name"`
	Status      string `json:"status"`
	StartedAt   int64  `json:"startedAt,omitempty"`
	CompletedAt int64  `json:"completedAt,omitempty"`
	Details     string `json:"details,omitempty"`
}

// SliceDistribution tracks where a block slice was distributed.
type SliceDistribution struct { // A
	SliceHash  string `json:"sliceHash"`
	SliceIndex uint8  `json:"sliceIndex"`
	TargetNode string `json:"targetNode"`
	Status     string `json:"status"`
}

// UploadFlowTracker manages upload flow state.
// Uses sync.Map for concurrent access since each upload is independent.
type UploadFlowTracker struct { // A
	flows sync.Map // map[string]*UploadFlow
}

// NewUploadFlowTracker creates a new UploadFlowTracker.
func NewUploadFlowTracker() *UploadFlowTracker { // A
	return &UploadFlowTracker{}
}

// Create creates a new upload flow and returns its ID.
func (t *UploadFlowTracker) Create() *UploadFlow { // A
	id := generateFlowID()
	now := time.Now().UnixMilli()

	flow := &UploadFlow{
		ID:        id,
		Status:    "started",
		StartedAt: now,
		Stages: []UploadFlowStage{
			{Name: "encrypting", Status: "pending"},
			{Name: "wal", Status: "pending"},
			{Name: "sealing", Status: "pending"},
			{Name: "distributing", Status: "pending"},
			{Name: "complete", Status: "pending"},
		},
	}

	t.flows.Store(id, flow)
	return flow
}

// Get retrieves an upload flow by ID.
func (t *UploadFlowTracker) Get(id string) (*UploadFlow, bool) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*UploadFlow), true
}

// UpdateStatus updates the status of an upload flow.
func (t *UploadFlowTracker) UpdateStatus(id, status string) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	flow := v.(*UploadFlow)
	flow.Status = status

	// Update stage statuses
	now := time.Now().UnixMilli()
	stageFound := false
	for i := range flow.Stages {
		if flow.Stages[i].Name == status {
			flow.Stages[i].Status = "in_progress"
			flow.Stages[i].StartedAt = now
			stageFound = true
		} else if !stageFound {
			flow.Stages[i].Status = "complete"
			if flow.Stages[i].CompletedAt == 0 {
				flow.Stages[i].CompletedAt = now
			}
		}
	}

	if status == "complete" {
		flow.CompletedAt = now
		for i := range flow.Stages {
			flow.Stages[i].Status = "complete"
			if flow.Stages[i].CompletedAt == 0 {
				flow.Stages[i].CompletedAt = now
			}
		}
	}
}

// SetVertexHash sets the vertex hash for an upload flow.
func (t *UploadFlowTracker) SetVertexHash(id, hash string) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	flow := v.(*UploadFlow)
	flow.VertexHash = hash
}

// SetBlockHash sets the block hash for an upload flow.
func (t *UploadFlowTracker) SetBlockHash(id, hash string) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	flow := v.(*UploadFlow)
	flow.BlockHash = hash
}

// AddSliceDistribution adds a slice distribution entry.
func (t *UploadFlowTracker) AddSliceDistribution(
	id string,
	dist SliceDistribution,
) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	flow := v.(*UploadFlow)
	flow.SliceDistribution = append(flow.SliceDistribution, dist)
}

// UpdateSliceStatus updates the status of a slice distribution.
func (t *UploadFlowTracker) UpdateSliceStatus(
	id, sliceHash, status string,
) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	flow := v.(*UploadFlow)
	for i := range flow.SliceDistribution {
		if flow.SliceDistribution[i].SliceHash == sliceHash {
			flow.SliceDistribution[i].Status = status
			break
		}
	}
}

// SetError sets an error on the upload flow.
func (t *UploadFlowTracker) SetError(id, errMsg string) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	flow := v.(*UploadFlow)
	flow.Status = "failed"
	flow.Error = errMsg
}

// Delete removes an upload flow.
func (t *UploadFlowTracker) Delete(id string) { // A
	t.flows.Delete(id)
}

// generateFlowID generates a random flow ID.
func generateFlowID() string { // A
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// handleUpload handles POST /api/upload for file uploads.
func (d *Dashboard) handleUpload(w http.ResponseWriter, r *http.Request) { // A
	// Handle OPTIONS for CORS/feature detection
	if r.Method == http.MethodOptions {
		if d.config.AllowUpload {
			w.Header().Set("X-Upload-Enabled", "true")
		} else {
			w.Header().Set("X-Upload-Enabled", "false")
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !d.config.AllowUpload {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Upload not enabled. Use --UNSECURE-upload-via-dashboard flag.",
		})
		return
	}

	// Parse multipart form
	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Failed to parse form: " + err.Error(),
		})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "No file provided",
		})
		return
	}
	defer func() { _ = file.Close() }()

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to read file: " + err.Error(),
		})
		return
	}

	// Create upload flow
	flow := d.uploadTracker.Create()

	// Start upload processing in background
	go d.processUpload(flow.ID, header.Filename, content)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"id":      flow.ID,
		"message": "Upload started",
	})
}

// handleUploadRoutes handles /api/upload/{id}/... routes.
func (d *Dashboard) handleUploadRoutes(
	w http.ResponseWriter,
	r *http.Request,
) { // A
	// Extract upload ID and sub-path from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/upload/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Upload ID required", http.StatusBadRequest)
		return
	}

	uploadID := parts[0]

	if len(parts) == 2 && parts[1] == "flow" {
		d.handleGetUploadFlow(w, r, uploadID)
		return
	}

	http.NotFound(w, r)
}

// handleGetUploadFlow returns the current state of an upload flow.
func (d *Dashboard) handleGetUploadFlow(
	w http.ResponseWriter,
	r *http.Request,
	uploadID string,
) { // A
	flow, ok := d.uploadTracker.Get(uploadID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Upload not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, flow)
}

// processUpload simulates the upload pipeline stages.
// In a real implementation, this would interact with the CAS and distribution
// system.
func (d *Dashboard) processUpload(
	flowID, filename string,
	content []byte,
) { // A
	// Stage 1: Encrypting
	d.uploadTracker.UpdateStatus(flowID, "encrypting")
	time.Sleep(100 * time.Millisecond) // Simulate work

	// In real implementation: encrypt content, create sealed chunks
	// For now, generate a fake vertex hash
	vertexHash := generateFakeHash("vertex")
	d.uploadTracker.SetVertexHash(flowID, vertexHash)

	// Stage 2: WAL Buffering
	d.uploadTracker.UpdateStatus(flowID, "wal")
	time.Sleep(100 * time.Millisecond)

	// Stage 3: Block Sealing
	d.uploadTracker.UpdateStatus(flowID, "sealing")
	time.Sleep(100 * time.Millisecond)

	blockHash := generateFakeHash("block")
	d.uploadTracker.SetBlockHash(flowID, blockHash)

	// Stage 4: Distributing
	d.uploadTracker.UpdateStatus(flowID, "distributing")

	// Get nodes for distribution
	ctx := context.Background()
	nodes, err := d.config.Carrier.GetNodes(ctx)
	if err != nil || len(nodes) == 0 {
		// Use fake nodes for demo
		nodes = nil
	}

	// Simulate slice distribution (4 data + 2 parity = 6 slices)
	for i := 0; i < 6; i++ {
		sliceHash := generateFakeHash("slice")
		targetNode := "node-" + string(rune('A'+i))
		if nodes != nil && i < len(nodes) {
			targetNode = string(nodes[i].NodeID)
		}

		if i > math.MaxUint8 {
			continue
		}
		d.uploadTracker.AddSliceDistribution(flowID, SliceDistribution{
			SliceHash:  sliceHash,
			SliceIndex: uint8(i),
			TargetNode: targetNode,
			Status:     "pending",
		})
	}

	// Simulate slice confirmations
	flow, _ := d.uploadTracker.Get(flowID)
	for i := range flow.SliceDistribution {
		time.Sleep(50 * time.Millisecond)
		d.uploadTracker.UpdateSliceStatus(
			flowID,
			flow.SliceDistribution[i].SliceHash,
			"confirmed",
		)
	}

	// Stage 5: Complete
	d.uploadTracker.UpdateStatus(flowID, "complete")
}

// generateFakeHash generates a fake hash for demo purposes.
func generateFakeHash(prefix string) string { // A
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) { // A
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
