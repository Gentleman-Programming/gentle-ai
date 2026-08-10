package reviewtransaction

// Wave 5 (Gate Cutover) Slice 7, task 8.3: the AST/call-graph guard proving
// no gate evaluation code path calls acquireStoreLock, writeAtomic, or
// os.Remove -- the exact three write primitives compact_approved_invalidation.go's
// deleted InvalidateApprovedCompactAuthority combined with a live gate
// derivation, the "gate mutates authority" anti-pattern design.md's own
// evidence-correction note names. Proven by call-absence (this file's own
// scanned set), not by a passing green path.
//
// This reuses shadow_readonly_guard_test.go / candidate_readonly_guard_test.go's
// established file-scoped AST-scan idiom rather than a full transitive
// call-graph traversal: the files a `review validate` call can reach to
// decide a verdict are named explicitly, and every one of them is scanned
// directly. compact_approved_invalidation.go itself -- the one file that
// ever combined gate evaluation with a write -- is included in the scanned
// set specifically so this guard is genuinely RED before its deletion and
// permanently GREEN after (the file no longer exists to violate anything).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// gateWriteGuardFiles are every production file a `review validate` /
// `EvaluateCompactGate` / `EvaluateNativeGate` / `EvaluateLegacyGate` call
// can reach to decide a verdict.
var gateWriteGuardFiles = []string{
	"gate.go", "compact_gate.go", "compact_gate_discovery_baseline.go",
	"legacy_projection.go", "native_request.go", "compact_approved_invalidation.go",
}

// gateWriteGuardForbidden are the exact write primitives forbidden anywhere
// in gateWriteGuardFiles: "pkg.Func" for a package-qualified call
// (os.Remove), or the bare identifier for a same-package call
// (acquireStoreLock, writeAtomic).
var gateWriteGuardForbidden = map[string]bool{
	"acquireStoreLock": true,
	"writeAtomic":      true,
	"os.Remove":        true,
}

func scanGateWriteGuardFile(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var violations []string
	ast.Inspect(tree, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := derivedObservationCallExprName(call)
		if gateWriteGuardForbidden[callee] {
			violations = append(violations, fileSet.Position(call.Pos()).String()+": "+callee)
		}
		return true
	})
	return violations, nil
}

// TestNoGateWritesAuthority_CallAbsenceGuard is task 8.3. Before this
// slice's deletion, scanning compact_approved_invalidation.go (included in
// gateWriteGuardFiles above) finds acquireStoreLock, writeAtomic, and
// os.Remove — proving the guard is genuinely wired to real source, not a
// vacuous pass. After deletion, that file no longer exists
// (scanGateWriteGuardFile treats a missing file as zero violations, not an
// error — the file's absence IS the proof), and every surviving gate file
// has zero matches, permanently: any future gate-path code that starts
// mutating authority fails this guard immediately.
func TestNoGateWritesAuthority_CallAbsenceGuard(t *testing.T) {
	var violations []string
	for _, name := range gateWriteGuardFiles {
		found, err := scanGateWriteGuardFile(name)
		if err != nil {
			t.Fatalf("scan %s: %v", name, err)
		}
		violations = append(violations, found...)
	}
	if len(violations) != 0 {
		t.Fatalf("gate evaluation code calls a write primitive (must be zero after Slice 7's deletion): %v", violations)
	}
}
