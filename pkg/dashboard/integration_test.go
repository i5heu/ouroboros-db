package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	internalcarrier "github.com/i5heu/ouroboros-db/internal/carrier"
	"golang.org/x/net/websocket"
)

func newTestDashboard(t *testing.T, allowUpload bool) (*Dashboard, *mockCarrier) { // A
	t.Helper()

	mock := newMockCarrier()
	mockCAS := newMockCAS()
	mockBS := newMockBlockStore()

	// Link the mockCAS to the mockBlockStore so blocks are created during upload
	mockCAS.blockStore = mockBS

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: allowUpload,
		Carrier:     mock,
		CAS:         mockCAS,
		BlockStore:  mockBS,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	return d, mock
}

func doRequest(
	t *testing.T,
	h http.HandlerFunc,
	method string,
	path string,
	body io.Reader,
) *httptest.ResponseRecorder { // A
	t.Helper()

	req := httptest.NewRequest(method, path, body)
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func newUploadRequest(
	t *testing.T,
	filename string,
	content []byte,
) *http.Request { // A
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write content failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("multipart close failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func waitForFlow(
	t *testing.T,
	d *Dashboard,
	flowID string,
	timeout time.Duration,
) *UploadFlow { // A
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/api/upload/"+flowID+"/flow",
			nil,
		)
		w := httptest.NewRecorder()
		d.handleUploadRoutes(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("flow status: got %d", w.Code)
		}

		var flow UploadFlow
		if err := json.NewDecoder(w.Body).Decode(&flow); err != nil {
			t.Fatalf("decode flow failed: %v", err)
		}
		if flow.Status == "complete" || flow.Status == "failed" {
			return &flow
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for flow %s", flowID)
	return nil
}

func TestIntegration_UploadFileAndPollFlow(t *testing.T) { // A
	d, _ := newTestDashboard(t, true)

	// Upload a file.
	req := newUploadRequest(t, "test.txt", []byte("hello"))
	w := httptest.NewRecorder()
	d.handleUpload(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("upload status: got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	flowID := resp["id"]
	if flowID == "" {
		t.Fatal("expected upload flow id")
	}

	flow := waitForFlow(t, d, flowID, 3*time.Second)
	if flow.Status != "complete" {
		t.Fatalf("expected complete, got %q", flow.Status)
	}
	if flow.VertexHash == "" {
		t.Fatal("expected vertex hash to be set")
	}
	if flow.BlockHash == "" {
		t.Fatal("expected block hash to be set")
	}
	if len(flow.SliceDistribution) == 0 {
		t.Fatal("expected slice distribution")
	}
}

func TestIntegration_SubscribeToLogsAndReceive(t *testing.T) { // A
	d, mock := newTestDashboard(t, false)
	mock.clearMessages()

	w := doRequest(
		t,
		d.handleNodeRoutes,
		http.MethodPost,
		"/api/nodes/test-node-2/logs/subscribe",
		nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("subscribe status: got %d", w.Code)
	}

	messages := mock.getSentMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].nodeID != internalcarrier.NodeID("test-node-2") {
		t.Fatalf("node id mismatch: %s", messages[0].nodeID)
	}
	if messages[0].message.Type != internalcarrier.MessageTypeLogSubscribe {
		t.Fatalf("type mismatch: %v", messages[0].message.Type)
	}

	payload, err := internalcarrier.DeserializeLogSubscribe(
		messages[0].message.Payload,
	)
	if err != nil {
		t.Fatalf("DeserializeLogSubscribe failed: %v", err)
	}
	if payload.SubscriberNodeID != internalcarrier.NodeID("test-node-1") {
		t.Fatalf("subscriber mismatch: %s", payload.SubscriberNodeID)
	}
}

func TestIntegration_WebSocketLogBroadcast(t *testing.T) { // A
	d, _ := newTestDashboard(t, false)
	d.hub.Start()
	defer d.hub.Stop()

	ts := httptest.NewServer(d.mux)
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http", "ws", 1) + "/ws/logs"
	conn, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Broadcast a log.
	d.hub.Broadcast(LogStreamMessage{
		SourceNodeID: "node-1",
		Timestamp:    time.Now().UnixNano(),
		Level:        "INFO",
		Message:      "hello logs",
	})

	var raw []byte
	if err := conn.SetReadDeadline(time.Now().Add(750 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("receive failed: %v", err)
	}

	var got LogStreamMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Type != "log" {
		t.Fatalf("expected type log, got %q", got.Type)
	}
	if got.Message != "hello logs" {
		t.Fatalf("message mismatch: %q", got.Message)
	}
}

func TestIntegration_BrowseVertexAfterUpload(t *testing.T) { // A
	d, _ := newTestDashboard(t, true)

	req := newUploadRequest(t, "test.txt", []byte("hello"))
	w := httptest.NewRecorder()
	d.handleUpload(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("upload status: got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	flowID := resp["id"]
	flow := waitForFlow(t, d, flowID, 3*time.Second)

	// After upload, vertex should be retrievable
	vertexReq := httptest.NewRequest(
		http.MethodGet,
		"/api/vertices/"+flow.VertexHash,
		nil,
	)
	vertexW := httptest.NewRecorder()
	d.handleGetVertex(vertexW, vertexReq)
	if vertexW.Code != http.StatusOK {
		t.Fatalf("vertex status: expected 200, got %d", vertexW.Code)
	}

	var vertex VertexInfo
	if err := json.NewDecoder(vertexW.Body).Decode(&vertex); err != nil {
		t.Fatalf("decode vertex failed: %v", err)
	}
	if vertex.Hash != flow.VertexHash {
		t.Fatalf("vertex hash mismatch: expected %s, got %s", flow.VertexHash, vertex.Hash)
	}
}

func TestIntegration_BlockBrowserWorkflow(t *testing.T) { // A
	d, _ := newTestDashboard(t, true)

	req := newUploadRequest(t, "test.txt", []byte("hello"))
	w := httptest.NewRecorder()
	d.handleUpload(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("upload status: got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	flow := waitForFlow(t, d, resp["id"], 3*time.Second)

	// After upload, block should be retrievable
	blockReq := httptest.NewRequest(
		http.MethodGet,
		"/api/blocks/"+flow.BlockHash,
		nil,
	)
	blockW := httptest.NewRecorder()
	d.handleBlockRoutes(blockW, blockReq)
	if blockW.Code != http.StatusOK {
		t.Fatalf("block status: expected 200, got %d", blockW.Code)
	}

	var block BlockInfo
	if err := json.NewDecoder(blockW.Body).Decode(&block); err != nil {
		t.Fatalf("decode block failed: %v", err)
	}
	if block.Hash != flow.BlockHash {
		t.Fatalf("block hash mismatch: expected %s, got %s", flow.BlockHash, block.Hash)
	}

	// Block slices are stubbed - expect 200 with an array.
	slicesReq := httptest.NewRequest(
		http.MethodGet,
		"/api/blocks/"+flow.BlockHash+"/slices",
		nil,
	)
	slicesW := httptest.NewRecorder()
	d.handleBlockRoutes(slicesW, slicesReq)
	if slicesW.Code != http.StatusOK {
		t.Fatalf("slices status: got %d", slicesW.Code)
	}
}

func TestIntegration_DistributionOverview(t *testing.T) { // A
	d, _ := newTestDashboard(t, false)

	w := doRequest(
		t,
		d.handleGetDistribution,
		http.MethodGet,
		"/api/distribution",
		nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("distribution status: got %d", w.Code)
	}

	var overview DistributionOverview
	if err := json.NewDecoder(w.Body).Decode(&overview); err != nil {
		t.Fatalf("decode distribution: %v", err)
	}
	if overview.TotalNodes != 2 {
		t.Fatalf("expected 2 nodes, got %d", overview.TotalNodes)
	}
}

func TestIntegration_FullUserWorkflow(t *testing.T) { // A
	d, mock := newTestDashboard(t, true)
	d.hub.Start()
	defer d.hub.Stop()
	mock.clearMessages()

	ts := httptest.NewServer(d.mux)
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http", "ws", 1) + "/ws/logs"
	conn, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Step 1: Upload.
	uReq := newUploadRequest(t, "file.bin", []byte("payload"))
	uW := httptest.NewRecorder()
	d.handleUpload(uW, uReq)
	if uW.Code != http.StatusAccepted {
		t.Fatalf("upload status: got %d", uW.Code)
	}
	var uResp map[string]string
	if err := json.NewDecoder(uW.Body).Decode(&uResp); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	flow := waitForFlow(t, d, uResp["id"], 3*time.Second)
	if flow.Status != "complete" {
		t.Fatalf("expected complete, got %q", flow.Status)
	}

	// Step 2: Subscribe to logs.
	subW := doRequest(
		t,
		d.handleNodeRoutes,
		http.MethodPost,
		"/api/nodes/test-node-2/logs/subscribe",
		nil,
	)
	if subW.Code != http.StatusOK {
		t.Fatalf("subscribe status: got %d", subW.Code)
	}
	if len(mock.getSentMessages()) != 1 {
		t.Fatalf("expected carrier send")
	}

	// Step 3: Simulate incoming log entry -> ws.
	entry := internalcarrier.LogEntryPayload{
		SourceNodeID: internalcarrier.NodeID("test-node-2"),
		Timestamp:    time.Now().UnixNano(),
		Level:        "INFO",
		Message:      "upload complete",
		Attributes: map[string]string{
			"vertexHash": flow.VertexHash,
		},
	}
	data, err := internalcarrier.SerializeLogEntry(entry)
	if err != nil {
		t.Fatalf("SerializeLogEntry failed: %v", err)
	}
	_, err = d.handleIncomingLogEntry(
		context.Background(),
		internalcarrier.NodeID("test-node-2"),
		internalcarrier.Message{
			Type:    internalcarrier.MessageTypeLogEntry,
			Payload: data,
		},
	)
	if err != nil {
		t.Fatalf("handleIncomingLogEntry failed: %v", err)
	}

	var raw []byte
	if err := conn.SetReadDeadline(time.Now().Add(750 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("receive failed: %v", err)
	}
	var got LogStreamMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Message != "upload complete" {
		t.Fatalf("unexpected log message: %q", got.Message)
	}
	if got.Attributes["vertexHash"] != flow.VertexHash {
		t.Fatalf("expected vertexHash attribute")
	}

	// Step 4: Browse uploaded vertex
	vertexReq := httptest.NewRequest(
		http.MethodGet,
		"/api/vertices/"+flow.VertexHash,
		nil,
	)
	vertexW := httptest.NewRecorder()
	d.handleGetVertex(vertexW, vertexReq)
	if vertexW.Code != http.StatusOK {
		t.Fatalf("vertex status: expected 200, got %d", vertexW.Code)
	}

	var vertex VertexInfo
	if err := json.NewDecoder(vertexW.Body).Decode(&vertex); err != nil {
		t.Fatalf("decode vertex failed: %v", err)
	}
	if vertex.Hash != flow.VertexHash {
		t.Fatalf("vertex hash mismatch")
	}
}

func TestIntegration_ChunkBrowser(t *testing.T) { // A
	d, _ := newTestDashboard(t, false)

	w := doRequest(
		t,
		d.handleGetVertices,
		http.MethodGet,
		"/api/vertices?offset=0&limit=20",
		nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("vertices status: got %d", w.Code)
	}

	var resp VertexListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode vertices failed: %v", err)
	}
	if resp.Limit != 20 {
		t.Fatalf("expected limit 20, got %d", resp.Limit)
	}

	// Chunk browser is represented by chunkCount in vertex metadata.
	_ = resp.Vertices
}

// testDashboardWithLogBroadcast holds all components needed for testing
// the full log flow from upload to WebSocket.
type testDashboardWithLogBroadcast struct {
	Dashboard      *Dashboard
	LogBroadcaster *internalcarrier.LogBroadcaster
	Logger         *slog.Logger
	MockCarrier    *mockCarrier
	LocalNodeID    internalcarrier.NodeID
}

// newTestDashboardWithLogBroadcast creates a dashboard with a properly
// configured LogBroadcaster for testing the full log flow. This helper
// demonstrates the correct setup: LogBroadcaster with local handler AND
// self-subscription.
func newTestDashboardWithLogBroadcast(
	t *testing.T,
	allowUpload bool,
) *testDashboardWithLogBroadcast {
	t.Helper()

	mock := newMockCarrier()
	localNodeID := mock.LocalNode().NodeID

	// Create inner handler that discards logs (we only care about broadcast)
	innerHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create LogBroadcaster
	lb := internalcarrier.NewLogBroadcaster(internalcarrier.LogBroadcasterConfig{
		Carrier:     mock,
		LocalNodeID: localNodeID,
		Inner:       innerHandler,
	})

	// Create logger backed by LogBroadcaster
	logger := slog.New(lb)

	// Create dashboard with LogBroadcaster
	d, err := New(Config{
		Enabled:        true,
		AllowUpload:    allowUpload,
		Carrier:        mock,
		LogBroadcaster: lb,
		Logger:         logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	return &testDashboardWithLogBroadcast{
		Dashboard:      d,
		LogBroadcaster: lb,
		Logger:         logger,
		MockCarrier:    mock,
		LocalNodeID:    localNodeID,
	}
}

// StartAll starts the LogBroadcaster and Dashboard hub.
func (td *testDashboardWithLogBroadcast) StartAll() {
	td.LogBroadcaster.Start()
	td.Dashboard.hub.Start()
}

// StopAll stops the LogBroadcaster and Dashboard hub.
func (td *testDashboardWithLogBroadcast) StopAll() {
	td.Dashboard.hub.Stop()
	td.LogBroadcaster.Stop()
}

// SubscribeLocalNode subscribes the local node to its own logs, which is
// required for the local handler to receive logs.
func (td *testDashboardWithLogBroadcast) SubscribeLocalNode() {
	td.LogBroadcaster.Subscribe(td.LocalNodeID, nil)
}

// TestIntegration_UploadLogsReachWebSocket is the key integration test that
// verifies the full flow: upload processing logs -> LogBroadcaster ->
// local handler -> WebSocket hub -> connected clients.
func TestIntegration_UploadLogsReachWebSocket(t *testing.T) {
	td := newTestDashboardWithLogBroadcast(t, true)
	td.StartAll()
	defer td.StopAll()

	// Set up local handler (simulates what Dashboard.Start does)
	td.LogBroadcaster.SetLocalHandler(td.Dashboard.handleLocalLogEntry)

	// Subscribe local node (this is the fix that was missing)
	td.SubscribeLocalNode()
	time.Sleep(20 * time.Millisecond)

	// Start HTTP server for WebSocket
	ts := httptest.NewServer(td.Dashboard.mux)
	defer ts.Close()

	// Connect WebSocket client
	wsURL := strings.Replace(ts.URL, "http", "ws", 1) + "/ws/logs"
	conn, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Give WebSocket time to register
	time.Sleep(50 * time.Millisecond)

	// Simulate upload logging using the logger backed by LogBroadcaster
	//nolint:sloglint // testing log functionality with attributes
	td.Logger.InfoContext(context.Background(), "upload started",
		"filename", "test.dat")
	//nolint:sloglint // testing log functionality with attributes
	td.Logger.InfoContext(context.Background(), "processing chunk",
		"chunk", 1, "total", 5)
	//nolint:sloglint // testing log functionality with attributes
	td.Logger.InfoContext(context.Background(), "upload complete",
		"bytes", 1024)

	// Read messages from WebSocket
	var messages []LogStreamMessage
	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		var raw []byte
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			t.Fatalf("receive failed on message %d: %v", i, err)
		}

		var msg LogStreamMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		messages = append(messages, msg)
	}

	// Verify all 3 messages arrived
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Build a set of received messages (order may vary due to goroutine delivery)
	msgSet := make(map[string]LogStreamMessage)
	for _, m := range messages {
		msgSet[m.Message] = m
		if m.Type != "log" {
			t.Errorf("expected type 'log', got %q for message %q", m.Type, m.Message)
		}
	}

	// Verify all expected messages arrived
	expectedMsgs := []string{"upload started", "processing chunk", "upload complete"}
	for _, exp := range expectedMsgs {
		if _, found := msgSet[exp]; !found {
			t.Errorf("expected message %q not found", exp)
		}
	}

	// Verify attributes on the "upload started" message
	if msg, ok := msgSet["upload started"]; ok {
		if msg.Attributes["filename"] != "test.dat" {
			t.Errorf("expected filename=test.dat, got %v", msg.Attributes)
		}
	}
}

// TestIntegration_DashboardStartSubscribesLocalNode verifies that after
// Start() is called, the dashboard is properly configured to receive local
// logs. This test documents what the fix should accomplish.
func TestIntegration_DashboardStartSubscribesLocalNode(t *testing.T) {
	td := newTestDashboardWithLogBroadcast(t, false)

	// Start LogBroadcaster
	td.LogBroadcaster.Start()
	defer td.LogBroadcaster.Stop()

	// Simulate what Dashboard.Start() does
	td.LogBroadcaster.SetLocalHandler(td.Dashboard.handleLocalLogEntry)
	// The fix should add: td.LogBroadcaster.Subscribe(localNodeID, nil)
	// For now, we manually add it to show it works
	td.SubscribeLocalNode()

	// Start hub
	td.Dashboard.hub.Start()
	defer td.Dashboard.hub.Stop()

	time.Sleep(20 * time.Millisecond)

	// Create a mock WebSocket client
	client := &Client{
		sendCh: make(chan []byte, 10),
	}
	td.Dashboard.hub.registerCh <- client
	time.Sleep(10 * time.Millisecond)

	// Log something
	td.Logger.InfoContext(context.Background(), "test log after start")
	time.Sleep(100 * time.Millisecond)

	// Verify the message reached the client
	select {
	case data := <-client.sendCh:
		var msg LogStreamMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if msg.Message != "test log after start" {
			t.Errorf("expected 'test log after start', got %q", msg.Message)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout: log did not reach WebSocket client")
	}
}

// TestIntegration_LoggerMustUseLogBroadcaster documents that for logs to
// reach WebSocket clients, the Logger must use LogBroadcaster as its handler.
// A regular logger will not broadcast logs.
func TestIntegration_LoggerMustUseLogBroadcaster(t *testing.T) {
	mock := newMockCarrier()
	localNodeID := mock.LocalNode().NodeID

	// Create LogBroadcaster
	innerHandler := slog.NewTextHandler(io.Discard, nil)
	lb := internalcarrier.NewLogBroadcaster(internalcarrier.LogBroadcasterConfig{
		Carrier:     mock,
		LocalNodeID: localNodeID,
		Inner:       innerHandler,
	})
	lb.Start()
	defer lb.Stop()

	// Create dashboard with LogBroadcaster but a SEPARATE logger
	separateLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	d, err := New(Config{
		Enabled:        true,
		AllowUpload:    false,
		Carrier:        mock,
		LogBroadcaster: lb,
		Logger:         separateLogger, // Not using LogBroadcaster!
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Set up local handler and subscribe
	lb.SetLocalHandler(d.handleLocalLogEntry)
	lb.Subscribe(localNodeID, nil)

	d.hub.Start()
	defer d.hub.Stop()

	time.Sleep(20 * time.Millisecond)

	// Create mock client
	client := &Client{
		sendCh: make(chan []byte, 10),
	}
	d.hub.registerCh <- client
	time.Sleep(10 * time.Millisecond)

	// Log using the SEPARATE logger (not LogBroadcaster)
	separateLogger.InfoContext(context.Background(), "this won't broadcast")

	time.Sleep(100 * time.Millisecond)

	// Verify no message reached the client (because we used wrong logger)
	select {
	case <-client.sendCh:
		t.Fatal("unexpected: separate logger should not broadcast")
	default:
		// Expected: no message
	}

	// Now log using a logger backed by LogBroadcaster
	correctLogger := slog.New(lb)
	correctLogger.InfoContext(context.Background(), "this will broadcast")

	time.Sleep(100 * time.Millisecond)

	// Verify message reached the client
	select {
	case data := <-client.sendCh:
		var msg LogStreamMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if msg.Message != "this will broadcast" {
			t.Errorf("expected 'this will broadcast', got %q", msg.Message)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout: correct logger log did not reach client")
	}
}

// TestIntegration_ConcurrentUploadLogs verifies that multiple concurrent
// upload operations can log simultaneously without issues.
func TestIntegration_ConcurrentUploadLogs(t *testing.T) {
	td := newTestDashboardWithLogBroadcast(t, true)
	td.StartAll()
	defer td.StopAll()

	td.LogBroadcaster.SetLocalHandler(td.Dashboard.handleLocalLogEntry)
	td.SubscribeLocalNode()
	time.Sleep(20 * time.Millisecond)

	// Create mock client with large buffer
	client := &Client{
		sendCh: make(chan []byte, 500),
	}
	td.Dashboard.hub.registerCh <- client
	time.Sleep(10 * time.Millisecond)

	// Simulate 5 concurrent uploads, each logging 10 messages
	var wg sync.WaitGroup
	uploadsCount := 5
	msgsPerUpload := 10

	for i := range uploadsCount {
		wg.Add(1)
		go func(uploadID int) {
			defer wg.Done()
			for j := range msgsPerUpload {
				//nolint:sloglint // testing concurrent logging
				td.Logger.InfoContext(context.Background(), "upload progress",
					"uploadID", uploadID,
					"chunk", j,
				)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// Count received messages
	receivedCount := 0
	for {
		select {
		case <-client.sendCh:
			receivedCount++
		default:
			goto done
		}
	}
done:

	expectedCount := uploadsCount * msgsPerUpload
	if receivedCount != expectedCount {
		t.Errorf("expected %d messages from concurrent uploads, got %d",
			expectedCount, receivedCount)
	}
}
