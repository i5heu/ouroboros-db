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
	"testing"
	"time"

	internalcarrier "github.com/i5heu/ouroboros-db/internal/carrier"
	"golang.org/x/net/websocket"
)

func newTestDashboard(t *testing.T, allowUpload bool) (*Dashboard, *mockCarrier) { // A
	t.Helper()

	mock := newMockCarrier()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	d, err := New(Config{
		Enabled:     true,
		AllowUpload: allowUpload,
		Carrier:     mock,
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

	// Vertex browse is stubbed - expect 404.
	vertexReq := httptest.NewRequest(
		http.MethodGet,
		"/api/vertices/"+flow.VertexHash,
		nil,
	)
	vertexW := httptest.NewRecorder()
	d.handleGetVertex(vertexW, vertexReq)
	if vertexW.Code != http.StatusNotFound {
		t.Fatalf("vertex status: got %d", vertexW.Code)
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

	// Block details are stubbed - expect 404.
	blockReq := httptest.NewRequest(
		http.MethodGet,
		"/api/blocks/"+flow.BlockHash,
		nil,
	)
	blockW := httptest.NewRecorder()
	d.handleBlockRoutes(blockW, blockReq)
	if blockW.Code != http.StatusNotFound {
		t.Fatalf("block status: got %d", blockW.Code)
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

	// Step 4: Browse vertex (stubbed).
	vertexReq := httptest.NewRequest(
		http.MethodGet,
		"/api/vertices/"+flow.VertexHash,
		nil,
	)
	vertexW := httptest.NewRecorder()
	d.handleGetVertex(vertexW, vertexReq)
	if vertexW.Code != http.StatusNotFound {
		t.Fatalf("vertex status: got %d", vertexW.Code)
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
