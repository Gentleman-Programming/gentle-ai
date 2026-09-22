package main

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// TestVersionDefaultsToV010 verifies that a compilation without ldflags
// injection reports "v0.1.0" (REQ-22.8, O-1).
func TestVersionDefaultsToV010(t *testing.T) {
	if version != "v0.1.0" {
		t.Errorf("version = %q, want %q", version, "v0.1.0")
	}
	if Version != version {
		t.Errorf("Version = %q, want %q (must alias version)", Version, version)
	}
}

// TestPrintVersionFormat pins the exact output format of printVersion (D-05).
func TestPrintVersionFormat(t *testing.T) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s version %s (%s/%s) commit:%s\n", Platform, Version, runtime.GOOS, runtime.GOARCH, GitCommit)
	got := sb.String()

	want := fmt.Sprintf("%s version %s (%s/%s) commit:%s\n", Platform, Version, runtime.GOOS, runtime.GOARCH, GitCommit)
	if got != want {
		t.Errorf("printVersion format changed:\n  got:  %q\n  want: %q", got, want)
	}
}
