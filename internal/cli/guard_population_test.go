package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const guardPopulationMarkerHint = "guard:population"

var guardPopulationMarkerPattern = regexp.MustCompile(`^guard:population\s+([a-z0-9-]+)\s+(too-tight|too-loose|fail-closed):\s*(\S.*)$`)

var guardPopulationProductionDirs = []struct{ dir, prefix string }{
	{".", "internal/cli"},
	{filepath.Join("..", "reviewtransaction"), "internal/reviewtransaction"},
}

type guardPopulationDeclaration struct {
	file       string
	line       int
	family     string
	direction  string
	population string
}

type guardPopulationAnalysis struct {
	declarations []guardPopulationDeclaration
	problems     []string
}

type guardPopulationMarker struct {
	line      int
	family    string
	direction string
	claim     string
	malformed string
	consumed  bool
}

func TestGuardPopulationAnalyzerBindsOnlyAdjacentGuardNodes(t *testing.T) {
	const valid = `package synthetic
func allowed(value int) bool {
	// guard:population synthetic-if too-tight: legitimate values are positive
	if value > 0 {
		return true
	}
	// guard:population synthetic-switch too-loose: legitimate values are small
	switch value {
	}
	// guard:population synthetic-return fail-closed: legitimate values are zero
	return value == 0
}
`
	analysis, err := guardPopulationAnalyzeSource("synthetic/valid.go", valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.problems) != 0 || len(analysis.declarations) != 3 {
		t.Fatalf("valid declarations were not bound: %+v", analysis)
	}
	if got := analysis.declarations[2]; got.family != "synthetic-return" || got.direction != "fail-closed" || got.population != "legitimate values are zero" {
		t.Fatalf("declaration fields = %+v, want parsed family, direction, and claim", got)
	}

	for _, tc := range []struct{ name, source, want string }{
		{name: "above a function", want: "not adjacent", source: `package synthetic
// guard:population arbitrary-if too-tight: this comment is not adjacent to a supported guard node
func f() {}
`},
		{name: "separated by a blank line", want: "not adjacent", source: `package synthetic
func f(value bool) bool {
	// guard:population detached too-tight: a blank line breaks adjacency

	return value
}
`},
		{name: "above a non-guard statement", want: "not adjacent", source: `package synthetic
func f() int {
	// guard:population assignment too-tight: an assignment is not a guard node
	value := 1
	return value
}
`},
		{name: "unknown direction", want: "malformed", source: `package synthetic
func f(value bool) bool {
	// guard:population synthetic too-strict: the direction vocabulary is closed
	return value
}
`},
		{name: "missing claim", want: "malformed", source: `package synthetic
func f(value bool) bool {
	// guard:population synthetic too-tight:
	return value
}
`},
		{name: "malformed and orphaned", want: "malformed", source: `package synthetic
// guard:population Synthetic too-tight: family names are lowercase
func f() {}
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			analysis, err := guardPopulationAnalyzeSource("synthetic/invalid.go", tc.source)
			if err != nil {
				t.Fatal(err)
			}
			if len(analysis.declarations) != 0 || len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], tc.want) {
				t.Fatalf("invalid declaration did not fail closed with %q: %+v", tc.want, analysis)
			}
		})
	}
}

func TestGuardPopulationDuplicateFamiliesAreRejected(t *testing.T) {
	const source = `package synthetic
func f(value int) bool {
	// guard:population shared-family too-tight: legitimate values are positive
	if value > 0 {
		return true
	}
	// guard:population shared-family too-loose: legitimate values are small
	return value < 10
}
`
	analysis, err := guardPopulationAnalyzeSource("synthetic/duplicate.go", source)
	if err != nil {
		t.Fatal(err)
	}
	problems := guardPopulationDuplicateFamilies(analysis.declarations)
	if len(problems) != 1 || !strings.Contains(problems[0], `"shared-family" is declared twice`) {
		t.Fatalf("duplicate family problems = %v, want one duplicate report", problems)
	}
	if problems := guardPopulationDuplicateFamilies(analysis.declarations[:1]); len(problems) != 0 {
		t.Fatalf("single family reported as duplicate: %v", problems)
	}
}

// TestEveryGuardPopulationDeclarationIsAdjacentAndUnique checks structure
// only: well-formed markers, adjacency to an if/switch/return guard node, and
// one declaration per family. Whether a claim is true is challenged in review
// against the behavior tests that exercise the guard, not by this scan.
func TestEveryGuardPopulationDeclarationIsAdjacentAndUnique(t *testing.T) {
	analysis := guardPopulationAnalyzeProduction(t)
	for _, problem := range analysis.problems {
		t.Error(problem)
	}
	for _, problem := range guardPopulationDuplicateFamilies(analysis.declarations) {
		t.Error(problem)
	}
	if len(analysis.declarations) == 0 {
		t.Fatal("found no guard-population declarations; the production scan is not reading the scoped packages")
	}
}

// TestProductionGuardPopulationMarkersFailClosedWhenDetached proves the scan
// rejects each real production marker once it is detached or malformed,
// rather than only passing on today's source.
func TestProductionGuardPopulationMarkersFailClosedWhenDetached(t *testing.T) {
	analysis := guardPopulationAnalyzeProduction(t)
	for _, declaration := range analysis.declarations {
		source, err := os.ReadFile(guardPopulationSourcePath(t, declaration.file))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(source), "\n")
		markerIndex := declaration.line - 2
		for name, mutate := range map[string]func([]string) []string{
			"detached": func(lines []string) []string {
				return append(append(append([]string{}, lines[:markerIndex+1]...), ""), lines[markerIndex+1:]...)
			},
			"malformed": func(lines []string) []string {
				mutated := append([]string{}, lines...)
				mutated[markerIndex] = strings.Replace(mutated[markerIndex], " "+declaration.direction+":", " bogus:", 1)
				return mutated
			},
		} {
			got, err := guardPopulationAnalyzeSource(declaration.file, strings.Join(mutate(lines), "\n"))
			if err != nil {
				t.Fatalf("parse %s mutation of %s: %v", name, declaration.file, err)
			}
			if len(got.problems) == 0 {
				t.Errorf("%s marker for family %q in %s was accepted", name, declaration.family, declaration.file)
			}
		}
	}
}

func guardPopulationSourcePath(t *testing.T, fileLabel string) string {
	t.Helper()
	for _, target := range guardPopulationProductionDirs {
		if name, ok := strings.CutPrefix(fileLabel, target.prefix+"/"); ok {
			return filepath.Join(target.dir, name)
		}
	}
	t.Fatalf("no production directory for %s", fileLabel)
	return ""
}

func guardPopulationAnalyzeProduction(t *testing.T) guardPopulationAnalysis {
	t.Helper()
	var merged guardPopulationAnalysis
	for _, target := range guardPopulationProductionDirs {
		entries, err := os.ReadDir(target.dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			source, err := os.ReadFile(filepath.Join(target.dir, name))
			if err != nil {
				t.Fatal(err)
			}
			analysis, err := guardPopulationAnalyzeSource(path.Join(target.prefix, name), string(source))
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			merged.declarations = append(merged.declarations, analysis.declarations...)
			merged.problems = append(merged.problems, analysis.problems...)
		}
	}
	sort.Strings(merged.problems)
	return merged
}

func guardPopulationDuplicateFamilies(declarations []guardPopulationDeclaration) []string {
	var problems []string
	families := make(map[string]guardPopulationDeclaration, len(declarations))
	for _, declaration := range declarations {
		if prior, exists := families[declaration.family]; exists {
			problems = append(problems, fmt.Sprintf("guard-population family %q is declared twice: %s:%d and %s:%d", declaration.family, prior.file, prior.line, declaration.file, declaration.line))
			continue
		}
		families[declaration.family] = declaration
	}
	return problems
}

func guardPopulationAnalyzeSource(fileLabel, source string) (guardPopulationAnalysis, error) {
	var analysis guardPopulationAnalysis
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fileLabel, source, parser.ParseComments)
	if err != nil {
		return analysis, err
	}

	markers := map[int]*guardPopulationMarker{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(comment.Text, "//"), "/*"), "*/"))
			if !strings.Contains(text, guardPopulationMarkerHint) {
				continue
			}
			line := fset.Position(comment.End()).Line
			match := guardPopulationMarkerPattern.FindStringSubmatch(text)
			marker := &guardPopulationMarker{line: line}
			if match == nil {
				marker.malformed = fmt.Sprintf("%s:%d has a malformed guard-population declaration", fileLabel, line)
			} else {
				marker.family, marker.direction, marker.claim = match[1], match[2], match[3]
			}
			markers[line] = marker
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.ReturnStmt:
		default:
			return true
		}
		line := fset.Position(node.Pos()).Line
		marker := markers[line-1]
		if marker == nil {
			return true
		}
		marker.consumed = true
		if marker.malformed != "" {
			analysis.problems = append(analysis.problems, marker.malformed)
			return true
		}
		analysis.declarations = append(analysis.declarations, guardPopulationDeclaration{
			file: fileLabel, line: line, family: marker.family, direction: marker.direction, population: marker.claim,
		})
		return true
	})

	for _, marker := range markers {
		if marker.consumed {
			continue
		}
		problem := marker.malformed
		if problem == "" {
			problem = fmt.Sprintf("%s:%d guard-population declaration is not adjacent to an if, switch, or return guard node", fileLabel, marker.line)
		}
		analysis.problems = append(analysis.problems, problem)
	}
	return analysis, nil
}
