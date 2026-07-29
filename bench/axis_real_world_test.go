package main

import (
	"strings"
	"testing"
)

// This file belongs to the real-world axis and is deleted with it:
// `rm bench/axis_real_world*.go` removes the axis whole, and the core corpus
// still builds, tests and reports exactly as it did.
//
// A run proves most of this axis for itself: every fixture reads its shape
// back out of git or the product before a counted command runs. What a run
// cannot prove is the half of an assertion that never fires on a healthy
// build — rw03 and rw09 pass by finding NOTHING in the emitted bytes, and a
// detector that could never fire would make that pass a tautology. So the
// detector's firing half is pinned here.

// The axis declares itself black-box, and unlike damaged-store that claim is
// TRUE here: nothing product-owned is read or written. The report prints the
// claim verbatim, so it is pinned rather than left to a reviewer's eye — in
// both directions, because an axis that quietly started writing product state
// under this declaration would be lying in every report it appears in.
func TestRealWorldAxisDeclaresItIsBlackBox(t *testing.T) {
	found := false
	for _, axis := range Axes() {
		if axis.Name != realWorldAxis {
			continue
		}
		found = true
		if !axis.BlackBox {
			t.Fatal("the real-world axis drives git, the filesystem and the product CLI only; declaring it not black-box would surrender the portability it actually has")
		}
		if len(axis.Journeys()) == 0 {
			t.Fatal("the real-world axis contributes no journeys")
		}
		for _, journey := range axis.Journeys() {
			if !strings.HasPrefix(journey.ID, "rw") {
				t.Fatalf("axis journey %q does not carry the axis id prefix, so a reader cannot tell it from a core journey at a glance", journey.ID)
			}
		}
	}
	if !found {
		t.Fatal("the real-world axis is not registered")
	}
}

// rw03 and rw09 pass by NOT finding a sentinel in the emitted bytes. That
// pass means nothing unless the detector demonstrably fires when the sentinel
// IS there — on either stream, because an echo on stderr is the same finding
// as an echo on stdout.
func TestMustNotEchoFiresOnEitherStream(t *testing.T) {
	check := mustNotEcho("sentinel-value-123", "test finding")
	if err := check(nil, Observation{Stdout: "clean", Stderr: "clean"}); err != nil {
		t.Fatalf("the detector fired on clean output: %v", err)
	}
	if err := check(nil, Observation{Stdout: "prefix sentinel-value-123 suffix"}); err == nil {
		t.Fatal("the sentinel was echoed on stdout and the detector did not fire; rw03 and rw09 would pass while measuring nothing")
	}
	if err := check(nil, Observation{Stderr: "prefix sentinel-value-123 suffix"}); err == nil {
		t.Fatal("the sentinel was echoed on stderr and the detector did not fire; rw03 and rw09 would pass while measuring nothing")
	}
}

// afterEach chains a lineage-remembering hook with an assertion; an assertion
// error must surface, not be swallowed by a later hook succeeding.
func TestAfterEachSurfacesTheFirstFailure(t *testing.T) {
	failing := mustNotEcho("boom", "test finding")
	calls := 0
	counting := func(*Sandbox, Observation) error { calls++; return nil }
	err := afterEach(counting, failing, counting)(nil, Observation{Stdout: "boom"})
	if err == nil {
		t.Fatal("a failing hook in the chain was swallowed")
	}
	if calls != 1 {
		t.Fatalf("hooks after the failure ran %d times; the chain must stop at the first error", calls)
	}
}
