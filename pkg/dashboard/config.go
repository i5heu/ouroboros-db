// Package dashboard provides an UNSECURE debug dashboard for OuroborosDB.
//
// WARNING: This dashboard is intended for development and debugging only.
// It should NEVER be enabled in production environments as it exposes
// internal cluster state and potentially sensitive data without authentication.
//
// The dashboard is enabled via the --UNSECURE-dashboard flag and optionally
// allows data uploads via --UNSECURE-upload-via-dashboard.
package dashboard

import (
	"log/slog"

	"github.com/i5heu/ouroboros-db/internal/carrier"
	"github.com/i5heu/ouroboros-db/pkg/distribution"
	"github.com/i5heu/ouroboros-db/pkg/index"
	"github.com/i5heu/ouroboros-db/pkg/storage"
)

// Config holds the configuration for the dashboard.
type Config struct { // A
	// Enabled indicates whether the dashboard should be started.
	// Corresponds to --UNSECURE-dashboard flag.
	Enabled bool

	// AllowUpload indicates whether data uploads are permitted.
	// Corresponds to --UNSECURE-upload-via-dashboard flag.
	AllowUpload bool

	// PreferredPort is the port to try first. If 0 or unavailable,
	// the dashboard will find an available port automatically.
	PreferredPort uint16

	// Carrier is used for inter-node communication (log subscription,
	// dashboard announcements).
	Carrier carrier.Carrier

	// LogBroadcaster is used to subscribe to logs from other nodes.
	LogBroadcaster *carrier.LogBroadcaster

	// BlockStore provides access to stored blocks.
	BlockStore storage.BlockStore

	// Index provides access to vertex indexes.
	Index index.Index

	// DistributionTracker provides access to block distribution state.
	DistributionTracker distribution.BlockDistributionTracker

	// Logger is used for dashboard logging.
	Logger *slog.Logger
}

// DefaultPort is the default port for the dashboard if no preferred port
// is specified.
const DefaultPort uint16 = 8420 // A
