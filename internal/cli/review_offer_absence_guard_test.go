package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// This file is the Wave 4 S3 primary absence proof (design.md decision 4)
// for internal/cli's half of the guard: a static AST guard asserting ZERO
// call edges into offer/ReviewCore symbols from internal/cli's SDD
// apply/verify/archive/status/continue surface, except through the
// package's one door for that surface (review_offer_door.go's
// offerEntryHook).
//
// Scope (deliberately narrow, per design.md decision 4's own rationale):
// only the SDD apply/verify/archive/status/continue files — sdd_attempt.go,
// sdd_status.go, sdd_verify_validate.go — are scanned. The ~30 other
// internal/cli files that import reviewtransaction (review_*.go) are
// explicit, user-invoked `review` subcommands (review start, review status,
// review finalize, etc.), not automatic SDD apply/verify/archive paths, and
// are intentionally out of this guard's scope — scanning them would be a
// false positive against code this guard is not meant to constrain.

// reviewOfferAbsenceScopedCLIFiles are the exact internal/cli files this
// guard scans. Any new SDD apply/verify/archive/status/continue file must
// be added here deliberately — the guard proves nothing about a file it
// does not scan.
var reviewOfferAbsenceScopedCLIFiles = []string{
	"sdd_attempt.go",
	"sdd_status.go",
	"sdd_verify_validate.go",
}

// reviewOfferAbsenceCLIForbiddenSelectors mirrors
// internal/sddstatus/review_offer_absence_guard_test.go's forbidden set.
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

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
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

// TestReviewOfferAbsenceGuardHoldsForScopedCLIFiles runs the scanner
// against exactly the SDD apply/verify/archive/status/continue files this
// guard names, failing with exact violation evidence if any of them
// reference an offer/ReviewCore symbol directly instead of through
// offerEntryHook.
func TestReviewOfferAbsenceGuardHoldsForScopedCLIFiles(t *testing.T) {
	for _, file := range reviewOfferAbsenceScopedCLIFiles {
		t.Run(file, func(t *testing.T) {
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

// TestOfferEntryHookIsTheOneCLIDoor proves offerEntryHook exists and is
// invocable — the door the absence guard exempts, and the corroborating
// OFF-mode bench call-absence counter overrides to prove it never fires.
func TestOfferEntryHookIsTheOneCLIDoor(t *testing.T) {
	called := false
	previous := offerEntryHook
	offerEntryHook = func() { called = true }
	defer func() { offerEntryHook = previous }()
	offerEntryHook()
	if !called {
		t.Fatal("offerEntryHook did not fire when invoked directly")
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
