package agents

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file is the Wave 4 Wave-0 CON-09/10/11 forbidden-construction guard
// (design.md "Wave-0 Prerequisite: Adapter Behavioral-Depth Trace"). The
// design's own trace concluded the Go adapter packages
// (internal/agents/opencode, internal/agents/pi, internal/agents/claude) are
// already clean — this test encodes the exact grep set the trace used as a
// guard so a future adapter change cannot silently reintroduce one of the
// four forbidden constructions the ReviewAdapter boundary names:
//
//  1. CLI flag literals for the review subprocess protocol
//     (e.g. "--result-artifact", "--expected-revision", "--consent", "--cwd"
//     used as review command-line arguments an adapter constructs itself).
//  2. Revision / expected-revision identifiers
//     (ExpectedRevision, expected_revision, expected-revision).
//  3. Target / lineage identity identifiers
//     (LineageID, lineage_id, TargetIdentity, target_identity).
//  4. Binding JSON shapes
//     (ReviewBinding, binding.json, gentle-ai.sdd-review-binding).
//
// Scope: this guard covers only the in-repo Go adapter dispatch surfaces
// (CON-09/10/11). The retired bundled OpenCode plugin asset and the
// Claude/OpenCode sdd-apply.md contract-pin text are tracked separately by
// slices S3/S4/S7 — they are asset text, not Go adapter dispatch code, and
// are intentionally out of this guard's file set.

// adapterForbiddenConstructionPatterns are the exact substrings the CON-09/
// 10/11 trace grepped for. Each carries the forbidden-construction category
// it evidences.
var adapterForbiddenConstructionPatterns = []struct {
	category string
	token    string
}{
	{"CLI flag literal", "--result-artifact"},
	{"CLI flag literal", "--expected-revision"},
	{"CLI flag literal", "--consent"},
	{"CLI flag literal", "--repository-context"},
	{"CLI flag literal", "--captured-results"},
	{"revision identity", "ExpectedRevision"},
	{"revision identity", "expected_revision"},
	{"revision identity", "expected-revision"},
	{"target/lineage identity", "LineageID"},
	{"target/lineage identity", "lineage_id"},
	{"target/lineage identity", "TargetIdentity"},
	{"target/lineage identity", "target_identity"},
	{"binding JSON", "ReviewBinding"},
	{"binding JSON", "binding.json"},
	{"binding JSON", "gentle-ai.sdd-review-binding"},
}

// adapterForbiddenConstructionPackageDirs are the CON-09/10/11 in-repo Go
// adapter dispatch surfaces this guard covers.
var adapterForbiddenConstructionPackageDirs = []string{
	filepath.Join("opencode"),
	filepath.Join("pi"),
	filepath.Join("claude"),
}

// TestAdapterForbiddenConstructionGuardCatchesKnownShapes unit-tests the
// scanner against synthetic sources so its detection logic is proven correct
// independent of whatever the current adapter packages contain.
func TestAdapterForbiddenConstructionGuardCatchesKnownShapes(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantViolation bool
	}{
		{
			name: "clean adapter source",
			src: `package opencode

func (a *Adapter) Detect() (bool, error) { return true, nil }
`,
			wantViolation: false,
		},
		{
			name: "constructs a review CLI flag literal",
			src: `package opencode

func dispatch() []string { return []string{"review", "--expected-revision", "abc"} }
`,
			wantViolation: true,
		},
		{
			name: "declares a lineage identity field",
			src: `package pi

type dispatchRequest struct {
	LineageID string
}
`,
			wantViolation: true,
		},
		{
			name: "declares a ReviewBinding shape",
			src: `package claude

type ReviewBinding struct {
	Lineage string
}
`,
			wantViolation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := scanAdapterForbiddenConstructions("synthetic_adapter_test_input.go", []byte(tt.src))
			if got := len(violations) > 0; got != tt.wantViolation {
				t.Fatalf("violations = %v (len=%d), want non-empty=%v", violations, len(violations), tt.wantViolation)
			}
		})
	}
}

// TestAdapterForbiddenConstructionGuardHoldsForProductionFiles runs the same
// scanner against every real production adapter.go (and sibling) file in the
// CON-09/10/11 Go package directories and fails with the exact violation
// evidence if any is found. This is the trace itself, encoded as a guard.
func TestAdapterForbiddenConstructionGuardHoldsForProductionFiles(t *testing.T) {
	files, err := productionAdapterFiles(t)
	if err != nil {
		t.Fatalf("productionAdapterFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no production adapter .go files found — guard has nothing to prove")
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			violations, err := scanAdapterForbiddenConstructionsFile(file)
			if err != nil {
				t.Fatalf("scanAdapterForbiddenConstructionsFile(%s): %v", file, err)
			}
			if len(violations) > 0 {
				t.Fatalf("%s references forbidden constructions: %v", file, violations)
			}
		})
	}
}

// productionAdapterFiles globs every non-test .go file in the CON-09/10/11
// adapter package directories.
func productionAdapterFiles(t *testing.T) ([]string, error) {
	t.Helper()
	var production []string
	for _, dir := range adapterForbiddenConstructionPackageDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			base := filepath.Base(match)
			if len(base) >= len("_test.go") && base[len(base)-len("_test.go"):] == "_test.go" {
				continue
			}
			production = append(production, match)
		}
	}
	sort.Strings(production)
	return production, nil
}

// scanAdapterForbiddenConstructions returns one human-readable violation
// string per forbidden-construction token found in src, each carrying the
// evidenced category. It is a substring grep, not an AST walk — this
// mirrors the exact method the CON-09/10/11 trace used (design.md: "grep the
// adapter's Go package ... for the four forbidden constructions"), so the
// guard proves the same thing the trace claims.
func scanAdapterForbiddenConstructions(filename string, src []byte) []string {
	var violations []string
	text := string(src)
	for _, pattern := range adapterForbiddenConstructionPatterns {
		if strings.Contains(text, pattern.token) {
			violations = append(violations, filename+": forbidden construction ("+pattern.category+") "+pattern.token)
		}
	}
	return violations
}

// scanAdapterForbiddenConstructionsFile reads path and scans it.
func scanAdapterForbiddenConstructionsFile(path string) ([]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return scanAdapterForbiddenConstructions(path, src), nil
}
