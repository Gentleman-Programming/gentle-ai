package reviewtransaction

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// This file is the AST guard for Wave 1's read-only shadow property
// (design decision 1: "Read-only-ness is enforced by an AST guard test
// instead of by package boundary"). It statically proves that every
// production shadow_*.go file in this package:
//
//  1. never declares or calls anything with a *CompactState pointer
//     receiver (every method on *CompactState mutates review state — see
//     compact.go's CompleteReview/Invalidate/BeginCorrection family);
//  2. never references the Store or CompactStore types at all — the shadow
//     algebra takes candidate identities, snapshots, and receipts directly
//     (see design.md's Interfaces/Contracts), never a mutable store handle;
//  3. never calls a known filesystem write primitive (os.WriteFile,
//     os.Create, os.OpenFile, os.MkdirAll, os.Remove, os.RemoveAll,
//     os.Rename, os.Symlink, os.Link, os.Chmod, os.Truncate).
//
// It deliberately does NOT forbid Git calls: read commands (rev-parse,
// diff, cat-file) and even `merge-tree --write-tree` (writes a content
// object, not a ref) are read-only with respect to authority per the
// design's threat matrix, and PR3's relation algebra needs them.
//
// The guard only inspects production shadow_*.go files — not the shadow
// test files themselves, which legitimately build real Git repositories.

// shadowMutatingTypeNames are the exact types whose reference — as a
// receiver, parameter, field, or otherwise — marks a shadow file as
// potentially mutating. CompactState is included because every one of its
// pointer-receiver methods mutates state; Store and CompactStore are
// included in full because the shadow algebra has no legitimate reason to
// reference either.
var shadowMutatingTypeNames = map[string]bool{
	"CompactState": true,
	"Store":        true,
	"CompactStore": true,
}

// shadowWritePrimitives are exact "pkg.Func" filesystem write calls the
// guard forbids inside shadow_*.go production files.
var shadowWritePrimitives = map[string]bool{
	"os.WriteFile":     true,
	"os.Create":        true,
	"os.OpenFile":      true,
	"os.MkdirAll":      true,
	"os.Mkdir":         true,
	"os.Remove":        true,
	"os.RemoveAll":     true,
	"os.Rename":        true,
	"os.Symlink":       true,
	"os.Link":          true,
	"os.Chmod":         true,
	"os.Truncate":      true,
	"os.WriteAt":       true,
	"ioutil.WriteFile": true,
}

// TestShadowReadOnlyGuardCatchesMutationShapes unit-tests the scanner
// itself against synthetic sources, so its three checks are proven correct
// independent of whatever shadow_*.go currently contains.
func TestShadowReadOnlyGuardCatchesMutationShapes(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantViolation bool
	}{
		{
			name: "clean read-only source",
			src: `package reviewtransaction

import "context"

func shadowExampleReadOnly(ctx context.Context, repo string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-parse", "HEAD")
	return string(output), err
}
`,
			wantViolation: false,
		},
		{
			name: "declares a *CompactState pointer receiver",
			src: `package reviewtransaction

func (state *CompactState) shadowMutates() error { return nil }
`,
			wantViolation: true,
		},
		{
			name: "references the Store type in a parameter",
			src: `package reviewtransaction

func shadowTouchesStore(store Store) error { return nil }
`,
			wantViolation: true,
		},
		{
			name: "references the CompactStore type in a variable declaration",
			src: `package reviewtransaction

func shadowTouchesCompactStore() {
	var store CompactStore
	_ = store
}
`,
			wantViolation: true,
		},
		{
			name: "calls a filesystem write primitive",
			src: `package reviewtransaction

import "os"

func shadowWritesAFile() error {
	return os.WriteFile("path", nil, 0o644)
}
`,
			wantViolation: true,
		},
		{
			name: "calls merge-tree --write-tree via runGit, which is allowed",
			src: `package reviewtransaction

import "context"

func shadowMergeTree(ctx context.Context, repo, left, right string) ([]byte, error) {
	return runGit(ctx, repo, nil, nil, "merge-tree", "--write-tree", left, right)
}
`,
			wantViolation: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := scanShadowReadOnlySource("synthetic_shadow_test_input.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("scanShadowReadOnlySource: %v", err)
			}
			if got := len(violations) > 0; got != tt.wantViolation {
				t.Fatalf("violations = %v (len=%d), want non-empty=%v", violations, len(violations), tt.wantViolation)
			}
		})
	}
}

// TestShadowReadOnlyGuardHoldsForProductionFiles used to glob every real
// production shadow_*.go file in this package directory (productionShadowFiles)
// and re-run the scanner against each. Wave 7 S2a retired the last two
// production shadow_*.go files (shadow_observer.go, shadow_authority_health.go)
// -- the glob-and-scan test is retired alongside them, since it would
// otherwise fail with "no production shadow_*.go files found" for having
// nothing left to prove. The scanner itself (scanShadowReadOnlyTree /
// scanShadowReadOnlySourceFile below) is NOT retired: it is reused verbatim
// by candidate_readonly_guard_test.go (explicit file list, not a glob) and
// by derived_observation_write_guard_test.go's shadowCallExprName helper --
// both cover LIVE, retained production files and stay in this package
// indefinitely.

func scanShadowReadOnlySourceFile(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	return scanShadowReadOnlyTree(fileSet, tree), nil
}

func scanShadowReadOnlySource(filename string, src []byte) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, filename, src, 0)
	if err != nil {
		return nil, err
	}
	return scanShadowReadOnlyTree(fileSet, tree), nil
}

// scanShadowReadOnlyTree walks one parsed file and returns one human-
// readable violation string per offending node, each carrying the exact
// source position so a failure is directly actionable.
func scanShadowReadOnlyTree(fileSet *token.FileSet, tree *ast.File) []string {
	var violations []string
	position := func(node ast.Node) string { return fileSet.Position(node.Pos()).String() }

	ast.Inspect(tree, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			if n.Recv == nil {
				return true
			}
			for _, field := range n.Recv.List {
				if name, isPointer := shadowNamedType(field.Type); name != "" && isPointer && shadowMutatingTypeNames[name] {
					violations = append(violations, position(n)+": func "+n.Name.Name+" declares a *"+name+" pointer receiver")
				}
			}
		case *ast.Field:
			if name, _ := shadowNamedType(n.Type); name != "" && shadowMutatingTypeNames[name] {
				violations = append(violations, position(n)+": references mutating type "+name)
			}
		case *ast.ValueSpec:
			if name, _ := shadowNamedType(n.Type); name != "" && shadowMutatingTypeNames[name] {
				violations = append(violations, position(n)+": declares a variable of mutating type "+name)
			}
		case *ast.CallExpr:
			if callee := shadowCallExprName(n); callee != "" && shadowWritePrimitives[callee] {
				violations = append(violations, position(n)+": calls filesystem write primitive "+callee)
			}
		}
		return true
	})
	return violations
}

// shadowNamedType returns the bare identifier name of a type expression
// (unwrapping exactly one leading pointer) and whether it was a pointer
// type. It returns ("", false) for anything else (selector types from
// other packages, generics, etc.) — those are outside this guard's scope.
func shadowNamedType(expr ast.Expr) (name string, isPointer bool) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name, true
		}
		return "", true
	case *ast.Ident:
		return t.Name, false
	default:
		return "", false
	}
}

// shadowCallExprName returns "pkg.Func" for a package-qualified call
// expression, or "" for anything else (local calls, method calls on a
// value, etc. — those are covered by the type checks above where relevant).
func shadowCallExprName(call *ast.CallExpr) string {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name + "." + selector.Sel.Name
}
