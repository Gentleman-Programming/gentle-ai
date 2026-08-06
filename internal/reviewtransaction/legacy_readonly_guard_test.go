package reviewtransaction

// RG.1 (Wave 7 S1, design decision 5 / tasks.md 1.9). D5 defines "one
// lifecycle" as: delete legacy MUTATION, retain legacy READ. This guard has
// two halves:
//
//   - RG.1a (retained-read half, GREEN from the moment this file lands):
//     every D5 retained-symbol name still parses as a declared identifier in
//     its home file -- a sanity fence so a later deletion slice cannot
//     accidentally sweep the forensic read path away along with the
//     mutation it sits beside.
//   - RG.1b (mutation-reachability half): no RETIRED legacy-mutation CLI
//     verb literal is reachable from internal/cli/review_facade.go's own
//     dispatch switches (runReviewCommand, runReviewCommandContext).
//
// RG.1b's own outcome (WU19, S8, row 24): the design's original plan
// assumed WU18 (switch removal) would land before WU19 ran, making
// compact-v2 a purely historical, frozen concern by classification time --
// under that assumption, each of the six D4 verbs
// (invalidate/abandon/recover/reclaim/dispose-result/reopen-results) would
// be classified as either dead (delete) or a residual D5 forensic-READ path
// (retain read-only). WU18 was DEFERRED (coordinator scope decision,
// specs/rdd-single-lifecycle/spec.md's amendment), so this assumption does
// not hold: GENTLE_AI_RDD_NEW_LINEAGE and the legacy start branch are still
// live, so compact-v2 REMAINS the default, actively-mutated lifecycle for
// every candidate started without the switch. WU19's classification of all
// six verbs, evidence-based (each one's own handler constructs/mutates a
// reviewtransaction.Compact* record -- ReviewAbandonResult's
// CompactReclaimRecord, RunReviewInvalidate's own "pristine reviewing
// authority" framing, and so on for reclaim/dispose-result/reopen-results):
// every one is LIVE, ACTIVE MUTATION surface for the current default
// lifecycle, not a legacy-frozen relic and not a D5 retained-READ path
// (D5's retained-read list is for FROZEN v1 forensic access, never for a
// still-mutable compact-v2 verb). None qualify for deletion; none qualify
// for read-only retention either -- they simply are not legacy yet. This
// guard's own scope narrows to match: legacyRetiredMutationVerbs now lists
// only the 5 verbs actually retired this wave (reconcile-authority/-batch,
// the 3 quarantine/repair verbs); legacyLiveCompactV2MutationVerbs is a
// new, separate, POSITIVE assertion that the 6 D4 verbs remain reachable
// -- this guard is GREEN either way it looks at the current dispatch
// table, honestly, rather than forced green by deleting live surface.
//
// Re-classification is a live task for the wave that lands switch removal:
// once GENTLE_AI_RDD_NEW_LINEAGE and the legacy branch are actually gone,
// re-run this exact classification (does the verb's own handler still
// mutate reachable compact-v2 state, or is that state now permanently
// frozen) and move each verb to delete or D5-retained-read accordingly.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// legacyRetainedReadSymbols is D5's closed retained-read list: file path
// (relative to this package directory) -> identifier names that file must
// still declare. sddstatus/legacy_binding_read.go, sddstatus/review_binding.go
// and candidate_decline.go live outside this package's own directory, so
// their paths are relative to this package's own directory.
// AuthoritativeStore/LoadChain/NewLegacyReadOnlyError are declared inside
// THIS package (store.go, compact_store.go) — review_facade.go:1632-1635
// (design.md) is only their call site, not their declaration.
var legacyRetainedReadSymbols = map[string][]string{
	"../sddstatus/legacy_binding_read.go": {"parseLegacyBinding"},
	"../sddstatus/review_binding.go":      {"parseBinding", "bindingBytes", "bindingDigest", "bindingPath"},
	"candidate_decline.go":                {"parseCandidateDeclineAuthorization"},
	"store.go":                            {"AuthoritativeStore", "LoadChain"},
	"compact_store.go":                    {"NewLegacyReadOnlyError"},
}

// legacyRetiredMutationVerbs is the exact case-clause literal set this wave
// actually retired: reconcile-authority/-batch (S3/S4), and the three
// quarantine/repair verbs (S5). The six D4 verbs of ambiguous vintage (S8,
// row 24) are NOT in this list -- WU19 classified them as still-live
// compact-v2 mutation surface (see legacyLiveCompactV2MutationVerbs below
// and this file's own package-level doc comment for the full
// WU18-deferral-driven rationale), not retired.
var legacyRetiredMutationVerbs = []string{
	"reconcile-authority", "reconcile-authority-batch",
	"quarantine-legacy", "quarantine-legacy-fix-scope", "repair-legacy-alias",
}

// legacyLiveCompactV2MutationVerbs is WU19's (S8, row 24) classification
// outcome for the six D4 verbs: each one's own handler mutates a
// reviewtransaction.Compact* record (ReviewAbandonResult's
// CompactReclaimRecord, RunReviewInvalidate's own "pristine reviewing
// authority" framing, and equivalently for reclaim/dispose-result/
// reopen-results) -- live, active mutation surface for the current default
// (switch-gated) compact-v2 lifecycle, confirmed reachable and expected to
// STAY reachable while GENTLE_AI_RDD_NEW_LINEAGE and the legacy start
// branch remain (WU18 deferred, specs/rdd-single-lifecycle/spec.md's
// amendment). This is a positive assertion, not a RED-until-something
// placeholder: TestLegacyReadOnlyGuardLiveCompactV2VerbsRemainReachable
// fails if any of these six literals disappear from dispatch without this
// list being updated in the same slice, the same discipline
// legacyRetiredMutationVerbs already holds retired verbs to.
var legacyLiveCompactV2MutationVerbs = []string{
	"invalidate", "abandon", "recover", "reclaim", "dispose-result", "reopen-results",
}

// legacyDispatchFunctions is the closed set of functions in
// internal/cli/review_facade.go whose case-clause literals this guard scans
// -- runReviewCommand (default review command dispatch) and
// runReviewCommandContext (the context-aware dispatch, which falls through
// to runReviewCommand by default but has its own reconcile-authority-batch
// case).
var legacyDispatchFunctions = []string{"runReviewCommand", "runReviewCommandContext"}

// TestLegacyReadOnlyGuardRetainedSymbolsDeclared is RG.1a: proves every D5
// retained-read symbol is still a declared identifier in its home file.
func TestLegacyReadOnlyGuardRetainedSymbolsDeclared(t *testing.T) {
	for file, symbols := range legacyRetainedReadSymbols {
		t.Run(file, func(t *testing.T) {
			declared, err := declaredIdentifiers(file)
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			for _, symbol := range symbols {
				if !declared[symbol] {
					t.Fatalf("%s no longer declares retained read symbol %q (D5 requires it to survive every legacy-mutation deletion slice)", file, symbol)
				}
			}
		})
	}
}

// TestLegacyReadOnlyGuardMutationVerbsUnreachable is RG.1b's retired-verb
// half: proves no verb literal this wave actually retired remains a case
// clause in review_facade.go's dispatch switches.
func TestLegacyReadOnlyGuardMutationVerbsUnreachable(t *testing.T) {
	reachable := dispatchReachableVerbs(t)
	var stillReachable []string
	for _, verb := range legacyRetiredMutationVerbs {
		if reachable[verb] {
			stillReachable = append(stillReachable, verb)
		}
	}
	if len(stillReachable) > 0 {
		t.Fatalf("retired verbs still reachable from review_facade.go dispatch: %v (each one this wave retired must stay unreachable)", stillReachable)
	}
}

// TestLegacyReadOnlyGuardLiveCompactV2VerbsRemainReachable is RG.1b's
// live-verb half, added at WU19 classification time: the six D4 verbs are
// NOT retired (see this file's own package-level doc comment and
// legacyLiveCompactV2MutationVerbs for the full WU18-deferral rationale) --
// this is a positive assertion that they stay reachable, exactly as
// reachable as any other still-live dispatch case, and fails loudly (not
// silently) if one disappears without this list being updated in the same
// slice.
func TestLegacyReadOnlyGuardLiveCompactV2VerbsRemainReachable(t *testing.T) {
	reachable := dispatchReachableVerbs(t)
	var missing []string
	for _, verb := range legacyLiveCompactV2MutationVerbs {
		if !reachable[verb] {
			missing = append(missing, verb)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("compact-v2 mutation verbs classified as still-live (WU19) are no longer reachable from review_facade.go dispatch: %v (either this was an unintentional deletion, or the verb genuinely retired -- move it to legacyRetiredMutationVerbs with the written reason in the same slice)", missing)
	}
}

// dispatchReachableVerbs scans every legacyDispatchFunctions entry and
// returns the set of verb literals reachable from review_facade.go's
// dispatch switches.
func dispatchReachableVerbs(t *testing.T) map[string]bool {
	t.Helper()
	reachable := map[string]bool{}
	for _, fn := range legacyDispatchFunctions {
		verbs, err := dispatchCaseLiterals("../cli/review_facade.go", fn)
		if err != nil {
			t.Fatalf("scan dispatch function %s: %v", fn, err)
		}
		for _, verb := range verbs {
			reachable[verb] = true
		}
	}
	return reachable
}

// declaredIdentifiers returns every top-level func/type/const/var name
// declared in path, plus every field/method name on those declarations that
// carries an *ast.Ident (a coarse but sufficient net for RG.1a's "still
// declared" check).
func declaredIdentifiers(path string) (map[string]bool, error) {
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	ast.Inspect(tree, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			names[n.Name.Name] = true
		case *ast.TypeSpec:
			names[n.Name.Name] = true
		case *ast.ValueSpec:
			for _, name := range n.Names {
				names[name.Name] = true
			}
		}
		return true
	})
	return names, nil
}

// dispatchCaseLiterals returns every string literal case-clause value in
// funcName's top-level switch statement(s) within path.
func dispatchCaseLiterals(path, funcName string) ([]string, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	fileSet := token.NewFileSet()
	tree, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var literals []string
	ast.Inspect(tree, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName || fn.Body == nil {
			return true
		}
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			clause, ok := inner.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if literal, ok := expr.(*ast.BasicLit); ok && literal.Kind == token.STRING {
					literals = append(literals, strings.Trim(literal.Value, `"`))
				}
			}
			return true
		})
		return false
	})
	return literals, nil
}
