package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
	"pgregory.net/rapid"
)

// Compile-time interface compliance check.
var _ interfaces.ClusterController = (*clusterController)(nil)

// mockCarrier is a minimal Carrier implementation
// for testing the ClusterController in isolation.
type mockCarrier struct { // A
	mu    sync.Mutex
	nodes []interfaces.PeerNode
}

func (m *mockCarrier) GetNodes() []interfaces.PeerNode { // A
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

func (m *mockCarrier) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.PeerNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.NodeID == nodeID {
			return n, nil
		}
	}
	return interfaces.PeerNode{},
		fmt.Errorf("node not found")
}

func (m *mockCarrier) BroadcastReliable( // A
	_ interfaces.Message,
) ([]interfaces.PeerNode, []interfaces.PeerNode, error) {
	return nil, nil, nil
}

func (m *mockCarrier) SendMessageToNodeReliable( // A
	_ keys.NodeID,
	_ interfaces.Message,
) error {
	return nil
}

func (m *mockCarrier) BroadcastUnreliable( // A
	_ interfaces.Message,
) []interfaces.PeerNode {
	return nil
}

func (m *mockCarrier) SendMessageToNodeUnreliable( // A
	_ keys.NodeID,
	_ interfaces.Message,
) error {
	return nil
}

func (m *mockCarrier) Broadcast( // A
	_ interfaces.Message,
) ([]interfaces.PeerNode, error) {
	return nil, nil
}

func (m *mockCarrier) SendMessageToNode( // A
	_ keys.NodeID,
	_ interfaces.Message,
) error {
	return nil
}

func (m *mockCarrier) OpenPeerChannel( // A
	_ interfaces.PeerNode,
	_ interfaces.NodeCert,
) error {
	return nil
}

func (m *mockCarrier) LeaveCluster( // A
	_ interfaces.PeerNode,
) error {
	return nil
}

func (m *mockCarrier) RemoveNode( // A
	_ keys.NodeID,
) error {
	return nil
}

func (m *mockCarrier) IsConnected( // A
	_ keys.NodeID,
) bool {
	return false
}

func (m *mockCarrier) StartListener( // A
	_ context.Context,
) error {
	return nil
}

func (m *mockCarrier) GetNodeConnection( // A
	_ keys.NodeID,
) (interfaces.NodeConnection, error) {
	return interfaces.NodeConnection{}, fmt.Errorf(
		"node not found",
	)
}

func testLogger() *slog.Logger { // A
	return slog.New(
		slog.NewTextHandler(os.Stderr, nil),
	)
}

func newTestController( // A
	t *testing.T,
) *clusterController {
	t.Helper()
	cc, err := NewClusterController(
		&mockCarrier{}, testLogger(),
	)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	return cc
}

func nodeID(b byte) keys.NodeID { // A
	var id keys.NodeID
	id[0] = b
	return id
}

func newTestPayload() proto.Message { // A
	return &pb.UserMessage{}
}

func testUserMessagePayload( // A
	from string,
	text string,
) *pb.UserMessage {
	return &pb.UserMessage{
		From: from,
		Text: text,
	}
}

func marshalTestPayload( // A
	t *testing.T,
	payload *pb.UserMessage,
) []byte {
	t.Helper()
	encoded, err := proto.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return encoded
}

func registerTestHandler( // A
	cc *clusterController,
	msgType interfaces.MessageType,
	scopes []auth.TrustScope,
	handler interfaces.MessageHandler,
) error {
	if msgType != interfaces.MessageTypeUserMessage {
		return registerSyntheticTestHandler(
			cc,
			msgType,
			scopes,
			handler,
		)
	}
	return cc.RegisterHandler(
		msgType, scopes, handler,
	)
}

func registerSyntheticTestHandler( // A
	cc *clusterController,
	msgType interfaces.MessageType,
	scopes []auth.TrustScope,
	handler interfaces.MessageHandler,
) error {
	if len(scopes) == 0 {
		return fmt.Errorf(
			"at least one scope is required",
		)
	}
	reg := interfaces.MessageRegistration{
		MsgType:       msgType,
		AllowedScopes: scopes,
		NewPayload: func() proto.Message {
			return &pb.UserMessage{}
		},
		Handler: handler,
	}
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if _, exists := cc.handlers[msgType]; exists {
		return fmt.Errorf(
			"handler already registered "+
				"for message type %d",
			msgType,
		)
	}
	cc.handlers[msgType] = &reg
	return nil
}

var echoHandler = interfaces.TypedHandler( // A
	func(
		_ context.Context,
		payload *interfaces.UserMessage,
		_ keys.NodeID,
		_ auth.TrustScope,
	) (*interfaces.UserMessage, error) {
		return payload, nil
	},
)

// TestNewClusterControllerNilCarrier verifies that
// a nil carrier is rejected.
func TestNewClusterControllerNilCarrier( // A
	t *testing.T,
) {
	t.Parallel()
	_, err := NewClusterController(
		nil, testLogger(),
	)
	if err == nil {
		t.Fatal("expected error for nil carrier")
	}
}

// TestNewClusterControllerNilLogger verifies that
// a nil logger is rejected.
func TestNewClusterControllerNilLogger( // A
	t *testing.T,
) {
	t.Parallel()
	_, err := NewClusterController(
		&mockCarrier{}, nil,
	)
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
}

// TestRegisterHandler verifies handler registration
// for a message type.
func TestRegisterHandler(t *testing.T) { // A
	t.Parallel()
	cc := newTestController(t)

	err := registerTestHandler(
		cc,
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	reg, ok := cc.GetHandler(
		interfaces.MessageTypeUserMessage,
	)
	if !ok {
		t.Fatal("handler not found after register")
	}
	if reg.MsgType != interfaces.MessageTypeUserMessage {
		t.Fatalf(
			"wrong msg type: got %d, want %d",
			reg.MsgType,
			interfaces.MessageTypeUserMessage,
		)
	}
	if len(reg.AllowedScopes) != 1 {
		t.Fatalf(
			"wrong scope count: %d",
			len(reg.AllowedScopes),
		)
	}
}

// TestRegisterHandlerDuplicate verifies that
// registering the same type twice returns an error.
func TestRegisterHandlerDuplicate( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	err := registerTestHandler(
		cc,
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	err = registerTestHandler(
		cc,
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeUser},
		echoHandler,
	)
	if err == nil {
		t.Fatal("expected error for duplicate")
	}
}

// TestRegisterHandlerNilHandler verifies that a nil
// handler function is rejected.
func TestRegisterHandlerNilHandler( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	err := cc.RegisterHandler(
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeAdmin},
		nil,
	)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

// TestRegisterHandlerInvalidSignature verifies that
// handlers with unsupported signatures are rejected.
func TestRegisterHandlerInvalidSignature( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	err := cc.RegisterHandler(
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeAdmin},
		func() {},
	)
	if err == nil {
		t.Fatal("expected error for invalid handler")
	}
}

// TestRegisterHandlerEmptyScopes verifies that empty
// scopes are rejected.
func TestRegisterHandlerEmptyScopes( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	err := cc.RegisterHandler(
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{},
		echoHandler,
	)
	if err == nil {
		t.Fatal("expected error for empty scopes")
	}
}

// TestUnregisterHandler verifies handler removal.
func TestUnregisterHandler(t *testing.T) { // A
	t.Parallel()
	cc := newTestController(t)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)

	err := cc.UnregisterHandler(
		interfaces.MessageTypeUserMessage,
	)
	if err != nil {
		t.Fatalf("unregister: %v", err)
	}

	_, ok := cc.GetHandler(
		interfaces.MessageTypeUserMessage,
	)
	if ok {
		t.Fatal("handler still found after unregister")
	}
}

// TestUnregisterHandlerNotFound verifies error on
// unregistering a non-existent handler.
func TestUnregisterHandlerNotFound( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	err := cc.UnregisterHandler(
		interfaces.MessageTypeHeartbeat,
	)
	if err == nil {
		t.Fatal("expected error for missing handler")
	}
}

// TestGetHandlerNotFound verifies that GetHandler
// returns false for unregistered types.
func TestGetHandlerNotFound(t *testing.T) { // A
	t.Parallel()
	cc := newTestController(t)

	_, ok := cc.GetHandler(
		interfaces.MessageTypeHeartbeat,
	)
	if ok {
		t.Fatal("expected not found")
	}
}

// TestGetEffectiveScopes verifies the scope
// hierarchy logic.
func TestGetEffectiveScopes(t *testing.T) { // A
	t.Parallel()
	cc := newTestController(t)

	tests := []struct { // A
		name  string
		scope auth.TrustScope
		want  []auth.TrustScope
	}{
		{
			name:  "admin implies user",
			scope: auth.ScopeAdmin,
			want: []auth.TrustScope{
				auth.ScopeAdmin,
				auth.ScopeUser,
			},
		},
		{
			name:  "user only",
			scope: auth.ScopeUser,
			want: []auth.TrustScope{
				auth.ScopeUser,
			},
		},
		{
			name:  "unknown scope",
			scope: auth.TrustScope(99),
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cc.GetEffectiveScopes(tt.scope)
			if len(got) != len(tt.want) {
				t.Fatalf(
					"len: got %d, want %d",
					len(got),
					len(tt.want),
				)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf(
						"scope[%d]: got %v, want %v",
						i, got[i], tt.want[i],
					)
				}
			}
		})
	}
}

// TestCheckAccessAdminAllowed verifies that an admin
// peer can access a handler that allows ScopeAdmin.
func TestCheckAccessAdminAllowed( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)

	d := cc.CheckAccess(
		interfaces.MessageTypeHeartbeat,
		auth.ScopeAdmin,
	)
	if !d.Allowed {
		t.Fatalf("expected allowed: %s", d.Reason)
	}
}

// TestCheckAccessAdminCanAccessUserHandler verifies
// that an admin peer can access user-scoped handlers
// because ScopeAdmin implies ScopeUser.
func TestCheckAccessAdminCanAccessUserHandler( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeUser},
		echoHandler,
	)

	d := cc.CheckAccess(
		interfaces.MessageTypeHeartbeat,
		auth.ScopeAdmin,
	)
	if !d.Allowed {
		t.Fatalf(
			"admin should access user handler: %s",
			d.Reason,
		)
	}
}

// TestCheckAccessUserDeniedAdminHandler verifies
// that a user peer cannot access admin-only handlers.
func TestCheckAccessUserDeniedAdminHandler( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)

	d := cc.CheckAccess(
		interfaces.MessageTypeHeartbeat,
		auth.ScopeUser,
	)
	if d.Allowed {
		t.Fatal("user should not access admin handler")
	}
	if d.Reason == "" {
		t.Fatal("expected denial reason")
	}
}

// TestCheckAccessBothScopes verifies that a handler
// allowing both scopes is accessible by both.
func TestCheckAccessBothScopes(t *testing.T) { // A
	t.Parallel()
	cc := newTestController(t)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{
			auth.ScopeAdmin,
			auth.ScopeUser,
		},
		echoHandler,
	)

	adminD := cc.CheckAccess(
		interfaces.MessageTypeHeartbeat,
		auth.ScopeAdmin,
	)
	if !adminD.Allowed {
		t.Fatalf("admin denied: %s", adminD.Reason)
	}

	userD := cc.CheckAccess(
		interfaces.MessageTypeHeartbeat,
		auth.ScopeUser,
	)
	if !userD.Allowed {
		t.Fatalf("user denied: %s", userD.Reason)
	}
}

// TestCheckAccessNoHandler verifies denial when no
// handler is registered for the message type.
func TestCheckAccessNoHandler(t *testing.T) { // A
	t.Parallel()
	cc := newTestController(t)

	d := cc.CheckAccess(
		interfaces.MessageTypeHeartbeat,
		auth.ScopeAdmin,
	)
	if d.Allowed {
		t.Fatal("should deny when no handler exists")
	}
}

// TestHandleIncomingMessageAuthorized verifies that
// an authorized message is dispatched to the handler
// and the response is returned.
func TestHandleIncomingMessageAuthorized( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	payload := marshalTestPayload(
		t,
		testUserMessagePayload("alice", "test-payload"),
	)
	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: payload,
	}
	resp, err := cc.HandleIncomingMessage(
		msg, nodeID(1), auth.ScopeAdmin,
	)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	encodedResponse, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if string(encodedResponse) != string(payload) {
		t.Fatalf(
			"payload: got %q, want %q",
			encodedResponse,
			payload,
		)
	}
}

// TestHandleIncomingMessageDenied verifies that an
// unauthorized message returns an error.
func TestHandleIncomingMessageDenied( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)

	payload := marshalTestPayload(
		t,
		testUserMessagePayload("alice", "denied"),
	)
	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: payload,
	}
	_, err := cc.HandleIncomingMessage(
		msg, nodeID(1), auth.ScopeUser,
	)
	if err == nil {
		t.Fatal("expected access denied error")
	}
}

// TestHandleIncomingMessageNoHandler verifies that
// a message with no registered handler is rejected.
func TestHandleIncomingMessageNoHandler( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: []byte("no-handler"),
	}
	_, err := cc.HandleIncomingMessage(
		msg, nodeID(1), auth.ScopeAdmin,
	)
	if err == nil {
		t.Fatal("expected error for missing handler")
	}
}

// TestHandleIncomingMessageMalformedPayload verifies
// that malformed protobuf input is rejected before
// the handler is invoked.
func TestHandleIncomingMessageMalformedPayload( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	called := false
	h := interfaces.TypedHandler(
		func(
			_ context.Context,
			_ *interfaces.UserMessage,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (*interfaces.ResponseEmptyPayload, error) {
			called = true
			return &interfaces.ResponseEmptyPayload{}, nil
		},
	)

	err := registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		h,
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err = cc.HandleIncomingMessage(
		interfaces.Message{
			Type:    interfaces.MessageTypeHeartbeat,
			Payload: []byte("not-protobuf"),
		},
		nodeID(1),
		auth.ScopeAdmin,
	)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if called {
		t.Fatal("handler should not run on bad payload")
	}
}

// TestHandleIncomingMessageHandlerError verifies
// that a handler error is propagated.
func TestHandleIncomingMessageHandlerError( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	errHandler := interfaces.TypedHandler(
		func(
			_ context.Context,
			_ *interfaces.UserMessage,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (*interfaces.ResponseEmptyPayload, error) {
			return nil,
				fmt.Errorf("handler failed")
		},
	)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		errHandler,
	)

	msg := interfaces.Message{
		Type: interfaces.MessageTypeHeartbeat,
		Payload: marshalTestPayload(
			t,
			testUserMessagePayload("alice", "boom"),
		),
	}
	_, err := cc.HandleIncomingMessage(
		msg, nodeID(1), auth.ScopeAdmin,
	)
	if err == nil {
		t.Fatal("expected handler error")
	}
}

// TestHandleIncomingMessagePeerIDPassed verifies
// that the correct peer NodeID is passed to the
// handler.
func TestHandleIncomingMessagePeerIDPassed( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	expectedPeer := nodeID(42)
	var receivedPeer keys.NodeID

	h := interfaces.TypedHandler(
		func(
			_ context.Context,
			_ *interfaces.UserMessage,
			peer keys.NodeID,
			_ auth.TrustScope,
		) (*interfaces.ResponseEmptyPayload, error) {
			receivedPeer = peer
			return &interfaces.ResponseEmptyPayload{}, nil
		},
	)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		h,
	)

	msg := interfaces.Message{
		Type: interfaces.MessageTypeHeartbeat,
		Payload: marshalTestPayload(
			t,
			testUserMessagePayload("alice", "peer"),
		),
	}
	_, err := cc.HandleIncomingMessage(
		msg, expectedPeer, auth.ScopeAdmin,
	)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if receivedPeer != expectedPeer {
		t.Fatalf(
			"peer: got %v, want %v",
			receivedPeer, expectedPeer,
		)
	}
}

// TestHandleIncomingMessageScopePassed verifies that
// the correct TrustScope is passed to the handler.
func TestHandleIncomingMessageScopePassed( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	var receivedScope auth.TrustScope
	h := interfaces.TypedHandler(
		func(
			_ context.Context,
			_ *interfaces.UserMessage,
			_ keys.NodeID,
			scope auth.TrustScope,
		) (*interfaces.ResponseEmptyPayload, error) {
			receivedScope = scope
			return &interfaces.ResponseEmptyPayload{}, nil
		},
	)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{
			auth.ScopeAdmin,
			auth.ScopeUser,
		},
		h,
	)

	msg := interfaces.Message{
		Type: interfaces.MessageTypeHeartbeat,
		Payload: marshalTestPayload(
			t,
			testUserMessagePayload("alice", "scope"),
		),
	}
	_, err := cc.HandleIncomingMessage(
		msg, nodeID(1), auth.ScopeUser,
	)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if receivedScope != auth.ScopeUser {
		t.Fatalf(
			"scope: got %v, want %v",
			receivedScope, auth.ScopeUser,
		)
	}
}

// TestHandleIncomingMessageResponsePayload verifies
// that handler response payloads are returned.
func TestHandleIncomingMessageResponsePayload( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	h := interfaces.TypedHandler(
		func(
			_ context.Context,
			_ *interfaces.UserMessage,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (*interfaces.UserMessageResponse, error) {
			return &interfaces.UserMessageResponse{
				Success: true,
			}, nil
		},
	)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		h,
	)

	msg := interfaces.Message{
		Type: interfaces.MessageTypeHeartbeat,
		Payload: marshalTestPayload(
			t,
			testUserMessagePayload("alice", "meta"),
		),
	}
	resp, err := cc.HandleIncomingMessage(
		msg, nodeID(1), auth.ScopeAdmin,
	)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	response, ok := resp.(*interfaces.UserMessageResponse)
	if !ok {
		t.Fatalf(
			"response type: got %T, want %T",
			resp,
			&interfaces.UserMessageResponse{},
		)
	}
	if !response.Success {
		t.Fatal("expected success response")
	}
}

// TestConcurrentRegisterAndHandle verifies that
// concurrent handler registration and message
// handling do not race.
func TestConcurrentRegisterAndHandle( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	var wg sync.WaitGroup
	msgTypes := []interfaces.MessageType{
		interfaces.MessageTypeHeartbeat,
		interfaces.MessageTypeLogPush,
		interfaces.MessageTypeBlockSyncRequest,
		interfaces.MessageTypeKeyEntryRequest,
		interfaces.MessageTypeKeyEntryResponse,
	}

	// Register handlers concurrently.
	for _, mt := range msgTypes {
		wg.Add(1)
		go func(mt interfaces.MessageType) {
			defer wg.Done()
			_ = registerTestHandler(
				cc,
				mt,
				[]auth.TrustScope{auth.ScopeAdmin},
				echoHandler,
			)
		}(mt)
	}
	wg.Wait()

	// Handle messages concurrently.
	for i := byte(0); i < 50; i++ {
		wg.Add(1)
		go func(i byte) {
			defer wg.Done()
			mt := msgTypes[int(i)%len(msgTypes)]
			msg := interfaces.Message{
				Type: mt,
				Payload: marshalTestPayload(
					t,
					testUserMessagePayload(
						"alice",
						"concurrent",
					),
				),
			}
			_, _ = cc.HandleIncomingMessage(
				msg,
				nodeID(i),
				auth.ScopeAdmin,
			)
		}(i)
	}
	wg.Wait()
}

// TestRegisterUnregisterReRegister verifies the
// full lifecycle: register -> unregister -> register.
func TestRegisterUnregisterReRegister( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	err := registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		echoHandler,
	)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	err = cc.UnregisterHandler(
		interfaces.MessageTypeHeartbeat,
	)
	if err != nil {
		t.Fatalf("unregister: %v", err)
	}

	err = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeUser},
		echoHandler,
	)
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}

	reg, ok := cc.GetHandler(
		interfaces.MessageTypeHeartbeat,
	)
	if !ok {
		t.Fatal("handler not found after re-register")
	}
	if reg.AllowedScopes[0] != auth.ScopeUser {
		t.Fatal("scope not updated after re-register")
	}
}

// TestPropertyRegisterUnregister uses rapid to
// verify that register/unregister is consistent.
func TestPropertyRegisterUnregister( // A
	t *testing.T,
) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		cc := newTestController(
			&testing.T{},
		)
		registered := make(
			map[interfaces.MessageType]bool,
		)
		supportedTypes := []interfaces.MessageType{
			interfaces.MessageTypeHeartbeat,
			interfaces.MessageTypeUserMessage,
			interfaces.MessageTypeLogPush,
			interfaces.MessageTypeBlockSyncRequest,
			interfaces.MessageTypeKeyEntryRequest,
			interfaces.MessageTypeKeyEntryResponse,
		}

		ops := rapid.IntRange(1, 50).Draw(t, "ops")
		for i := 0; i < ops; i++ {
			mt := supportedTypes[rapid.IntRange(
				0,
				len(supportedTypes)-1,
			).Draw(t, "msgType")]

			if registered[mt] {
				// Try unregister.
				err := cc.UnregisterHandler(mt)
				if err != nil {
					t.Fatalf(
						"unregister %d: %v",
						mt, err,
					)
				}
				delete(registered, mt)

				_, ok := cc.GetHandler(mt)
				if ok {
					t.Fatal("found after unregister")
				}
			} else {
				// Try register.
				err := registerTestHandler(
					cc,
					mt,
					[]auth.TrustScope{auth.ScopeAdmin},
					echoHandler,
				)
				if err != nil {
					t.Fatalf(
						"register %d: %v",
						mt, err,
					)
				}
				registered[mt] = true

				_, ok := cc.GetHandler(mt)
				if !ok {
					t.Fatal("not found after register")
				}
			}
		}
	})
}

// TestCheckAccessUnknownScope verifies that an
// unknown TrustScope (not Admin or User) is denied
// access, exercising the default branch in
// getEffectiveScopesUnlocked.
func TestCheckAccessUnknownScope( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	_ = registerTestHandler(
		cc,
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeUser},
		echoHandler,
	)

	d := cc.CheckAccess(
		interfaces.MessageTypeHeartbeat,
		auth.TrustScope(99),
	)
	if d.Allowed {
		t.Fatal(
			"unknown scope should be denied",
		)
	}
}

// TestPropertyAccessDecision uses rapid to verify
// access control consistency across random scopes.
func TestPropertyAccessDecision( // A
	t *testing.T,
) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		cc := newTestController(
			&testing.T{},
		)

		allowAdmin := rapid.Bool().Draw(
			t, "allowAdmin",
		)
		allowUser := rapid.Bool().Draw(
			t, "allowUser",
		)

		var scopes []auth.TrustScope
		if allowAdmin {
			scopes = append(scopes, auth.ScopeAdmin)
		}
		if allowUser {
			scopes = append(scopes, auth.ScopeUser)
		}
		if len(scopes) == 0 {
			return // Skip empty scope case.
		}

		err := registerTestHandler(
			cc,
			interfaces.MessageTypeHeartbeat,
			scopes,
			echoHandler,
		)
		if err != nil {
			t.Fatalf("register: %v", err)
		}

		// Admin should always be allowed because
		// ScopeAdmin implies ScopeUser.
		adminD := cc.CheckAccess(
			interfaces.MessageTypeHeartbeat,
			auth.ScopeAdmin,
		)
		if !adminD.Allowed {
			t.Fatalf(
				"admin denied with scopes %v: %s",
				scopes,
				adminD.Reason,
			)
		}

		// User should be allowed only if ScopeUser
		// is in allowed scopes.
		userD := cc.CheckAccess(
			interfaces.MessageTypeHeartbeat,
			auth.ScopeUser,
		)
		if allowUser && !userD.Allowed {
			t.Fatalf(
				"user denied but user scope allowed",
			)
		}
		if !allowUser && userD.Allowed {
			t.Fatalf(
				"user allowed but user scope not set",
			)
		}
	})
}

// TestRegisterHandlerValueResponse verifies that a
// typed handler wrapped via TypedHandler is accepted
// and dispatched correctly.
func TestRegisterHandlerValueResponse( // A
	t *testing.T,
) {
	t.Parallel()
	cc := newTestController(t)

	err := interfaces.RegisterTypedHandler(
		cc,
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeAdmin},
		func(
			_ context.Context,
			payload *interfaces.UserMessage,
			_ keys.NodeID,
			_ auth.TrustScope,
		) (*interfaces.ResponseEmptyPayload, error) {
			if payload.Text != "typed" {
				return nil, fmt.Errorf(
					"unexpected payload text %q",
					payload.Text,
				)
			}
			return &interfaces.ResponseEmptyPayload{}, nil
		},
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	resp, err := cc.HandleIncomingMessage(
		interfaces.Message{
			Type: interfaces.MessageTypeUserMessage,
			Payload: marshalTestPayload(
				t,
				testUserMessagePayload("alice", "typed"),
			),
		},
		nodeID(1),
		auth.ScopeAdmin,
	)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if _, ok := resp.(*interfaces.ResponseEmptyPayload); !ok {
		t.Fatalf(
			"response type: got %T, want %T",
			resp,
			&interfaces.ResponseEmptyPayload{},
		)
	}
}
