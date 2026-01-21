package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-db/internal/carrier"
)

// mockCarrier implements carrier.Carrier for testing.
type mockCarrier struct { // A
	nodes    []carrier.Node
	handlers map[carrier.MessageType][]carrier.MessageHandler
}

func newMockCarrier() *mockCarrier { // A
	return &mockCarrier{
		nodes: []carrier.Node{
			{
				NodeID:    carrier.NodeID("test-node-1"),
				Addresses: []string{"127.0.0.1:4242"},
			},
			{
				NodeID:    carrier.NodeID("test-node-2"),
				Addresses: []string{"127.0.0.1:4243"},
			},
		},
		handlers: make(map[carrier.MessageType][]carrier.MessageHandler),
	}
}

func (m *mockCarrier) GetNodes(ctx context.Context) ([]carrier.Node, error) { // A
	return m.nodes, nil
}

func (m *mockCarrier) Broadcast(
	ctx context.Context,
	message carrier.Message,
) (*carrier.BroadcastResult, error) { // A
	return &carrier.BroadcastResult{}, nil
}

func (m *mockCarrier) SendMessageToNode(
	ctx context.Context,
	nodeID carrier.NodeID,
	message carrier.Message,
) error { // A
	return nil
}

func (m *mockCarrier) JoinCluster(
	ctx context.Context,
	clusterNode carrier.Node,
	cert carrier.NodeCert,
) error { // A
	return nil
}

func (m *mockCarrier) LeaveCluster(ctx context.Context) error { // A
	return nil
}

func (m *mockCarrier) Start(ctx context.Context) error { // A
	return nil
}

func (m *mockCarrier) Stop(ctx context.Context) error { // A
	return nil
}

func (m *mockCarrier) RegisterHandler(
	msgType carrier.MessageType,
	handler carrier.MessageHandler,
) { // A
	m.handlers[msgType] = append(m.handlers[msgType], handler)
}

func (m *mockCarrier) BootstrapFromAddresses(
	ctx context.Context,
	addresses []string,
) error { // A
	return nil
}

func TestDashboard_New(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: false,
		Carrier:     mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if d == nil {
		t.Fatal("New() returned nil")
	}
}

func TestDashboard_New_RequiresCarrier(t *testing.T) { // A
	_, err := New(Config{
		Enabled: true,
	})
	if err == nil {
		t.Fatal("Expected error when carrier is nil")
	}
}

func TestDashboard_StartStop(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: false,
		Carrier:     mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()

	err = d.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	if d.Port() == 0 {
		t.Error("Port should be non-zero after Start")
	}

	addr := d.Address()
	if addr == "" {
		t.Error("Address should be non-empty after Start")
	}

	err = d.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestDashboard_DisabledNoStart(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: false,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()

	// Start should be a no-op when disabled
	err = d.Start(ctx)
	if err != nil {
		t.Fatalf("Start() should not fail when disabled: %v", err)
	}

	if d.Port() != 0 {
		t.Error("Port should be zero when disabled")
	}
}

func TestHandler_GetNodes(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	w := httptest.NewRecorder()

	d.handleGetNodes(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var nodes []NodeInfo
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}
}

func TestHandler_GetNodes_MethodNotAllowed(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/nodes", nil)
	w := httptest.NewRecorder()

	d.handleGetNodes(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandler_GetVertices(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/vertices?offset=0&limit=10", nil)
	w := httptest.NewRecorder()

	d.handleGetVertices(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response VertexListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Limit != 10 {
		t.Errorf("Expected limit 10, got %d", response.Limit)
	}
}

func TestHandler_GetBlocks(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/blocks", nil)
	w := httptest.NewRecorder()

	d.handleGetBlocks(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandler_GetDistribution(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/distribution", nil)
	w := httptest.NewRecorder()

	d.handleGetDistribution(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var overview DistributionOverview
	if err := json.NewDecoder(resp.Body).Decode(&overview); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if overview.TotalNodes != 2 {
		t.Errorf("Expected 2 nodes, got %d", overview.TotalNodes)
	}
}

func TestHandler_Upload_Disabled(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: false, // Disabled
		Carrier:     mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a simple multipart request
	body := strings.NewReader("--boundary\r\n" +
		"Content-Disposition: form-data; name=\"file\"; filename=\"test.txt\"\r\n" +
		"Content-Type: text/plain\r\n\r\n" +
		"test content\r\n" +
		"--boundary--\r\n")

	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	w := httptest.NewRecorder()

	d.handleUpload(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 when upload disabled, got %d", w.Code)
	}
}

func TestHandler_Upload_Options(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: true,
		Carrier:     mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodOptions, "/api/upload", nil)
	w := httptest.NewRecorder()

	d.handleUpload(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for OPTIONS, got %d", w.Code)
	}

	if w.Header().Get("X-Upload-Enabled") != "true" {
		t.Error("Expected X-Upload-Enabled header to be true")
	}
}

func TestHandler_NodeRoutes_Subscribe(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/nodes/test-node-2/logs/subscribe",
		nil,
	)
	w := httptest.NewRecorder()

	d.handleNodeRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
		body, _ := io.ReadAll(w.Body)
		t.Logf("Response body: %s", body)
	}
}

func TestHandler_NodeRoutes_Unsubscribe(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/nodes/test-node-2/logs/unsubscribe",
		nil,
	)
	w := httptest.NewRecorder()

	d.handleNodeRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandler_BlockRoutes(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled: true,
		Carrier: mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test get block
	req := httptest.NewRequest(http.MethodGet, "/api/blocks/abc123", nil)
	w := httptest.NewRecorder()

	d.handleBlockRoutes(w, req)

	// Should return 404 since no blocks exist
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent block, got %d", w.Code)
	}

	// Test get block slices
	req = httptest.NewRequest(http.MethodGet, "/api/blocks/abc123/slices", nil)
	w = httptest.NewRecorder()

	d.handleBlockRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for slices, got %d", w.Code)
	}
}

func TestHandler_UploadRoutes_GetFlow(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: true,
		Carrier:     mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a flow first
	flow := d.uploadTracker.Create()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/upload/"+flow.ID+"/flow",
		nil,
	)
	w := httptest.NewRecorder()

	d.handleUploadRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var returnedFlow UploadFlow
	if err := json.NewDecoder(w.Body).Decode(&returnedFlow); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if returnedFlow.ID != flow.ID {
		t.Errorf("Expected flow ID %s, got %s", flow.ID, returnedFlow.ID)
	}
}

func TestHandler_UploadRoutes_NotFound(t *testing.T) { // A
	mock := newMockCarrier()

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: true,
		Carrier:     mock,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/upload/nonexistent/flow",
		nil,
	)
	w := httptest.NewRecorder()

	d.handleUploadRoutes(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}
