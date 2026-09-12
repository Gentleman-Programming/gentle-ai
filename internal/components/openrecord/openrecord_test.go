package openrecord

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
)

type fakeDetector struct {
	paths map[string]string
}

func (d fakeDetector) LookPath(name string) (string, error) {
	if path, ok := d.paths[name]; ok && path != "" {
		return path, nil
	}
	return "", fmt.Errorf("not found: %s", name)
}

type fakeRunner struct {
	calls  [][]string
	failOn func(name string, args []string) error
	// simulateEmit writes a minimal manifest into the staging dir when it sees
	// `openrecord skills --emit <dir> --with-qmd`, the way the real binary
	// would — a fake runner otherwise leaves the staging dir empty, and
	// fanOut correctly treats a missing manifest as an error.
	simulateEmit bool
}

func (r *fakeRunner) Run(name string, args ...string) error {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if r.failOn != nil {
		if err := r.failOn(name, args); err != nil {
			return err
		}
	}
	if r.simulateEmit && name == "openrecord" && len(args) >= 2 && args[0] == "skills" && args[1] == "--emit" {
		return writeFakeManifest(args[2])
	}
	return nil
}

func writeFakeManifest(dir string) error {
	content := `{"version":1,"emitter":"openrecord@test","with_qmd":true,"entries":[{"path":"openrecord-consult/SKILL.md","hash":"deadbeef"}]}`
	if err := os.MkdirAll(filepath.Join(dir, "openrecord-consult"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "openrecord-consult", "SKILL.md"), []byte("stub skill"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestName), []byte(content), 0o644)
}

// TestBinaryInstallCommandRouteParity pins that the first-time install route
// matches effectiveMethod's own predicate: Go on PATH routes to go install,
// its absence routes to the install script.
func TestBinaryInstallCommandRouteParity(t *testing.T) {
	t.Run("go on PATH prefers go install", func(t *testing.T) {
		command, err := binaryInstallCommand(fakeDetector{paths: map[string]string{"go": "/usr/bin/go"}})
		if err != nil {
			t.Fatalf("binaryInstallCommand() error = %v", err)
		}
		if len(command) < 2 || command[0] != "go" || command[1] != "install" {
			t.Fatalf("binaryInstallCommand() = %v, want a go install invocation", command)
		}
	})

	t.Run("go absent falls back to the install script", func(t *testing.T) {
		command, err := binaryInstallCommand(fakeDetector{})
		if err != nil {
			t.Fatalf("binaryInstallCommand() error = %v", err)
		}
		if len(command) == 0 || command[0] == "go" {
			t.Fatalf("binaryInstallCommand() = %v, want the install script, not go install", command)
		}
	})
}

// TestInstallSequenceOrdering covers the three documented sequencing cases:
// binary already on PATH skips step 1; a failed qmd install stops before any
// skills are emitted or written; the happy path runs every step in order.
func TestInstallSequenceOrdering(t *testing.T) {
	t.Run("binary already on PATH skips the install step", func(t *testing.T) {
		detector := fakeDetector{paths: map[string]string{"openrecord": "/usr/local/bin/openrecord"}}
		runner := &fakeRunner{simulateEmit: true}

		if _, err := Install(t.TempDir(), nil, runner, detector); err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		for _, call := range runner.calls {
			if len(call) >= 2 && call[0] == "go" && call[1] == "install" {
				t.Fatalf("go install was run even though openrecord was already on PATH: %v", runner.calls)
			}
		}
	})

	t.Run("qmd install failure stops before any skills are emitted", func(t *testing.T) {
		detector := fakeDetector{paths: map[string]string{"go": "/usr/bin/go"}}
		runner := &fakeRunner{failOn: func(name string, args []string) error {
			if name == "openrecord" && len(args) > 0 && args[0] == "qmd" {
				return fmt.Errorf("qmd install failed")
			}
			return nil
		}}

		if _, err := Install(t.TempDir(), nil, runner, detector); err == nil {
			t.Fatal("Install() did not return an error when qmd install failed")
		}
		for _, call := range runner.calls {
			if len(call) >= 2 && call[0] == "openrecord" && call[1] == "skills" {
				t.Fatalf("skills --emit ran despite a failed qmd install: %v", runner.calls)
			}
		}
	})

	t.Run("happy path runs every step in order", func(t *testing.T) {
		detector := fakeDetector{paths: map[string]string{"go": "/usr/bin/go"}}
		runner := &fakeRunner{simulateEmit: true}

		if _, err := Install(t.TempDir(), nil, runner, detector); err != nil {
			t.Fatalf("Install() error = %v", err)
		}

		var sawInstall, sawQmd, sawEmit bool
		var installIdx, qmdIdx, emitIdx int
		for i, call := range runner.calls {
			switch {
			case call[0] == "go" && call[1] == "install":
				sawInstall, installIdx = true, i
			case call[0] == "openrecord" && call[1] == "qmd":
				sawQmd, qmdIdx = true, i
			case call[0] == "openrecord" && call[1] == "skills":
				sawEmit, emitIdx = true, i
			}
		}
		if !sawInstall || !sawQmd || !sawEmit {
			t.Fatalf("did not observe all three steps: %v", runner.calls)
		}
		if !(installIdx < qmdIdx && qmdIdx < emitIdx) {
			t.Fatalf("steps ran out of order: %v", runner.calls)
		}
	})

	t.Run("nil runner is rejected up front", func(t *testing.T) {
		if _, err := Install(t.TempDir(), nil, nil, fakeDetector{}); err == nil {
			t.Fatal("Install() with a nil runner did not error")
		}
	})
}

// TestDetectStatus pins that presence alone is not enough: DetectStatus
// requires `<path> version` to actually succeed.
func TestDetectStatus(t *testing.T) {
	t.Run("installed when version succeeds", func(t *testing.T) {
		detector := fakeDetector{paths: map[string]string{"openrecord": "/usr/local/bin/openrecord"}}
		status := DetectStatus(t.TempDir(), detector, &fakeRunner{})
		if status.CLI != communitytool.AvailabilityAvailable {
			t.Fatalf("CLI = %v, want available", status.CLI)
		}
	})

	t.Run("not installed when version fails", func(t *testing.T) {
		detector := fakeDetector{paths: map[string]string{"openrecord": "/usr/local/bin/openrecord"}}
		runner := &fakeRunner{failOn: func(string, []string) error { return fmt.Errorf("boom") }}
		status := DetectStatus(t.TempDir(), detector, runner)
		if status.CLI == communitytool.AvailabilityAvailable {
			t.Fatal("CLI reported available despite `version` failing")
		}
	})

	t.Run("not installed when not resolvable at all", func(t *testing.T) {
		status := DetectStatus(t.TempDir(), fakeDetector{}, &fakeRunner{})
		if status.CLI == communitytool.AvailabilityAvailable {
			t.Fatal("CLI reported available with no resolvable binary")
		}
	})
}
