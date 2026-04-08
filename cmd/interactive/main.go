// Command interactive starts a small operator-facing
// OuroborosDB node process that can listen for QUIC
// peers and exchange simple user messages over the
// carrier transport.
//
// The command is intended as a manual bring-up and
// debugging tool rather than a long-running daemon.
// It loads a node identity from a .oucert file,
// builds a trust store from admin and user CA .oukey
// files, starts the cluster carrier listener, and
// then exposes a REPL for inspection and messaging.
//
// Supported workflows include:
//   - binding to a fixed or random listen address
//   - listing currently known peers
//   - broadcasting a "hello world"-style message
//
// Configuration overview:
//   - Storage.Paths[0] is populated from -data-dir and
//     is used as the local node state directory.
//   - Network.ListenAddress is populated from -listen
//     and controls the local QUIC bind address.
//   - Network.BootstrapAddresses is populated from
//     -bootstrap and is passed to the carrier for
//     automatic peer discovery.
//   - Identity.NodeCertPath is populated from
//     -node-cert and must point to a node .oucert file.
//   - Identity.AdminCAPaths is populated from
//     -admin-ca and seeds trusted admin roots.
//   - Identity.UserCAPaths is populated from -user-ca
//     and seeds optional anchored user CAs.
//
// Flag overview:
//   - -data-dir: local state directory used by the DB
//   - -listen: local QUIC listen address, use :0 for a
//     random free port
//   - -bootstrap: comma-separated peer seed addresses
//   - -node-cert: path to the node credential bundle
//   - -admin-ca: comma-separated admin CA key files
//   - -user-ca: comma-separated user CA key files
//
// The implementation keeps startup logic in this file
// so operators can read one entrypoint and understand
// how credentials, trust material, transport startup,
// and message handling are wired together.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/internal/auth"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
	"github.com/i5heu/ouroboros-db/internal/control"
	transport "github.com/i5heu/ouroboros-db/internal/transport"
	"github.com/i5heu/ouroboros-db/pkg/authfile"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"google.golang.org/protobuf/proto"
)

// parseSlogLevel converts a string level name to the
// corresponding slog.Level. Unknown names fall back to
// slog.LevelInfo.
func parseSlogLevel(name string) slog.Level { // A
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// main delegates all operational work to run and
// converts any returned error into a conventional
// non-zero process exit.
func main() { // A
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run performs the full startup sequence for the
// interactive node:
//   - parse command-line flags into an Ouroboros config
//   - construct the local database instance
//   - load the node's authenticated identity bundle
//   - load trusted admin/user CA material
//   - create the QUIC carrier and cluster controller
//   - register the hello-world message handler
//   - start listening for inbound peer connections
//   - hand control over to the REPL loop
//
// Keeping the flow linear here makes the startup path
// easy to audit when debugging transport or trust
// initialization problems.
func run() error { // A
	conf, logLevel := parseConfig()
	if conf.Identity.NodeCertPath == "" {
		return fmt.Errorf("-node-cert is required")
	}

	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})
	resolvedLogger := slog.New(h)
	conf.Logger = resolvedLogger

	db, err := ouroboros.New(&conf)
	if err != nil {
		return err
	}

	nodeIdentity, err := loadNodeIdentity(
		conf.Identity.NodeCertPath,
	)
	if err != nil {
		return err
	}
	carrierAuth, err := loadCarrierAuth(&conf, resolvedLogger)
	if err != nil {
		return err
	}

	tr, err := transport.New(&transport.CarrierConfig{
		BootstrapAddresses: conf.EffectiveBootstrapAddresses(),
		SelfCert:           nodeIdentity.Certs()[0],
		ListenAddress:      conf.EffectiveListenAddress(),
		Logger:             resolvedLogger,
		Auth:               carrierAuth,
		NodeIdentity:       nodeIdentity,
	})
	if err != nil {
		return err
	}

	controller, err := control.NewClusterController(
		tr,
		resolvedLogger,
	)
	if err != nil {
		return err
	}
	tr.SetController(controller)

	if err := interfaces.RegisterTypedHandler(
		controller,
		interfaces.MessageTypeUserMessage,
		[]auth.TrustScope{auth.ScopeUser},
		helloHandler(),
	); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	go func() {
		if err := tr.StartListener(ctx); err != nil &&
			err != context.Canceled {
			resolvedLogger.ErrorContext(
				ctx,
				"listener stopped",
				auth.LogKeyReason,
				err.Error(),
			)
			cancel()
		}
	}()

	fmt.Printf("node started: %s\n", shortNodeID(nodeIdentity.NodeID()))
	fmt.Printf("db node ID:   %s\n", shortNodeID(db.NodeID()))
	fmt.Printf("listening on: %s\n", tr.ListenAddress())

	return repl(ctx, cancel, tr, nodeIdentity.NodeID())
}

// parseConfig translates CLI flags into the top-level
// Ouroboros config used by both the local DB instance
// and the interactive networking layer.
//
// Notable flag behavior:
//   - -listen defaults to :0 so the OS can allocate a
//     free port for ad-hoc local testing
//   - -bootstrap accepts a comma-separated seed list
//   - -admin-ca and -user-ca accept comma-separated
//     trust anchors that are loaded into CarrierAuth
//   - -node-cert points at the node's persistent
//     credential bundle created by cmd/certgen
//
// The returned config is intentionally explicit even
// where fields overlap today:
//   - Paths and Storage.Paths both receive -data-dir so
//     existing constructor call sites keep working while
//     newer code can rely on StorageConfig.
//   - Network carries carrier-facing runtime settings.
//   - Identity carries auth/trust file locations.
func parseConfig() (ouroboros.Config, slog.Level) { // A
	var (
		dataDir  string
		listen   string
		boot     string
		nodeCert string
		adminCAs string
		userCAs  string
		logLevel string
	)
	configureUsage()
	flag.StringVar(
		&dataDir,
		"data-dir",
		"./data/interactive",
		"path to the local data directory",
	)
	flag.StringVar(
		&listen,
		"listen",
		":0",
		"local QUIC listen address",
	)
	flag.StringVar(
		&boot,
		"bootstrap",
		"",
		"comma-separated bootstrap host:port seeds",
	)
	flag.StringVar(
		&nodeCert,
		"node-cert",
		"",
		"path to the node .oucert file",
	)
	flag.StringVar(
		&adminCAs,
		"admin-ca",
		"",
		"comma-separated admin CA .oukey files",
	)
	flag.StringVar(
		&userCAs,
		"user-ca",
		"",
		"comma-separated user CA .oukey files",
	)
	flag.StringVar(
		&logLevel,
		"log-level",
		"info",
		"log verbosity: debug, info, warn, error",
	)
	flag.Parse()

	return ouroboros.Config{
		Paths: []string{dataDir},
		Storage: ouroboros.StorageConfig{
			Paths: []string{dataDir},
		},
		Network: ouroboros.NetworkConfig{
			ListenAddress:      listen,
			BootstrapAddresses: splitCSV(boot),
		},
		Identity: ouroboros.IdentityConfig{
			NodeCertPath: nodeCert,
			AdminCAPaths: splitCSV(adminCAs),
			UserCAPaths:  splitCSV(userCAs),
		},
	}, parseSlogLevel(logLevel)
}

// configureUsage replaces the default flag package help
// output with a command-specific description that shows
// how CLI flags map into the interactive node config.
//
// This keeps the operator-facing usage text close to the
// entrypoint and avoids requiring readers to infer the
// meaning of the flags from the code path alone.
func configureUsage() { // A
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		_, _ = fmt.Fprintf(out, "Usage:\n")
		_, _ = fmt.Fprintf(
			out,
			"  interactive -node-cert <node.oucert> [flags]\n\n",
		)
		_, _ = fmt.Fprintf(out, "Purpose:\n")
		_, _ = fmt.Fprintf(
			out,
			"  Start an interactive OuroborosDB node using QUIC transport and a local REPL.\n\n",
		)
		_, _ = fmt.Fprintf(out, "Config Mapping:\n")
		_, _ = fmt.Fprintf(
			out,
			"  -data-dir   -> Config.Paths and Config.Storage.Paths\n",
		)
		_, _ = fmt.Fprintf(
			out,
			"  -listen     -> Config.Network.ListenAddress\n",
		)
		_, _ = fmt.Fprintf(
			out,
			"  -bootstrap  -> Config.Network.BootstrapAddresses\n",
		)
		_, _ = fmt.Fprintf(
			out,
			"  -node-cert  -> Config.Identity.NodeCertPath\n",
		)
		_, _ = fmt.Fprintf(
			out,
			"  -admin-ca   -> Config.Identity.AdminCAPaths\n",
		)
		_, _ = fmt.Fprintf(
			out,
			"  -user-ca    -> Config.Identity.UserCAPaths\n\n",
		)
		_, _ = fmt.Fprintf(out, "Flags:\n")
		flag.PrintDefaults()
		_, _ = fmt.Fprintf(out, "\nExamples:\n")
		_, _ = fmt.Fprintf(
			out,
			"  interactive -node-cert ./node.oucert -admin-ca ./admin.oukey\n",
		)
		_, _ = fmt.Fprintf(
			out,
			"  interactive -node-cert ./node.oucert -admin-ca ./admin.oukey"+
				" -listen :9443 -bootstrap 127.0.0.1:9444\n",
		)
	}
}

// loadNodeIdentity reconstructs the local node's
// authenticated identity from a stored .oucert file.
//
// The returned certpkg.NodeIdentity contains the node's
// persistent ML-DSA-87 key material, the CA-signed
// certificate bundle, and a fresh session identity
// used by the QUIC/TLS transport.
func loadNodeIdentity( // A
	path string,
) (*certpkg.NodeIdentity, error) {
	ac, certFile, err := authfile.ReadNodeCert(path)
	if err != nil {
		return nil, err
	}
	identity, err := authfile.NodeCertToIdentity(ac, certFile)
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// loadCarrierAuth builds the verifier-side trust store
// used by the carrier for inbound and outbound peer
// authentication.
//
// Admin CA files are loaded first so anchored user CA
// files can be verified against their referenced admin
// roots during insertion.
func loadCarrierAuth( // A
	conf *ouroboros.Config,
	logger *slog.Logger,
) (interfaces.CarrierAuth, error) {
	carrierAuth := auth.NewCarrierAuth(logger)
	if len(conf.Identity.AdminCAPaths) == 0 &&
		len(conf.Identity.UserCAPaths) == 0 {
		_, certFile, err := authfile.ReadNodeCert(
			conf.Identity.NodeCertPath,
		)
		if err != nil {
			return nil, err
		}
		if err := authfile.AddEmbeddedTrust(
			carrierAuth,
			certFile,
		); err != nil {
			return nil, err
		}
		return carrierAuth, nil
	}
	for _, path := range conf.Identity.AdminCAPaths {
		if err := addCAFile(carrierAuth, path); err != nil {
			return nil, err
		}
	}
	for _, path := range conf.Identity.UserCAPaths {
		if err := addCAFile(carrierAuth, path); err != nil {
			return nil, err
		}
	}
	return carrierAuth, nil
}

// addCAFile loads one .oukey file and inserts it into
// the carrier trust store according to its declared
// file type.
//
// Admin CAs are inserted directly. User CAs require
// their anchor signature and referenced admin hash so
// CarrierAuth can verify the user trust chain.
func addCAFile( // A
	carrierAuth interfaces.CarrierAuth,
	path string,
) error {
	ac, file, err := authfile.ReadCAKey(path)
	if err != nil {
		return err
	}
	pub := ac.GetPublicKey()
	pubBytes, err := auth.MarshalPubKeyBytes(&pub)
	if err != nil {
		return fmt.Errorf("marshal CA pubkey: %w", err)
	}
	switch file.Type {
	case "admin-ca":
		return carrierAuth.AddAdminPubKey(pubBytes)
	case "user-ca":
		return carrierAuth.AddUserPubKey(
			pubBytes,
			file.AnchorSig,
			file.AnchorAdmin,
		)
	default:
		return fmt.Errorf("unsupported CA key type %q", file.Type)
	}
}

// repl runs the command-line control loop used after
// networking has started.
//
// Supported commands are intentionally small and map
// directly to carrier operations:
//   - help: show command summary
//   - id: print the local node ID
//   - listen: print the bound listen address
//   - peers: list currently known peers
//   - hello [text]: broadcast a user message
//   - broadcast [text]: alias of hello
//   - quit/exit: stop the process gracefully
//
// The loop also exits when the process context is
// cancelled, which happens on SIGINT/SIGTERM or when
// the listener fails fatally.
func repl( //nolint:cyclop // A: REPL command dispatch inherently requires multiple branches
	ctx context.Context,
	cancel context.CancelFunc,
	rc transport.RuntimeCarrier,
	nodeID keys.NodeID,
) error {
	scanner := bufio.NewScanner(os.Stdin)
	printHelp()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if !scanner.Scan() {
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		cmd := fields[0]
		args := fields[1:]

		switch cmd {
		case "help":
			printHelp()
		case "id":
			fmt.Printf("node ID: %s\n", shortNodeID(nodeID))
		case "listen":
			fmt.Printf("listening on: %s\n", rc.ListenAddress())
		case "peers":
			printPeers(rc)
		case "hello", "broadcast":
			text := "hello world"
			if len(args) > 0 {
				text = strings.Join(args, " ")
			}
			if err := broadcastHello(
				rc,
				shortNodeID(nodeID),
				text,
			); err != nil {
				fmt.Printf("broadcast failed: %v\n", err)
				continue
			}
		case "reconnect":
			fmt.Println("reconnecting to bootstrap...")
			if err := rc.Reconnect(); err != nil {
				fmt.Printf("reconnect failed: %v\n", err)
				continue
			}
			fmt.Println("reconnected successfully")
		case "exit", "quit":
			cancel()
			return nil
		default:
			fmt.Printf("unknown command %q\n", cmd)
		}
	}
}

// broadcastHello sends a user-message payload to all
// currently known peers over reliable carrier streams.
//
// The message format is deliberately small and schema-
// backed so operators can confirm basic connectivity
// before more complex cluster message types are added.
func broadcastHello( // A
	rc transport.RuntimeCarrier,
	from string,
	text string,
) error {
	payload, err := proto.Marshal(&interfaces.UserMessage{
		From: from,
		Text: text,
	})
	if err != nil {
		return fmt.Errorf("encode hello payload: %w", err)
	}
	success, failed, err := rc.BroadcastReliable(
		interfaces.Message{
			Type:    interfaces.MessageTypeUserMessage,
			Payload: payload,
		},
	)
	if err != nil {
		return err
	}
	fmt.Printf(
		"broadcast delivered to %d peers, %d failed\n",
		len(success),
		len(failed),
	)
	return nil
}

// helloHandler returns the inbound message handler used
// for MessageTypeUserMessage.
//
// It receives the decoded protobuf payload, logs the
// sender details, and mirrors the message to stdout so
// the interactive command behaves like a simple chat/
// debug console.
func helloHandler() func( // A
	ctx context.Context,
	message *interfaces.UserMessage,
	peer keys.NodeID,
	trustScope auth.TrustScope,
) (*interfaces.ResponseEmptyPayload, error) {
	return func(
		_ context.Context,
		message *interfaces.UserMessage,
		peer keys.NodeID,
		_ auth.TrustScope,
	) (*interfaces.ResponseEmptyPayload, error) {
		fmt.Printf(
			"received from %s (%s): %s\n",
			message.From,
			shortNodeID(peer),
			message.Text,
		)
		return &interfaces.ResponseEmptyPayload{}, nil
	}
}

// printPeers renders the current peer snapshot from the
// carrier registry with role and connection status so
// operators can tell at a glance which peers are live.
func printPeers(c interfaces.Carrier) { // A
	peers := c.GetNodes()
	if len(peers) == 0 {
		fmt.Println("no peers connected")
		return
	}
	for _, peer := range peers {
		status := "disconnected"
		if c.IsConnected(peer.NodeID) {
			status = "connected"
		}
		fmt.Printf(
			"- %s  addr=%-21s  role=%-6s  status=%s\n",
			shortNodeID(peer.NodeID),
			strings.Join(peer.Addresses, ","),
			peer.Role.String(),
			status,
		)
	}
}

// printHelp prints the supported REPL commands.
func printHelp() { // A
	fmt.Println("commands: help, id, listen, peers")
	fmt.Println(
		"          hello [text], broadcast [text]",
	)
	fmt.Println("          reconnect, quit")
}

// splitCSV parses a simple comma-separated flag value
// into a trimmed slice while discarding empty entries.
//
// The helper keeps flag parsing consistent for the
// bootstrap, admin-ca, and user-ca inputs.
func splitCSV(raw string) []string { // A
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// shortNodeID returns a shortened hexadecimal prefix of
// a full node ID for operator-facing logs and prompts.
//
// The interactive tool prefers short IDs in terminal
// output so peer lists and received-message prints stay
// readable during manual testing.
func shortNodeID(nodeID keys.NodeID) string { // A
	return fmt.Sprintf("%x", nodeID[:8])
}
