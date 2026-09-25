package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// This guard rejects direct review-offer calls in the native review CLI's
// entrypoints, including read-only assessment and mode management. Review
// remains an explicit user choice, not a hidden side effect of another CLI
// surface. The inventory is deliberately scoped: it does not claim to scan
// every review implementation file.

// reviewOfferAbsenceScopedCLIFiles lists existing native review CLI entrypoints.
// Keep this inventory current when entrypoints move or are retired.
var reviewOfferAbsenceScopedCLIFiles = []string{
	"review.go",
	"review_assess.go",
	"review_facade.go",
	"review_mode.go",
}

// reviewOfferAbsenceCLIForbiddenSelectors names the offer/core calls that
// must never be introduced into the scoped CLI entrypoints.
var reviewOfferAbsenceCLIForbiddenSelectors = map[string]bool{
	"OfferReviewAfterVerify": true,
	"ReviewCore":             true,
}

func TestReviewOfferAbsenceGuardCatchesKnownShapesCLI(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantViolation bool
	}{
		{
			name: "clean source",
			src: `package cli

func example() int { return 1 }
`,
			wantViolation: false,
		},
		{
			name: "calls OfferReviewAfterVerify directly",
			src: `package cli

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

func example(ctx context.Context) {
	reviewtransaction.OfferReviewAfterVerify(ctx, "", reviewtransaction.OfferRequest{})
}
`,
			wantViolation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := scanReviewOfferAbsenceCLI("synthetic_offer_absence_cli_test_input.go", []byte(tt.src))
			if got := len(violations) > 0; got != tt.wantViolation {
				t.Fatalf("violations = %v (len=%d), want non-empty=%v", violations, len(violations), tt.wantViolation)
			}
		})
	}
}

// TestReviewOfferAbsenceGuardHoldsForScopedCLIFiles requires each inventoried
// entrypoint to exist and rejects direct offer/ReviewCore references in it.
func TestReviewOfferAbsenceGuardHoldsForScopedCLIFiles(t *testing.T) {
	if len(reviewOfferAbsenceScopedCLIFiles) == 0 {
		t.Fatal("native review CLI entrypoint inventory must not be empty")
	}
	for _, file := range reviewOfferAbsenceScopedCLIFiles {
		t.Run(file, func(t *testing.T) {
			if _, err := os.Stat(file); err != nil {
				t.Fatalf("inventoried native review CLI entrypoint %q must exist: %v", file, err)
			}
			violations, err := scanReviewOfferAbsenceCLIFile(file)
			if err != nil {
				t.Fatalf("scanReviewOfferAbsenceCLIFile(%s): %v", file, err)
			}
			if len(violations) > 0 {
				t.Fatalf("%s references offer/ReviewCore symbols outside the door: %v", file, violations)
			}
		})
	}
}

func scanReviewOfferAbsenceCLIFile(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	return scanReviewOfferAbsenceCLITree(fileSet, tree), nil
}

func scanReviewOfferAbsenceCLI(filename string, src []byte) []string {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, filename, src, 0)
	if err != nil {
		return []string{err.Error()}
	}
	return scanReviewOfferAbsenceCLITree(fileSet, tree)
}

func scanReviewOfferAbsenceCLITree(fileSet *token.FileSet, tree *ast.File) []string {
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
		if reviewOfferAbsenceCLIForbiddenSelectors[selector.Sel.Name] {
			violations = append(violations, position(selector)+": references reviewtransaction."+selector.Sel.Name)
		}
		return true
	})
	return violations
}
