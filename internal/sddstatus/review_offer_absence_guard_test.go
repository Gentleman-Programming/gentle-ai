package sddstatus

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"testing"
)

// No SDD production file may offer review or invoke ReviewCore.
// Keep this structural guard alongside the command behavior tests.

// reviewOfferAbsenceForbiddenSelectors are the exact reviewtransaction
// selector names this guard forbids in SDD.
var reviewOfferAbsenceForbiddenSelectors = map[string]bool{
	"OfferReviewAfterVerify": true,
	"ReviewCore":             true,
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

import "github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"

func example() reviewtransaction.GateResult { return reviewtransaction.GateAllow }
`,
			wantViolation: false,
		},
		{
			name: "calls OfferReviewAfterVerify directly",
			src: `package sddstatus

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
			name: "references the ReviewCore type",
			src: `package sddstatus

import "github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"

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
// against every real production internal/sddstatus file in SDD,
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
				t.Fatalf("%s references offer/ReviewCore symbols in SDD: %v", file, violations)
			}
		})
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
