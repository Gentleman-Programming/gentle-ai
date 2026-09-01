package cli

// Wave 5 (Gate Cutover), Slice 5: pre-PR chain composition deletion (design
// decision 1's "DELETED edges: EvaluateCompactPrePRChain..."). This file
// carries the slice's two named call-graph regression guards (tasks 6.1 and
// 6.2) — both trivially true once EvaluateCompactPrePRChain and its call
// sites are deleted, but implemented as real, persisting tests per the
// coordinator's own instruction, not skipped as "obviously true".

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// reviewPrePRCompositionScanRoots names the two production trees a call to
// EvaluateCompactPrePRChain could legitimately appear in: the funnel that
// used to call it (internal/cli) and the package that used to define it
// (internal/reviewtransaction). Test files are excluded — a lingering
// reference in an already-deleted-alongside test file is not a production
// regression, and this slice deletes those test files anyway.
func reviewPrePRCompositionScanRoots(t *testing.T) []string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve pre-PR composition deletion test source")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	return []string{
		filepath.Join(moduleRoot, "internal", "cli"),
		filepath.Join(moduleRoot, "internal", "reviewtransaction"),
	}
}

// scanForEvaluateCompactPrePRChainCalls walks every non-test .go file under
// each root and reports every CallExpr resolving to "EvaluateCompactPrePRChain"
// — a bare identifier (a same-package call, the shape internal/reviewtransaction's
// own former call sites used) or a "pkg.EvaluateCompactPrePRChain" selector
// (a cross-package call, the shape internal/cli's former funnel call sites
// used). This is a pure name-match scan (no type resolution): after this
// slice deletes the function itself, ANY match is unambiguously a
// regression — either a stray old call site or a reintroduced one — since no
// production code path can name a symbol that does not exist.
func scanForEvaluateCompactPrePRChainCalls(t *testing.T, roots []string) []string {
	t.Helper()
	var violations []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(root, name)
			fileSet := token.NewFileSet()
			tree, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(tree, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				var callee string
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					callee = fun.Name
				case *ast.SelectorExpr:
					callee = fun.Sel.Name
				}
				if callee == "EvaluateCompactPrePRChain" {
					violations = append(violations, fileSet.Position(call.Pos()).String())
				}
				return true
			})
		}
	}
	return violations
}

// TestPrePRComposition_ZeroCallers is task 6.1: no gate calls
// EvaluateCompactPrePRChain to authorize delivery. Before this slice's
// deletion this scan finds review_facade.go's exact two composition-retry
// call sites (proving the guard is genuinely wired to real source, not a
// vacuous pass); after deletion it finds zero, permanently, because the
// symbol no longer exists to name.
func TestPrePRComposition_ZeroCallers(t *testing.T) {
	violations := scanForEvaluateCompactPrePRChainCalls(t, reviewPrePRCompositionScanRoots(t))
	if len(violations) != 0 {
		t.Fatalf("EvaluateCompactPrePRChain is still called from production code (must be zero after Slice 5's deletion): %v", violations)
	}
}

// TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved is #2239's
// named corroboration (Gate Regression Test Index, tasks.md): the original
// bug was the kill switch racing composition's own authority read. Slice 2
// already fixed the ordering (one early consultation before ANY authority
// read); this slice deletes composition itself, so the race is not merely
// fixed but structurally impossible — the same call-absence
// TestPrePRComposition_ZeroCallers proves also proves #2239 trivially:
// composition cannot run before, after, or racing the kill switch if it
// never runs at all.
func TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved(t *testing.T) {
	violations := scanForEvaluateCompactPrePRChainCalls(t, reviewPrePRCompositionScanRoots(t))
	if len(violations) != 0 {
		t.Fatalf("#2239 requires composition never to run at all (not merely after the kill switch): %v", violations)
	}
}
