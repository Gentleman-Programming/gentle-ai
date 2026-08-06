package sddstatus

import (
	"context"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// review_reverify.go is Wave 4 S6 (design.md's "Amendment (coordinator-
// resolved): targeted re-verify call site", 2026-08-03): targeted
// re-verify is a routing decision owned by internal/sddstatus's Resolve()/
// resolveEngramStatus(), mirroring the post-verify offer's own call-site
// resolution (decision 3's amendment) -- the routing surface owns
// integration, the orchestrator consumes it, RunSDDVerifyValidate stays
// context-free. It fires only when the change's governing receipt (S5c''s
// resolveGoverningReceiptRef path) records an applied review correction.
// Status.ReVerify itself stays purely additive/informational (the S6/cycle-3
// shape), the same non-invasive pattern Status.ReviewOffer already
// established.
//
// Wave 5 Phase 9 (design.md's "Amendment (corrective verify cycle 3):
// re-verify archive-gating deferred to Wave 5") reintroduces archive-gating
// enforcement, done differently this time: blockArchiveForUnsatisfiedReVerify
// anchors the demand to CompactCorrectionAttempt's own FixDeltaHash/
// Snapshot.CandidateTree -- written once, at CompleteCorrection time
// (compact.go), into an append-only slice -- never a live value re-derived
// from the current verify-report on every Resolve() the way cycle 3's
// removed attempt did (Wave 4 CRITICAL-A's livelock). Satisfaction is a
// structural check: does any passing native SDD runtime attempt's own
// FinishCandidateTree already equal that frozen candidateTree? An ordinary
// `sdd-attempt finish --outcome passed` run after the correction naturally
// captures that equality (FinishCandidateTree is simply the operator's
// current working tree at finish time) -- no new CLI flag, sub-operation, or
// top-level verb is needed; the runnable continuation named in the blocked
// reason is the plain, already-existing 8-base-flag finish shape. This is a
// disclosed deviation from tasks.md's Phase 9 pre-plan, which anticipated
// possibly needing the existing --remediates-evidence-revision trio: that
// trio's own validation (validateRuntimeRemediationSuccessor,
// runtime_ledger.go) demands an approved review successor lineage, a full
// review round trip -- a heavier, semantically distinct axis ("this failed
// runtime attempt's evidence is repaired by an approved successor binding")
// than "re-verify the corrected candidate," and reusing it would have
// repeated Wave 4's unrunnable-continuation defect at one remove.
//
// Data-source note (recorded explicitly, matching the amendment's own
// permission): the terminal CompactReceipt SDDReceiptRef points at carries
// no correction changed-path data at all -- only an opaque FixDeltaHash.
// Extending that schema is explicitly out of scope for this amendment. The
// real, already-existing source used instead is the full CompactState
// loaded from the same lineage's compact authoritative store (the same
// store resolveCompactRemediationAuthority's sibling code path already
// loads for remediation purposes):
// CorrectionAttempts[last].Snapshot.Paths. Whenever that is empty or
// absent, branch 7.2 (not reliably derivable) is what fires -- expected to
// be the common case in production until a future wave (Wave 5 or 7, per
// the amendment's Open Questions) revisits the receipt shape to carry
// correction-path data directly.
//
// "Verify evidence scope" is approximated by the compact authority's own
// GenesisPaths narrowed to the openspec/changes/<changeName>/ prefix -- the
// same prefix check compactAuthorityPathsBound (review_gate.go) already
// uses to prove a compact authority is bound to this SDD change. This is a
// deliberate, investigated choice, not an arbitrary one: reviewtransaction's
// own state machine validates every correction attempt's changed paths as a
// SUBSET of the full GenesisPaths (confirmed empirically while building
// this slice's own test fixture -- store.Replace refused a correction path
// outside GenesisPaths with "compact correction attempt is outside frozen
// scope"), so using the FULL GenesisPaths as the evidence-scope side would
// make branch 7.1's empty intersection structurally unreachable (correction
// paths ⊆ GenesisPaths always, so their intersection with GenesisPaths
// itself is never empty unless the correction paths are themselves empty --
// already branch 7.2's territory). Narrowing to the OpenSpec planning-
// artifact slice of GenesisPaths keeps a genuine, meaningful three-way
// split: a correction confined to ordinary source files (the common case)
// leaves nothing about WHAT verify checks changed -- empty intersection,
// branch 7.1; a correction that also touches specs/tasks/design/proposal
// under this change's own openspec path might have changed what verify
// needs to prove -- non-empty intersection, full re-verify. No already-
// exported path-diff primitive exists from RuntimeObjective's candidate-
// tree pair without new reviewtransaction plumbing, which is out of scope
// for this slice.

const (
	// ReVerifyModeTargeted names a cheap re-run of the objective's existing
	// evidence goal: the correction's changed paths did not intersect the
	// verify evidence scope, so nothing new needs proving -- the re-run
	// only refreshes the evidence-revision binding to the corrected
	// candidate.
	ReVerifyModeTargeted = "targeted"
	// ReVerifyModeFull names a full re-verify of the objective's evidence
	// goal: either the correction's changed-path set could not be reliably
	// derived, or it genuinely intersects the verify evidence scope, so the
	// full goal must be re-proven rather than assumed still valid.
	ReVerifyModeFull = "full"
)

// ReVerifyBlock is the orchestrator-facing routing decision task 8.6's
// prose instructs acting on: run sdd-verify with the stated Scope before
// archive.
//
// Corrective verify cycle 3 (CRITICAL-A) removed a short-lived
// EvidenceRevision field and its archive-blocking enforcement
// (blockArchiveForUnsatisfiedReVerify, cycle 3's own task 5 addition): the
// demanded revision was re-derived from the LIVE verify-report on every
// Resolve(), so a compliant re-verify re-labeled the demand instead of
// clearing it (a livelock), and the only existing write path capable of
// recording satisfaction (`sdd-attempt finish
// --remediates-evidence-revision`) requires `--expected-binding-revision`
// and `--successor-lineage` together -- a full review round trip, which
// defeats the "run a cheap targeted re-verify" scenario this block exists
// for. See design.md's "Amendment (corrective verify cycle 3): re-verify
// archive-gating deferred to Wave 5" and the matching spec amendment. This
// type stays purely additive/informational, the shape S6 originally shipped
// and cycle 3 restored -- Wave 5 Phase 9's replacement archive-gating
// enforcement (blockArchiveForUnsatisfiedReVerify, below) is a SEPARATE
// mechanism with its own frozen anchor, deliberately not reusing this
// block's fields at all (see this file's top-level doc comment).
type ReVerifyBlock struct {
	Mode   string   `json:"mode"`
	Scope  []string `json:"scope,omitempty"`
	Reason string   `json:"reason"`
}

// correctionEvidence is the intermediate shape deriveCorrectionEvidence
// produces from a loaded CompactState, isolated from classifyTargetedReVerify
// so each of tasks 7.1-7.3's branches is independently, genuinely testable
// with synthetic inputs regardless of what today's schema can supply in
// production.
type correctionEvidence struct {
	applied    bool
	paths      []string
	derivable  bool
	failClosed bool
	// fixDeltaHash and candidateTree (Wave 5 Phase 9) are the frozen archive-
	// gating anchor: CompactCorrectionAttempt's own FixDeltaHash and
	// Snapshot.CandidateTree, both written exactly once at CompleteCorrection
	// time (compact.go) into an append-only slice. Populated whenever applied
	// && !failClosed, regardless of derivable -- deriving them needs none of
	// the path data derivable gates on.
	fixDeltaHash  string
	candidateTree string
}

// deriveCorrectionEvidence inspects the compact authority's last recorded
// correction attempt, if any. failClosed reports task 7.3's empty-index/
// unborn-HEAD case: the correction's own captured commit state cannot be
// trusted for a path diff at all, so the caller must fail closed (emit no
// block) rather than guess a scope. derivable=false with failClosed=false
// reports task 7.2's case: a correction was recorded but carries no usable
// path data -- the schema-limited common case this amendment anticipates.
func deriveCorrectionEvidence(compact *reviewtransaction.CompactState) correctionEvidence {
	if compact == nil || len(compact.CorrectionAttempts) == 0 {
		return correctionEvidence{}
	}
	last := compact.CorrectionAttempts[len(compact.CorrectionAttempts)-1]
	if last.Snapshot.UnbornHead {
		return correctionEvidence{applied: true, failClosed: true}
	}
	if len(last.Snapshot.Paths) == 0 {
		return correctionEvidence{applied: true, fixDeltaHash: last.FixDeltaHash, candidateTree: last.Snapshot.CandidateTree}
	}
	return correctionEvidence{
		applied: true, derivable: true, paths: append([]string(nil), last.Snapshot.Paths...),
		fixDeltaHash: last.FixDeltaHash, candidateTree: last.Snapshot.CandidateTree,
	}
}

// intersectPaths returns the paths present in both sets, order-preserving
// on a, duplicate-free.
func intersectPaths(a, b []string) []string {
	inB := make(map[string]struct{}, len(b))
	for _, path := range b {
		inB[path] = struct{}{}
	}
	seen := make(map[string]struct{}, len(a))
	var overlap []string
	for _, path := range a {
		if _, ok := inB[path]; !ok {
			continue
		}
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}
		overlap = append(overlap, path)
	}
	return overlap
}

const (
	reVerifyNotDerivableReason      = "the correction's changed-path set is not reliably derivable from the governing compact authority; re-verifying the objective's full evidence goal"
	reVerifyEmptyIntersectionReason = "the correction's changed paths do not intersect the verify evidence scope; re-running the objective's evidence goal against the unaffected scope"
	reVerifyIntersectingReason      = "the correction's changed paths intersect the verify evidence scope; re-verifying the objective's full evidence goal"
)

// classifyTargetedReVerify implements tasks 7.1-7.3's three distinct
// branches as a pure function. emit reports whether a routing block should
// be surfaced at all: task 7.3's fail-closed case emits nothing (the
// pre-existing native runtime error routing already owns fail-closed
// reporting for unreadable commit state elsewhere; a routing block here
// would only duplicate or contradict it), and "no correction applied at
// all" is structural absence, the same guard pattern the offer block uses.
func classifyTargetedReVerify(evidence correctionEvidence, scope []string) (ReVerifyBlock, bool) {
	if !evidence.applied || evidence.failClosed {
		return ReVerifyBlock{}, false
	}
	if !evidence.derivable {
		return ReVerifyBlock{Mode: ReVerifyModeFull, Reason: reVerifyNotDerivableReason}, true
	}
	overlap := intersectPaths(evidence.paths, scope)
	if len(overlap) == 0 {
		return ReVerifyBlock{Mode: ReVerifyModeTargeted, Scope: append([]string(nil), scope...), Reason: reVerifyEmptyIntersectionReason}, true
	}
	return ReVerifyBlock{Mode: ReVerifyModeFull, Scope: overlap, Reason: reVerifyIntersectingReason}, true
}

// verifyEvidenceScope narrows the compact authority's GenesisPaths down to
// the OpenSpec planning-artifact paths (specs/tasks/design/proposal) that
// define what SDD's own verify checks -- see this file's top-level doc
// comment for the full investigation behind this choice.
func verifyEvidenceScope(genesisPaths []string, changeName string) []string {
	prefix := "openspec/changes/" + changeName + "/"
	var scope []string
	for _, path := range genesisPaths {
		if strings.HasPrefix(path, prefix) {
			scope = append(scope, path)
		}
	}
	return scope
}

// applyTargetedReVerifyRouting is the one call site (design.md's
// amendment): Resolve() and resolveEngramStatus() both call it
// symmetrically, exactly mirroring applyReviewOfferRouting's own shape.
// Status.ReVerify itself stays purely additive (corrective verify cycle 3,
// CRITICAL-A: deliberately restored to S6's original non-invasive shape --
// see ReVerifyBlock's doc comment). Wave 5 Phase 9 additionally calls
// blockArchiveForUnsatisfiedReVerify here, which DOES mutate
// Dependencies.Archive and BlockedReasons -- a separate mechanism with its
// own frozen anchor, not a reuse of ReVerifyBlock's fields (see this file's
// top-level doc comment). Both fire only in the same window the offer
// already requires -- SDD's own verify already passed -- since a correction
// with no completed SDD verify to potentially invalidate has nothing to
// route.
func applyTargetedReVerifyRouting(ctx context.Context, status *Status, repo, changeName string, governingRef *reviewtransaction.SDDReceiptRef, runtimeStatus *RuntimeStatus, reviewDisabled bool) {
	if reviewDisabled || governingRef == nil || status.Dependencies.Verify != DependencyAllDone {
		return
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, governingRef.Lineage)
	if err != nil {
		return
	}
	record, err := store.Load()
	if err != nil {
		return
	}
	evidence := deriveCorrectionEvidence(&record.State)
	scope := verifyEvidenceScope(record.State.GenesisPaths, changeName)
	block, emit := classifyTargetedReVerify(evidence, scope)
	if emit {
		status.ReVerify = &block
	}
	blockArchiveForUnsatisfiedReVerify(status, repo, changeName, runtimeStatus, evidence)
}

// archiveReVerifyDemanded reports whether a review correction was applied
// and its frozen anchor is trustworthy enough to demand fresh runtime
// evidence for -- the same structural-absence guard classifyTargetedReVerify
// uses (no correction at all, or the fail-closed unborn-HEAD branch, name
// nothing).
func archiveReVerifyDemanded(evidence correctionEvidence) bool {
	return evidence.applied && !evidence.failClosed
}

// archiveReVerifySatisfied reports whether any recorded native SDD runtime
// attempt already passed against the corrected candidate tree. This is a
// structural check, not a flag an operator sets: an ordinary
// `sdd-attempt finish --outcome passed` run after the correction naturally
// captures FinishCandidateTree equal to the corrected tree, because that is
// simply the operator's current working tree at finish time.
func archiveReVerifySatisfied(evidence correctionEvidence, attempts []RuntimeAttempt) bool {
	for _, attempt := range attempts {
		if attempt.Outcome == AttemptPassed && attempt.FinishCandidateTree == evidence.candidateTree {
			return true
		}
	}
	return false
}

// archiveReVerifyContinuation names the exact, literally-runnable
// `gentle-ai sdd-attempt` invocation that satisfies the demand: the plain,
// already-existing 8-base-flag finish shape (missingSDDAttemptFlags's
// "finish" case, internal/cli/sdd_attempt.go) -- no new flag, sub-operation,
// or top-level verb. When no attempt is currently active, `begin` is named
// first, since `finish` alone would be refused with
// ErrRuntimeNoActiveAttempt. Values the caller cannot know ahead of time
// (the operator's own idempotency token, diagnosis prose, etc.) are named as
// `<placeholder>` text, the same convention runtimeRemediationExitRefusal
// already established for this CLI surface.
func archiveReVerifyContinuation(workspaceRoot, changeName string, runtimeStatus RuntimeStatus) string {
	finish := fmt.Sprintf(
		"gentle-ai sdd-attempt finish --cwd %s --change %q --expected-revision %s --request-id \"<unique-request-id>\" --outcome passed --evidence-revision \"<fresh-evidence-sha256>\" --diagnosis \"<proven-diagnosis>\" --harness-disposition <reused|invalidated> --cleanup-evidence \"<cleanup-evidence>\" --process-evidence \"<process-evidence>\"",
		pathquote.Quote(workspaceRoot), changeName, runtimeStatus.Revision,
	)
	if runtimeStatus.ActiveAttempt != nil {
		return finish
	}
	begin := fmt.Sprintf(
		"gentle-ai sdd-attempt begin --cwd %s --change %q --expected-revision %s --request-id \"<unique-request-id>\" --work-unit \"<work-unit>\" --evidence-goal \"<evidence-goal>\" --max-attempts <n> --max-changed-lines <n>",
		pathquote.Quote(workspaceRoot), changeName, runtimeStatus.Revision,
	)
	return begin + ", then (with the new revision it returns) " + finish
}

// blockArchiveForUnsatisfiedReVerify is Wave 5 Phase 9's replacement for the
// livelocking archive-gating enforcement corrective verify cycle 3 removed
// (see this file's top-level doc comment and ReVerifyBlock's doc comment for
// the full Wave 4 CRITICAL-A post-mortem and the frozen-anchor fix). It
// blocks Dependencies.Archive only -- never Apply, Verify, or any of the
// five delivery gates commit/push/pr/release own (a wholly separate domain,
// internal/reviewtransaction's gate.go/compact_gate.go, S1-S7's territory,
// not this file's) -- and only when a correction was actually applied and
// its frozen anchor is not yet satisfied by any passing native runtime
// attempt. nil runtimeStatus (no native runtime authority resolved this
// Resolve()) is structural absence: nothing to demand fresh evidence from.
func blockArchiveForUnsatisfiedReVerify(status *Status, workspaceRoot, changeName string, runtimeStatus *RuntimeStatus, evidence correctionEvidence) {
	if runtimeStatus == nil || !archiveReVerifyDemanded(evidence) {
		return
	}
	if archiveReVerifySatisfied(evidence, runtimeStatus.Attempts) {
		return
	}
	status.Dependencies.Archive = DependencyBlocked
	status.BlockedReasons = append(status.BlockedReasons, fmt.Sprintf(
		"a review correction (fix_delta_hash %s) changed the candidate; archive is blocked until a passing native SDD runtime attempt records evidence against the corrected candidate -- run `%s`",
		evidence.fixDeltaHash, archiveReVerifyContinuation(workspaceRoot, changeName, *runtimeStatus),
	))
}
