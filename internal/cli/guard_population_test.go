package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	guardPopulationMarkerHint = "guard:population"
	guardPopulationCount      = 8
)

var guardPopulationMarkerPattern = regexp.MustCompile(`^guard:population\s+([a-z0-9-]+)\s+(too-tight|too-loose|fail-closed):\s*(\S.*)$`)

var guardPopulationProductionDirs = []struct{ dir, prefix string }{
	{".", "internal/cli"},
	{filepath.Join("..", "reviewtransaction"), "internal/reviewtransaction"},
	{filepath.Join("..", "sddstatus"), "internal/sddstatus"},
}

type guardPopulationDeclaration struct {
	file        string
	line        int
	family      string
	direction   string
	population  string
	nodeKind    string
	fingerprint string
}

func (declaration guardPopulationDeclaration) registryKey() string {
	return strings.Join([]string{
		declaration.file,
		declaration.family,
		declaration.direction,
		declaration.nodeKind,
		declaration.fingerprint,
		strconv.Quote(declaration.population),
	}, "\t")
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
func allowed(value bool) bool {
	// guard:population synthetic-family too-tight: legitimate values satisfy the external contract
	return value
}
`
	analysis, err := guardPopulationAnalyzeSource("synthetic/valid.go", valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.problems) != 0 || len(analysis.declarations) != 1 {
		t.Fatalf("valid declaration was not bound: %+v", analysis)
	}
	if got := analysis.declarations[0]; got.nodeKind != "return" || got.fingerprint == "" {
		t.Fatalf("declaration lacks an AST guard binding: %+v", got)
	}

	const orphaned = `package synthetic
// guard:population arbitrary-if too-tight: this comment is not adjacent to a supported guard node
func f() {}
`
	analysis, err = guardPopulationAnalyzeSource("synthetic/orphaned.go", orphaned)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.declarations) != 0 || len(analysis.problems) != 1 || !strings.Contains(analysis.problems[0], "adjacent") {
		t.Fatalf("orphaned declaration did not fail closed: %+v", analysis)
	}
}

func TestGuardPopulationRegistryComparisonRejectsBothDriftDirections(t *testing.T) {
	current := map[string]bool{"declared": true}

	added, missing := guardPopulationRegistryDiff(current, map[string]bool{})
	if len(added) != 1 || added[0] != "declared" || len(missing) != 0 {
		t.Fatalf("unregistered declaration drift not detected: added=%v missing=%v", added, missing)
	}

	added, missing = guardPopulationRegistryDiff(current, map[string]bool{"declared": true, "removed": true})
	if len(added) != 0 || len(missing) != 1 || missing[0] != "removed" {
		t.Fatalf("missing declaration drift not detected: added=%v missing=%v", added, missing)
	}
}

func TestEveryRegisteredGuardPopulationDeclarationMatchesProduction(t *testing.T) {
	analysis := guardPopulationAnalyzeProduction(t)
	for _, problem := range analysis.problems {
		t.Error(problem)
	}
	if t.Failed() {
		return
	}
	if len(analysis.declarations) != guardPopulationCount {
		t.Fatalf("found %d guard-population declarations, want the frozen v2.2.1 scope of %d", len(analysis.declarations), guardPopulationCount)
	}

	current := make(map[string]bool, len(analysis.declarations))
	families := make(map[string]guardPopulationDeclaration, len(analysis.declarations))
	for _, declaration := range analysis.declarations {
		if prior, exists := families[declaration.family]; exists {
			t.Errorf("guard-population family %q is declared twice: %s:%d and %s:%d", declaration.family, prior.file, prior.line, declaration.file, declaration.line)
		}
		families[declaration.family] = declaration
		current[declaration.registryKey()] = true
	}
	if t.Failed() {
		return
	}

	registryPath := filepath.Join("..", "..", ".guard-population-baseline.txt")
	if os.Getenv("GENTLE_AI_GUARD_POPULATION_UPDATE") == "1" {
		if err := writeGuardPopulationRegistry(registryPath, current); err != nil {
			t.Fatal(err)
		}
		t.Logf("guard-population baseline updated: %d declarations", len(current))
		return
	}

	baseline, err := readGuardPopulationRegistry(registryPath)
	if err != nil {
		t.Fatalf("read guard-population baseline: %v", err)
	}
	added, missing := guardPopulationRegistryDiff(current, baseline)
	for _, key := range added {
		t.Errorf("guard-population declaration is absent from the frozen registry:\n+ %s", key)
	}
	for _, key := range missing {
		t.Errorf("frozen guard-population declaration is missing or its AST binding drifted:\n- %s", key)
	}
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
		kind, fingerprint, supported := guardPopulationNodeBinding(fset, node)
		if !supported {
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
			file: fileLabel, line: line, family: marker.family, direction: marker.direction,
			population: marker.claim, nodeKind: kind, fingerprint: fingerprint,
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

func guardPopulationNodeBinding(fset *token.FileSet, node ast.Node) (string, string, bool) {
	var kind string
	switch node.(type) {
	case *ast.IfStmt:
		kind = "if"
	case *ast.SwitchStmt:
		kind = "switch"
	case *ast.ReturnStmt:
		kind = "return"
	default:
		return "", "", false
	}
	var rendered bytes.Buffer
	if err := format.Node(&rendered, fset, node); err != nil {
		return kind, "format-error", true
	}
	digest := sha256.Sum256(rendered.Bytes())
	return kind, fmt.Sprintf("sha256:%x", digest), true
}

func readGuardPopulationRegistry(filePath string) (map[string]bool, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	registry := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if registry[line] {
			return nil, fmt.Errorf("duplicate registry entry %q", line)
		}
		registry[line] = true
	}
	return registry, nil
}

func writeGuardPopulationRegistry(filePath string, registry map[string]bool) error {
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	content := "# Frozen guard-population declarations. See docs/architecture/guard-population.md.\n" + strings.Join(keys, "\n") + "\n"
	return os.WriteFile(filePath, []byte(content), 0o644)
}

func guardPopulationRegistryDiff(current, baseline map[string]bool) (added, missing []string) {
	for key := range current {
		if !baseline[key] {
			added = append(added, key)
		}
	}
	for key := range baseline {
		if !current[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(added)
	sort.Strings(missing)
	return added, missing
}
