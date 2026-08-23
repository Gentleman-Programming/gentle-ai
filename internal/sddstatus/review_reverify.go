package sddstatus

import (
	"context"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Targeted re-verification describes the evidence scope affected by a review
// correction. The status field is advisory: it never blocks archive readiness
// or redirects delivery away from ordinary repository policy.

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

// ReVerifyBlock provides advisory review-correction context to the
// orchestrator. SDD requirements, tasks, and verification—not this field—
// determine archive readiness.
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
		return correctionEvidence{applied: true}
	}
	return correctionEvidence{
		applied: true, derivable: true, paths: append([]string(nil), last.Snapshot.Paths...),
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
// branches as a pure function. emit reports whether advisory context should
// be surfaced at all: task 7.3's fail-closed case emits nothing, and no
// correction applied is structural absence.
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

// applyTargetedReVerifyRouting projects advisory correction context after
// SDD verification passes. It does not mutate dependencies or routing.
func applyTargetedReVerifyRouting(ctx context.Context, status *Status, repo, changeName string, governingRef *reviewtransaction.SDDReceiptRef, reviewDisabled bool) {
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
}
