package clusterlog

import (
	"log/slog"
	"time"
)

// LogLevel represents the severity of a log entry.
type LogLevel int // H

const ( // H
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// ttlByLevel maps each LogLevel to its retention
// duration before automatic cleanup.
var ttlByLevel = map[LogLevel]time.Duration{ // H
	LogLevelDebug: 24 * time.Hour,
	LogLevelInfo:  3 * 24 * time.Hour,
	LogLevelWarn:  3 * 24 * time.Hour,
	LogLevelError: 5 * 24 * time.Hour,
}

// String returns the human-readable name of the
// LogLevel.
func (l LogLevel) String() string { // H
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// toSlogLevel converts a LogLevel to the
// corresponding slog.Level value.
func (l LogLevel) toSlogLevel() slog.Level { // H
	switch l {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
