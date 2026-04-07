package logtypes

import (
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

func TestLogLevelString(t *testing.T) { // A
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LogLevelDebug, "DEBUG"},
		{LogLevelInfo, "INFO"},
		{LogLevelWarn, "WARN"},
		{LogLevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}
	for _, tc := range tests {
		if got := tc.level.String(); got != tc.want {
			t.Errorf("LogLevel(%d).String() = %q, want %q",
				tc.level, got, tc.want)
		}
	}
}

func TestLogLevelToSlogLevel(t *testing.T) { // A
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LogLevelDebug, "DEBUG"},
		{LogLevelInfo, "INFO"},
		{LogLevelWarn, "WARN"},
		{LogLevelError, "ERROR"},
		{LogLevel(99), "INFO"},
	}
	for _, tc := range tests {
		got := tc.level.ToSlogLevel().String()
		if got != tc.want {
			t.Errorf("LogLevel(%d).ToSlogLevel() = %q, want %q",
				tc.level, got, tc.want)
		}
	}
}

func TestLogEntryIsExpired(t *testing.T) { // A
	now := time.Now()
	tests := []struct {
		name    string
		entry   LogEntry
		expired bool
	}{
		{
			name: "debug_25h_expired",
			entry: LogEntry{
				Timestamp: now.Add(-25 * time.Hour),
				Level:     LogLevelDebug,
			},
			expired: true,
		},
		{
			name: "debug_12h_not_expired",
			entry: LogEntry{
				Timestamp: now.Add(-12 * time.Hour),
				Level:     LogLevelDebug,
			},
			expired: false,
		},
		{
			name: "info_4d_expired",
			entry: LogEntry{
				Timestamp: now.Add(-4 * 24 * time.Hour),
				Level:     LogLevelInfo,
			},
			expired: true,
		},
		{
			name: "info_2d_not_expired",
			entry: LogEntry{
				Timestamp: now.Add(-2 * 24 * time.Hour),
				Level:     LogLevelInfo,
			},
			expired: false,
		},
		{
			name: "error_6d_expired",
			entry: LogEntry{
				Timestamp: now.Add(-6 * 24 * time.Hour),
				Level:     LogLevelError,
			},
			expired: true,
		},
		{
			name: "error_4d_not_expired",
			entry: LogEntry{
				Timestamp: now.Add(-4 * 24 * time.Hour),
				Level:     LogLevelError,
			},
			expired: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.entry.IsExpired(now)
			if got != tc.expired {
				t.Errorf("IsExpired = %v, want %v", got, tc.expired)
			}
		})
	}
}

func TestLogEntryIsExpiredUnknownLevel(t *testing.T) { // A
	now := time.Now()
	entry := LogEntry{
		Timestamp: now.Add(-4 * 24 * time.Hour),
		Level:     LogLevel(99),
	}
	if !entry.IsExpired(now) {
		t.Error("unknown level should fall back to Info TTL and be expired")
	}

	fresh := LogEntry{
		Timestamp: now.Add(-2 * 24 * time.Hour),
		Level:     LogLevel(99),
	}
	if fresh.IsExpired(now) {
		t.Error("unknown level 2d old should not be expired (Info TTL = 3d)")
	}
}

func TestLogEntryFields(t *testing.T) { // A
	entry := LogEntry{
		Timestamp: time.Now(),
		NodeID:    keys.NodeID{1, 2, 3},
		Level:     LogLevelInfo,
		Message:   "test message",
		Fields:    map[string]string{"key": "value"},
	}
	if entry.Message != "test message" {
		t.Errorf("Message = %q, want %q", entry.Message, "test message")
	}
	if entry.Fields["key"] != "value" {
		t.Errorf("Fields[key] = %q, want %q", entry.Fields["key"], "value")
	}
}
