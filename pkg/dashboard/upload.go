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
	"sync/atomic"
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

type uploadFlowState struct { // A
	ID        string
	StartedAt int64

	status      atomic.Value // string
	completedAt atomic.Int64
	vertexHash  atomic.Value // string
	blockHash   atomic.Value // string
	errorMsg    atomic.Value // string

	slicesMu          sync.RWMutex
	stages            []UploadFlowStage
	sliceDistribution []SliceDistribution
}

func (s *uploadFlowState) snapshot() UploadFlow { // A
	snap := UploadFlow{
		ID:        s.ID,
		StartedAt: s.StartedAt,
	}

	if v := s.status.Load(); v != nil {
		snap.Status = v.(string)
	}
	snap.CompletedAt = s.completedAt.Load()

	if v := s.vertexHash.Load(); v != nil {
		snap.VertexHash = v.(string)
	}
	if v := s.blockHash.Load(); v != nil {
		snap.BlockHash = v.(string)
	}
	if v := s.errorMsg.Load(); v != nil {
		snap.Error = v.(string)
	}

	s.slicesMu.RLock()
	if len(s.stages) > 0 {
		snap.Stages = append([]UploadFlowStage(nil), s.stages...)
	}
	if len(s.sliceDistribution) > 0 {
		snap.SliceDistribution = append(
			[]SliceDistribution(nil),
			s.sliceDistribution...,
		)
	}
	s.slicesMu.RUnlock()

	return snap
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
	flows sync.Map // map[string]*uploadFlowState
}

// NewUploadFlowTracker creates a new UploadFlowTracker.
func NewUploadFlowTracker() *UploadFlowTracker { // A
	return &UploadFlowTracker{}
}

// Create creates a new upload flow and returns its ID.
func (t *UploadFlowTracker) Create() *UploadFlow { // A
	id := generateFlowID()
	now := time.Now().UnixMilli()

	state := &uploadFlowState{
		ID:        id,
		StartedAt: now,
		stages: []UploadFlowStage{
			{Name: "encrypting", Status: "pending"},
			{Name: "wal", Status: "pending"},
			{Name: "sealing", Status: "pending"},
			{Name: "distributing", Status: "pending"},
			{Name: "complete", Status: "pending"},
		},
	}
	state.status.Store("started")

	t.flows.Store(id, state)

	flow := state.snapshot()
	return &flow
}

// Get retrieves an upload flow by ID.
func (t *UploadFlowTracker) Get(id string) (*UploadFlow, bool) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return nil, false
	}
	state := v.(*uploadFlowState)
	flow := state.snapshot()
	return &flow, true
}

// UpdateStatus updates the status of an upload flow.
func (t *UploadFlowTracker) UpdateStatus(id, status string) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	now := time.Now().UnixMilli()
	state := v.(*uploadFlowState)

	state.status.Store(status)

	// Update stage statuses
	state.slicesMu.Lock()
	defer state.slicesMu.Unlock()

	stageFound := false
	for i := range state.stages {
		if state.stages[i].Name == status {
			state.stages[i].Status = "in_progress"
			state.stages[i].StartedAt = now
			stageFound = true
		} else if !stageFound {
			state.stages[i].Status = "complete"
			if state.stages[i].CompletedAt == 0 {
				state.stages[i].CompletedAt = now
			}
		}
	}

	if status == "complete" {
		state.completedAt.Store(now)

		for i := range state.stages {
			state.stages[i].Status = "complete"
			if state.stages[i].CompletedAt == 0 {
				state.stages[i].CompletedAt = now
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
	state := v.(*uploadFlowState)
	state.vertexHash.Store(hash)
}

// SetBlockHash sets the block hash for an upload flow.
func (t *UploadFlowTracker) SetBlockHash(id, hash string) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	state := v.(*uploadFlowState)
	state.blockHash.Store(hash)
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
	state := v.(*uploadFlowState)
	state.slicesMu.Lock()
	state.sliceDistribution = append(state.sliceDistribution, dist)
	state.slicesMu.Unlock()
}

// UpdateSliceStatus updates the status of a slice distribution.
func (t *UploadFlowTracker) UpdateSliceStatus(
	id, sliceHash, status string,
) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	state := v.(*uploadFlowState)
	state.slicesMu.Lock()
	for i := range state.sliceDistribution {
		if state.sliceDistribution[i].SliceHash == sliceHash {
			state.sliceDistribution[i].Status = status
			break
		}
	}
	state.slicesMu.Unlock()
}

// SetError sets an error on the upload flow.
func (t *UploadFlowTracker) SetError(id, errMsg string) { // A
	v, ok := t.flows.Load(id)
	if !ok {
		return
	}
	state := v.(*uploadFlowState)
	state.status.Store("failed")
	state.errorMsg.Store(errMsg)
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

	writeJSON(w, http.StatusOK, *flow)
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
	flow, ok := d.uploadTracker.Get(flowID)
	if !ok {
		return
	}
	snapshot := append(
		[]SliceDistribution(nil),
		flow.SliceDistribution...,
	)
	for _, dist := range snapshot {
		time.Sleep(50 * time.Millisecond)
		d.uploadTracker.UpdateSliceStatus(
			flowID,
			dist.SliceHash,
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
