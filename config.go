package ouroboros

import (
	"log/slog"
	"os"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// Config configures the database instance. Only Paths[0] is used at the
// moment; future versions may use multiple paths for sharding or tiering.
type Config struct {
	// Paths contains data directories.
	Paths []string
	// MinimumFreeGB is a free-space threshold for on-disk operations.
	MinimumFreeGB uint
	// Logger is an optional structured logger. If nil, a stderr logger is used.
	Logger *slog.Logger
	// UiPort specifies the port for the built-in web UI. If 0, the UI is
	// disabled.
	UiPort uint16
	// ClusterListenAddress is the QUIC listen address used by the cluster
	// transport. If empty, a local ephemeral port is used.
	ClusterListenAddress string
	// TrustedAdminPubKeys contains concatenated KEM+sign admin public keys
	// trusted for carrier peer authentication.
	TrustedAdminPubKeys [][]byte
	// LocalNodeCerts optionally provides the local carrier node cert bundle.
	LocalNodeCerts []interfaces.NodeCert
	// LocalCASignatures optionally provides the CA signatures matching
	// LocalNodeCerts.
	LocalCASignatures [][]byte
}

func defaultLogger() *slog.Logger { // A
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(h)
}
