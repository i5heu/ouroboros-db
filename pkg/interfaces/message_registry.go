package interfaces

import (
	"context"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
)

type messageSchema struct { // A
	newPayload func() proto.Message
}

var messageSchemas = map[MessageType]messageSchema{ // A
	MessageTypeUserMessage: {
		newPayload: func() proto.Message {
			return &pb.UserMessage{}
		},
	},
}

// TypedHandler wraps a concrete typed handler into
// a MessageHandler. In and Out must be proto.Message
// implementations. The wrapper performs a single
// type assertion on the decoded payload at dispatch
// time — no reflection, no per-type adapter lists.
//
// Adding a new message type only requires a
// newPayload entry in messageSchemas; handler
// authors can wrap their function with TypedHandler
// directly or use RegisterTypedHandler.
func TypedHandler[In, Out proto.Message]( // A
	fn func(
		context.Context,
		In,
		keys.NodeID,
		auth.TrustScope,
	) (Out, error),
) MessageHandler {
	return func(
		ctx context.Context,
		payload proto.Message,
		peer keys.NodeID,
		scope auth.TrustScope,
	) (proto.Message, error) {
		msg, ok := payload.(In)
		if !ok {
			var zero In
			return nil, fmt.Errorf(
				"payload type %T does not "+
					"match %T",
				payload,
				zero,
			)
		}
		return fn(ctx, msg, peer, scope)
	}
}

// RegisterTypedHandler wraps a concrete typed handler
// with TypedHandler and registers it with the
// controller. This keeps call sites concise while
// preserving the zero-reflection dispatch path.
func RegisterTypedHandler[In, Out proto.Message]( // A
	controller ClusterController,
	msgType MessageType,
	scopes []auth.TrustScope,
	fn func(
		context.Context,
		In,
		keys.NodeID,
		auth.TrustScope,
	) (Out, error),
) error {
	if controller == nil {
		return fmt.Errorf(
			"controller must not be nil",
		)
	}
	return controller.RegisterHandler(
		msgType,
		scopes,
		TypedHandler(fn),
	)
}

// BuildMessageRegistration creates a registration
// for the given message type. handler must be a
// MessageHandler (use TypedHandler or
// RegisterTypedHandler for concrete typed handlers).
func BuildMessageRegistration( // A
	msgType MessageType,
	scopes []auth.TrustScope,
	handler any,
) (MessageRegistration, error) {
	schema, ok := messageSchemas[msgType]
	if !ok {
		return MessageRegistration{}, fmt.Errorf(
			"no message schema for type %d",
			msgType,
		)
	}
	mh, ok := handler.(MessageHandler)
	if !ok || mh == nil {
		return MessageRegistration{}, fmt.Errorf(
			"handler must be a non-nil " +
				"MessageHandler; use " +
				"TypedHandler or " +
				"RegisterTypedHandler " +
				"for typed handlers",
		)
	}
	return MessageRegistration{
		MsgType:       msgType,
		AllowedScopes: scopes,
		NewPayload:    schema.newPayload,
		Handler:       mh,
	}, nil
}
