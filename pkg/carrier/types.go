package carrier

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// Dispatcher handles authenticated inbound carrier
// messages.
type Dispatcher func( // A
	ctx context.Context,
	msg interfaces.Message,
	authCtx auth.AuthContext,
) (interfaces.Response, error)

// Config configures a carrier transport instance.
type Config struct { // A
	Logger              *slog.Logger
	ListenAddress       string
	LocalSigner         *keys.AsyncCrypt
	LocalNodeCerts      []interfaces.NodeCert
	LocalCASignatures   [][]byte
	TrustedAdminPubKeys [][]byte
	BootstrapPeers      []interfaces.PeerNode
	Dispatcher          Dispatcher
}

func (cfg Config) validate() error { // A
	if cfg.Logger == nil {
		return fmt.Errorf("logger must not be nil")
	}
	if cfg.ListenAddress == "" {
		return fmt.Errorf("listen address must not be empty")
	}
	if cfg.LocalSigner == nil {
		return fmt.Errorf("local signer must not be nil")
	}
	if len(cfg.LocalCASignatures) > 0 &&
		len(cfg.LocalCASignatures) != len(cfg.LocalNodeCerts) {
		return fmt.Errorf(
			"local cert/signature count mismatch",
		)
	}
	return nil
}
