package ouroboros

import (
	"fmt"
	"log/slog"
	"os"
)

// StorageConfig groups persistent storage settings.
type StorageConfig struct { // A
	// Paths contains data directories.
	Paths []string
	// MinimumFreeGB is a free-space threshold for
	// on-disk operations.
	MinimumFreeGB uint
}

// NetworkConfig groups node-to-node runtime settings.
type NetworkConfig struct { // A
	// ListenAddress is the local QUIC bind address.
	// Use ":0" to request a random free port.
	ListenAddress string
	// BootstrapAddresses are seed peers used for
	// initial cluster joins.
	BootstrapAddresses []string
}

// IdentityConfig groups credential file locations.
type IdentityConfig struct { // A
	// NodeCertPath points to the node .oucert file.
	NodeCertPath string
	// AdminCAPaths lists trusted admin CA .oukey files.
	AdminCAPaths []string
	// UserCAPaths lists trusted user CA .oukey files.
	UserCAPaths []string
}

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
	// Storage groups persistent storage settings.
	Storage StorageConfig
	// Network groups runtime transport settings.
	Network NetworkConfig
	// Identity groups credential file locations.
	Identity IdentityConfig
}

// PrimaryPath returns the effective data path.
func (c Config) PrimaryPath() (string, error) { // A
	if len(c.Storage.Paths) > 0 {
		return c.Storage.Paths[0], nil
	}
	if len(c.Paths) > 0 {
		return c.Paths[0], nil
	}
	return "", fmt.Errorf(
		"at least one path must be provided in config",
	)
}

// EffectivePaths returns the normalized storage path list.
func (c Config) EffectivePaths() []string { // A
	if len(c.Storage.Paths) > 0 {
		out := make([]string, len(c.Storage.Paths))
		copy(out, c.Storage.Paths)
		return out
	}
	out := make([]string, len(c.Paths))
	copy(out, c.Paths)
	return out
}

// EffectiveMinimumFreeGB returns the normalized free-space threshold.
func (c Config) EffectiveMinimumFreeGB() uint { // A
	if c.Storage.MinimumFreeGB != 0 {
		return c.Storage.MinimumFreeGB
	}
	return c.MinimumFreeGB
}

// EffectiveListenAddress returns the configured bind address.
func (c Config) EffectiveListenAddress() string { // A
	return c.Network.ListenAddress
}

// EffectiveBootstrapAddresses returns a copy of the configured seeds.
func (c Config) EffectiveBootstrapAddresses() []string { // A
	out := make([]string, len(c.Network.BootstrapAddresses))
	copy(out, c.Network.BootstrapAddresses)
	return out
}

func defaultLogger() *slog.Logger { // A
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(h)
}
