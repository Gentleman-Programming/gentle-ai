package sddstatus

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"testing"
)

// This file is the Wave 4 S3 primary absence proof (design.md decision 4):
// a static AST guard asserting ZERO call edges into offer/ReviewCore
// symbols from any internal/sddstatus production file, except through the
// package's one door (review_door.go's reviewEntryHook). Modelled on
// internal/reviewtransaction/shadow_readonly_guard_test.go.
//
// Scope: internal/sddstatus is SDD's apply/verify/archive/status/continue
// surface in its entirety — every production (non-test) .go file in this
// package is scanned, except review_door.go itself (the one door).
//
// Forbidden symbols (design's "offer/ReviewCore symbols"):
//   - reviewtransaction.OfferReviewAfterVerify
//   - reviewtransaction.ReviewCore (the type, and any selector on it)
//
// This deliberately does NOT forbid every reviewtransaction reference —
// internal/sddstatus legitimately calls many other reviewtransaction
// symbols today (EvaluateCompactGate, GateAllow, Transaction, etc.) for
// review-gate/authority evaluation that is out of this guard's narrow scope
// (design decision 4 names offer/ReviewCore specifically, not the whole
// package).

// reviewOfferAbsenceForbiddenSelectors are the exact reviewtransaction
// selector names this guard forbids outside the door file.
var reviewOfferAbsenceForbiddenSelectors = map[string]bool{
	"OfferReviewAfterVerify": true,
	"ReviewCore":             true,
}

// reviewOfferAbsenceDoorFiles are production files exempt from the scan —
// the one place internal/sddstatus is allowed to reference offer/ReviewCore
// symbols, fronted by reviewEntryHook.
var reviewOfferAbsenceDoorFiles = map[string]bool{
	"review_door.go": true,
}

// TestReviewOfferAbsenceGuardCatchesKnownShapes unit-tests the scanner
// against synthetic sources, independent of whatever the package currently
// contains.
func TestReviewOfferAbsenceGuardCatchesKnownShapes(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantViolation bool
	}{
		{
			name: "clean source touching unrelated reviewtransaction symbols",
			src: `package sddstatus

import "github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"

func example() reviewtransaction.GateResult { return reviewtransaction.GateAllow }
`,
			wantViolation: false,
		},
		{
			name: "calls OfferReviewAfterVerify directly",
			src: `package sddstatus

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func example(ctx context.Context) {
	reviewtransaction.OfferReviewAfterVerify(ctx, "", reviewtransaction.OfferRequest{})
}
`,
			wantViolation: true,
		},
		{
			name: "references the ReviewCore type",
			src: `package sddstatus

import "github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"

func example() reviewtransaction.ReviewCore { return reviewtransaction.ReviewCore{} }
`,
			wantViolation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := scanReviewOfferAbsence("synthetic_offer_absence_test_input.go", []byte(tt.src))
			if got := len(violations) > 0; got != tt.wantViolation {
				t.Fatalf("violations = %v (len=%d), want non-empty=%v", violations, len(violations), tt.wantViolation)
			}
		})
	}
}

// TestReviewOfferAbsenceGuardHoldsForProductionFiles runs the same scanner
// against every real production internal/sddstatus file outside the door,
// and fails with exact violation evidence if any offer/ReviewCore symbol is
// referenced. This is decision 4's primary proof, encoded as a guard.
func TestReviewOfferAbsenceGuardHoldsForProductionFiles(t *testing.T) {
	files, err := productionReviewOfferAbsenceFiles(t)
	if err != nil {
		t.Fatalf("productionReviewOfferAbsenceFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no production .go files found — guard has nothing to prove")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			violations, err := scanReviewOfferAbsenceFile(file)
			if err != nil {
				t.Fatalf("scanReviewOfferAbsenceFile(%s): %v", file, err)
			}
			if len(violations) > 0 {
				t.Fatalf("%s references offer/ReviewCore symbols outside the door: %v", file, violations)
			}
		})
	}
}

// TestReviewEntryHookIsTheOneDoor proves reviewEntryHook exists and is
// invocable — the door the absence guard exempts, and the corroborating
// OFF-mode bench call-absence counter overrides to prove it never fires.
func TestReviewEntryHookIsTheOneDoor(t *testing.T) {
	called := false
	previous := reviewEntryHook
	reviewEntryHook = func() { called = true }
	defer func() { reviewEntryHook = previous }()
	reviewEntryHook()
	if !called {
		t.Fatal("reviewEntryHook did not fire when invoked directly")
	}
}

func productionReviewOfferAbsenceFiles(t *testing.T) ([]string, error) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(".", "*.go"))
	if err != nil {
		return nil, err
	}
	production := make([]string, 0, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		if len(base) >= len("_test.go") && base[len(base)-len("_test.go"):] == "_test.go" {
			continue
		}
		if reviewOfferAbsenceDoorFiles[base] {
			continue
		}
		production = append(production, match)
	}
	sort.Strings(production)
	return production, nil
}

func scanReviewOfferAbsenceFile(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	return scanReviewOfferAbsenceTree(fileSet, tree), nil
}

func scanReviewOfferAbsence(filename string, src []byte) []string {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, filename, src, 0)
	if err != nil {
		return []string{err.Error()}
	}
	return scanReviewOfferAbsenceTree(fileSet, tree)
}

// scanReviewOfferAbsenceTree walks one parsed file for any selector
// expression whose identifier matches a forbidden offer/ReviewCore symbol
// name. It does not resolve import aliases beyond the literal package
// identifier ("reviewtransaction") already used consistently throughout
// this codebase, matching the precedent scanners in this repository.
func scanReviewOfferAbsenceTree(fileSet *token.FileSet, tree *ast.File) []string {
	var violations []string
	position := func(node ast.Node) string { return fileSet.Position(node.Pos()).String() }
	ast.Inspect(tree, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := selector.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "reviewtransaction" {
			return true
		}
		if reviewOfferAbsenceForbiddenSelectors[selector.Sel.Name] {
			violations = append(violations, position(selector)+": references reviewtransaction."+selector.Sel.Name)
		}
		return true
	})
	return violations
}
