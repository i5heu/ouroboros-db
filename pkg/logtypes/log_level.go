package logtypes

import (
	"log/slog"
	"time"
)

type LogLevel int // A

const ( // A
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

var ttlByLevel = map[LogLevel]time.Duration{ // A
	LogLevelDebug: 24 * time.Hour,
	LogLevelInfo:  3 * 24 * time.Hour,
	LogLevelWarn:  3 * 24 * time.Hour,
	LogLevelError: 5 * 24 * time.Hour,
}

func (l LogLevel) String() string { // A
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

func (l LogLevel) ToSlogLevel() slog.Level { // A
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

func (e *LogEntry) IsExpired(now time.Time) bool { // A
	ttl, ok := ttlByLevel[e.Level]
	if !ok {
		ttl = ttlByLevel[LogLevelInfo]
	}
	return now.After(e.Timestamp.Add(ttl))
}
