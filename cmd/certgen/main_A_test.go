package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildCertgen(t *testing.T) string { // A
	t.Helper()
	bin := filepath.Join(t.TempDir(), "certgen")
	cmd := exec.Command( //#nosec G204 // safe: test binary built from known source
		"go", "build", "-o", bin, ".",
	)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func run( // A
	t *testing.T, bin string, args ...string,
) string {
	t.Helper()
	cmd := exec.Command(bin, args...) //#nosec G204 // safe: test binary from buildCertgen
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf(
			"certgen %s: %v\n%s",
			strings.Join(args, " "), err, out,
		)
	}
	return string(out)
}

func runExpectFail( // A
	t *testing.T, bin string, args ...string,
) string {
	t.Helper()
	cmd := exec.Command(bin, args...) //#nosec G204 // safe: test binary from buildCertgen
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf(
			"expected failure: certgen %s",
			strings.Join(args, " "),
		)
	}
	return string(out)
}

func verifyAdminCAOutput( // A
	t *testing.T, bin, adminKey string,
) {
	t.Helper()
	out := run(t, bin, "admin-ca", "-out", adminKey)
	if !strings.Contains(out, "admin CA created") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "hash:") {
		t.Fatal("missing hash in admin-ca output")
	}
	if _, err := os.Stat(adminKey); err != nil {
		t.Fatalf("admin key not written: %v", err)
	}
}

func verifyUserCAOutput( // A
	t *testing.T, bin, adminKey, userKey string,
) {
	t.Helper()
	out := run(t, bin, "user-ca",
		"-admin-key", adminKey,
		"-out", userKey,
	)
	if !strings.Contains(out, "user CA created") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "anchored to admin") {
		t.Fatal("missing anchor info")
	}
}

func verifySignNodeOutput( // A
	t *testing.T, bin, caKey, nodeOut string,
	wantNodeID bool,
) {
	t.Helper()
	out := run(t, bin, "sign-node",
		"-ca-key", caKey,
		"-out", nodeOut,
	)
	if !strings.Contains(out, "node cert created") {
		t.Fatalf("unexpected output: %s", out)
	}
	if wantNodeID && !strings.Contains(out, "node ID:") {
		t.Fatal("missing node ID")
	}
}

func verifyShowType( // A
	t *testing.T, bin, file, wantType string,
) {
	t.Helper()
	out := run(t, bin, "show", "-file", file)
	if !strings.Contains(out, wantType) {
		t.Fatalf("show %s: %s", file, out)
	}
}

func TestFullWorkflow(t *testing.T) { // A
	bin := buildCertgen(t)
	dir := t.TempDir()

	adminKey := filepath.Join(dir, "admin.oukey")
	userKey := filepath.Join(dir, "user.oukey")
	nodeAdmin := filepath.Join(dir, "node-admin.oucert")
	nodeUser := filepath.Join(dir, "node-user.oucert")

	verifyAdminCAOutput(t, bin, adminKey)
	verifyUserCAOutput(t, bin, adminKey, userKey)
	verifySignNodeOutput(t, bin, adminKey, nodeAdmin, true)
	verifySignNodeOutput(t, bin, userKey, nodeUser, false)
	verifyShowType(t, bin, adminKey, "type:    admin-ca")
	verifyShowType(t, bin, userKey, "type:    user-ca")
	verifyShowType(t, bin, nodeAdmin, "type:      node-cert")
	verifyShowType(t, bin, nodeUser, "type:      node-cert")
}

func TestSignNodeCustomValidity(t *testing.T) { // A
	bin := buildCertgen(t)
	dir := t.TempDir()

	adminKey := filepath.Join(dir, "admin.oukey")
	nodeCert := filepath.Join(dir, "node.oucert")

	run(t, bin, "admin-ca", "-out", adminKey)
	out := run(t, bin, "sign-node",
		"-ca-key", adminKey,
		"-out", nodeCert,
		"-validity", "48h",
	)
	if !strings.Contains(out, "node cert created") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestMissingFlags(t *testing.T) { // A
	bin := buildCertgen(t)

	runExpectFail(t, bin, "admin-ca")
	runExpectFail(t, bin, "user-ca")
	runExpectFail(t, bin, "user-ca", "-out", "x")
	runExpectFail(t, bin, "sign-node")
	runExpectFail(t, bin, "show")
}

func TestShowGarbageFile(t *testing.T) { // A
	bin := buildCertgen(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.oukey")
	_ = os.WriteFile(
		bad, []byte("not protobuf"), 0o600,
	)
	runExpectFail(t, bin, "show", "-file", bad)
}

func TestNoArgs(t *testing.T) { // A
	bin := buildCertgen(t)
	runExpectFail(t, bin)
}

func TestUnknownCommand(t *testing.T) { // A
	bin := buildCertgen(t)
	runExpectFail(t, bin, "bogus")
}
