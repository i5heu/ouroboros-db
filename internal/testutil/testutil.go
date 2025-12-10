package testutil

import (
	"flag"
	"testing"
)

var RunLong = flag.Bool("long", false, "run long/heavy tests")

func RequireLong(t *testing.T) {
	t.Helper()
	if !*RunLong {
		t.Skip("skipping long test (use -long to enable)")
	}
}

func IsLongEnabled() bool {
	return *RunLong
}
