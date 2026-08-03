package reviewtransaction

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// This file is the AST guard task C4b promised (the design's own Testing
// Strategy Unit row: "AST guard proves no DeriveObservation result is
// persisted", spec rdd-authority-store "Five Persisted States, Everything
// Else Derived"). It reuses the exact scanning idiom
// shadow_readonly_guard_test.go / candidate_readonly_guard_test.go already
// established (ast.Inspect over a parsed file, one violation string per
// offending node with its source position) rather than inventing a new
// pattern.
//
// It statically proves that none of this package's "shadow/candidate/core"
// production files — the files that can see a DerivedObservation value at
// all — ever marshals or writes one:
//
//  1. no local variable assigned from a DeriveObservation(...) call (or a
//     selector expression rooted at one) is ever passed as an argument to a
//     JSON marshal call (json.Marshal, json.MarshalIndent) or a known
//     filesystem/store write primitive (os.WriteFile, writeAtomic,
//     publishImmutable, (AuthorityStore).Mutate's apply closure body is out
//     of this guard's scope — that closure legitimately writes
//     NewLineageAuthority fields, never a DerivedObservation).
//
// The guard is intentionally narrower than a full dataflow analysis (no
// type checker, no interprocedural tracking): it tracks only same-function,
// direct assignment from a DeriveObservation(...) call — the one shape any
// caller in this package actually uses (see review_governing_authority.go's
// `observation := reviewtransaction.DeriveObservation(...)` in the sibling
// cli package, and review_core.go's own callers). A synthetic-source unit
// test proves the scanner catches this exact shape before it is trusted
// against real production files.
var derivedObservationGuardFiles = []string{
	"review_core.go", "review_core_transition.go", "authority_store.go",
	"new_lineage_discovery.go", "candidate_relation.go", "candidate_identity.go",
}

// derivedObservationWritePrimitives are the exact write/marshal calls the
// guard forbids a DeriveObservation-derived value from reaching: "pkg.Func"
// for a package-qualified call (json.Marshal, os.WriteFile), or the bare
// function name for a same-package call. writeAtomic and publishImmutable
// (W11 remediation) are exactly HOW authority_store.go persists
// review-state.json and review-receipt.json — the doc comment above always
// claimed them in scope, but the scanner's original callee resolution
// (shadowCallExprName, selector-only) could never match a bare identifier
// call, so neither was ever actually checked. Proven by mutation: adding
// `writeAtomic(path, []byte(observation.Relation), 0o644)` to
// authority_store.go left the pre-fix guard green.
var derivedObservationWritePrimitives = map[string]bool{
	"json.Marshal":       true,
	"json.MarshalIndent": true,
	"os.WriteFile":       true,
	"ioutil.WriteFile":   true,
	"writeAtomic":        true,
	"publishImmutable":   true,
}

// TestDerivedObservationWriteGuardCatchesMarshalShapes unit-tests the
// scanner itself against synthetic sources, independent of whatever the
// named production files currently contain.
func TestDerivedObservationWriteGuardCatchesMarshalShapes(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantViolation bool
	}{
		{
			name: "clean read-only source using DeriveObservation only for its Relation field",
			src: `package reviewtransaction

func derivedExampleReadOnly(authority NewLineageAuthority, live CandidateIdentity, evidence CoreValidateEvidence) CandidateRelation {
	observation := DeriveObservation(authority, live, evidence)
	return observation.Relation
}
`,
			wantViolation: false,
		},
		{
			name: "marshals a DeriveObservation result directly",
			src: `package reviewtransaction

import "encoding/json"

func derivedMarshalsDirectly(authority NewLineageAuthority, live CandidateIdentity, evidence CoreValidateEvidence) ([]byte, error) {
	observation := DeriveObservation(authority, live, evidence)
	return json.Marshal(observation)
}
`,
			wantViolation: true,
		},
		{
			name: "marshals a field selected off a DeriveObservation result",
			src: `package reviewtransaction

import "encoding/json"

func derivedMarshalsField(authority NewLineageAuthority, live CandidateIdentity, evidence CoreValidateEvidence) ([]byte, error) {
	observation := DeriveObservation(authority, live, evidence)
	return json.MarshalIndent(observation.Relation, "", "  ")
}
`,
			wantViolation: true,
		},
		{
			name: "writes a DeriveObservation result to a file",
			src: `package reviewtransaction

import "os"

func derivedWritesToFile(authority NewLineageAuthority, live CandidateIdentity, evidence CoreValidateEvidence) error {
	observation := DeriveObservation(authority, live, evidence)
	return os.WriteFile("path", []byte(observation.Relation), 0o644)
}
`,
			wantViolation: true,
		},
		{
			// W11: writeAtomic and publishImmutable are same-package bare
			// calls, not "pkg.Func" selector calls — the exact shape the
			// original guard's callee resolution could never match.
			name: "writes a DeriveObservation result via writeAtomic",
			src: `package reviewtransaction

func derivedWritesAtomicDirectly(authority NewLineageAuthority, live CandidateIdentity, evidence CoreValidateEvidence) error {
	observation := DeriveObservation(authority, live, evidence)
	return writeAtomic("path", []byte(observation.Relation), 0o644)
}
`,
			wantViolation: true,
		},
		{
			name: "publishes a DeriveObservation result via publishImmutable",
			src: `package reviewtransaction

func derivedPublishesImmutableDirectly(authority NewLineageAuthority, live CandidateIdentity, evidence CoreValidateEvidence) error {
	observation := DeriveObservation(authority, live, evidence)
	return publishImmutable("path", []byte(observation.Relation), 0o644)
}
`,
			wantViolation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := scanDerivedObservationWriteSource("synthetic_derived_observation_test_input.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("scanDerivedObservationWriteSource: %v", err)
			}
			if got := len(violations) > 0; got != tt.wantViolation {
				t.Fatalf("violations = %v (len=%d), want non-empty=%v", violations, len(violations), tt.wantViolation)
			}
		})
	}
}

// TestDerivedObservationWriteGuardHoldsForProductionFiles runs the same
// scanner against every named production "shadow/candidate/core" file and
// fails with the exact violation evidence if any is found.
func TestDerivedObservationWriteGuardHoldsForProductionFiles(t *testing.T) {
	for _, file := range derivedObservationGuardFiles {
		t.Run(file, func(t *testing.T) {
			violations, err := scanDerivedObservationWriteSourceFile(file)
			if err != nil {
				t.Fatalf("scanDerivedObservationWriteSourceFile(%s): %v", file, err)
			}
			if len(violations) > 0 {
				t.Fatalf("%s marshals or writes a DeriveObservation result: %v", file, violations)
			}
		})
	}
}

func scanDerivedObservationWriteSourceFile(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	return scanDerivedObservationWriteTree(fileSet, tree), nil
}

func scanDerivedObservationWriteSource(filename string, src []byte) ([]string, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, filename, src, 0)
	if err != nil {
		return nil, err
	}
	return scanDerivedObservationWriteTree(fileSet, tree), nil
}

// scanDerivedObservationWriteTree walks one parsed file function by
// function. Within each function body it tracks the set of local identifier
// names directly assigned from a `DeriveObservation(...)` call (`x :=
// DeriveObservation(...)` or `x = DeriveObservation(...)`), then flags any
// call to a forbidden write/marshal primitive whose argument list contains
// one of those identifiers, or a selector expression rooted at one (e.g.
// `x.Relation`).
func scanDerivedObservationWriteTree(fileSet *token.FileSet, tree *ast.File) []string {
	var violations []string
	position := func(node ast.Node) string { return fileSet.Position(node.Pos()).String() }

	ast.Inspect(tree, func(node ast.Node) bool {
		funcDecl, ok := node.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			return true
		}
		derivedNames := map[string]bool{}
		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			switch stmt := inner.(type) {
			case *ast.AssignStmt:
				for index, rhs := range stmt.Rhs {
					if index >= len(stmt.Lhs) {
						continue
					}
					if !derivedObservationCallExpr(rhs) {
						continue
					}
					if ident, ok := stmt.Lhs[index].(*ast.Ident); ok {
						derivedNames[ident.Name] = true
					}
				}
			case *ast.CallExpr:
				callee := derivedObservationCallExprName(stmt)
				if callee == "" || !derivedObservationWritePrimitives[callee] {
					return true
				}
				for _, arg := range stmt.Args {
					if name := derivedObservationReferencedIdent(arg, derivedNames); name != "" {
						violations = append(violations, position(stmt)+": "+callee+" is called with a DeriveObservation-derived value ("+name+")")
					}
				}
			}
			return true
		})
		return true
	})
	return violations
}

// derivedObservationCallExprName returns the callee name to match against
// derivedObservationWritePrimitives: "pkg.Func" for a package-qualified call
// (delegating to shadowCallExprName, the shadow guard's own selector-only
// resolution — json.Marshal, os.WriteFile), or the bare identifier itself
// for a same-package call (W11: writeAtomic, publishImmutable — exactly the
// two authority_store.go actually uses, and exactly what
// shadowCallExprName's selector-only lookup could never see). Anything else
// (method calls on a value, etc.) returns "".
func derivedObservationCallExprName(call *ast.CallExpr) string {
	if callee := shadowCallExprName(call); callee != "" {
		return callee
	}
	if ident, ok := call.Fun.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// derivedObservationCallExpr reports whether expr is a call to
// DeriveObservation (bare identifier form — this package's own call site
// shape; a cross-package call would be `reviewtransaction.DeriveObservation`,
// which cannot occur inside this same package's own files).
func derivedObservationCallExpr(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == "DeriveObservation"
}

// derivedObservationReferencedIdent reports the first tracked identifier
// name referenced anywhere inside expr's subtree — the bare variable
// (`observation`), a field access off it (`observation.Relation`), or
// either wrapped in an arbitrary number of conversions/composite
// expressions (`[]byte(observation.Relation)`, `string(observation.Relation)`).
// Walking the whole subtree rather than matching one fixed shape is
// deliberate: a write/marshal call's argument can wrap a tracked value in
// any expression form, and this guard's job is to catch the reference
// itself, not a specific syntax shape around it.
func derivedObservationReferencedIdent(expr ast.Expr, tracked map[string]bool) string {
	found := ""
	ast.Inspect(expr, func(node ast.Node) bool {
		if found != "" {
			return false
		}
		if ident, ok := node.(*ast.Ident); ok && tracked[ident.Name] {
			found = ident.Name
			return false
		}
		return true
	})
	return found
}
