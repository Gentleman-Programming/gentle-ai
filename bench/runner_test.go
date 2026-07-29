package main

import (
	"os"
	"path/filepath"
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
