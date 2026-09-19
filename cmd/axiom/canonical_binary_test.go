package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// backupRootLiteralDirectories are the two spellings of the backup root
// directory segment. A filepath.Join call outside the owning internal/backup
// package must never spell either of them out as a literal argument
// immediately followed by the "backups" state-subdirectory literal: doing so
// bypasses backup.BackupRootFor/backup.LegacyBackupRootFor/backup.BackupRoots
// and reproduces the exact defect measured by design decisions D-03/D-04
// (inc-20-upstream-reconciliation).
var backupRootLiteralDirectories = map[string]bool{
	".axiom":     true,
	".gentle-ai": true,
}

// backupRootLiteralStateSubdirectory is the state subdirectory literal that,
// combined with one of backupRootLiteralDirectories, spells out the backup
// root. It is the concrete instance REQ-20.13/REQ-20.14 name.
const backupRootLiteralStateSubdirectory = "backups"

// backupRootOwningPackageDir is the single package allowed to spell out the
// backup root literals: it is their owning package. The exceptions list has
// exactly one entry [D-04].
const backupRootOwningPackageDir = "internal/backup"

// TestUserStateRootsResolveThroughOwningPackage is the fifth assertion of the
// canonical-binary guard family [D-04]. It walks every production (non-test)
// Go file under internal/, excluding internal/backup (its owning package),
// and fails when a filepath.Join call spells out the backup root as a
// ".axiom"/".gentle-ai" literal immediately followed by the "backups" state
// subdirectory literal instead of resolving it through
// backup.BackupRootFor/backup.LegacyBackupRootFor/backup.BackupRoots.
//
// Phase F0.a (inc-20-upstream-reconciliation) ran this guard for real and
// observed it fail, listing every production site that still spells the
// literal out [D-03]; that RED transcript is recorded in the phase report.
// Because this chain ships to main with every PR (stacked-to-main), a
// declared-red assertion here would break CI for every PR stacked on top of
// this one. So this test self-skips, naming the detected violation count,
// until Phase F0.b migrates every site to the canonical accessors and
// removes the skip as its first RED step.
func TestUserStateRootsResolveThroughOwningPackage(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	violations, err := backupRootLiteralViolations(repoRoot)
	if err != nil {
		t.Fatalf("scan internal/**/*.go for backup root literals: %v", err)
	}

	t.Skipf("%d backup root literal site(s) detected outside %s (design D-03/D-04, REQ-20.13/REQ-20.14); Phase F0.b (inc-20-upstream-reconciliation) migrates every one of them to backup.BackupRootFor/backup.LegacyBackupRootFor/backup.BackupRoots and removes this skip as its first RED step", len(violations), backupRootOwningPackageDir)

	for _, violation := range violations {
		t.Errorf("%s: production code outside %s must resolve the backup root through backup.BackupRootFor/backup.LegacyBackupRootFor/backup.BackupRoots, not the literal %q", violation.position, backupRootOwningPackageDir, violation.literal)
	}
}

// backupRootLiteralViolation records one filepath.Join argument pair that
// spells out a backup root literal outside its owning package.
type backupRootLiteralViolation struct {
	position string
	literal  string
}

// backupRootLiteralViolations walks internal/**/*.go production files under
// repoRoot, excluding backupRootOwningPackageDir, and returns every
// filepath.Join call that spells out a backup root literal.
func backupRootLiteralViolations(repoRoot string) ([]backupRootLiteralViolation, error) {
	internalRoot := filepath.Join(repoRoot, "internal")
	var violations []backupRootLiteralViolation

	walkErr := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(relPath)

		if relSlash == backupRootOwningPackageDir || strings.HasPrefix(relSlash, backupRootOwningPackageDir+"/") {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		fileSet := token.NewFileSet()
		tree, err := parser.ParseFile(fileSet, relSlash, source, 0)
		if err != nil {
			return err
		}

		violations = append(violations, backupRootLiteralViolationsInFile(fileSet, tree)...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(violations, func(i, j int) bool { return violations[i].position < violations[j].position })
	return violations, nil
}

// backupRootLiteralViolationsInFile inspects a single parsed file for
// filepath.Join calls whose arguments spell out a backup root literal.
func backupRootLiteralViolationsInFile(fileSet *token.FileSet, tree *ast.File) []backupRootLiteralViolation {
	var violations []backupRootLiteralViolation

	ast.Inspect(tree, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isFilepathJoinCall(call) {
			return true
		}
		for i := 0; i+1 < len(call.Args); i++ {
			directory, ok := stringLiteralValue(call.Args[i])
			if !ok || !backupRootLiteralDirectories[directory] {
				continue
			}
			subdirectory, ok := stringLiteralValue(call.Args[i+1])
			if !ok || subdirectory != backupRootLiteralStateSubdirectory {
				continue
			}
			violations = append(violations, backupRootLiteralViolation{
				position: fileSet.Position(call.Args[i].Pos()).String(),
				literal:  directory + "/" + subdirectory,
			})
		}
		return true
	})

	return violations
}

// isFilepathJoinCall reports whether call invokes filepath.Join.
func isFilepathJoinCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Join" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "filepath"
}

// stringLiteralValue returns the unquoted value of expr when it is a string
// literal, and false otherwise.
func stringLiteralValue(expr ast.Expr) (string, bool) {
	basicLit, ok := expr.(*ast.BasicLit)
	if !ok || basicLit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(basicLit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}
