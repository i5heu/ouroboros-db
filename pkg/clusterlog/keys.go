package clusterlog

// Slog attribute key constants used throughout the
// clusterlog package. Centralised here for sloglint
// compliance and consistent structured logging.
const ( // AC
	keyNodeID    = "nodeID"
	keyLevel     = "level"
	keyTimestamp = "timestamp"
	keyFields    = "fields"
	keyTarget    = "targetNodeID"
	keySource    = "sourceNodeID"
	keySince     = "since"
	keyEntries   = "entries"
	keyError     = "error"
)
