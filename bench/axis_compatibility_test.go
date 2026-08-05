package main

import (
	"strings"
	"testing"
)

// This file belongs to the compatibility axis and is deleted with it:
// `rm bench/axis_compatibility*.go` removes the axis whole, and the core
// corpus still builds, tests and reports exactly as it did.
//
// A driven run proves most of this axis for itself against a real binary
// (see the commit that added this axis for the before/after evidence: cw01
// fails against a pre-#2447-fix binary and passes against a fixed one). What
// a `go test` run can pin, cheaply and on every change, is the axis's own
// declarations and the half of each detector that never fires on a healthy
// build -- exactly the axis_real_world_test.go idiom.

func TestCompatibilityAxisDeclaresItselfAndItsJourneys(t *testing.T) {
	found := false
	for _, axis := range Axes() {
		if axis.Name != compatibilityAxis {
			continue
		}
		found = true
		if !axis.BlackBox {
			t.Fatal("the compatibility axis drives the product's CLI only; declaring it not black-box would surrender the portability it actually has")
		}
		if len(axis.Properties) == 0 {
			t.Fatal("the compatibility axis declares no properties")
		}
		journeys := axis.Journeys()
		if len(journeys) == 0 {
			t.Fatal("the compatibility axis contributes no journeys")
		}
		for _, journey := range journeys {
			if !strings.HasPrefix(journey.ID, "cw") {
				t.Fatalf("axis journey %q does not carry the axis id prefix, so a reader cannot tell it from a core journey at a glance", journey.ID)
			}
			if len(journey.Steps) == 0 {
				t.Fatalf("journey %q has no steps", journey.ID)
			}
		}
		want := map[string]bool{
			"cw01-direct-start-refuses-uncompletable-base-diff":       true,
			"cw02-hyphenated-review-start-always-refuses":             true,
			"cw03-negotiated-start-then-direct-cwd-capture-completes": true,
		}
		for _, journey := range journeys {
			delete(want, journey.ID)
		}
		if len(want) != 0 {
			t.Fatalf("expected compatibility journeys not found: %v", want)
		}
	}
	if !found {
		t.Fatal("the compatibility axis is not registered")
	}
}

// compatAssertNamedRefusal is cw01 and cw02's shared detector. A pass on a
// genuine refusal (nonzero exit, runnable continuation) means nothing unless
// the detector demonstrably fires on the two ways a refusal can go wrong:
// exiting 0 (no refusal at all), and refusing with nothing runnable in the
// message (a dead end).
func TestCompatAssertNamedRefusalFiresOnSuccessAndOnDeadEnd(t *testing.T) {
	if err := compatAssertNamedRefusal(Observation{ExitCode: 1, Stdout: "Error: cannot proceed; rerun with `gentle-ai review start --contract x`"}, "test"); err != nil {
		t.Fatalf("the detector rejected a genuine named refusal: %v", err)
	}
	if err := compatAssertNamedRefusal(Observation{ExitCode: 0, Stdout: `{"action":"created"}`}, "test"); err == nil {
		t.Fatal("a successful (exit 0) start was accepted as a refusal; cw01/cw02 would pass while measuring nothing")
	}
	if err := compatAssertNamedRefusal(Observation{ExitCode: 1, Stderr: "Error: cannot proceed"}, "test"); err == nil {
		t.Fatal("a refusal naming no runnable command was accepted; cw01/cw02 would pass on a dead end")
	}
}

// compatAssertHyphenatedStartRefuses adds one more requirement on top of
// compatAssertNamedRefusal: the refusal must specifically name `gentle-ai
// review start` (with a space), not just any command.
func TestCompatAssertHyphenatedStartRefusesRequiresTheSpacedVerb(t *testing.T) {
	ok := Observation{ExitCode: 1, Stderr: "legacy v1 review lineage is read-only: use gentle-ai review start"}
	if err := compatAssertHyphenatedStartRefuses(nil, ok); err != nil {
		t.Fatalf("rejected a refusal that names gentle-ai review start: %v", err)
	}
	wrongVerb := Observation{ExitCode: 1, Stderr: "refused; rerun `gentle-ai review status --next-transition`"}
	if err := compatAssertHyphenatedStartRefuses(nil, wrongVerb); err == nil {
		t.Fatal("accepted a refusal that names a DIFFERENT gentle-ai command, not `review start`; cw02 would pass without proving the redirect it claims")
	}
}

// compatAssertNoRepositoryContext is cw03's whole point: it must fail the
// moment --repository-context appears among the counted invocation's
// arguments, on either spelling (space or =).
func TestCompatAssertNoRepositoryContextFiresOnEitherSpelling(t *testing.T) {
	if err := compatAssertNoRepositoryContext(nil, Observation{Args: []string{"review", "capture-result", "--cwd", "/repo"}}); err != nil {
		t.Fatalf("the detector fired on a clean --cwd invocation: %v", err)
	}
	if err := compatAssertNoRepositoryContext(nil, Observation{Args: []string{"review", "capture-result", "--repository-context", "rctx1_x"}}); err == nil {
		t.Fatal("--repository-context (space form) was used and the detector did not fire; cw03 would pass while measuring the opaque-handle path it claims not to use")
	}
	if err := compatAssertNoRepositoryContext(nil, Observation{Args: []string{"review", "capture-result", "--repository-context=rctx1_x"}}); err == nil {
		t.Fatal("--repository-context= (equals form) was used and the detector did not fire; cw03 would pass while measuring the opaque-handle path it claims not to use")
	}
}
