package clusterlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	rapid "pgregory.net/rapid"
)

func ExampleClusterLog() { // A
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	carrier := &mockCarrier{}

	var self keys.NodeID
	self[0] = 1
	var subscriber keys.NodeID
	subscriber[0] = 2

	cl := New(logger, carrier, self)
	defer cl.Stop()

	ctx := context.Background()
	cl.SubscribeLog(ctx, self, subscriber)

	cl.Info(ctx, "hello", map[string]string{"k": "v"})
	cl.Warn(ctx, "careful", nil)

	entries := cl.Tail(2)
	fmt.Printf("%s:%s\n", entries[0].Level.String(), entries[0].Message)
	fmt.Printf("%s:%s\n", entries[1].Level.String(), entries[1].Message)
	fmt.Printf("pushes:%d\n", len(carrier.getMessages()))

	// Output:
	// INFO:hello
	// WARN:careful
	// pushes:2
}

func TestAllLogs(t *testing.T) { // AC
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		carrier := &mockCarrier{}
		cl := New(newTestLogger(), carrier, nodeID(1))
		defer cl.Stop()

		// generate a sequence of operations
		n := rapid.IntRange(1, 50).Draw(t, "n")
		ops := make([]int, n)
		for i := 0; i < n; i++ {
			ops[i] = rapid.IntRange(
				0,
				4,
			).Draw(
				t,
				"op",
			) // 0=Info,1=Warn,2=Debug,3=Err,4=Log
		}

		expected := make([]LogEntry, 0, n)
		for i, op := range ops {
			msg := fmt.Sprintf("m-%d-%d", i, rapid.IntRange(0, 1000).Draw(t, "r"))
			var fields map[string]string
			if rapid.Bool().Draw(t, "hasFields") {
				k := fmt.Sprintf("k%d", rapid.IntRange(0, 100).Draw(t, "rk"))
				v := fmt.Sprintf("v%d", rapid.IntRange(0, 1000).Draw(t, "rv"))
				fields = map[string]string{k: v}
			}

			switch op {
			case 0:
				cl.Info(ctx, msg, fields)
				expected = append(
					expected,
					LogEntry{Message: msg, Level: LogLevelInfo, Fields: fields},
				)
			case 1:
				cl.Warn(ctx, msg, fields)
				expected = append(
					expected,
					LogEntry{Message: msg, Level: LogLevelWarn, Fields: fields},
				)
			case 2:
				cl.Debug(ctx, msg, fields)
				expected = append(
					expected,
					LogEntry{Message: msg, Level: LogLevelDebug, Fields: fields},
				)
			case 3:
				var err error
				if rapid.Bool().Draw(t, "hasErr") {
					err = fmt.Errorf("err-%d", i)
				}
				if err != nil {
					cl.Err(ctx, msg, err, fields)
					if fields == nil {
						fields = map[string]string{keyError: err.Error()}
					} else {
						f := make(map[string]string)
						for k, v := range fields {
							f[k] = v
						}
						f[keyError] = err.Error()
						fields = f
					}
				} else {
					cl.Err(ctx, msg, nil, fields)
				}
				expected = append(
					expected,
					LogEntry{Message: msg, Level: LogLevelError, Fields: fields},
				)
			case 4:
				// explicit Log with random level
				lvl := LogLevel(rapid.IntRange(0, 3).Draw(t, "lvl"))
				cl.Log(ctx, lvl, msg, fields)
				expected = append(
					expected,
					LogEntry{Message: msg, Level: lvl, Fields: fields},
				)
			}
		}

		// now verify the stored entries match expected in order and content
		entries := cl.Tail(0)
		if len(entries) != len(expected) {
			t.Fatalf("expected %d entries, got %d", len(expected), len(entries))
		}
		for i := range expected {
			if entries[i].Message != expected[i].Message {
				t.Fatalf(
					"entry %d message mismatch: got %q want %q",
					i,
					entries[i].Message,
					expected[i].Message,
				)
			}
			if entries[i].Level != expected[i].Level {
				t.Fatalf(
					"entry %d level mismatch: got %v want %v",
					i,
					entries[i].Level,
					expected[i].Level,
				)
			}
			// fields match (nil vs empty map considered equal)
			if (entries[i].Fields == nil) != (expected[i].Fields == nil) {
				// if one is nil and the other empty, consider equal
				if !(entries[i].Fields != nil && len(entries[i].Fields) == 0 && expected[i].Fields == nil) &&
					!(expected[i].Fields != nil && len(expected[i].Fields) == 0 && entries[i].Fields == nil) {
					t.Fatalf(
						"entry %d fields nil mismatch: got %v want %v",
						i,
						entries[i].Fields,
						expected[i].Fields,
					)
				}
			} else if entries[i].Fields != nil && expected[i].Fields != nil {
				if len(entries[i].Fields) != len(expected[i].Fields) {
					t.Fatalf(
						"entry %d fields length mismatch: got %d want %d",
						i,
						len(entries[i].Fields),
						len(expected[i].Fields),
					)
				}
				for k, v := range expected[i].Fields {
					if entries[i].Fields[k] != v {
						t.Fatalf(
							"entry %d field %q mismatch: got %q want %q",
							i,
							k,
							entries[i].Fields[k],
							v,
						)
					}
				}
			}
		}

		// also verify QueryByLevel returns an appropriate subset
		for lvl := LogLevelDebug; lvl <= LogLevelError; lvl++ {
			res := cl.QueryByLevel(lvl, time.Time{})
			// every returned entry should have matching level
			for _, e := range res {
				if e.Level != lvl {
					t.Fatalf(
						"QueryByLevel returned wrong level: got %v want %v",
						e.Level,
						lvl,
					)
				}
			}
		}
	})
}

func TestSubscribeUnsubscribe(t *testing.T) { // AC
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		// create a small cluster of nodes including self
		n := rapid.IntRange(2, 6).Draw(t, "n")
		ids := make([]keys.NodeID, 0, n)
		nodes := make([]interfaces.Node, 0, n)
		for i := 0; i < n; i++ {
			id := nodeID(byte(i + 1))
			ids = append(ids, id)
			nodes = append(nodes, interfaces.Node{NodeID: id})
		}

		self := ids[0]
		carrier := &mockCarrier{nodes: nodes}
		cl := New(newTestLogger(), carrier, self)
		defer cl.Stop()

		// expected subscriber map: source -> set(subscriber)
		expected := make(map[keys.NodeID]map[keys.NodeID]struct{})

		ops := rapid.IntRange(1, 50).Draw(t, "ops")
		for i := 0; i < ops; i++ {
			op := rapid.IntRange(0, 5).Draw(t, "op")
			switch op {
			case 0: // subscribe single
				src := ids[rapid.IntRange(0, n-1).Draw(t, "src")]
				sub := ids[rapid.IntRange(0, n-1).Draw(t, "sub")]
				cl.SubscribeLog(ctx, src, sub)
				if expected[src] == nil {
					expected[src] = make(map[keys.NodeID]struct{})
				}
				expected[src][sub] = struct{}{}
			case 1: // unsubscribe single
				src := ids[rapid.IntRange(0, n-1).Draw(t, "src")]
				sub := ids[rapid.IntRange(0, n-1).Draw(t, "sub")]
				cl.UnsubscribeLog(ctx, src, sub)
				if expected[src] != nil {
					delete(expected[src], sub)
					if len(expected[src]) == 0 {
						delete(expected, src)
					}
				}
			case 2: // subscribe all
				sub := ids[rapid.IntRange(0, n-1).Draw(t, "sub")]
				cl.SubscribeLogAll(ctx, sub)
				for _, src := range ids {
					if expected[src] == nil {
						expected[src] = make(map[keys.NodeID]struct{})
					}
					expected[src][sub] = struct{}{}
				}
			case 3: // unsubscribe all
				sub := ids[rapid.IntRange(0, n-1).Draw(t, "sub")]
				cl.UnsubscribeLogAll(ctx, sub)
				for _, src := range ids {
					if expected[src] != nil {
						delete(expected[src], sub)
						if len(expected[src]) == 0 {
							delete(expected, src)
						}
					}
				}
			case 4: // self log -> should push to subscribers of self (except self)
				msg := fmt.Sprintf("m-%d", i)
				prev := len(carrier.getMessages())
				cl.Info(ctx, msg, nil)
				new := carrier.getMessages()[prev:]
				expSubs := expected[self]
				expCount := 0
				for id := range expSubs {
					if id != self {
						expCount++
					}
				}
				if len(new) != expCount {
					t.Fatalf("expected %d pushes, got %d", expCount, len(new))
				}
				// verify each push payload contains the log message
				for _, m := range new {
					if m.Msg.Type != interfaces.MessageTypeLogPush {
						t.Fatalf("expected LogPush, got %v", m.Msg.Type)
					}
					var ents []LogEntry
					if err := json.Unmarshal(m.Msg.Payload, &ents); err != nil {
						t.Fatalf("unmarshal payload: %v", err)
					}
					if len(ents) == 0 || ents[len(ents)-1].Message != msg {
						t.Fatalf("unexpected payload entries: %v", ents)
					}
				}
			case 5: // send historical logs
				src := ids[rapid.IntRange(0, n-1).Draw(t, "srcsend")]
				target := ids[rapid.IntRange(0, n-1).Draw(t, "target")]
				// create a historical entry for src
				e := LogEntry{
					Timestamp: time.Now(),
					NodeID:    src,
					Level:     LogLevelInfo,
					Message:   fmt.Sprintf("hist-%d", i),
				}
				cl.mu.Lock()
				cl.entries = append(cl.entries, e)
				cl.mu.Unlock()

				prev := len(carrier.getMessages())
				if err := cl.SendLog(ctx, target, src, time.Time{}); err != nil {
					t.Fatalf("SendLog error: %v", err)
				}
				new := carrier.getMessages()[prev:]
				if len(new) != 1 {
					t.Fatalf("expected 1 send, got %d", len(new))
				}
				if new[0].Target != target ||
					new[0].Msg.Type != interfaces.MessageTypeLogSendResponse {
					t.Fatalf("unexpected send message: %v", new[0])
				}
				var ents []LogEntry
				if err := json.Unmarshal(new[0].Msg.Payload, &ents); err != nil {
					t.Fatalf("unmarshal send payload: %v", err)
				}
				if len(ents) == 0 {
					t.Fatalf("empty send payload")
				}
			}
		}
	})
}
