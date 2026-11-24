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
	// The CLI decoder decodes intended_untracked as []string and every
	// --intended-untracked flag carries one repo-relative path, so the
	// published schema's valid case is the string representation.
	for _, selection := range []struct{ scope, paths string }{
		{"exclude", `[]`},
		{"select", `["candidate.go"]`},
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

	// Issue #4311: the schema published one changed-path object per entry
	// while the decoder accepts only strings. A host that follows such an
	// object representation must see the lane report a divergence, mirroring
	// the decoder's refusal rather than conformance.
	rejected := &battery{repoRoot: repoRoot}
	rejected.record("select-object", []byte(`{"schema":"`+identity+`","untracked_scope":"select","expected_untracked_inventory":"`+inventory+`","intended_untracked":[{"path":"candidate.go","status":"A","old_mode":"000000","new_mode":"100644","deleted":false,"type_changed":false,"mode_only":false,"intended_untracked":true}]}`))
	rejected.runSchemaLane()
	if len(rejected.checks) != 1 || rejected.checks[0].Lane != schemaLane || rejected.checks[0].Name != "conformance: "+identity ||
		rejected.checks[0].Status != statusFail || !strings.HasPrefix(rejected.checks[0].Note, "1/1 envelope(s) diverge") {
		t.Fatalf("schema lane checks for the object representation = %#v, want one divergence", rejected.checks)
	}
}
