package reviewtransaction

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestCompactReceiptAndGateAPIsAreAbsentFromProductionPackages prevents the
// retired compact-v2 receipt publication/replay/gate path from returning via a
// direct reviewtransaction, CLI, or SDD dependency.
func TestCompactReceiptAndGateAPIsAreAbsentFromProductionPackages(t *testing.T) {
	for _, directory := range []string{".", "../cli", "../sddstatus"} {
		for _, path := range compactReceiptGateProductionFiles(t, directory) {
			violations, err := compactReceiptGateAPIViolations(path)
			if err != nil {
				t.Fatalf("scan %s: %v", path, err)
			}
			if len(violations) > 0 {
				t.Errorf("%s retains retired compact-v2 receipt/gate APIs: %v", path, violations)
			}
		}
	}
}

func compactReceiptGateProductionFiles(t *testing.T, directory string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	production := make([]string, 0, len(matches))
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		production = append(production, path)
	}
	sort.Strings(production)
	return production
}

var retiredCompactReceiptGateAPI = map[string]bool{
	"AssessCompactGateTarget":        true,
	"CompactGateTargetApplicability": true,
	"CompactGateTargetAssessment":    true,
	"CompactReceipt":                 true,
	"CompactReceiptEqual":            true,
	"CompactReceiptSchema":           true,
	"CompactReceiptSchemaOf":         true,
	"EvaluateCompactGate":            true,
	"ParseCompactReceipt":            true,
	"SDDReceiptRef":                  true,
	"ValidateSDDReceiptRef":          true,
	"WriteCompactReceiptAtomic":      true,
}

func compactReceiptGateAPIViolations(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	violations := []string{}
	ast.Inspect(tree, func(node ast.Node) bool {
		switch expression := node.(type) {
		case *ast.Ident:
			if retiredCompactReceiptGateAPI[expression.Name] {
				violations = append(violations, fileSet.Position(expression.Pos()).String()+": "+expression.Name)
			}
		}
		return true
	})
	return violations, nil
}
