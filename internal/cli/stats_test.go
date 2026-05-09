package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/state"
)

func TestRunStats_errors(t *testing.T) {
	var buf bytes.Buffer
	if err := RunStats([]string{}, &buf); err == nil {
		t.Fatal("expected error for empty args")
	}
	if err := RunStats([]string{"nope"}, &buf); err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestRunStats_help(t *testing.T) {
	var buf bytes.Buffer
	if err := RunStats([]string{"--help"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "stats engram") {
		t.Fatalf("help output: %q", buf.String())
	}
}

func TestRunStats_engram_defaultDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var buf bytes.Buffer
	if err := RunStats([]string{"engram"}, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Engram data directory") {
		t.Fatalf("missing header: %s", out)
	}
	if !strings.Contains(out, filepath.Join(home, ".engram")) {
		t.Fatalf("expected default path in output: %s", out)
	}
	if !strings.Contains(out, "default (~/.engram)") {
		t.Fatalf("expected default source: %s", out)
	}
	if !strings.Contains(out, "Suggested locations") {
		t.Fatalf("expected suggestions section: %s", out)
	}
	if strings.Contains(out, "Volume free space: (unavailable:") {
		t.Fatalf("expected volume space to use existing parent for missing default dir: %s", out)
	}
	if strings.Contains(out, "free) —") {
		t.Fatalf("suggested locations should not duplicate free-space text: %s", out)
	}
}

func TestRunStats_engram_fromState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	custom := filepath.Join(home, "my-engram-data")
	if err := state.Write(home, state.InstallState{EngramDataDir: custom}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := RunStats([]string{"engram"}, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, custom) {
		t.Fatalf("expected custom dir in output: %s", out)
	}
	if !strings.Contains(out, "state.json") {
		t.Fatalf("expected state source in output: %s", out)
	}
}

func TestRunStats_engram_missingCustomDirReportsParentVolume(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	missingDataDir := filepath.Join(home, "engram-data", "missing-child")
	if err := state.Write(home, state.InstallState{EngramDataDir: missingDataDir}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := RunStats([]string{"engram"}, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, missingDataDir) {
		t.Fatalf("expected custom missing path in output: %s", out)
	}
	if strings.Contains(out, "Volume free space: (unavailable:") {
		t.Fatalf("expected volume space to use existing parent for missing custom dir: %s", out)
	}
}

func TestRunStats_engram_malformedStateWarnsAndFallsBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	stateDir := filepath.Join(home, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(`{"engram_data_dir":`), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := RunStats([]string{"engram"}, &buf); err != nil {
		t.Fatalf("stats should warn, not fail, on malformed state: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "State warning:") {
		t.Fatalf("expected state warning in output: %s", out)
	}
	if !strings.Contains(out, filepath.Join(home, ".engram")) {
		t.Fatalf("expected fallback default path in output: %s", out)
	}
}
