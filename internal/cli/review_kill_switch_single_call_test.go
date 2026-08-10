package cli

// TestKillSwitchOrdering_SingleCallBeforeAuthorityRead is Wave 5 Slice 2's
// task 3.1: a static AST guard proving runReviewFacadeValidate calls
// reviewDeliveryDisposition EXACTLY ONCE. Before Slice 2 the funnel had
// THREE such call sites (Amendment C's own discoveryErr branch, the
// contested compact+legacy branch, and the disabledDiscovery branch --
// design.md's stale "two late reads" prose undercounted this; see the
// design.md amendment landed alongside this test). A behavioral (fixture)
// proof cannot distinguish "one call" from "N calls that all happen to
// agree" without an unwieldy toggle-mid-evaluation rig; a call-count guard
// proves it directly and durably, in the pattern review_offer_absence_guard_test.go
// and review_named_continuation_test.go already use for call-absence proofs
// in this package.
//
// This is also the FIRST call any authority read must follow: the guard
// additionally asserts the reviewDeliveryDisposition call is lexically
// positioned before the function's first call to any of the authority-read
// entry points (resolveGoverningAuthority, discoverCompactFacadeGateReview,
// ResolveCandidateDeclineForGate, discoverFacadeReview), proving "before any
// authority read" by source position, not just by call count.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

var killSwitchAuthorityReadSelectors = map[string]bool{
	"resolveGoverningAuthority":       true,
	"discoverCompactFacadeGateReview": true,
	"ResolveCandidateDeclineForGate":  true,
	"discoverFacadeReview":            true,
}

func TestKillSwitchOrdering_SingleCallBeforeAuthorityRead(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, filepath.Join(".", "review_facade.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse review_facade.go: %v", err)
	}

	var target *ast.FuncDecl
	ast.Inspect(file, func(node ast.Node) bool {
		if fn, ok := node.(*ast.FuncDecl); ok && fn.Name.Name == "runReviewFacadeValidate" {
			target = fn
			return false
		}
		return true
	})
	if target == nil || target.Body == nil {
		t.Fatal("runReviewFacadeValidate not found in review_facade.go")
	}

	killSwitchCalls := 0
	var killSwitchPos, firstAuthorityReadPos token.Pos
	ast.Inspect(target.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "reviewDeliveryDisposition" {
			killSwitchCalls++
			if killSwitchPos == token.NoPos {
				killSwitchPos = call.Pos()
			}
		}
		if killSwitchAuthorityReadSelectors[ident.Name] && firstAuthorityReadPos == token.NoPos {
			firstAuthorityReadPos = call.Pos()
		}
		return true
	})

	if killSwitchCalls != 1 {
		t.Fatalf("runReviewFacadeValidate calls reviewDeliveryDisposition %d times, want exactly 1 (three pre-Slice-2 call sites collapsed into one)", killSwitchCalls)
	}
	if killSwitchPos == token.NoPos {
		t.Fatal("reviewDeliveryDisposition call not found")
	}
	if firstAuthorityReadPos == token.NoPos {
		t.Fatal("no authority-read call found to order against -- update killSwitchAuthorityReadSelectors if the funnel's read entry points were renamed")
	}
	if killSwitchPos >= firstAuthorityReadPos {
		t.Fatalf("kill switch consulted at %v is not lexically before the first authority read at %v",
			fileSet.Position(killSwitchPos), fileSet.Position(firstAuthorityReadPos))
	}
}
