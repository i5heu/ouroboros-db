package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/carrier"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

const demoIdentityFile = "carrier_demo_identity.json" // A

type nodeServer struct { // A
	db      *ouroboros.OuroborosDB
	carrier *carrier.Carrier
	logger  *slog.Logger

	mu       sync.Mutex
	received []receivedMessage
}

type receivedMessage struct { // A
	From    string `json:"from"`
	Type    int32  `json:"type"`
	Payload string `json:"payload"`
}

type joinRequest struct { // A
	Addresses []string `json:"addresses"`
	NodeID    string   `json:"nodeID"`
}

type broadcastRequest struct { // A
	Type    int32  `json:"type"`
	Payload string `json:"payload"`
	Mode    string `json:"mode"`
}

func main() { // A
	mode := flag.String("mode", "serve", "mode: serve or pubkey")
	dataDir := flag.String("data-dir", "", "node data directory")
	clusterAddr := flag.String("cluster-addr", "127.0.0.1:0", "cluster listen address")
	controlAddr := flag.String("control-addr", "127.0.0.1:0", "control http address")
	trustedAdmin := flag.String("trusted-admin", "", "base64 trusted admin public key")
	flag.Parse()

	if *mode == "pubkey" {
		if *dataDir == "" {
			fatalf("data-dir is required")
		}
		if err := printPubKey(*dataDir); err != nil {
			fatalf("pubkey: %v", err)
		}
		return
	}
	if *mode == "issue-cert" {
		if *dataDir == "" {
			fatalf("data-dir is required")
		}
		args := flag.Args()
		if len(args) != 2 {
			fatalf(
				"issue-cert requires admin-key-dir and trusted-admin-base64 arguments",
			)
		}
		if err := issueDemoCert(
			*dataDir,
			args[0],
			args[1],
		); err != nil {
			fatalf("issue-cert: %v", err)
		}
		return
	}
	if *mode != "serve" {
		fatalf("unsupported mode %q", *mode)
	}
	if *dataDir == "" {
		fatalf("data-dir is required")
	}
	if err := runServer(*dataDir, *clusterAddr, *controlAddr, *trustedAdmin); err != nil {
		fatalf("serve: %v", err)
	}
}

func runServer( // A
	dataDir string,
	clusterAddr string,
	controlAddr string,
	trustedAdmin string,
) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	trustedKeys, err := decodeTrustedKeys(trustedAdmin)
	if err != nil {
		return err
	}
	localNodeCerts, localCASignatures, err := loadDemoIdentity(dataDir)
	if err != nil {
		return err
	}
	db, err := ouroboros.New(ouroboros.Config{
		Paths:                []string{dataDir},
		Logger:               logger,
		ClusterListenAddress: clusterAddr,
		TrustedAdminPubKeys:  trustedKeys,
		LocalNodeCerts:       localNodeCerts,
		LocalCASignatures:    localCASignatures,
	})
	if err != nil {
		return err
	}
	cc := db.ClusterController()
	transport, ok := cc.Carrier().(*carrier.Carrier)
	if !ok {
		return errors.New("cluster carrier is not pkg/carrier")
	}
	ns := &nodeServer{
		db:      db,
		carrier: transport,
		logger:  logger,
	}
	if err := cc.RegisterHandler(
		interfaces.MessageTypeHeartbeat,
		[]auth.TrustScope{auth.ScopeAdmin},
		ns.handleIncoming,
	); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", ns.handleHealth)
	mux.HandleFunc("/peer", ns.handlePeer)
	mux.HandleFunc("/join", ns.handleJoin)
	mux.HandleFunc("/broadcast", ns.handleBroadcast)
	mux.HandleFunc("/messages", ns.handleMessages)
	ln, err := net.Listen("tcp", controlAddr)
	if err != nil {
		return err
	}
	logger.Info("clusterdemo ready",
		"nodeID", db.NodeID().String(),
		"clusterAddr", transport.ListenAddress(),
		"controlAddr", ln.Addr().String(),
	)
	return http.Serve(ln, mux)
}

func (ns *nodeServer) handleIncoming( // A
	_ context.Context,
	msg interfaces.Message,
	authCtx auth.AuthContext,
) (interfaces.Response, error) {
	ns.mu.Lock()
	ns.received = append(ns.received, receivedMessage{
		From:    authCtx.NodeID.String(),
		Type:    msg.GetType(),
		Payload: string(msg.GetPayload()),
	})
	ns.mu.Unlock()
	return interfaces.NewWireResponse(
		msg.GetPayload(),
		"",
		map[string]string{"from": authCtx.NodeID.String()},
	), nil
}

func (ns *nodeServer) handleHealth( // A
	w http.ResponseWriter,
	_ *http.Request,
) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (ns *nodeServer) handlePeer( // A
	w http.ResponseWriter,
	_ *http.Request,
) {
	peer := ns.carrier.LocalPeer()
	writeJSON(w, http.StatusOK, map[string]any{
		"nodeID":       peer.NodeID.String(),
		"nodeIDBase64": base64.StdEncoding.EncodeToString(peer.NodeID[:]),
		"addresses":    peer.Addresses,
	})
}

func (ns *nodeServer) handleJoin( // A
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	nodeID, err := decodeNodeID(req.NodeID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := ns.carrier.JoinCluster(interfaces.PeerNode{
		NodeID:    nodeID,
		Addresses: append([]string(nil), req.Addresses...),
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "joined"})
}

func (ns *nodeServer) handleBroadcast( // A
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req broadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	msg := interfaces.NewWireMessage(
		interfaces.MessageType(req.Type),
		[]byte(req.Payload),
	)
	switch req.Mode {
	case "", "reliable":
		success, failed, err := ns.carrier.BroadcastReliable(msg)
		writeJSON(w, http.StatusOK, map[string]any{
			"success": peerNodeIDs(success),
			"failed":  peerNodeIDs(failed),
			"error":   errorString(err),
		})
	case "unreliable":
		attempted := ns.carrier.BroadcastUnreliable(msg)
		writeJSON(w, http.StatusOK, map[string]any{
			"attempted": peerNodeIDs(attempted),
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown mode"})
	}
}

func (ns *nodeServer) handleMessages( // A
	w http.ResponseWriter,
	_ *http.Request,
) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	out := make([]receivedMessage, len(ns.received))
	copy(out, ns.received)
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

func decodeTrustedKeys( // A
	trustedAdmin string,
) ([][]byte, error) {
	if trustedAdmin == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(trustedAdmin)
	if err != nil {
		return nil, err
	}
	return [][]byte{decoded}, nil
}

func printPubKey(dataDir string) error { // A
	nodeDB, err := ouroboros.New(ouroboros.Config{Paths: []string{dataDir}})
	if err != nil {
		return err
	}
	pubKey := nodeDB.Crypt().Keys.GetPublicKey()
	kem, err := pubKey.MarshalBinaryKEM()
	if err != nil {
		return err
	}
	sign, err := pubKey.MarshalBinarySign()
	if err != nil {
		return err
	}
	combined := make([]byte, len(kem)+len(sign))
	copy(combined, kem)
	copy(combined[len(kem):], sign)
	_, err = fmt.Fprintln(os.Stdout, base64.StdEncoding.EncodeToString(combined))
	return err
}

type storedIdentity struct { // A
	NodeCerts    []storedNodeCert `json:"nodeCerts"`
	CASignatures []string         `json:"caSignatures"`
}

type storedNodeCert struct { // A
	IssuerCAHash    string `json:"issuerCAHash"`
	ValidFrom       int64  `json:"validFrom"`
	ValidUntil      int64  `json:"validUntil"`
	SerialBase64    string `json:"serialBase64"`
	CertNonceBase64 string `json:"certNonceBase64"`
}

func issueDemoCert( // A
	dataDir string,
	adminKeyDir string,
	trustedAdminBase64 string,
) error {
	adminSigner, err := keys.NewCryptFromFile(
		filepath.Join(adminKeyDir, "node.key"),
	)
	if err != nil {
		return err
	}
	nodeDB, err := ouroboros.New(ouroboros.Config{
		Paths: []string{dataDir},
	})
	if err != nil {
		return err
	}
	trustedBytes, err := base64.StdEncoding.DecodeString(trustedAdminBase64)
	if err != nil {
		return err
	}
	adminCA, err := auth.NewAdminCA(trustedBytes)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	serial := []byte(fmt.Sprintf("serial-%d", now))
	nonce := []byte(fmt.Sprintf("nonce-%d", now))
	cert, err := auth.NewNodeCert(
		nodeDB.Crypt().Keys.GetPublicKey(),
		adminCA.Hash(),
		now-60,
		now+3600,
		serial,
		nonce,
	)
	if err != nil {
		return err
	}
	canonical, err := auth.CanonicalNodeCert(cert)
	if err != nil {
		return err
	}
	sig, err := adminSigner.Sign(
		auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical),
	)
	if err != nil {
		return err
	}
	identity := storedIdentity{
		NodeCerts: []storedNodeCert{{
			IssuerCAHash:    cert.IssuerCAHash(),
			ValidFrom:       cert.ValidFrom(),
			ValidUntil:      cert.ValidUntil(),
			SerialBase64:    base64.StdEncoding.EncodeToString(cert.Serial()),
			CertNonceBase64: base64.StdEncoding.EncodeToString(cert.CertNonce()),
		}},
		CASignatures: []string{
			base64.StdEncoding.EncodeToString(sig),
		},
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(dataDir, demoIdentityFile),
		data,
		0o600,
	)
}

func loadDemoIdentity( // A
	dataDir string,
) ([]interfaces.NodeCert, [][]byte, error) {
	path := filepath.Join(dataDir, demoIdentityFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var identity storedIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, nil, err
	}
	nodeDB, err := ouroboros.New(ouroboros.Config{Paths: []string{dataDir}})
	if err != nil {
		return nil, nil, err
	}
	certs := make([]interfaces.NodeCert, 0, len(identity.NodeCerts))
	for _, stored := range identity.NodeCerts {
		serial, err := base64.StdEncoding.DecodeString(stored.SerialBase64)
		if err != nil {
			return nil, nil, err
		}
		nonce, err := base64.StdEncoding.DecodeString(stored.CertNonceBase64)
		if err != nil {
			return nil, nil, err
		}
		cert, err := auth.NewNodeCert(
			nodeDB.Crypt().Keys.GetPublicKey(),
			stored.IssuerCAHash,
			stored.ValidFrom,
			stored.ValidUntil,
			serial,
			nonce,
		)
		if err != nil {
			return nil, nil, err
		}
		certs = append(certs, cert)
	}
	signatures := make([][]byte, 0, len(identity.CASignatures))
	for _, encoded := range identity.CASignatures {
		sig, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, nil, err
		}
		signatures = append(signatures, sig)
	}
	return certs, signatures, nil
}

func decodeNodeID(value string) ([32]byte, error) { // A
	var nodeID [32]byte
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nodeID, err
	}
	if len(decoded) != len(nodeID) {
		return nodeID, fmt.Errorf("invalid nodeID length")
	}
	copy(nodeID[:], decoded)
	return nodeID, nil
}

func peerNodeIDs(peers []interfaces.PeerNode) []string { // A
	out := make([]string, 0, len(peers))
	for _, peer := range peers {
		out = append(out, peer.NodeID.String())
	}
	return out
}

func errorString(err error) string { // A
	if err == nil {
		return ""
	}
	return err.Error()
}

func writeJSON( // A
	w http.ResponseWriter,
	status int,
	payload any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		_, _ = w.Write([]byte(`{"error":"json encode failed"}`))
	}
}

func fatalf(format string, args ...any) { // A
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
