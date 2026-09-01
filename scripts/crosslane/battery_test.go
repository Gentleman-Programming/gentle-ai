package main

import "testing"

func TestCommittedMediumCandidateFailsWhenBaseWriteFails(t *testing.T) {
	battery := &battery{
		workRoot: t.TempDir(),
		lineages: map[string]lineageScope{},
	}

	repo, baseTree, ok := battery.committedMediumCandidate(
		"test", "invalid-base-write", ".", "base", "candidate",
	)
	if ok || repo != "" || baseTree != "" {
		t.Fatalf("committed candidate after base write failure = repo %q, base %q, ok %t", repo, baseTree, ok)
	}
	if len(battery.checks) != 1 {
		t.Fatalf("failure checks = %#v, want exactly one committed-process failure", battery.checks)
	}
	failure := battery.checks[0]
	if failure.Lane != "test" || failure.Name != "committed process base" || failure.Status != statusFail {
		t.Fatalf("base write failure = %#v, want committed process base FAIL", failure)
	}
}
