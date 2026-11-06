package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaLaneCompilesPublishedIntendedUntrackedSelection(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	b := &battery{repoRoot: repoRoot}
	const identity = "gentle-ai.review-intended-untracked-selection/v1"
	inventory := "sha256:" + strings.Repeat("a", 64)
	for _, selection := range []struct{ scope, paths string }{
		{"exclude", `[]`},
		{"select", `[{"path":"candidate.go","status":"A","old_mode":"000000","new_mode":"100644","deleted":false,"type_changed":false,"mode_only":false,"intended_untracked":true}]`},
	} {
		b.record(selection.scope, []byte(`{"schema":"`+identity+`","untracked_scope":"`+selection.scope+`","expected_untracked_inventory":"`+inventory+`","intended_untracked":`+selection.paths+`}`))
	}

	// Exercise the lane's actual registration by each published $id, including
	// resolution of sibling $refs, rather than synthesizing resource URLs.
	b.runSchemaLane()
	want := check{schemaLane, "conformance: " + identity, statusPass, "2 captured envelope(s) match the published schema"}
	if len(b.checks) != 1 || b.checks[0] != want {
		t.Fatalf("schema lane checks = %#v, want %#v", b.checks, want)
	}
}
