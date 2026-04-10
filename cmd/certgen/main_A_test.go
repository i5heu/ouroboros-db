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
	cmd := exec.Command(
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
	cmd := exec.Command(bin, args...)
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
) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf(
			"expected failure: certgen %s\n%s",
			strings.Join(args, " "), out,
		)
	}
}

func TestFullWorkflow(t *testing.T) { // A
	bin := buildCertgen(t)
	dir := t.TempDir()

	adminKey := filepath.Join(dir, "admin.oukey")
	userKey := filepath.Join(dir, "user.oukey")
	nodeAdmin := filepath.Join(dir, "node-admin.oucert")
	nodeUser := filepath.Join(dir, "node-user.oucert")

	// Create admin CA.
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

	// Create user CA anchored to admin.
	out = run(t, bin, "user-ca",
		"-admin-key", adminKey,
		"-out", userKey,
	)
	if !strings.Contains(out, "user CA created") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "anchored to admin") {
		t.Fatal("missing anchor info")
	}

	// Sign node cert via admin CA.
	out = run(t, bin, "sign-node",
		"-ca-key", adminKey,
		"-out", nodeAdmin,
	)
	if !strings.Contains(out, "node cert created") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "node ID:") {
		t.Fatal("missing node ID")
	}

	// Sign node cert via user CA.
	out = run(t, bin, "sign-node",
		"-ca-key", userKey,
		"-out", nodeUser,
	)
	if !strings.Contains(out, "node cert created") {
		t.Fatalf("unexpected output: %s", out)
	}

	// Show all files.
	out = run(t, bin, "show", "-file", adminKey)
	if !strings.Contains(out, "type:    admin-ca") {
		t.Fatalf("show admin: %s", out)
	}

	out = run(t, bin, "show", "-file", userKey)
	if !strings.Contains(out, "type:    user-ca") {
		t.Fatalf("show user: %s", out)
	}

	out = run(t, bin, "show", "-file", nodeAdmin)
	if !strings.Contains(out, "type:      node-cert") {
		t.Fatalf("show node: %s", out)
	}

	out = run(t, bin, "show", "-file", nodeUser)
	if !strings.Contains(out, "type:      node-cert") {
		t.Fatalf("show node-user: %s", out)
	}
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
