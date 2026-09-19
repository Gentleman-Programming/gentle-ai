package main

import (
	"fmt"
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

	"gopkg.in/yaml.v3"
)

// canonicalBinaryPath is the product's own build target. Every build,
// workflow and ratchet target in the repository must resolve through this
// path, never through deprecatedShimPath [D-04].
const canonicalBinaryPath = "./cmd/axiom"

// deprecatedShimPath is named so this guard family can reject its
// reappearance as a BUILD TARGET, not so production code invokes it.
// cmd/gentle-ai/main.go itself stays as the deprecation pass-through into
// app.RunArgs that D2.4 requires; only its use as a build target elsewhere
// is disallowed.
const deprecatedShimPath = "./cmd/gentle-ai"

// repositoryRoot resolves the repository root from cmd/axiom, where every
// test in this file runs. Shared so the five assertions of this guard
// family never duplicate this resolution [D-04, task 3.9].
func repositoryRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return repoRoot
}

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

// backupRootLiteralFragments are the two full path fragments that spell out
// a backup root end to end within a single string literal token. REQ-20.14's
// 2026-09-19 amendment widened the guard beyond filepath.Join: string
// concatenation (internal/cli/restore.go built its root as
// homeDir+"/.gentle-ai/backups"), an fmt.Sprintf format string, and a fully
// spelled-out literal assignment all carry the offending directory and its
// "backups" state subdirectory inside ONE *ast.BasicLit, unlike
// filepath.Join's split arguments. Scanning every string literal in a file
// for these fragments — regardless of the expression that contains the
// literal — catches all three additional forms with one check.
var backupRootLiteralFragments = []string{
	".axiom/" + backupRootLiteralStateSubdirectory,
	".gentle-ai/" + backupRootLiteralStateSubdirectory,
}

// backupRootOwningPackageDir is the single package allowed to spell out the
// backup root literals: it is their owning package. The exceptions list has
// exactly one entry [D-04].
const backupRootOwningPackageDir = "internal/backup"

// TestUserStateRootsResolveThroughOwningPackage is the fifth assertion of the
// canonical-binary guard family [D-04]. It walks every production (non-test)
// Go file under internal/, excluding internal/backup (its owning package),
// and fails when production code spells out the backup root as a
// ".axiom"/".gentle-ai" literal immediately followed by the "backups" state
// subdirectory literal — via filepath.Join, string concatenation, an
// fmt.Sprintf format string, or a single fully-spelled-out literal — instead
// of resolving it through
// backup.BackupRootFor/backup.LegacyBackupRootFor/backup.BackupRoots.
//
// Phase F0.a (inc-20-upstream-reconciliation) ran this guard for real,
// scoped to the filepath.Join form only, and observed it fail, listing every
// production site that still spelled the literal out that way [D-03]; that
// RED transcript is recorded in the phase report. Because this chain ships
// to main with every PR (stacked-to-main), a declared-red assertion there
// would have broken CI for every PR stacked on top of it, so Phase F0.a
// shipped this test self-skipped.
//
// Phase F0.b (this phase) removed that skip as its first RED step, then
// extended detection to the string-concatenation form before migrating
// anything (REQ-20.14 amendment, 2026-09-19): internal/cli/restore.go built
// the legacy root by concatenating homeDir with a literal
// "/.gentle-ai/backups" suffix, a form the filepath.Join-only detector could
// not see. That extension raised the observed violation count from eight
// sites to nine. Phase F0.b then migrated all nine to the canonical
// accessors, so this assertion now runs unskipped and green.
func TestUserStateRootsResolveThroughOwningPackage(t *testing.T) {
	repoRoot := repositoryRoot(t)

	violations, err := backupRootLiteralViolations(repoRoot)
	if err != nil {
		t.Fatalf("scan internal/**/*.go for backup root literals: %v", err)
	}

	for _, violation := range violations {
		t.Errorf("%s: production code outside %s must resolve the backup root through backup.BackupRootFor/backup.LegacyBackupRootFor/backup.BackupRoots, not the literal %q", violation.position, backupRootOwningPackageDir, violation.literal)
	}
}

// backupRootLiteralViolation records one site that spells out a backup root
// literal outside its owning package, whether via a filepath.Join argument
// pair or a single string literal already carrying the full path fragment.
type backupRootLiteralViolation struct {
	position string
	literal  string
}

// backupRootLiteralViolations walks internal/**/*.go production files under
// repoRoot, excluding backupRootOwningPackageDir, and returns every site
// that spells out a backup root literal via filepath.Join, string
// concatenation, fmt.Sprintf, or a fully spelled-out literal.
func backupRootLiteralViolations(repoRoot string) ([]backupRootLiteralViolation, error) {
	internalRoot := filepath.Join(repoRoot, "internal")
	var violations []backupRootLiteralViolation

	walkErr := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", path, err)
		}
		relSlash := filepath.ToSlash(relPath)

		if relSlash == backupRootOwningPackageDir || strings.HasPrefix(relSlash, backupRootOwningPackageDir+"/") {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", relSlash, err)
		}

		fileSet := token.NewFileSet()
		tree, err := parser.ParseFile(fileSet, relSlash, source, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relSlash, err)
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

// backupRootLiteralViolationsInFile inspects a single parsed file for two
// independent violation shapes: a filepath.Join call whose adjacent
// arguments spell out a backup root literal (directory, then "backups" as
// separate arguments), and any single string literal that already spells
// out a full backupRootLiteralFragments entry on its own — which is how the
// concatenation (x + "/.gentle-ai/backups"), fmt.Sprintf
// ("%s/.gentle-ai/backups"), and fully-spelled-out-literal forms all carry
// the offending path, regardless of the surrounding expression.
func backupRootLiteralViolationsInFile(fileSet *token.FileSet, tree *ast.File) []backupRootLiteralViolation {
	var violations []backupRootLiteralViolation

	ast.Inspect(tree, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			if !isFilepathJoinCall(n) {
				return true
			}
			for i := 0; i+1 < len(n.Args); i++ {
				directory, ok := stringLiteralValue(n.Args[i])
				if !ok || !backupRootLiteralDirectories[directory] {
					continue
				}
				subdirectory, ok := stringLiteralValue(n.Args[i+1])
				if !ok || subdirectory != backupRootLiteralStateSubdirectory {
					continue
				}
				violations = append(violations, backupRootLiteralViolation{
					position: fileSet.Position(n.Args[i].Pos()).String(),
					literal:  directory + "/" + subdirectory,
				})
			}
		case *ast.BasicLit:
			value, ok := stringLiteralValue(n)
			if !ok {
				return true
			}
			for _, fragment := range backupRootLiteralFragments {
				if strings.Contains(value, fragment) {
					violations = append(violations, backupRootLiteralViolation{
						position: fileSet.Position(n.Pos()).String(),
						literal:  fragment,
					})
					break
				}
			}
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

// goreleaserBuildTarget is the subset of a .goreleaser.yaml builds[] entry
// this guard cares about: enough to confirm the canonical binary is one of
// the published artifacts, and nothing about signing, archives or brews.
type goreleaserBuildTarget struct {
	ID     string `yaml:"id"`
	Main   string `yaml:"main"`
	Binary string `yaml:"binary"`
}

// goreleaserConfig is the subset of .goreleaser.yaml this guard parses.
type goreleaserConfig struct {
	Builds []goreleaserBuildTarget `yaml:"builds"`
}

// TestReleaseArtifactBuildsCanonicalBinary fails until .goreleaser.yaml
// publishes a build whose main package is canonicalBinaryPath under the
// "axiom" binary name [D-04, D-08]. Phase F0.a and F0.b ran this guard
// before it existed; this phase (F0.c1) adds it RED (only
// main: ./cmd/gentle-ai published) and turns it GREEN by splitting
// .goreleaser.yaml's single build into the two D2.4 requires.
func TestReleaseArtifactBuildsCanonicalBinary(t *testing.T) {
	repoRoot := repositoryRoot(t)
	path := filepath.Join(repoRoot, ".goreleaser.yaml")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var config goreleaserConfig
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	for _, build := range config.Builds {
		if build.Main == canonicalBinaryPath && build.Binary == "axiom" {
			return
		}
	}
	t.Errorf("%s: no builds[] entry publishes main: %s with binary: axiom", path, canonicalBinaryPath)
}

// buildTargetException documents why one specific workflow file is allowed
// to keep naming deprecatedShimPath as a build target. The exceptions list
// TestWorkflowsBuildCanonicalBinary consults is empty for this phase: F0.c1's
// audit found no legitimate site that still needs to build the deprecated
// shim as its own target.
type buildTargetException struct {
	File   string // repository-relative path, forward slashes
	Reason string // why the shim is correct here; never empty
}

// workflowBuildTargetExceptions is the written-reason allowlist
// TestWorkflowsBuildCanonicalBinary consults.
var workflowBuildTargetExceptions = []buildTargetException{}

// TestWorkflowsBuildCanonicalBinary rejects any non-comment line under
// .github/workflows/*.yml that names deprecatedShimPath, unless its file is
// listed in workflowBuildTargetExceptions with a written reason [D-04].
// Comment lines that merely explain why the shim is wrong here (for example
// ci.yml's "Build the canonical binary, not ./cmd/gentle-ai" notes above the
// job's own ./cmd/axiom builds) are not build targets and are excluded, so
// this guard does not force an exception entry for explanatory prose.
func TestWorkflowsBuildCanonicalBinary(t *testing.T) {
	repoRoot := repositoryRoot(t)
	pattern := filepath.Join(repoRoot, ".github", "workflows", "*.yml")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(matches) == 0 {
		t.Fatalf("no workflow files matched %s", pattern)
	}

	exceptionsByFile := make(map[string]string, len(workflowBuildTargetExceptions))
	for _, exception := range workflowBuildTargetExceptions {
		if exception.Reason == "" {
			t.Fatalf("exception for %s has no written reason", exception.File)
		}
		exceptionsByFile[exception.File] = exception.Reason
	}

	for _, match := range matches {
		relPath, err := filepath.Rel(repoRoot, match)
		if err != nil {
			t.Fatalf("resolve relative path for %s: %v", match, err)
		}
		relSlash := filepath.ToSlash(relPath)
		if _, excepted := exceptionsByFile[relSlash]; excepted {
			continue
		}

		content, err := os.ReadFile(match)
		if err != nil {
			t.Fatalf("read %s: %v", relSlash, err)
		}

		for i, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.Contains(line, deprecatedShimPath) {
				t.Errorf("%s:%d: names %s as a build target; the canonical binary is %s, or add a written exception to workflowBuildTargetExceptions", relSlash, i+1, deprecatedShimPath, canonicalBinaryPath)
			}
		}
	}
}

// canonicalDispatchVerbs parses file with go/parser and returns the string
// literals used as case values in its first top-level switch statement --
// the dispatch switch on the first CLI argument in both cmd/axiom/main.go's
// main() and internal/app.RunArgs. It stops descending as soon as that
// switch is found, so a subcommand's own nested switch (for example
// "project"'s list/switch/add/remove in main.go) is never mistaken for the
// top-level dispatch switch.
func canonicalDispatchVerbs(t *testing.T, file string) map[string]struct{} {
	t.Helper()

	source, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}

	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, file, source, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	verbs := make(map[string]struct{})
	found := false
	ast.Inspect(tree, func(node ast.Node) bool {
		if found {
			return false
		}
		switchStmt, ok := node.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		found = true
		for _, stmt := range switchStmt.Body.List {
			caseClause, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range caseClause.List {
				if value, ok := stringLiteralValue(expr); ok {
					verbs[value] = struct{}{}
				}
			}
		}
		return false
	})
	if !found {
		t.Fatalf("%s: no top-level switch statement found", file)
	}

	return verbs
}

// TestAppDispatchIsSubsetOfCanonicalDispatch is the root assertion of the
// canonical-binary guard family [D-04]: every verb reachable through the
// deprecated internal/app.RunArgs dispatch switch MUST also be reachable
// through cmd/axiom/main.go's own dispatch switch. This is the guard that
// would have caught codegraph, telemetry, skill-registry and
// bench-model-picker existing only in internal/app before this increment.
//
// Characterization test (task 3.5): the four verbs above were already
// rewired into cmd/axiom/main.go before this phase started (main.go:396-415),
// so this assertion is expected to pass the first time it runs, not to go
// through a RED step.
func TestAppDispatchIsSubsetOfCanonicalDispatch(t *testing.T) {
	repoRoot := repositoryRoot(t)

	appVerbs := canonicalDispatchVerbs(t, filepath.Join(repoRoot, "internal", "app", "app.go"))
	axiomVerbs := canonicalDispatchVerbs(t, filepath.Join(repoRoot, "cmd", "axiom", "main.go"))

	for verb := range appVerbs {
		if _, ok := axiomVerbs[verb]; !ok {
			t.Errorf("verb %q is reachable through the deprecated cmd/gentle-ai shim (internal/app.RunArgs) but not through the canonical cmd/axiom binary", verb)
		}
	}
}

// TestDeadcodeRatchetTargetsCanonicalBinary fixes
// scripts/deadcode-ratchet.sh's default DEADCODE_TARGET so the dead-code
// ratchet measures reachability from the canonical binary, not the
// deprecated shim [D-04].
//
// Characterization test (task 3.6): the target was already corrected to
// ./cmd/axiom before this increment started (scripts/deadcode-ratchet.sh:41),
// so this assertion is expected to pass the first time it runs. It turns
// that fact into evidence instead of an unverified assumption.
func TestDeadcodeRatchetTargetsCanonicalBinary(t *testing.T) {
	repoRoot := repositoryRoot(t)
	path := filepath.Join(repoRoot, "scripts", "deadcode-ratchet.sh")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	want := `target="${DEADCODE_TARGET:-` + canonicalBinaryPath + `}"`
	if !strings.Contains(string(content), want) {
		t.Errorf("%s: does not default DEADCODE_TARGET to %s (want to contain %q)", path, canonicalBinaryPath, want)
	}
}
