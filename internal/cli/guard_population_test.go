package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const guardPopulationMarkerHint = "guard:population"

var guardPopulationMarkerPattern = regexp.MustCompile(`^guard:population\s+([a-z0-9-]+)\s+(too-tight|too-loose|fail-closed):\s*(\S.*)$`)

type guardPopulationDeclaration struct {
	file   string
	line   int
	family string
}

type guardPopulationAnalysis struct {
	declarations []guardPopulationDeclaration
	problems     []string
}

type guardPopulationMarker struct {
	line      int
	family    string
	malformed string
	consumed  bool
}

func TestGuardPopulationMarkersAreWellFormedAdjacentAndUnique(t *testing.T) {
	analysis := guardPopulationAnalyzeProduction(t)
	for _, problem := range analysis.problems {
		t.Error(problem)
	}
	families := make(map[string]guardPopulationDeclaration, len(analysis.declarations))
	for _, declaration := range analysis.declarations {
		if prior, exists := families[declaration.family]; exists {
			t.Errorf("guard-population family %q is declared twice: %s:%d and %s:%d", declaration.family, prior.file, prior.line, declaration.file, declaration.line)
		}
		families[declaration.family] = declaration
	}
}

func TestGuardPopulationAnalyzerRequiresValidAdjacentMarkers(t *testing.T) {
	for _, tt := range []struct {
		name         string
		source       string
		wantDecls    int
		wantProblems int
	}{
		{
			name: "valid adjacent marker",
			source: `package synthetic
func allowed(value bool) bool {
	// guard:population synthetic-family too-tight: legitimate values satisfy the external contract
	return value
}`,
			wantDecls: 1,
		},
		{
			name: "orphaned marker",
			source: `package synthetic
// guard:population synthetic-family too-tight: this marker is not adjacent to a guard
func f() {}`,
			wantProblems: 1,
		},
		{
			name: "malformed marker",
			source: `package synthetic
func f(value bool) bool {
	// guard:population INVALID
	return value
}`,
			wantProblems: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := guardPopulationAnalyzeSource("synthetic.go", tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if len(analysis.declarations) != tt.wantDecls || len(analysis.problems) != tt.wantProblems {
				t.Fatalf("analysis = %+v, want %d declarations and %d problems", analysis, tt.wantDecls, tt.wantProblems)
			}
		})
	}
}

func guardPopulationAnalyzeProduction(t *testing.T) guardPopulationAnalysis {
	t.Helper()
	internalRoot := ".."
	var merged guardPopulationAnalysis
	err := filepath.WalkDir(internalRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		source, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(internalRoot, filePath)
		if err != nil {
			return err
		}
		label := filepath.ToSlash(filepath.Join("internal", relative))
		analysis, err := guardPopulationAnalyzeSource(label, string(source))
		if err != nil {
			return fmt.Errorf("parse %s: %w", label, err)
		}
		merged.declarations = append(merged.declarations, analysis.declarations...)
		merged.problems = append(merged.problems, analysis.problems...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(merged.problems)
	return merged
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
				marker.malformed = fmt.Sprintf("%s:%d has a malformed guard-population marker", fileLabel, line)
			} else {
				marker.family = match[1]
			}
			markers[line] = marker
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		if !guardPopulationSupportedNode(node) {
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
		} else {
			analysis.declarations = append(analysis.declarations, guardPopulationDeclaration{file: fileLabel, line: line, family: marker.family})
		}
		return true
	})

	for _, marker := range markers {
		if marker.consumed {
			continue
		}
		problem := marker.malformed
		if problem == "" {
			problem = fmt.Sprintf("%s:%d guard-population marker is not immediately adjacent to an if, switch, or return guard node", fileLabel, marker.line)
		}
		analysis.problems = append(analysis.problems, problem)
	}
	return analysis, nil
}

func guardPopulationSupportedNode(node ast.Node) bool {
	switch node.(type) {
	case *ast.IfStmt, *ast.SwitchStmt, *ast.ReturnStmt:
		return true
	default:
		return false
	}
}
