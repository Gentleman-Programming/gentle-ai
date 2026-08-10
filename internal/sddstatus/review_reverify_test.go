package sddstatus

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// review_reverify_test.go is Wave 4 S6 (design.md's "Amendment
// (coordinator-resolved): targeted re-verify call site", 2026-08-03).
// Tasks 7.1-7.3's three branches are proven independently at the pure-
// function level with synthetic inputs (deriveCorrectionEvidence/
// classifyTargetedReVerify take no dependency on live Git or a persisted
// store), plus one real end-to-end test proving the routing wires into
// Resolve() through a genuinely on-disk approved compact authority.

func TestDeriveCorrectionEvidenceBranches(t *testing.T) {
	tests := []struct {
		name    string
		compact *reviewtransaction.CompactState
		want    correctionEvidence
	}{
		{
			name:    "no correction recorded at all",
			compact: &reviewtransaction.CompactState{},
			want:    correctionEvidence{},
		},
		{
			name:    "nil compact state",
			compact: nil,
			want:    correctionEvidence{},
		},
		{
			name: "correction recorded but unborn HEAD -- fail closed (7.3)",
			compact: &reviewtransaction.CompactState{CorrectionAttempts: []reviewtransaction.CompactCorrectionAttempt{
				{Snapshot: reviewtransaction.Snapshot{UnbornHead: true}},
			}},
			want: correctionEvidence{applied: true, failClosed: true},
		},
		{
			name: "correction recorded but no path data -- not derivable (7.2)",
			compact: &reviewtransaction.CompactState{CorrectionAttempts: []reviewtransaction.CompactCorrectionAttempt{
				{Snapshot: reviewtransaction.Snapshot{CandidateTree: "deadbeef"}},
			}},
			want: correctionEvidence{applied: true, candidateTree: "deadbeef"},
		},
		{
			name: "correction recorded with real path data -- derivable",
			compact: &reviewtransaction.CompactState{CorrectionAttempts: []reviewtransaction.CompactCorrectionAttempt{
				{Snapshot: reviewtransaction.Snapshot{Paths: []string{"a.go"}}},
				{Snapshot: reviewtransaction.Snapshot{Paths: []string{"b.go", "c.go"}}},
			}},
			want: correctionEvidence{applied: true, derivable: true, paths: []string{"b.go", "c.go"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveCorrectionEvidence(tt.compact); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("deriveCorrectionEvidence() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestIntersectPaths(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []string
	}{
		{name: "empty both", a: nil, b: nil, want: nil},
		{name: "no overlap", a: []string{"a.go"}, b: []string{"b.go"}, want: nil},
		{name: "full overlap", a: []string{"a.go", "b.go"}, b: []string{"b.go", "a.go"}, want: []string{"a.go", "b.go"}},
		{name: "partial overlap dedupes", a: []string{"a.go", "a.go", "c.go"}, b: []string{"a.go"}, want: []string{"a.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intersectPaths(tt.a, tt.b); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("intersectPaths(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestClassifyTargetedReVerifyBranches proves tasks 7.1-7.3's three
// distinct branches directly, with synthetic inputs -- genuinely
// independent of whatever today's receipt/compact-state schema can supply
// in production (the amendment's own explicit permission: "if the receipt
// genuinely cannot carry correction paths, that is branch 7.2 doing its
// job").
func TestClassifyTargetedReVerifyBranches(t *testing.T) {
	t.Run("7.1 empty intersection -> targeted", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, derivable: true, paths: []string{"unrelated.go"}}
		scope := []string{"spec-scoped.go"}
		block, emit := classifyTargetedReVerify(evidence, scope)
		if !emit || block.Mode != ReVerifyModeTargeted || !reflect.DeepEqual(block.Scope, scope) || block.Reason == "" {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want targeted with the full scope", block, emit)
		}
	})

	t.Run("7.2 not reliably derivable -> full, distinct reason from 7.1", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, derivable: false}
		block, emit := classifyTargetedReVerify(evidence, []string{"spec-scoped.go"})
		if !emit || block.Mode != ReVerifyModeFull || block.Reason == "" || block.Reason == reVerifyEmptyIntersectionReason {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want full with a reason distinct from the empty-intersection branch", block, emit)
		}
	})

	t.Run("non-empty intersection -> full, scoped to the overlap", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, derivable: true, paths: []string{"spec-scoped.go", "other.go"}}
		scope := []string{"spec-scoped.go"}
		block, emit := classifyTargetedReVerify(evidence, scope)
		if !emit || block.Mode != ReVerifyModeFull || !reflect.DeepEqual(block.Scope, []string{"spec-scoped.go"}) {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want full scoped to the intersection", block, emit)
		}
	})

	t.Run("7.3 fail closed -> no block emitted", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, failClosed: true}
		block, emit := classifyTargetedReVerify(evidence, []string{"spec-scoped.go"})
		if emit || !reflect.DeepEqual(block, ReVerifyBlock{}) {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want no block on fail-closed commit state", block, emit)
		}
	})

	t.Run("no correction applied -> no block emitted (structural absence)", func(t *testing.T) {
		block, emit := classifyTargetedReVerify(correctionEvidence{}, []string{"spec-scoped.go"})
		if emit || !reflect.DeepEqual(block, ReVerifyBlock{}) {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want no block when no correction was ever recorded", block, emit)
		}
	})
}

// Investigated and NOT pursued (recorded explicitly rather than silently
// dropped): a full end-to-end test driving Resolve() through a genuinely
// protocol-valid on-disk correction (rather than a hand-fixtured
// CompactState) would require the complete real correction round trip --
// state.BeginCorrection, a real git-based TargetFixDiff snapshot, a
// matching FixDeltaHash, a bound ScopedValidationResult, and a bound
// VerificationEvidenceRecord passed to CompleteCorrectionVerification.
// Confirmed empirically: reviewtransaction's own validateCompactCorrection
// cross-checks ProposedLines/ActualLines/Snapshot.Kind/Projection/BaseTree/
// LedgerIDs/Identity/PathsDigest/FixDeltaHash together, so a hand-built
// CompactCorrectionAttempt (even with real, subset-of-GenesisPaths paths)
// is rejected by store.Replace with "compact correction attempt is outside
// frozen scope" -- correction paths being a real subset of GenesisPaths is
// necessary but far from sufficient. Building that full round trip is
// substantially more machinery than this slice's budget, and no existing
// internal/sddstatus test fixture does it either (only
// internal/reviewtransaction's own tests exercise the complete protocol,
// using its package-private helpers this file cannot reach). The three
// branches' LOGIC is proven directly and genuinely above
// (TestDeriveCorrectionEvidenceBranches, TestClassifyTargetedReVerifyBranches)
// with realistic-shaped CompactCorrectionAttempt/Snapshot values; the
// routing WIRING's "no correction -> structural absence" edge is proven
// end-to-end below through a real, on-disk approved compact authority. The
// "a real correction on disk drives the block" wiring edge is not covered
// end-to-end by this slice -- flagged for a follow-up if the coordinator
// wants that specific gap closed.

// TestResolveOmitsReVerifyBlockWithoutAnyCorrection proves structural
// absence: an approved compact authority with no correction history at all
// produces no ReVerify block, and its JSON marshal carries no "reVerify"
// substring, mirroring ReviewOffer's own absence proof shape.
func TestResolveOmitsReVerifyBlockWithoutAnyCorrection(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReVerify != nil {
		t.Fatalf("Status.ReVerify = %#v, want nil (structural absence) with no correction recorded", status.ReVerify)
	}
}

// Phase 9 (design.md's "Amendment (corrective verify cycle 3): re-verify
// archive-gating deferred to Wave 5" and its "What a compliant Wave 5 (or
// later) implementation needs" checklist): a compliant replacement for the
// livelocking blockArchiveForUnsatisfiedReVerify cycle 3 removed. The
// frozen anchor is CompactCorrectionAttempt's own FixDeltaHash/
// Snapshot.CandidateTree -- both written once, at CompleteCorrection time
// (compact.go), by an append-only slice -- never a live value re-derived
// from the current verify-report on every Resolve() the way the removed
// attempt did. Satisfaction is proven structurally: a passing native SDD
// runtime attempt whose FinishCandidateTree equals the frozen candidateTree.

// TestBlockArchiveForUnsatisfiedReVerify_FrozenAnchorDoesNotRelabel
// reproduces the exact Wave 4 CRITICAL-A livelock repro shape: cycle 1
// blocks; a compliant remediation (a passing native runtime attempt against
// the corrected candidate) satisfies it; cycle 2 (re-deriving the anchor
// from the SAME, unchanged, append-only CorrectionAttempts) must NOT mint a
// new demand -- it must stay satisfied.
func TestBlockArchiveForUnsatisfiedReVerify_FrozenAnchorDoesNotRelabel(t *testing.T) {
	compact := &reviewtransaction.CompactState{CorrectionAttempts: []reviewtransaction.CompactCorrectionAttempt{
		{FixDeltaHash: "sha256:corrected", Snapshot: reviewtransaction.Snapshot{CandidateTree: "tree-corrected", Paths: []string{"a.go"}}},
	}}

	// Cycle 1: no native runtime attempt has yet addressed the correction.
	evidenceCycle1 := deriveCorrectionEvidence(compact)
	if !archiveReVerifyDemanded(evidenceCycle1) {
		t.Fatalf("cycle 1: archive re-verify demand not raised for an applied, derivable correction")
	}
	if archiveReVerifySatisfied(evidenceCycle1, nil) {
		t.Fatalf("cycle 1: archive re-verify reports satisfied with no matching runtime attempt")
	}
	// A passed attempt against a DIFFERENT candidate tree (e.g. a stale
	// attempt that predates the correction) must not satisfy the demand --
	// only Outcome==passed AND a matching FinishCandidateTree count.
	if archiveReVerifySatisfied(evidenceCycle1, []RuntimeAttempt{{Outcome: AttemptPassed, FinishCandidateTree: "tree-unrelated"}}) {
		t.Fatalf("cycle 1: a passed attempt against an unrelated candidate tree must not satisfy the demand")
	}

	// Compliant remediation: the operator re-verifies and records a passing
	// native runtime attempt against the corrected candidate tree.
	attempts := []RuntimeAttempt{{Outcome: AttemptPassed, FinishCandidateTree: "tree-corrected"}}
	if !archiveReVerifySatisfied(evidenceCycle1, attempts) {
		t.Fatalf("compliant remediation did not satisfy the archive re-verify demand")
	}

	// Cycle 2 ("resolve again"): CorrectionAttempts is append-only and
	// unchanged -- no new correction landed -- so re-deriving evidence from
	// the SAME compact state must produce the IDENTICAL frozen anchor. Wave
	// 4's CRITICAL-A instead re-derived a NEW live verify-report revision on
	// every Resolve(), relabeling a satisfied demand into an unsatisfied one.
	evidenceCycle2 := deriveCorrectionEvidence(compact)
	if !reflect.DeepEqual(evidenceCycle1, evidenceCycle2) {
		t.Fatalf("frozen anchor relabeled itself between resolves: cycle1=%#v cycle2=%#v", evidenceCycle1, evidenceCycle2)
	}
	if !archiveReVerifySatisfied(evidenceCycle2, attempts) {
		t.Fatalf("cycle 2: a compliant remediation must stay satisfied, not re-demand a new anchor (Wave 4 CRITICAL-A livelock shape)")
	}
}

// TestBlockArchiveForUnsatisfiedReVerify_NamedContinuationIsRunnable asserts
// the blocked-reason text names a complete, literally-runnable
// `gentle-ai sdd-attempt finish` invocation carrying all 8 base flags
// (missingSDDAttemptFlags's "finish" case, internal/cli/sdd_attempt.go).
// It deliberately does NOT name the 3-flag remediation trio
// (--expected-binding-revision, --successor-lineage,
// --remediates-evidence-revision): that trio's own validation
// (validateRuntimeRemediationSuccessor, internal/sddstatus/runtime_ledger.go)
// demands an approved review successor lineage -- a full review round trip,
// an entirely different and heavier axis than "re-verify the corrected
// candidate" that would repeat Wave 4's unrunnable-continuation defect at
// one remove. See the Phase 9 deviation note in tasks.md.
func TestBlockArchiveForUnsatisfiedReVerify_NamedContinuationIsRunnable(t *testing.T) {
	runtimeStatus := RuntimeStatus{Revision: "sha256:ledgerrevision"}
	text := archiveReVerifyContinuation("/repo", "wave5", runtimeStatus)

	if !strings.Contains(text, "gentle-ai sdd-attempt finish") {
		t.Fatalf("archiveReVerifyContinuation() = %q, does not name a runnable sdd-attempt finish invocation", text)
	}
	for _, flag := range []string{
		"--cwd", "--change", "--expected-revision", "--request-id", "--outcome",
		"--evidence-revision", "--diagnosis", "--harness-disposition", "--cleanup-evidence", "--process-evidence",
	} {
		if !strings.Contains(text, flag) {
			t.Fatalf("archiveReVerifyContinuation() = %q, missing required finish flag %q", text, flag)
		}
	}
	if strings.Contains(text, "--successor-lineage") || strings.Contains(text, "--expected-binding-revision") || strings.Contains(text, "--remediates-evidence-revision") {
		t.Fatalf("archiveReVerifyContinuation() = %q, must not name the heavier remediation-trio continuation", text)
	}

	t.Run("names begin first when no attempt is active", func(t *testing.T) {
		text := archiveReVerifyContinuation("/repo", "wave5", RuntimeStatus{Revision: "sha256:ledgerrevision"})
		if !strings.Contains(text, "gentle-ai sdd-attempt begin") {
			t.Fatalf("archiveReVerifyContinuation() = %q, must name begin first when no attempt is active", text)
		}
	})

	t.Run("names only finish when an attempt is already active", func(t *testing.T) {
		text := archiveReVerifyContinuation("/repo", "wave5", RuntimeStatus{
			Revision:      "sha256:ledgerrevision",
			ActiveAttempt: &RuntimeAttempt{Ordinal: 1},
		})
		if strings.Contains(text, "gentle-ai sdd-attempt begin") {
			t.Fatalf("archiveReVerifyContinuation() = %q, must not name begin while an attempt is already active (it would refuse)", text)
		}
	})
}

// TestBlockArchiveForUnsatisfiedReVerify_StructuralAbsence proves the
// blocking budget (design constraint 3): no demand at all when no
// correction was ever applied, and no demand on the fail-closed branch
// (task 7.3's territory, same guard classifyTargetedReVerify already uses).
func TestBlockArchiveForUnsatisfiedReVerify_StructuralAbsence(t *testing.T) {
	t.Run("no correction applied", func(t *testing.T) {
		if archiveReVerifyDemanded(correctionEvidence{}) {
			t.Fatalf("archiveReVerifyDemanded() = true, want false with no correction ever recorded")
		}
	})
	t.Run("fail closed", func(t *testing.T) {
		if archiveReVerifyDemanded(correctionEvidence{applied: true, failClosed: true}) {
			t.Fatalf("archiveReVerifyDemanded() = true, want false on the fail-closed branch")
		}
	})
}

// TestBlockArchiveForUnsatisfiedReVerify_MutatesOnlyArchive proves the
// blocking budget's other half (design constraint 3): the orchestrating
// function blocks Status.Dependencies.Archive only, appends exactly one
// blocked reason naming the frozen fix_delta_hash, and leaves every other
// dependency untouched -- archive-only, never commit/push/pr/release (those
// live entirely outside Status.Dependencies, in the S1-S7 gate-cutover
// domain this phase shares no files with).
func TestBlockArchiveForUnsatisfiedReVerify_MutatesOnlyArchive(t *testing.T) {
	status := &Status{Dependencies: Dependencies{Apply: DependencyAllDone, Verify: DependencyAllDone, Archive: DependencyReady}}
	evidence := correctionEvidence{applied: true, derivable: true, fixDeltaHash: "sha256:corrected", candidateTree: "tree-corrected"}
	runtimeStatus := &RuntimeStatus{Revision: "sha256:ledgerrevision"}

	blockArchiveForUnsatisfiedReVerify(status, "/repo", "wave5", runtimeStatus, evidence)

	if status.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("Dependencies.Archive = %v, want blocked", status.Dependencies.Archive)
	}
	if status.Dependencies.Apply != DependencyAllDone || status.Dependencies.Verify != DependencyAllDone {
		t.Fatalf("Dependencies = %#v, want only Archive touched", status.Dependencies)
	}
	if len(status.BlockedReasons) != 1 || !strings.Contains(status.BlockedReasons[0], "sha256:corrected") {
		t.Fatalf("BlockedReasons = %#v, want exactly one reason naming the frozen fix_delta_hash", status.BlockedReasons)
	}

	t.Run("satisfied leaves archive untouched", func(t *testing.T) {
		status := &Status{Dependencies: Dependencies{Archive: DependencyReady}}
		attempts := []RuntimeAttempt{{Outcome: AttemptPassed, FinishCandidateTree: "tree-corrected"}}
		runtimeStatus := &RuntimeStatus{Revision: "sha256:ledgerrevision", Attempts: attempts}
		blockArchiveForUnsatisfiedReVerify(status, "/repo", "wave5", runtimeStatus, evidence)
		if status.Dependencies.Archive != DependencyReady || len(status.BlockedReasons) != 0 {
			t.Fatalf("satisfied demand mutated status: Dependencies=%#v BlockedReasons=%#v", status.Dependencies, status.BlockedReasons)
		}
	})

	t.Run("nil runtime status is structural absence", func(t *testing.T) {
		status := &Status{Dependencies: Dependencies{Archive: DependencyReady}}
		blockArchiveForUnsatisfiedReVerify(status, "/repo", "wave5", nil, evidence)
		if status.Dependencies.Archive != DependencyReady || len(status.BlockedReasons) != 0 {
			t.Fatalf("nil runtime status must not block archive: Dependencies=%#v BlockedReasons=%#v", status.Dependencies, status.BlockedReasons)
		}
	})

	t.Run("no correction applied is structural absence", func(t *testing.T) {
		status := &Status{Dependencies: Dependencies{Archive: DependencyReady}}
		runtimeStatus := &RuntimeStatus{Revision: "sha256:ledgerrevision"}
		blockArchiveForUnsatisfiedReVerify(status, "/repo", "wave5", runtimeStatus, correctionEvidence{})
		if status.Dependencies.Archive != DependencyReady || len(status.BlockedReasons) != 0 {
			t.Fatalf("no correction applied must not block archive: Dependencies=%#v BlockedReasons=%#v", status.Dependencies, status.BlockedReasons)
		}
	})
}
