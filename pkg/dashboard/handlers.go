package dashboard

import (
	"context"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/internal/carrier"
)

// NodeInfo represents information about a cluster node for the API.
type NodeInfo struct { // A
	NodeID      string   `json:"nodeId"`
	Addresses   []string `json:"addresses"`
	VertexCount int      `json:"vertexCount"`
	BlockCount  int      `json:"blockCount"`
	SliceCount  int      `json:"sliceCount"`
	Available   bool     `json:"available"`
}

// VertexInfo represents vertex information for the API.
type VertexInfo struct { // A
	Hash          string   `json:"hash"`
	Parent        string   `json:"parent"`
	Created       int64    `json:"created"`
	ChunkCount    int      `json:"chunkCount"`
	StoredOnNodes []string `json:"storedOnNodes,omitempty"`
}

// VertexListResponse is the response for listing vertices.
type VertexListResponse struct { // A
	Vertices []VertexInfo `json:"vertices"`
	Total    int          `json:"total"`
	Offset   int          `json:"offset"`
	Limit    int          `json:"limit"`
	HasMore  bool         `json:"hasMore"`
}

// BlockInfo represents block information for the API.
type BlockInfo struct { // A
	Hash        string   `json:"hash"`
	Created     int64    `json:"created"`
	ChunkCount  int      `json:"chunkCount"`
	VertexCount int      `json:"vertexCount"`
	Status      string   `json:"status"`
	Nodes       []string `json:"nodes,omitempty"`
}

// BlockListResponse is the response for listing blocks.
type BlockListResponse struct { // A
	Blocks  []BlockInfo `json:"blocks"`
	Total   int         `json:"total"`
	Offset  int         `json:"offset"`
	Limit   int         `json:"limit"`
	HasMore bool        `json:"hasMore"`
}

// DistributionOverview provides cluster-wide distribution statistics.
type DistributionOverview struct { // A
	TotalNodes       int                `json:"totalNodes"`
	TotalVertices    int                `json:"totalVertices"`
	TotalBlocks      int                `json:"totalBlocks"`
	TotalSlices      int                `json:"totalSlices"`
	NodeDistribution []NodeDistribution `json:"nodeDistribution"`
}

// NodeDistribution shows data distribution for a single node.
type NodeDistribution struct { // A
	NodeID       string `json:"nodeId"`
	Address      string `json:"address"`
	VertexCount  int    `json:"vertexCount"`
	BlockCount   int    `json:"blockCount"`
	SliceCount   int    `json:"sliceCount"`
	StorageBytes int64  `json:"storageBytes"`
}

// handleGetNodes returns all cluster nodes.
// GET /api/nodes
// The local node is always returned first.
func (d *Dashboard) handleGetNodes(
	w http.ResponseWriter,
	r *http.Request,
) { // A
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	nodes, err := d.config.Carrier.GetNodes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to get nodes: " + err.Error(),
		})
		return
	}

	// Get local node ID to put it first
	localNode := d.config.Carrier.LocalNode()
	localNodeID := localNode.NodeID

	nodeInfos := make([]NodeInfo, 0, len(nodes))

	// Add local node first
	for _, n := range nodes {
		if n.NodeID == localNodeID {
			nodeInfos = append(nodeInfos, NodeInfo{
				NodeID:    string(n.NodeID),
				Addresses: n.Addresses,
				Available: true, // TODO: Check actual availability
			})
			break
		}
	}

	// Add remaining nodes
	for _, n := range nodes {
		if n.NodeID != localNodeID {
			nodeInfos = append(nodeInfos, NodeInfo{
				NodeID:    string(n.NodeID),
				Addresses: n.Addresses,
				Available: true, // TODO: Check actual availability
			})
		}
	}

	writeJSON(w, http.StatusOK, nodeInfos)
}

// handleNodeRoutes handles /api/nodes/{nodeID}/... routes.
// POST /api/nodes/{nodeID}/logs/subscribe
// POST /api/nodes/{nodeID}/logs/unsubscribe
func (d *Dashboard) handleNodeRoutes(
	w http.ResponseWriter,
	r *http.Request,
) { // A
	// Extract node ID and sub-path from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
	parts := strings.SplitN(path, "/", 3)

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Node ID required", http.StatusBadRequest)
		return
	}

	nodeID := carrier.NodeID(parts[0])

	// Check for logs/subscribe or logs/unsubscribe
	if len(parts) >= 3 && parts[1] == "logs" {
		switch parts[2] {
		case "subscribe":
			d.handleLogSubscribe(w, r, nodeID)
			return
		case "unsubscribe":
			d.handleLogUnsubscribe(w, r, nodeID)
			return
		}
	}

	http.NotFound(w, r)
}

// handleLogSubscribe subscribes to logs from a specific node.
func (d *Dashboard) handleLogSubscribe( // A
	w http.ResponseWriter,
	r *http.Request,
	nodeID carrier.NodeID,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get our local node ID directly from the carrier
	localNode := d.config.Carrier.LocalNode()
	localNodeID := localNode.NodeID

	// If subscribing to local node, use LogBroadcaster directly
	if nodeID == localNodeID && d.config.LogBroadcaster != nil {
		d.config.LogBroadcaster.Subscribe(localNodeID, nil)
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "Subscribed to local logs",
		})
		return
	}

	// Verify target node exists before sending
	nodes, err := d.config.Carrier.GetNodes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to get nodes: " + err.Error(),
		})
		return
	}

	nodeExists := false
	for _, n := range nodes {
		if n.NodeID == nodeID {
			nodeExists = true
			break
		}
	}
	if !nodeExists {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Node not found: " + string(nodeID),
		})
		return
	}

	// Send subscription request to remote target node
	payload := carrier.LogSubscribePayload{
		SubscriberNodeID: localNodeID,
		Levels:           nil, // All levels
	}

	data, err := carrier.SerializeLogSubscribe(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to serialize request",
		})
		return
	}

	msg := carrier.Message{
		Type:    carrier.MessageTypeLogSubscribe,
		Payload: data,
	}

	err = d.config.Carrier.SendMessageToNode(ctx, nodeID, msg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to send subscription: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Subscribed to logs from " + string(nodeID),
	})
}

// handleLogUnsubscribe unsubscribes from logs from a specific node.
func (d *Dashboard) handleLogUnsubscribe( // A
	w http.ResponseWriter,
	r *http.Request,
	nodeID carrier.NodeID,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get our local node ID directly from the carrier
	localNode := d.config.Carrier.LocalNode()
	localNodeID := localNode.NodeID

	// If unsubscribing from local node, use LogBroadcaster directly
	if nodeID == localNodeID && d.config.LogBroadcaster != nil {
		d.config.LogBroadcaster.Unsubscribe(localNodeID)
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "Unsubscribed from local logs",
		})
		return
	}

	// Send unsubscription request to remote node
	payload := carrier.LogUnsubscribePayload{
		SubscriberNodeID: localNodeID,
	}

	data, err := carrier.SerializeLogUnsubscribe(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to serialize request",
		})
		return
	}

	msg := carrier.Message{
		Type:    carrier.MessageTypeLogUnsubscribe,
		Payload: data,
	}

	err = d.config.Carrier.SendMessageToNode(ctx, nodeID, msg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to send unsubscription: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Unsubscribed from logs from " + string(nodeID),
	})
}

// handleGetVertices returns a paginated list of vertices.
// GET /api/vertices?offset=0&limit=20
func (d *Dashboard) handleGetVertices(
	w http.ResponseWriter,
	r *http.Request,
) { // A
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse pagination params
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// In a real implementation, this would query the index
	// For now, return an empty response
	// TODO: Implement when index provides vertex listing

	response := VertexListResponse{
		Vertices: []VertexInfo{},
		Total:    0,
		Offset:   offset,
		Limit:    limit,
		HasMore:  false,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetVertex returns details for a specific vertex.
// GET /api/vertices/{hash}
func (d *Dashboard) handleGetVertex( // A
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract hash from URL
	hashStr := strings.TrimPrefix(r.URL.Path, "/api/vertices/")
	if hashStr == "" {
		http.Error(w, "Vertex hash required", http.StatusBadRequest)
		return
	}

	// If CAS is not configured, return not found
	if d.config.CAS == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Vertex not found (CAS not configured)",
		})
		return
	}

	// Parse the hash string into a hash.Hash
	// hash.Hash is SHA-512: 64 bytes = 128 hex characters
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil || len(hashBytes) != 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid vertex hash format (expected 128 hex chars)",
		})
		return
	}

	var vertexHash hash.Hash
	copy(vertexHash[:], hashBytes)

	// Retrieve vertex from CAS
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	vertex, err := d.config.CAS.GetVertex(ctx, vertexHash)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Vertex not found: " + err.Error(),
		})
		return
	}

	// Convert to API response format
	info := VertexInfo{
		Hash:       vertex.Hash.String(),
		Parent:     vertex.Parent.String(),
		Created:    vertex.Created,
		ChunkCount: len(vertex.ChunkHashes),
	}

	writeJSON(w, http.StatusOK, info)
}

// handleGetBlocks returns a paginated list of blocks.
// GET /api/blocks?offset=0&limit=20
func (d *Dashboard) handleGetBlocks(
	w http.ResponseWriter,
	r *http.Request,
) { // A
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse pagination params
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// In a real implementation, this would query the block store
	// For now, return an empty response
	// TODO: Implement when BlockStore provides block listing

	response := BlockListResponse{
		Blocks:  []BlockInfo{},
		Total:   0,
		Offset:  offset,
		Limit:   limit,
		HasMore: false,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleBlockRoutes handles /api/blocks/{hash}/... routes.
// GET /api/blocks/{hash}
// GET /api/blocks/{hash}/slices
func (d *Dashboard) handleBlockRoutes(
	w http.ResponseWriter,
	r *http.Request,
) { // A
	// Extract hash and sub-path from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/blocks/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Block hash required", http.StatusBadRequest)
		return
	}

	hashStr := parts[0]

	if len(parts) == 1 {
		// GET /api/blocks/{hash}
		d.handleGetBlock(w, r, hashStr)
		return
	}

	if parts[1] == "slices" {
		// GET /api/blocks/{hash}/slices
		d.handleGetBlockSlices(w, r, hashStr)
		return
	}

	http.NotFound(w, r)
}

// handleGetBlock returns details for a specific block.
func (d *Dashboard) handleGetBlock(
	w http.ResponseWriter,
	r *http.Request,
	hashStr string,
) { // A
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If BlockStore is not configured, return not found
	if d.config.BlockStore == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Block not found (BlockStore not configured)",
		})
		return
	}

	// Parse the hash string into a hash.Hash
	// hash.Hash is SHA-512: 64 bytes = 128 hex characters
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil || len(hashBytes) != 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid block hash format (expected 128 hex chars)",
		})
		return
	}

	var blockHash hash.Hash
	copy(blockHash[:], hashBytes)

	// Retrieve block from BlockStore
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	block, err := d.config.BlockStore.GetBlock(ctx, blockHash)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Block not found: " + err.Error(),
		})
		return
	}

	// Convert to API response format
	info := BlockInfo{
		Hash:        block.Hash.String(),
		Created:     block.Header.Created,
		ChunkCount:  int(block.Header.ChunkCount),
		VertexCount: int(block.Header.VertexCount),
		Status:      "sealed",
	}

	writeJSON(w, http.StatusOK, info)
}

// handleGetBlockSlices returns the slices for a specific block.
func (d *Dashboard) handleGetBlockSlices(
	w http.ResponseWriter,
	r *http.Request,
	hashStr string,
) { // A
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// In a real implementation, this would query the distribution tracker
	// TODO: Implement when DistributionTracker is available

	writeJSON(w, http.StatusOK, []interface{}{})
}

// handleGetDistribution returns cluster-wide distribution statistics.
// GET /api/distribution
func (d *Dashboard) handleGetDistribution( // A
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	nodes, err := d.config.Carrier.GetNodes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to get nodes: " + err.Error(),
		})
		return
	}

	// Get counts from UploadCounter if available
	var totalVertices, totalBlocks, totalSlices int
	if d.config.UploadCounter != nil {
		totalVertices, totalBlocks, totalSlices = d.config.UploadCounter.GetCounts()
	}

	// Build distribution overview
	nodeDistribution := make([]NodeDistribution, 0, len(nodes))
	for _, n := range nodes {
		addr := ""
		if len(n.Addresses) > 0 {
			addr = n.Addresses[0]
		}
		nodeDistribution = append(nodeDistribution, NodeDistribution{
			NodeID:  string(n.NodeID),
			Address: addr,
			// Per-node counts would require distributed queries
		})
	}

	overview := DistributionOverview{
		TotalNodes:       len(nodes),
		TotalVertices:    totalVertices,
		TotalBlocks:      totalBlocks,
		TotalSlices:      totalSlices,
		NodeDistribution: nodeDistribution,
	}

	writeJSON(w, http.StatusOK, overview)
}
