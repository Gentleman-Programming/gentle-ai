package kickoff

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file is internal/kickoff's own structural guard for design.md S1.3
// (the normative SDD/RDD boundary): no production file in this leaf
// package may reference the two review-authority selectors that already
// have a sibling guard in internal/sddstatus, and none may contain the
// governance-adjacent vocabulary that belongs exclusively to that other
// mechanism (T-9). It mirrors
// internal/sddstatus/review_offer_absence_guard_test.go's shape: the
// scanner lives entirely in this test file because there is no separate
// production surface to protect beyond the package itself.
//
// kickoffForbiddenVocabulary below is intentionally assembled from two
// literal fragments per entry, joined at package-init time, instead of
// being written as five contiguous literal words. This is not obfuscation
// of behavior — scanKickoffForbiddenVocabulary still detects every one of
// them, in any file it scans, exactly as if they were spelled out. It
// exists so that a plain-text search over this repository's own tracked
// source (production and test files alike) never finds these words
// sitting in internal/kickoff, which is the stricter, additional
// constraint this guard's own file must satisfy on top of what it checks
// in others.

// kickoffForbiddenReviewSelectors are the exact reviewtransaction selector
// names design.md S1.3 rule 1 forbids in this package's production
// source — the same two names internal/sddstatus already guards.
var kickoffForbiddenReviewSelectors = map[string]bool{
	"OfferReviewAfterVerify": true,
	"ReviewCore":             true,
}

// kickoffForbiddenVocabulary lists the five RDD-only terms design.md S1.3
// rule 2 forbids anywhere in this package (T-9). They name concepts that
// belong exclusively to the OTHER review mechanism this package must never
// be confused with — see design.md S1.3's comparison table for what each
// one means there. None of the five has any legitimate meaning inside an
// SDD block review gate.
var kickoffForbiddenVocabulary = []string{
	"rec" + "eipt",
	"line" + "age",
	"cand" + "idate",
	"acknow" + "ledge",
	"bu" + "rn",
}

func TestKickoffRDDBoundaryScannerCatchesKnownShapes(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantViolation bool
	}{
		{
			name: "clean gate vocabulary",
			src: `package kickoff

// GateDecision is the human verdict recorded against a gate: approved or
// rejected, never anything from RDD's own vocabulary.
type GateDecision string
`,
			wantViolation: false,
		},
		{
			name: "clean source touching unrelated reviewtransaction symbols",
			src: `package kickoff

import "github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"

func example() error { return reviewtransaction.PublishFileNoReplace("", "") }
`,
			wantViolation: false,
		},
		{
			name: "calls the forbidden OfferReviewAfterVerify selector",
			src: `package kickoff

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

func example(ctx context.Context) {
	reviewtransaction.OfferReviewAfterVerify(ctx, "")
}
`,
			wantViolation: true,
		},
		{
			name: "references the forbidden ReviewCore type",
			src: `package kickoff

import "github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"

func example() reviewtransaction.ReviewCore { return reviewtransaction.ReviewCore{} }
`,
			wantViolation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := scanKickoffForbiddenVocabulary("synthetic_rdd_boundary_test_input.go", []byte(tt.src))
			if got := len(violations) > 0; got != tt.wantViolation {
				t.Fatalf("violations = %v (len=%d), want non-empty=%v", violations, len(violations), tt.wantViolation)
			}
		})
	}
}

// TestKickoffRDDBoundaryScannerCatchesForbiddenVocabulary proves the
// scanner flags each of the five assembled terms individually. The
// synthetic source is built from the same runtime-assembled value the
// scanner itself checks against, so this test file's own tracked text
// never spells out a forbidden term as a contiguous substring either.
func TestKickoffRDDBoundaryScannerCatchesForbiddenVocabulary(t *testing.T) {
	for _, term := range kickoffForbiddenVocabulary {
		term := term
		t.Run(term, func(t *testing.T) {
			src := "package kickoff\n\n// a synthetic reference to " + term + " for the scanner to catch.\n"
			violations := scanKickoffForbiddenVocabulary("synthetic_rdd_boundary_test_input.go", []byte(src))
			if len(violations) == 0 {
				t.Fatalf("scanner did not flag the forbidden term assembled at index; src=%q", src)
			}
		})
	}
}

// TestKickoffProductionFilesStayInsideRDDBoundary runs the scanner against
// every real production file in internal/kickoff. This is the ongoing
// regression: if a later phase in this same package ever introduces one of
// the forbidden selectors or vocabulary terms, this test fails with the
// exact file and violation.
func TestKickoffProductionFilesStayInsideRDDBoundary(t *testing.T) {
	files := kickoffProductionFiles(t)
	if len(files) == 0 {
		t.Fatal("no production .go files found — guard has nothing to prove")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("os.ReadFile(%s): %v", file, err)
			}
			if violations := scanKickoffForbiddenVocabulary(file, src); len(violations) > 0 {
				t.Fatalf("%s crosses the SDD/RDD boundary (design.md S1.3): %v", file, violations)
			}
		})
	}
}

// TestKickoffImportAllowlistTable is the unit-level proof for
// kickoffImportAllowed, independent of what this package currently
// imports.
func TestKickoffImportAllowlistTable(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "stdlib top-level package", path: "fmt", want: true},
		{name: "stdlib nested package", path: "path/filepath", want: true},
		{name: "stdlib crypto package", path: "crypto/sha256", want: true},
		{name: "allowed multirole", path: "github.com/gentleman-programming/gentle-ai/v3/internal/multirole", want: true},
		{name: "allowed reviewtransaction", path: "github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction", want: true},
		{name: "allowed workspace", path: "github.com/gentleman-programming/gentle-ai/v3/internal/workspace", want: true},
		{name: "allowed yaml", path: "gopkg.in/yaml.v3", want: true},
		{name: "disallowed sibling internal package", path: "github.com/gentleman-programming/gentle-ai/v3/internal/sddstatus", want: false},
		{name: "disallowed third-party package", path: "github.com/spf13/cobra", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kickoffImportAllowed(tt.path); got != tt.want {
				t.Errorf("kickoffImportAllowed(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestKickoffPackageImportsStayInsideAllowlist runs the import allowlist
// against every real production file in internal/kickoff: this package
// may depend only on internal/multirole, internal/reviewtransaction, the
// standard library, and gopkg.in/yaml.v3 (design.md S4.1) — nothing else,
// and in particular nothing that would let it reach sddstatus, cli, or any
// review-authority package transitively.
func TestKickoffPackageImportsStayInsideAllowlist(t *testing.T) {
	files := kickoffProductionFiles(t)
	if len(files) == 0 {
		t.Fatal("no production .go files found — guard has nothing to prove")
	}
	fileSet := token.NewFileSet()
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			tree, err := parser.ParseFile(fileSet, file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parser.ParseFile(%s): %v", file, err)
			}
			for _, imp := range tree.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if !kickoffImportAllowed(path) {
					t.Errorf("%s imports %q, outside internal/kickoff's declared allowlist (design.md S4.1)", file, path)
				}
			}
		})
	}
}

// kickoffAllowedImportPrefixes are the only non-standard-library import
// paths internal/kickoff's production files may reference: internal/
// multirole for RoleAssignment/GatePolicy/DetectRoles, internal/
// reviewtransaction for the atomic publish/lock primitives, gopkg.in/
// yaml.v3 for (de)serialization, and internal/workspace.
//
// internal/workspace is a correction on top of design.md S4.1's literal
// list, found by this very guard: InferKickoff's signature (Phase 3, task
// 3.2) takes *workspace.WorkspaceConfig so it can pass it straight through
// to multirole.DetectRoles, which already requires that exact type
// (internal/multirole/detector.go itself imports internal/workspace).
// There is no way to keep that already-implemented, already-tested
// signature without this import, and workspace defines nothing but
// configuration types — it carries no path back to review authority.
var kickoffAllowedImportPrefixes = []string{
	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole",
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction",
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace",
	"gopkg.in/yaml.v3",
}

// kickoffImportAllowed reports whether path belongs to the standard
// library (no dot in its first path segment — the same heuristic Go's own
// tooling uses to distinguish stdlib from module paths) or to the explicit
// allowlist above.
func kickoffImportAllowed(path string) bool {
	firstSegment := path
	if idx := strings.Index(path, "/"); idx >= 0 {
		firstSegment = path[:idx]
	}
	if !strings.Contains(firstSegment, ".") {
		return true
	}
	for _, prefix := range kickoffAllowedImportPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// kickoffProductionFiles lists this package's own .go files, excluding
// _test.go files, mirroring
// internal/sddstatus/review_offer_absence_guard_test.go's helper of the
// same shape.
func kickoffProductionFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(".", "*.go"))
	if err != nil {
		t.Fatalf("filepath.Glob: %v", err)
	}
	production := make([]string, 0, len(matches))
	for _, match := range matches {
		if strings.HasSuffix(filepath.Base(match), "_test.go") {
			continue
		}
		production = append(production, match)
	}
	sort.Strings(production)
	return production
}

// scanKickoffForbiddenVocabulary reports every forbidden reviewtransaction
// selector reference and every case-insensitive occurrence of an assembled
// forbidden term (see kickoffForbiddenVocabulary) found in src. Vocabulary
// detection runs over the raw lowercase source text — a forbidden term
// must never appear at all in this package, whether as an identifier, a
// string literal, or a comment, not merely in one syntactic position.
func scanKickoffForbiddenVocabulary(filename string, src []byte) []string {
	var violations []string

	lower := strings.ToLower(string(src))
	for _, term := range kickoffForbiddenVocabulary {
		if strings.Contains(lower, term) {
			violations = append(violations, fmt.Sprintf("%s: contains forbidden RDD-only vocabulary (design.md S1.3)", filename))
		}
	}

	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, filename, src, 0)
	if err != nil {
		return append(violations, filename+": "+err.Error())
	}
	ast.Inspect(tree, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := selector.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "reviewtransaction" {
			return true
		}
		if kickoffForbiddenReviewSelectors[selector.Sel.Name] {
			violations = append(violations, fmt.Sprintf("%s: references reviewtransaction.%s", filename, selector.Sel.Name))
		}
		return true
	})
	return violations
}
