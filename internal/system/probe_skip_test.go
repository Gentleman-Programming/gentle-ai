package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// recordProbes puts executable stand-ins for every probed tool on PATH and
// replaces the probe runner with a recorder, so the test observes which
// commands detection would execute without running any of them.
func recordProbes(t *testing.T) func() []string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"git", "curl", "node", "npm", "brew", "go"} {
		file := name
		if runtime.GOOS == "windows" {
			file += ".exe"
		}
		if err := os.WriteFile(filepath.Join(bin, file), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var mu sync.Mutex
	var ran []string
	restore := runProbe
	t.Cleanup(func() { runProbe = restore })
	runProbe = func(cmd *exec.Cmd) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, filepath.Base(cmd.Args[0])+" "+strings.Join(cmd.Args[1:], " "))
		return []byte("1.0.0"), nil
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), ran...)
	}
}

func TestDetectProbesToolsByDefault(t *testing.T) {
	t.Setenv(EngramBinEnv, "")
	ran := recordProbes(t)

	result, err := Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	want := []string{"git --version", "curl --version", "node --version", "npm --version", "go version"}
	if runtime.GOOS == "darwin" {
		want = append(want, "brew --version")
	}
	if runtime.GOOS != "windows" {
		want = append(want, "npm config get prefix")
	}
	got := strings.Join(ran(), "\n")
	for _, command := range want {
		if !strings.Contains(got, command) {
			t.Fatalf("default detection did not run %q; ran:\n%s", command, got)
		}
	}
	if len(result.Dependencies.Dependencies) == 0 {
		t.Fatal("default detection reported no dependencies")
	}
}

func TestDetectRunsNoToolWithCallerSuppliedEngram(t *testing.T) {
	t.Setenv(EngramBinEnv, filepath.Join(t.TempDir(), "engram"))
	ran := recordProbes(t)

	result, err := Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	report := DetectDependencies(context.Background(), result.System.Profile)

	if got := ran(); len(got) != 0 {
		t.Fatalf("detection ran %q; with %s set it must run no git, curl, node, npm, brew or go command", got, EngramBinEnv)
	}
	for _, r := range []DependencyReport{result.Dependencies, report} {
		if !r.AllPresent || len(r.Dependencies) != 0 || len(r.MissingRequired) != 0 {
			t.Fatalf("skipped detection report = %#v, want an empty report with AllPresent", r)
		}
	}
	if !result.System.Supported {
		t.Fatalf("platform support must still come from PATH lookups: %#v", result.System)
	}
}
