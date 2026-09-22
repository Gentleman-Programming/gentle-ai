package skillregistry

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Structural scope guard for INC-22 (tasks.md phase 15, design.md §6 "Estructural —
// alcance"). Keep this guard alongside the index engine tests.
//
// It encodes three boundaries that prose alone cannot keep honest:
//
//  1. No INC-22 production file outside internal/skillregistry may import
//     internal/components/filemerge: the merge engine is reused, never spread.
//  2. internal/skillregistry must not import internal/components/engram: the
//     Engram mirror enters through the MirrorFunc port only (D-11).
//  3. No production file may compose an install instruction that names the
//     upstream module or upstream binary (REQ-22.2 at the structural level).

// inc22ProductionFiles are the production files this increment creates, per
// design.md §4. A listed file that does not exist yet is skipped: the guard is
// placed after S6 and later slices keep adding to this set.
var inc22ProductionFiles = []string{
	"internal/update/upgrade/source_build.go",
	"internal/components/engram/save.go",
	"internal/skillregistry/types.go",
	"internal/skillregistry/mirror.go",
	"internal/skillregistry/table.go",
	"internal/skillregistry/agents.go",
	"internal/app/skill_index.go",
	"internal/app/upgrade_report.go",
}

const (
	filemergeImportPath = "github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	engramImportPath    = "github.com/gentleman-programming/gentle-ai/v3/internal/components/engram"
	skillregistryPrefix = "internal/skillregistry/"
)

// TestImportBoundaryGuardCatchesKnownShapes unit-tests the scanner against
// synthetic sources, independent of whatever the tree currently contains.
func TestImportBoundaryGuardCatchesKnownShapes(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		src           string
		wantViolation bool
	}{
		{
			name: "skillregistry may import filemerge",
			path: "internal/skillregistry/agents.go",
			src: `package skillregistry

import "github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"

func example() { _ = filemerge.WriteFileAtomic }
`,
			wantViolation: false,
		},
		{
			name: "increment file outside skillregistry imports filemerge",
			path: "internal/components/engram/save.go",
			src: `package engram

import "github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"

func example() { _ = filemerge.WriteFileAtomic }
`,
			wantViolation: true,
		},
		{
			name: "skillregistry imports engram",
			path: "internal/skillregistry/mirror.go",
			src: `package skillregistry

import "github.com/gentleman-programming/gentle-ai/v3/internal/components/engram"

func example() { _ = engram.SaveTopic }
`,
			wantViolation: true,
		},
		{
			name: "clean increment file without either import",
			path: "internal/app/upgrade_report.go",
			src: `package app

func example() {}
`,
			wantViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := scanImportBoundary("synthetic_import_boundary_input.go", []byte(tt.src), tt.path)
			if got := len(violations) > 0; got != tt.wantViolation {
				t.Fatalf("violations = %v (len=%d), want non-empty=%v", violations, len(violations), tt.wantViolation)
			}
		})
	}
}

// TestImportBoundaryGuardHoldsForInc22Files runs the import-boundary scanner
// against the real INC-22 production files present in the tree and against
// every internal/skillregistry production file.
func TestImportBoundaryGuardHoldsForInc22Files(t *testing.T) {
	root := moduleRoot(t)

	checked := 0
	for _, rel := range inc22ProductionFiles {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		checked++
		violations, err := scanImportBoundaryFile(abs, rel)
		if err != nil {
			t.Fatalf("scanImportBoundaryFile(%s): %v", rel, err)
		}
		if len(violations) > 0 {
			t.Fatalf("%s violates the import boundary: %v", rel, violations)
		}
	}
	if checked == 0 {
		t.Fatal("no INC-22 production files found — guard has nothing to prove")
	}

	skillFiles := productionFilesIn(t, filepath.Join(root, "internal", "skillregistry"))
	if len(skillFiles) == 0 {
		t.Fatal("no internal/skillregistry production files found — guard has nothing to prove")
	}
	for _, abs := range skillFiles {
		rel := toSlash(abs, root)
		violations, err := scanImportBoundaryFile(abs, rel)
		if err != nil {
			t.Fatalf("scanImportBoundaryFile(%s): %v", rel, err)
		}
		if len(violations) > 0 {
			t.Fatalf("%s violates the import boundary: %v", rel, violations)
		}
	}
}

// TestImportBoundaryForbidsUpstreamInstallLiterals scans every production file
// for install-instruction string literals that name the upstream module or the
// upstream GitHub repository (REQ-22.2 structural form). Module declarations
// and telemetry repository defaults are not install instructions and are not
// flagged: the rule is about composed install commands.
func TestImportBoundaryForbidsUpstreamInstallLiterals(t *testing.T) {
	root := moduleRoot(t)

	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module root: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no production .go files found — guard has nothing to prove")
	}

	for _, abs := range files {
		rel := toSlash(abs, root)
		violations, err := scanInstallInstructionLiteralsFile(abs, rel)
		if err != nil {
			t.Fatalf("scanInstallInstructionLiteralsFile(%s): %v", rel, err)
		}
		if len(violations) > 0 {
			t.Fatalf("%s composes an upstream install instruction: %v", rel, violations)
		}
	}
}

// scanImportBoundary parses one source buffer and reports import-boundary
// violations for the given module-relative path.
func scanImportBoundary(filename string, src []byte, relPath string) []string {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, filename, src, 0)
	if err != nil {
		return []string{err.Error()}
	}
	return scanImportBoundaryTree(fileSet, tree, relPath)
}

func scanImportBoundaryFile(absPath, relPath string) ([]string, error) {
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	return scanImportBoundary(absPath, src, relPath), nil
}

func scanImportBoundaryTree(fileSet *token.FileSet, tree *ast.File, relPath string) []string {
	var violations []string
	position := func(node ast.Node) string { return fileSet.Position(node.Pos()).String() }
	inSkillregistry := strings.HasPrefix(relPath, skillregistryPrefix)

	for _, spec := range tree.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		switch importPath {
		case filemergeImportPath:
			if !inSkillregistry {
				violations = append(violations, position(spec)+": INC-22 production file outside internal/skillregistry imports "+filemergeImportPath)
			}
		case engramImportPath:
			if inSkillregistry {
				violations = append(violations, position(spec)+": internal/skillregistry imports "+engramImportPath+" (the mirror is a port, never an import)")
			}
		}
	}
	return violations
}

// scanInstallInstructionLiterals reports string literals that compose an
// install instruction naming the upstream module path or the upstream GitHub
// repository.
func scanInstallInstructionLiteralsFile(absPath, relPath string) ([]string, error) {
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, absPath, src, 0)
	if err != nil {
		return nil, err
	}

	var violations []string
	position := func(node ast.Node) string { return fileSet.Position(node.Pos()).String() }
	ast.Inspect(tree, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value := strings.Trim(lit.Value, "`\"")
		installShaped := strings.Contains(value, "go install") ||
			strings.Contains(value, "git clone") ||
			strings.Contains(value, "raw.githubusercontent.com") ||
			strings.Contains(value, "/releases/download/")
		if !installShaped {
			return true
		}
		if strings.Contains(value, "github.com/gentleman-programming/gentle-ai") ||
			strings.Contains(value, "github.com/Gentleman-Programming/gentle-ai") ||
			strings.Contains(value, "cmd/gentle-ai") {
			violations = append(violations, position(lit)+": install instruction literal names upstream: "+value)
		}
		return true
	})
	_ = relPath
	return violations, nil
}

// moduleRoot walks upward from the test working directory until it finds
// go.mod, so the guard runs regardless of the package directory it lives in.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test working directory")
		}
		dir = parent
	}
}

// productionFilesIn lists non-test .go files directly inside dir.
func productionFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	return files
}

func toSlash(absPath, root string) string {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}
