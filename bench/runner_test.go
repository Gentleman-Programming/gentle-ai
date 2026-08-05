package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBinary writes an executable that answers a fixed argv with a fixed
// message, so the capability probe can be tested without a real gentle-ai.
func fakeBinary(t *testing.T, script string) *Sandbox {
	t.Helper()
	root := t.TempDir()
	sandbox, err := newSandbox(filepath.Join(root, "fake"), root)
	if err != nil {
		t.Fatalf("newSandbox: %v", err)
	}
	if err := os.MkdirAll(sandbox.Repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(sandbox.Binary, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return sandbox
}

// A build that HAS the flag fails on state, not on shape. The probe must read
// that as supported: "sdd-attempt requires --cwd" is the repository's answer,
// not the CLI's.
func TestProbeCapabilityAcceptsAStateFailure(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Error: sdd-attempt requires --cwd" >&2; exit 1`)
	capability := &Capability{
		Verb:  []string{"sdd-attempt", "finish"},
		Probe: []string{"sdd-attempt", "finish", "--expected-binding-revision=probe"},
	}
	supported, reason := newCapabilityProbe(sandbox).supported(capability)
	if !supported {
		t.Fatalf("supported = false (%s), want true: a state failure is not a missing surface", reason)
	}
}

// A build that LACKS the flag rejects the shape, and the journey must record
// `unsupported` rather than a state failure it never had.
func TestProbeCapabilityRejectsAMissingFlag(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Error: flag provided but not defined: -expected-binding-revision" >&2; exit 1`)
	capability := &Capability{
		Verb:  []string{"sdd-attempt", "finish"},
		Probe: []string{"sdd-attempt", "finish", "--expected-binding-revision=probe"},
	}
	supported, reason := newCapabilityProbe(sandbox).supported(capability)
	if supported {
		t.Fatal("supported = true, want false: the binary rejected the shape of the command")
	}
	if reason == "" {
		t.Fatal("an unsupported surface must say which argv it probed")
	}
}

// The reason Probe exists at all: `sdd-attempt <op> --help` is rejected with
// the same words a missing flag produces, so the DEFAULT probe reports a build
// that fully supports the verb as lacking it. This test pins that trap, so
// nobody quietly converts these capabilities back to the help-reading form.
func TestHelpProbeMisreadsAVerbThatParsesItsOwnFlags(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Error: flag provided but not defined: -help" >&2; exit 1`)
	supported, _ := newCapabilityProbe(sandbox).supported(&Capability{Verb: []string{"sdd-attempt", "finish"}})
	if supported {
		t.Skip("the help probe no longer misreads this shape; Probe may be unnecessary")
	}
}

// Probe and the default help read must not share a cache slot, or one verb's
// answer would silently decide the other's.
func TestProbeAndHelpProbeDoNotShareACacheEntry(t *testing.T) {
	sandbox := fakeBinary(t, `
case "$*" in
  *--help*) echo "Error: flag provided but not defined: -help" >&2; exit 1 ;;
  *)        echo "Error: sdd-attempt requires --cwd" >&2; exit 1 ;;
esac`)
	probe := newCapabilityProbe(sandbox)
	if supported, _ := probe.supported(&Capability{Verb: []string{"sdd-attempt", "finish"}}); supported {
		t.Fatal("the help read should have reported unsupported for this fake")
	}
	supported, reason := probe.supported(&Capability{
		Verb:  []string{"sdd-attempt", "finish"},
		Probe: []string{"sdd-attempt", "finish", "--expected-binding-revision=probe"},
	})
	if !supported {
		t.Fatalf("supported = false (%s), want true: the probe answer must not come from the help cache", reason)
	}
}

// readBack must never charge its git subprocesses to the next counted
// invocation, or an uncounted proof would inflate a measured dimension.
func TestReadBackBlanksGitTrace(t *testing.T) {
	sandbox := fakeBinary(t, `echo "GIT_TRACE=[$GIT_TRACE]"`)
	observation := sandbox.readBack("sdd-attempt", "status")
	if observation.Stdout != "GIT_TRACE=[]\n" {
		t.Fatalf("readBack stdout = %q, want a blanked GIT_TRACE", observation.Stdout)
	}
	counted := sandbox.invoke([]string{"sdd-attempt", "status"})
	if counted.Stdout == "GIT_TRACE=[]\n" {
		t.Fatal("a counted invocation lost GIT_TRACE, so git_subprocesses would stop being observable")
	}
}

func TestSandboxEnvIncludesBenchReceiptMutationPath(t *testing.T) {
	sandbox, err := newSandbox("gentle-ai", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sandbox.BenchReceiptMutationPath = filepath.Join(sandbox.Root, "receipt.json")
	for _, entry := range sandbox.env() {
		if entry == "GENTLE_AI_BENCH_MUTATE_RECEIPT="+sandbox.BenchReceiptMutationPath {
			return
		}
	}
	t.Fatal("sandbox environment has no benchmark receipt mutation path")
}

func TestSandboxEnvKeepsTempFilesInsideTheSandbox(t *testing.T) {
	sandbox, err := newSandbox("gentle-ai", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(sandbox.Root, "tmp")
	found := map[string]bool{}
	for _, entry := range sandbox.env() {
		if entry == "TMP="+want {
			found["TMP"] = true
			continue
		}
		if entry == "TEMP="+want {
			found["TEMP"] = true
			continue
		}
		if entry == "TMPDIR="+want {
			found["TMPDIR"] = true
			continue
		}
		if strings.HasPrefix(entry, "TMP=") || strings.HasPrefix(entry, "TEMP=") || strings.HasPrefix(entry, "TMPDIR=") {
			t.Fatalf("temporary directory = %q, want %q", entry, want)
		}
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("sandbox temp directory %q: %v", want, err)
	}
	if !found["TMP"] || !found["TEMP"] || !found["TMPDIR"] {
		t.Fatalf("sandbox temp variables = %v, want TMP, TEMP and TMPDIR", found)
	}
}

func TestSandboxEnvKeepsWindowsHomeInsideTheSandbox(t *testing.T) {
	sandbox, err := newSandbox("gentle-ai", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range sandbox.env() {
		if entry == "USERPROFILE="+sandbox.Home {
			return
		}
	}
	t.Fatalf("sandbox environment has no USERPROFILE=%q", sandbox.Home)
}

func TestSelectedAuthorityCaptureHelpersSelectTheSandboxLineage(t *testing.T) {
	tests := []struct {
		name string
		run  func(*journeyRun) error
	}{
		{name: "results", run: sddCaptureSelectedAuthorityLenses},
		{name: "evidence", run: sddCaptureSelectedAuthorityEvidence},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox := fakeBinary(t, `printf '%s\n' '{"next_transition":{"kind":"complete"}}'`)
			sandbox.Lineage = "review-sdd-newer"
			run := &journeyRun{sandbox: sandbox, accumulator: newAccumulator(), step: tt.name}

			if err := tt.run(run); err != nil {
				t.Fatalf("capture helper: %v", err)
			}
			if len(run.accumulator.records) != 1 {
				t.Fatalf("invocations = %d, want 1", len(run.accumulator.records))
			}
			args := run.accumulator.records[0].Args
			if len(args) < 2 || args[len(args)-2] != "--lineage" || args[len(args)-1] != sandbox.Lineage {
				t.Fatalf("status args = %q, want selected lineage %q", args, sandbox.Lineage)
			}
		})
	}
}
