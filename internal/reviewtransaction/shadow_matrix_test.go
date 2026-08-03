// Package reviewtransaction — differential matrix (Wave 1, Slice 6). This is
// Wave 1's exit evidence (spec.md "Differential Matrix Exit Evidence"): it
// compares the shadow relation algebra (shadowRelate, seven values,
// shadow_relation.go) against the live classifier (classifyCompactTargetRelation,
// five values, compact_target_relation.go) over a covering-array corpus
// spanning all four selectors, all seven shadow relations, base movement,
// contraction (including Amendment B degradation), ambiguity, and unknown.
//
// The corpus is built at the pure-function level: CandidateIdentity and
// Snapshot values are constructed directly rather than resolved through real
// Git fixtures. Selector-variant normalization onto a canonical tuple is
// already proven by Slice 2's shadow_identity_test.go; this matrix's job is
// comparing algebra OUTPUTS for equivalent already-normalized identities, so
// each row tags its selector as coverage metadata and reuses one shared
// identity/snapshot construction for both the shadow and live sides.
package reviewtransaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

var updateShadowMatrixGolden = flag.Bool("update", false, "update the shadow differential matrix golden file")

// shadowMatrixVerdict is the row verdict taxonomy (design.md Decision 6,
// tasks.md 6.2): exactly these four classes, never a fifth.
// no-shadow-decision is never collapsed into no-live-decision — a row where
// the shadow side could not produce ANY relation at all (a resolver-level
// failure) is a materially different, more dangerous class than a row where
// shadow legitimately computed ambiguous/unknown (design.md Decision 6's own
// rationale for keeping the two separate).
type shadowMatrixVerdict string

const (
	shadowMatrixAgreement        shadowMatrixVerdict = "agreement"
	shadowMatrixDivergence       shadowMatrixVerdict = "divergence"
	shadowMatrixNoLiveDecision   shadowMatrixVerdict = "no-live-decision"
	shadowMatrixNoShadowDecision shadowMatrixVerdict = "no-shadow-decision"
)

// shadowMatrixExpectedLive is the Shadow (7) -> Live (5) comparison map
// (design.md Decision 6) for the four relations that have a genuine live
// counterpart. ambiguous and unknown are handled separately
// (shadowRelationHasNoLiveCounterpart); unrelated has no live bucket at all
// and is handled as its own explained-divergence case below.
var shadowMatrixExpectedLive = map[ShadowRelation]compactTargetRelationKind{
	ShadowRelationExact:                 compactTargetSame,
	ShadowRelationCompatibleBaseAdvance: compactTargetCompatibleAdvance,
	ShadowRelationProvableContraction:   compactTargetProvableContraction,
	ShadowRelationChanged:               compactTargetChangedScope,
}

// shadowMatrixCase is one covering-array row's input.
type shadowMatrixCase struct {
	Name     string
	Selector shadowSelectorKind

	// ShadowUnavailable simulates a resolver-level failure that produces no
	// ShadowRelation at all (the "no-shadow-decision" bucket): the shadow
	// identity resolver could not even produce a candidate to feed
	// shadowRelate, unlike a legitimate ambiguous/unknown output.
	ShadowUnavailable bool
	ShadowInput       shadowRelationInput

	LiveFrozen       Snapshot
	LiveGenesisPaths []string
	LiveEvidence     compactTargetRelationEvidence

	// ExplainedReason, when non-empty, is recorded on a divergence row and
	// marks it Explained: true (tasks.md 6.3's EXPLAINED DIVERGENCE class).
	// Rows with no ExplainedReason that still diverge are unexplained.
	ExplainedReason string
}

// shadowMatrixRow is one golden-file row: the corpus input reduced to its
// comparison outcome. Field order is fixed for deterministic JSON.
type shadowMatrixRow struct {
	Name           string                    `json:"name"`
	Selector       shadowSelectorKind        `json:"selector"`
	ShadowRelation ShadowRelation            `json:"shadow_relation,omitempty"`
	LiveRelation   compactTargetRelationKind `json:"live_relation"`
	Verdict        shadowMatrixVerdict       `json:"verdict"`
	Explained      bool                      `json:"explained"`
	Reason         string                    `json:"reason,omitempty"`
}

// evaluateShadowMatrixCase runs both algebras on one case and classifies the
// row verdict. It is the differential matrix's entire comparison logic —
// everything else in this file only builds inputs.
func evaluateShadowMatrixCase(c shadowMatrixCase) shadowMatrixRow {
	row := shadowMatrixRow{Name: c.Name, Selector: c.Selector}
	row.LiveRelation = classifyCompactTargetRelation(c.LiveFrozen, c.ShadowInput.LiveSnapshot, c.LiveGenesisPaths, c.LiveEvidence).Kind

	if c.ShadowUnavailable {
		row.Verdict = shadowMatrixNoShadowDecision
		row.Reason = c.ExplainedReason
		return row
	}

	shadowResult := shadowRelate(c.ShadowInput)
	row.ShadowRelation = shadowResult

	if shadowRelationHasNoLiveCounterpart(shadowResult) {
		row.Verdict = shadowMatrixNoLiveDecision
		return row
	}

	if shadowResult == ShadowRelationUnrelated {
		row.Verdict = shadowMatrixDivergence
		row.Explained = true
		row.Reason = c.ExplainedReason
		return row
	}

	if expected, known := shadowMatrixExpectedLive[shadowResult]; known && expected == row.LiveRelation {
		row.Verdict = shadowMatrixAgreement
		return row
	}
	row.Verdict = shadowMatrixDivergence
	if c.ExplainedReason != "" {
		row.Explained = true
		row.Reason = c.ExplainedReason
	}
	return row
}

// shadowMatrixExitBarBlockers returns unexplained divergences on the three
// core relations spec.md's exit bar protects (exact, compatible_base_advance,
// provable_contraction). Explained divergences — including the shadow=unrelated
// vocabulary-gap class — are excluded by construction and never block; they
// are documented instead (tasks.md 6.3, 6.4).
func shadowMatrixExitBarBlockers(rows []shadowMatrixRow) []shadowMatrixRow {
	var blockers []shadowMatrixRow
	for _, row := range rows {
		if row.Verdict != shadowMatrixDivergence || row.Explained {
			continue
		}
		switch row.ShadowRelation {
		case ShadowRelationExact, ShadowRelationCompatibleBaseAdvance, ShadowRelationProvableContraction:
			blockers = append(blockers, row)
		}
	}
	return blockers
}

// --- Corpus construction ----------------------------------------------------

type shadowMatrixSelectorShape struct {
	Kind       TargetKind
	Projection Projection
}

var shadowMatrixSelectorShapes = map[shadowSelectorKind]shadowMatrixSelectorShape{
	shadowSelectorWorkspace:        {TargetCurrentChanges, ProjectionWorkspace},
	shadowSelectorStaged:           {TargetCurrentChanges, ProjectionStaged},
	shadowSelectorCommittedRange:   {TargetBaseDiff, ProjectionWorkspace},
	shadowSelectorWorkspaceOverlay: {TargetBaseWorkspaceOverlay, ProjectionWorkspace},
}

var shadowMatrixSelectors = []shadowSelectorKind{
	shadowSelectorWorkspace, shadowSelectorStaged, shadowSelectorCommittedRange, shadowSelectorWorkspaceOverlay,
}

// shadowMatrixUnknownFlavors rotates the three commit/push-state threat-matrix
// unknown triggers (design.md Threat Matrix; shadow_relation_test.go 3.5/3.6)
// across the four selectors so the base cross product exercises all three.
var shadowMatrixUnknownFlavors = []string{"unresolvable", "unborn-head", "empty-paths", "unresolvable"}

func shadowMatrixTree(seed string) string {
	sum := sha256.Sum256([]byte("gentle-ai.shadow-matrix-tree/v1\x00" + seed))
	return hex.EncodeToString(sum[:20]) // 40 lowercase hex chars: gitTreePattern's short form
}

func shadowMatrixDigest(seed string) string {
	sum := sha256.Sum256([]byte("gentle-ai.shadow-matrix-digest/v1\x00" + seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func shadowMatrixIdentity(name string, baseTree, candidateTree string) CandidateIdentity {
	return CandidateIdentity{
		RepositoryID: "repo/" + name, BaseTree: baseTree, CandidateTree: candidateTree,
		ChangedPathsModesDigest: shadowMatrixDigest(name + "/modes"), PolicyHash: "policy/" + name,
	}
}

// shadowMatrixExactCase: frozen and live are the identical identity — the
// literal spec.md scenario "identical candidate and policy resolve to exact".
func shadowMatrixExactCase(selector shadowSelectorKind) shadowMatrixCase {
	name := string(selector) + "/exact"
	shape := shadowMatrixSelectorShapes[selector]
	paths := []string{"pkg/" + name + "/file.go"}
	tree, candidate := shadowMatrixTree(name+"/base"), shadowMatrixTree(name+"/candidate")
	identity := shadowMatrixIdentity(name, tree, candidate)
	snapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: tree, CandidateTree: candidate, Paths: paths, PathsDigest: digestPaths(paths)}
	return shadowMatrixCase{
		Name: name, Selector: selector,
		ShadowInput: shadowRelationInput{Frozen: identity, Live: identity, LiveSnapshot: snapshot, ApplicableAuthorities: 1},
		LiveFrozen:  snapshot, LiveGenesisPaths: paths,
	}
}

// shadowMatrixCompatibleBaseAdvanceCase: base moved, candidate preserved, a
// valid delegated BaseAdvanceCompatibility proof binds the exact base pair —
// Amendment A's delegation seam (shadowDeriveBaseAdvance calls
// deriveBaseAdvanceCompatibility directly; here the already-derived proof is
// supplied, since deriving it for real requires a live git repo).
func shadowMatrixCompatibleBaseAdvanceCase(selector shadowSelectorKind) shadowMatrixCase {
	name := string(selector) + "/compatible-base-advance"
	shape := shadowMatrixSelectorShapes[selector]
	paths := []string{"pkg/" + name + "/file.go"}
	pathsDigest := digestPaths(paths)
	oldBase, newBase := shadowMatrixTree(name+"/old-base"), shadowMatrixTree(name+"/new-base")
	candidate := shadowMatrixTree(name + "/candidate")

	frozenSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: oldBase, CandidateTree: candidate, Paths: paths, PathsDigest: pathsDigest}
	liveSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: newBase, CandidateTree: candidate, Paths: paths, PathsDigest: pathsDigest}
	frozenIdentity := shadowMatrixIdentity(name+"/frozen", oldBase, candidate)
	liveIdentity := shadowMatrixIdentity(name+"/live", newBase, candidate)

	proof := &BaseAdvanceCompatibility{
		Status: currentChangesBoundaryCompatibleStatus, Compatible: true,
		OriginalMergeBaseTree: oldBase, NewBaseTree: newBase,
		OriginalPatchIdentity: shadowMatrixDigest(name + "/patch"), DeliveredPatchIdentity: shadowMatrixDigest(name + "/patch"),
		DeliveredPathsDigest: pathsDigest, BaseAdvancePathsDigest: shadowMatrixDigest(name + "/base-advance-paths"),
		PathsDisjoint: true, MergedResultTree: shadowMatrixTree(name + "/merged"), CIStatus: currentChangesBoundaryCIStatus,
	}

	return shadowMatrixCase{
		Name: name, Selector: selector,
		ShadowInput: shadowRelationInput{Frozen: frozenIdentity, Live: liveIdentity, LiveSnapshot: liveSnapshot, BaseAdvance: proof, ApplicableAuthorities: 1},
		LiveFrozen:  frozenSnapshot, LiveGenesisPaths: paths,
		LiveEvidence: compactTargetRelationEvidence{CompatibleAdvance: proof},
	}
}

// shadowMatrixProvableContractionCase: the live path set is a genuine, known
// subset of the frozen genesis paths, and admitted-finding evidence is known
// and does not reference any excluded path.
func shadowMatrixProvableContractionCase(selector shadowSelectorKind) shadowMatrixCase {
	name := string(selector) + "/provable-contraction"
	shape := shadowMatrixSelectorShapes[selector]
	genesisPaths := []string{"pkg/" + name + "/a.go", "pkg/" + name + "/b.go", "pkg/" + name + "/c.go"}
	livePaths := []string{"pkg/" + name + "/a.go"}
	base := shadowMatrixTree(name + "/base")
	frozenCandidate, liveCandidate := shadowMatrixTree(name+"/frozen-candidate"), shadowMatrixTree(name+"/live-candidate")

	frozenSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: base, CandidateTree: frozenCandidate, Paths: genesisPaths, PathsDigest: digestPaths(genesisPaths)}
	liveSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: base, CandidateTree: liveCandidate, Paths: livePaths, PathsDigest: digestPaths(livePaths)}

	return shadowMatrixCase{
		Name: name, Selector: selector,
		ShadowInput: shadowRelationInput{
			Frozen: shadowMatrixIdentity(name+"/frozen", base, frozenCandidate), Live: shadowMatrixIdentity(name+"/live", base, liveCandidate),
			LiveSnapshot: liveSnapshot, GenesisPaths: genesisPaths, AdmittedPathsKnown: true, ApplicableAuthorities: 1,
		},
		LiveFrozen: frozenSnapshot, LiveGenesisPaths: genesisPaths,
	}
}

// shadowMatrixAmendmentBDegradationCase: the same contraction shape as above,
// but AdmittedPathsKnown=false — Amendment B's no-input degradation. Shadow
// fail-closes to changed; live has no such concept and still reports
// provable-contraction over the identical path subset. This is a real,
// documented vocabulary gap (explained), not a fabricated agreement.
func shadowMatrixAmendmentBDegradationCase(selector shadowSelectorKind) shadowMatrixCase {
	name := string(selector) + "/changed-amendment-b-degradation"
	shape := shadowMatrixSelectorShapes[selector]
	genesisPaths := []string{"pkg/" + name + "/a.go", "pkg/" + name + "/b.go"}
	livePaths := []string{"pkg/" + name + "/a.go"}
	base := shadowMatrixTree(name + "/base")
	frozenCandidate, liveCandidate := shadowMatrixTree(name+"/frozen-candidate"), shadowMatrixTree(name+"/live-candidate")

	frozenSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: base, CandidateTree: frozenCandidate, Paths: genesisPaths, PathsDigest: digestPaths(genesisPaths)}
	liveSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: base, CandidateTree: liveCandidate, Paths: livePaths, PathsDigest: digestPaths(livePaths)}

	return shadowMatrixCase{
		Name: name, Selector: selector,
		ShadowInput: shadowRelationInput{
			Frozen: shadowMatrixIdentity(name+"/frozen", base, frozenCandidate), Live: shadowMatrixIdentity(name+"/live", base, liveCandidate),
			LiveSnapshot: liveSnapshot, GenesisPaths: genesisPaths, AdmittedPathsKnown: false, ApplicableAuthorities: 1,
		},
		LiveFrozen: frozenSnapshot, LiveGenesisPaths: genesisPaths,
		ExplainedReason: "Amendment B no-input degradation (design.md Decision 5): shadow has no admitted-finding evidence in this mode, so it fail-closes provable_contraction to changed; live has no such degradation concept and still reports provable-contraction for the identical path subset -- a documented vocabulary gap, not a fabricated agreement",
	}
}

// shadowMatrixChangedCase: an overlapping (not sub-set, not disjoint) path
// relation, both algebras independently reach "changed".
func shadowMatrixChangedCase(selector shadowSelectorKind) shadowMatrixCase {
	name := string(selector) + "/changed"
	shape := shadowMatrixSelectorShapes[selector]
	genesisPaths := []string{"pkg/" + name + "/a.go", "pkg/" + name + "/b.go"}
	livePaths := []string{"pkg/" + name + "/a.go", "pkg/" + name + "/z.go"}
	base := shadowMatrixTree(name + "/base")
	frozenCandidate, liveCandidate := shadowMatrixTree(name+"/frozen-candidate"), shadowMatrixTree(name+"/live-candidate")

	frozenSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: base, CandidateTree: frozenCandidate, Paths: genesisPaths, PathsDigest: digestPaths(genesisPaths)}
	liveSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: base, CandidateTree: liveCandidate, Paths: livePaths, PathsDigest: digestPaths(livePaths)}

	return shadowMatrixCase{
		Name: name, Selector: selector,
		ShadowInput: shadowRelationInput{
			Frozen: shadowMatrixIdentity(name+"/frozen", base, frozenCandidate), Live: shadowMatrixIdentity(name+"/live", base, liveCandidate),
			LiveSnapshot: liveSnapshot, GenesisPaths: genesisPaths, AdmittedPathsKnown: true, ApplicableAuthorities: 1,
		},
		LiveFrozen: frozenSnapshot, LiveGenesisPaths: genesisPaths,
	}
}

// shadowMatrixAmbiguousCase: ApplicableAuthorities > 1 forces ambiguous
// before any other check runs (design.md Decision 5's ordering).
func shadowMatrixAmbiguousCase(selector shadowSelectorKind) shadowMatrixCase {
	name := string(selector) + "/ambiguous"
	shape := shadowMatrixSelectorShapes[selector]
	paths := []string{"pkg/" + name + "/file.go"}
	tree, candidate := shadowMatrixTree(name+"/base"), shadowMatrixTree(name+"/candidate")
	identity := shadowMatrixIdentity(name, tree, candidate)
	snapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: tree, CandidateTree: candidate, Paths: paths, PathsDigest: digestPaths(paths)}
	return shadowMatrixCase{
		Name: name, Selector: selector,
		ShadowInput: shadowRelationInput{Frozen: identity, Live: identity, LiveSnapshot: snapshot, ApplicableAuthorities: 2},
		LiveFrozen:  snapshot, LiveGenesisPaths: paths,
	}
}

// shadowMatrixUnknownCase covers the three commit/push-state threat-matrix
// unknown triggers: an explicitly unresolvable boundary, an unborn HEAD, and
// an empty live change set. All three fail closed before the exact check.
func shadowMatrixUnknownCase(selector shadowSelectorKind, flavor string) shadowMatrixCase {
	name := string(selector) + "/unknown-" + flavor
	shape := shadowMatrixSelectorShapes[selector]
	paths := []string{"pkg/" + name + "/file.go"}
	tree, candidate := shadowMatrixTree(name+"/base"), shadowMatrixTree(name+"/candidate")
	liveTree, liveCandidate := shadowMatrixTree(name+"/live-base"), shadowMatrixTree(name+"/live-candidate")

	snapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: liveTree, CandidateTree: liveCandidate, Paths: paths, PathsDigest: digestPaths(paths)}
	input := shadowRelationInput{
		Frozen: shadowMatrixIdentity(name+"/frozen", tree, candidate), Live: shadowMatrixIdentity(name+"/live", liveTree, liveCandidate),
		LiveSnapshot: snapshot, ApplicableAuthorities: 1,
	}
	switch flavor {
	case "unresolvable":
		input.LiveUnresolvable = true
	case "unborn-head":
		input.LiveSnapshot.UnbornHead = true
	case "empty-paths":
		input.LiveSnapshot.Paths = nil
	}
	return shadowMatrixCase{Name: name, Selector: selector, ShadowInput: input, LiveFrozen: snapshot, LiveGenesisPaths: paths}
}

// shadowMatrixUnrelatedCase: no frozen authority exists at all (the literal
// spec.md scenario "no governing lineage resolves to unrelated"). Live has no
// "unrelated" bucket, so it independently reports either unsafe (frozen Kind
// incompatible/absent, the natural default) or changed-scope (an explicit
// scope-change recovery ceremony) for the identical live candidate. Both
// variants are EXPLAINED DIVERGENCE (tasks.md 6.3), never no-live-decision.
func shadowMatrixUnrelatedCase(selector shadowSelectorKind, liveUnsafe bool) shadowMatrixCase {
	shape := shadowMatrixSelectorShapes[selector]
	variant := "unsafe"
	if !liveUnsafe {
		variant = "changed-scope"
	}
	name := string(selector) + "/unrelated-vs-live-" + variant
	livePaths := []string{"pkg/" + name + "/file.go"}
	liveBase, liveCandidate := shadowMatrixTree(name+"/live-base"), shadowMatrixTree(name+"/live-candidate")
	liveIdentity := shadowMatrixIdentity(name+"/live", liveBase, liveCandidate)
	liveSnapshot := Snapshot{Kind: shape.Kind, Projection: shape.Projection, BaseTree: liveBase, CandidateTree: liveCandidate, Paths: livePaths, PathsDigest: digestPaths(livePaths)}

	var frozenSnapshot Snapshot
	explicitScopeChange := false
	if !liveUnsafe {
		frozenSnapshot = Snapshot{Kind: shape.Kind, Projection: shape.Projection}
		explicitScopeChange = true
	}

	return shadowMatrixCase{
		Name: name, Selector: selector,
		ShadowInput: shadowRelationInput{Frozen: CandidateIdentity{}, Live: liveIdentity, LiveSnapshot: liveSnapshot, ApplicableAuthorities: 1},
		LiveFrozen:  frozenSnapshot, LiveGenesisPaths: nil,
		LiveEvidence: compactTargetRelationEvidence{ExplicitScopeChange: explicitScopeChange},
		ExplainedReason: fmt.Sprintf(
			"design.md Decision 6: shadow has no live vocabulary bucket for unrelated (no governing frozen authority exists to compare); this row is recorded as an explained divergence, never absorbed into no-live-decision -- live independently reports %q for the same live candidate",
			variant),
	}
}

// shadowMatrixNoShadowDecisionCase simulates a resolver-level failure: the
// shadow identity resolver could not produce ANY candidate (an ambiguous or
// unresolvable selector, shadow_identity.go), so shadowRelate never runs at
// all. Live still reaches a real decision from its own snapshot pair —
// design.md Decision 6's "dangerous class": live decided, shadow did not.
func shadowMatrixNoShadowDecisionCase(selector shadowSelectorKind, reason string) shadowMatrixCase {
	name := string(selector) + "/no-shadow-decision"
	shape := shadowMatrixSelectorShapes[selector]
	paths := []string{"pkg/" + name + "/file.go"}
	frozenSnapshot := Snapshot{
		Kind: shape.Kind, Projection: shape.Projection, BaseTree: shadowMatrixTree(name + "/frozen-base"),
		CandidateTree: shadowMatrixTree(name + "/frozen-candidate"), Paths: paths, PathsDigest: digestPaths(paths),
	}
	liveSnapshot := Snapshot{
		Kind: shape.Kind, Projection: shape.Projection, BaseTree: shadowMatrixTree(name + "/live-base"),
		CandidateTree: shadowMatrixTree(name + "/live-candidate"), Paths: paths, PathsDigest: digestPaths(paths),
	}
	return shadowMatrixCase{
		Name: name, Selector: selector, ShadowUnavailable: true,
		ShadowInput: shadowRelationInput{LiveSnapshot: liveSnapshot},
		LiveFrozen:  frozenSnapshot, LiveGenesisPaths: paths,
		ExplainedReason: reason,
	}
}

// shadowMatrixCorpus is the covering array: all four selectors crossed with
// all seven shadow relations (28 rows), plus dimension-specific rows for
// Amendment B degradation, both unrelated-divergence variants, and
// no-shadow-decision. tasks.md 6.1 asks for ~40-60 rows; this corpus totals
// in that range without padding for its own sake.
func shadowMatrixCorpus() []shadowMatrixCase {
	var cases []shadowMatrixCase
	for i, selector := range shadowMatrixSelectors {
		cases = append(cases,
			shadowMatrixExactCase(selector),
			shadowMatrixCompatibleBaseAdvanceCase(selector),
			shadowMatrixProvableContractionCase(selector),
			shadowMatrixChangedCase(selector),
			shadowMatrixAmbiguousCase(selector),
			shadowMatrixUnknownCase(selector, shadowMatrixUnknownFlavors[i]),
			shadowMatrixUnrelatedCase(selector, i%2 == 0),
		)
	}
	// Amendment B degradation is a shadow-relation-algebra concern
	// (shadow_relation.go), not a selector concern -- exercised once per
	// selector so it is proven independent of how the candidate was resolved.
	for _, selector := range shadowMatrixSelectors {
		cases = append(cases, shadowMatrixAmendmentBDegradationCase(selector))
	}
	// The unrelated dimension's two live-side variants are already alternated
	// across all four selectors above (i%2); this pair fills in the variant
	// each selector did not exercise, so both variants get full 4-selector
	// coverage rather than a 2/2 split.
	for i, selector := range shadowMatrixSelectors {
		cases = append(cases, shadowMatrixUnrelatedCase(selector, i%2 != 0))
	}
	cases = append(cases,
		shadowMatrixNoShadowDecisionCase(shadowSelectorStaged,
			"shadow identity resolution failed before any relation could be computed: staged selector supplied both base_ref and ledger_ids, an ambiguity shadowSelectorAmbiguityReason detects at the selector layer before shadowRelate ever runs"),
		shadowMatrixNoShadowDecisionCase(shadowSelectorCommittedRange,
			"shadow identity resolution failed before any relation could be computed: committed-range selector's revision could not be resolved by SnapshotBuilder.Build (unresolvable Git revision)"),
		shadowMatrixNoShadowDecisionCase(shadowSelectorWorkspaceOverlay,
			"shadow identity resolution failed before any relation could be computed: workspace-overlay selector's base_ref could not be resolved by SnapshotBuilder.Build"),
		shadowMatrixNoShadowDecisionCase(shadowSelectorWorkspace,
			"shadow identity resolution failed before any relation could be computed: OpenRepositoryIdentityLease could not resolve a repository identity for the candidate repository"),
	)
	return cases
}

// --- Tests -------------------------------------------------------------------

// TestShadowMatrixCoveringArrayGolden is 6.1/6.2/6.5: the golden covering
// array. It must be clean -- zero unexplained divergences on the three
// exit-bar relations -- since the deliberately-injected unexplained
// divergence (6.4) is proven separately, never mixed into this golden.
func TestShadowMatrixCoveringArrayGolden(t *testing.T) {
	corpus := shadowMatrixCorpus()
	if len(corpus) < 40 || len(corpus) > 60 {
		t.Fatalf("corpus has %d rows, want 40-60 (tasks.md 6.1 covering array)", len(corpus))
	}

	rows := make([]shadowMatrixRow, len(corpus))
	for i, c := range corpus {
		rows[i] = evaluateShadowMatrixCase(c)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	counts := map[shadowMatrixVerdict]int{}
	explainedDivergences := 0
	for _, row := range rows {
		counts[row.Verdict]++
		if row.Verdict == shadowMatrixDivergence && row.Explained {
			explainedDivergences++
		}
		if row.Verdict == shadowMatrixDivergence && row.Reason == "" {
			t.Errorf("row %q: unexplained divergence in the golden corpus (shadow=%s live=%s) -- either the corpus construction is wrong or this belongs in the exit-bar test instead", row.Name, row.ShadowRelation, row.LiveRelation)
		}
	}
	t.Logf("rows: %d total = %d agreement + %d divergence (%d explained) + %d no-live-decision + %d no-shadow-decision",
		len(rows), counts[shadowMatrixAgreement], counts[shadowMatrixDivergence], explainedDivergences,
		counts[shadowMatrixNoLiveDecision], counts[shadowMatrixNoShadowDecision])

	if blockers := shadowMatrixExitBarBlockers(rows); len(blockers) != 0 {
		t.Fatalf("golden corpus has %d exit-bar blockers, want 0: %#v", len(blockers), blockers)
	}

	actual, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	actual = append(actual, '\n')

	goldenPath := filepath.Join("testdata", "shadow-differential-matrix.golden")
	if *updateShadowMatrixGolden {
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", goldenPath, err)
		}
		t.Logf("updated golden file: %s (%d rows)", goldenPath, len(rows))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("missing golden %s -- run: go test ./internal/reviewtransaction/... -run TestShadowMatrixCoveringArrayGolden -update (%v)", goldenPath, err)
	}
	if string(want) != string(actual) {
		t.Fatalf("golden mismatch for %s -- rerun with -update after inspecting the diff\nwant:\n%s\ngot:\n%s", goldenPath, want, actual)
	}
}

// TestShadowMatrixUnexplainedDivergenceOnCoreRelationBlocksWave2 is 6.4: it
// injects one unexplained divergence and asserts the exit bar reports it as
// blocking.
//
// This row used to exercise a REAL algebraic corner: shadowBaseAdvanceApplies
// (shadow_relation.go) bound a BaseAdvanceCompatibility proof to the
// Frozen/Live BaseTree pair only, while classifyCompactTargetRelation's own
// compatible-advance branch additionally requires the CandidateTree to be
// preserved -- a discovered gap now FIXED in shadow_relation.go (see
// TestShadowRelateBaseAdvanceProofBindsToCandidateTree, which pins the
// regression). With the fix in place the real corpus (shadowMatrixCorpus,
// exercised by TestShadowMatrixCoveringArrayGolden) has zero unexplained
// divergences on any exit-bar relation, so this test now injects a purely
// SYNTHETIC row instead: a live-side "frozen" snapshot deliberately built
// with a different BaseTree than the shadow-side identity it is paired
// with -- a shape no real gate call site could ever produce, since both
// sides always originate from the same resolved receipt. It exists solely
// to keep shadowMatrixExitBarBlockers proven end-to-end, not to model any
// real caller input.
func TestShadowMatrixUnexplainedDivergenceOnCoreRelationBlocksWave2(t *testing.T) {
	const name = "synthetic/exit-bar-mechanism-proof"
	base, candidate := shadowMatrixTree(name+"/base"), shadowMatrixTree(name+"/candidate")
	// mismatchedBase deliberately differs from base -- the synthetic part of
	// this row. A real caller's LiveFrozen always shares the same BaseTree
	// resolution as the shadow-side Frozen identity.
	mismatchedBase := shadowMatrixTree(name + "/synthetic-mismatched-base")
	paths := []string{"pkg/" + name + "/file.go"}
	pathsDigest := digestPaths(paths)

	identity := shadowMatrixIdentity(name, base, candidate)
	liveSnapshot := Snapshot{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, BaseTree: base, CandidateTree: candidate, Paths: paths, PathsDigest: pathsDigest}
	syntheticLiveFrozen := Snapshot{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, BaseTree: mismatchedBase, CandidateTree: candidate, Paths: paths, PathsDigest: pathsDigest}

	c := shadowMatrixCase{
		Name: name, Selector: shadowSelectorWorkspace,
		ShadowInput: shadowRelationInput{
			Frozen: identity, Live: identity, LiveSnapshot: liveSnapshot, ApplicableAuthorities: 1,
		},
		LiveFrozen: syntheticLiveFrozen, LiveGenesisPaths: paths,
		// No ExplainedReason: this divergence is deliberately left unexplained.
	}

	row := evaluateShadowMatrixCase(c)
	if row.ShadowRelation != ShadowRelationExact {
		t.Fatalf("shadow relation = %s, want exact (the identical Frozen/Live shadow identity)", row.ShadowRelation)
	}
	if row.LiveRelation == compactTargetSame {
		t.Fatalf("live relation = %s, want anything but same (the synthetic mismatched-BaseTree live-side input must not agree)", row.LiveRelation)
	}
	if row.Verdict != shadowMatrixDivergence || row.Explained {
		t.Fatalf("row = %#v, want an unexplained divergence", row)
	}

	blockers := shadowMatrixExitBarBlockers([]shadowMatrixRow{row})
	if len(blockers) != 1 || blockers[0].Name != name {
		t.Fatalf("shadowMatrixExitBarBlockers = %#v, want exactly this row reported as blocking Wave 2 (spec.md \"Unexplained Divergence Blocks Wave 2\")", blockers)
	}
}

// TestShadowMatrixRealCorpusHasZeroUnexplainedDivergences is the explicit
// counterpart to the synthetic exit-bar proof above: it asserts the REAL
// corpus (shadowMatrixCorpus, the same rows TestShadowMatrixCoveringArrayGolden
// checks against the golden file) has exactly zero exit-bar blockers now that
// shadowBaseAdvanceApplies's CandidateTree binding gap is fixed. This is
// asserted as its own standalone test (rather than only inline inside the
// golden test) so a future regression fails with an unambiguous name.
func TestShadowMatrixRealCorpusHasZeroUnexplainedDivergences(t *testing.T) {
	corpus := shadowMatrixCorpus()
	rows := make([]shadowMatrixRow, len(corpus))
	for i, c := range corpus {
		rows[i] = evaluateShadowMatrixCase(c)
	}
	if blockers := shadowMatrixExitBarBlockers(rows); len(blockers) != 0 {
		t.Fatalf("real corpus has %d exit-bar blockers, want 0: %#v", len(blockers), blockers)
	}
}
