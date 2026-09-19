package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewCorrectionContextBudgetCode is the correction-stage sibling of
// lens_context_budget_exceeded, and it is deliberately NOT the same code.
// The lens code's continuation tells an operator that nothing was created and
// nothing has to be released; at the correction stage the opposite is true.
// Review authority exists, lens results are already admitted, so
// compactPristineReviewing is false and `review invalidate` refuses. The one
// exit that still accepts this state is `review abandon`
// (compactAbandonTerminalState refuses only approved, escalated and
// invalidated), and an operator who is handed the lens continuation here is
// told to do the one thing that cannot work.
const reviewCorrectionContextBudgetCode = "correction_context_budget_exceeded"

// reviewCorrectionContextBudgetReason classifies the correction-stage refusal
// for a caller that branches on codes rather than prose. `next_action` is
// "stop" for the same reason START's budget reason uses it: no flag, lineage,
// or projection makes an over-budget targeted validation assemble, so
// "correct the request you sent" would be an actively wrong instruction. The
// exact runnable release travels with the cause, rendered against the live
// eligibility probe.
var reviewCorrectionContextBudgetReason = reviewPreflightReason{
	Code:       reviewCorrectionContextBudgetCode,
	Message:    "The correction evidence and its recorded findings exceed the native context budget, so the targeted validation run cannot be assembled; this review authority has to be released instead of retried.",
	NextAction: "stop",
}

// reviewProviderCaptureBudgetReason classifies the same deterministic budget
// refusal reached through the transaction-wide refuter capture. The refuter
// runs while collection is still open, so its code stays the lens one: what
// changes is only that the classification now survives to the caller instead
// of being flattened to the generic "correct the request you sent" shape by
// reviewPreflightError.
var reviewProviderCaptureBudgetReason = reviewPreflightReason{
	Code:       "lens_context_budget_exceeded",
	Message:    "The provider role request exceeds the native context budget and is never truncated, so retrying this candidate cannot succeed.",
	NextAction: "stop",
}

// reviewCorrectionContextBudgetProbe assembles the REAL targeted-validator
// request for the current state and reports whether it fits.
//
// It reuses reviewProviderNewTargetedValidatorRequest rather than re-deriving
// the assembly, because the whole point of the #4680 shape is that what
// exceeds the budget is not the candidate START measured: it is the corrected
// snapshot's materialized evidence plus ValidationRequest.FixFindings and
// FixClassifications, whose Claim, ProofRefs and Proof are unbounded free
// text a START-time probe cannot know. Only the real assembly measures that.
//
// It is read-only: reviewProviderNewTargetedValidatorRequest builds in memory
// and opens no authority store for writing, and the probe discards whatever
// it produced.
//
// The tri-state is load-bearing. An assembly that fails for an unrelated
// reason -- an unreachable tree, an expired deadline, a correction that is no
// longer open -- is UNPROVEN, never over budget: reporting a stop from an
// inconclusive probe would strand a lineage whose validation was fine.
func reviewCorrectionContextBudgetProbe(
	ctx context.Context, repo string, state reviewtransaction.CompactState, revision string,
) reviewLensContextProbeOutcome {
	correction, err := reviewProviderTargetedValidatorCorrection(ctx, repo, state)
	if err != nil {
		return reviewLensContextUnproven
	}
	if _, err := reviewProviderNewTargetedValidatorRequest(ctx, repo, state, revision, correction); err != nil {
		if reviewIsLensContextBudgetRefusal(err) {
			return reviewLensContextOverBudget
		}
		return reviewLensContextUnproven
	}
	return reviewLensContextRepresentable
}

// reviewCorrectionContextBudgetExhausted is the STATUS-side predicate, shaped
// exactly like reviewLensContextStatusBudgetExhausted: only the deterministic
// over-budget verdict stops STATUS, and an unproven probe changes nothing.
func reviewCorrectionContextBudgetExhausted(ctx context.Context, repo string, state reviewtransaction.CompactState, revision string) bool {
	return reviewCorrectionContextBudgetProbe(ctx, repo, state, revision) == reviewLensContextOverBudget
}

// reviewIsLensContextBudgetRefusal reports whether err carries the typed
// deterministic budget refusal, wrapped however many times.
func reviewIsLensContextBudgetRefusal(err error) bool {
	var refusal *reviewLensContextError
	return errors.As(err, &refusal) && refusal.Code == "lens_context_budget_exceeded"
}

// reviewCorrectionContextBudgetRefusal is the capture-time counterpart of the
// STATUS stop: `review capture-validation` used to flatten this typed refusal
// through reviewPreflightError, so the operator got prose with no code to
// branch on while STATUS kept reoffering targeted validation. Here the code
// survives, and the cause names the concrete release instead of leaving the
// operator to look its values up.
func reviewCorrectionContextBudgetRefusal(ctx context.Context, repo, lineage string, err error) error {
	if !reviewIsLensContextBudgetRefusal(err) {
		return reviewPreflightError(err)
	}
	eligibility, inspectErr := reviewtransaction.InspectCompactPristineAbandonment(ctx, repo, lineage)
	if inspectErr != nil {
		eligibility = reviewtransaction.CompactAbandonEligibility{}
	}
	return reviewPreflightRefusal(reviewCorrectionContextBudgetReason, &reviewLensContextError{
		Code:   reviewCorrectionContextBudgetCode,
		Action: reviewCorrectionContextBudgetAction(eligibility, repo, lineage),
	})
}

// reviewProviderCaptureBudgetRefusal attaches the same surviving
// classification to the refuter capture, which flattened it identically.
func reviewProviderCaptureBudgetRefusal(err error) error {
	if !reviewIsLensContextBudgetRefusal(err) {
		return reviewPreflightError(err)
	}
	return reviewPreflightRefusal(reviewProviderCaptureBudgetReason, err)
}

// reviewCorrectionContextBudgetAction renders the operator-facing exit.
//
// InspectCompactPristineAbandonment is read-only, takes no lock and writes
// nothing, and it publishes Revision and SnapshotIdentity precisely so a
// caller naming the abandonment can print those values concrete. Where it
// says the lineage is not eligible, this says so plainly rather than printing
// a command that would be refused: a followable refusal is the whole point,
// and an unrunnable command is worse than an honest "ask a maintainer".
func reviewCorrectionContextBudgetAction(eligibility reviewtransaction.CompactAbandonEligibility, repo, lineage string) string {
	if !eligibility.Eligible {
		return reviewCorrectionContextBudgetPreamble +
			" This review's authority cannot be released automatically in its current state, so ask a maintainer to inspect it, or run `" +
			reviewModeDisableCloneCommand + "` " + reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead."
	}
	return reviewCorrectionContextBudgetPreamble + fmt.Sprintf(
		" Review authority DOES exist for this work, so it has to be released rather than left in place: run `gentle-ai review abandon --cwd %q --lineage %q --expected-revision %q --reason operator_disposition --actor <you> --maintainer-authorization <binding>`"+
			" (run `gentle-ai review abandon` with no flags to print the exact binding template and where every value is read; the frozen candidate it binds is %q)."+
			" Then review this change as smaller candidates, or run `%s` %s to deliver under ordinary repository policy instead.",
		repo, lineage, eligibility.Revision, eligibility.SnapshotIdentity,
		reviewModeDisableCloneCommand, reviewModeDisableCloneCaveat)
}

// reviewCorrectionContextBudgetPreamble is the one statement of fact every
// rendering opens with: what exceeded the budget, and that retrying cannot
// change it.
const reviewCorrectionContextBudgetPreamble = "This candidate's correction evidence plus its recorded findings cannot fit the runtime context budget, and that evidence is never truncated, so no retry of this targeted validation can succeed."

// reviewCorrectionReleaseContinuation renders the one runnable follow-up a
// correction_context_budget_exceeded stop can offer, following the
// managed-assets precedent: the stop itself carries the exact command, so no
// caller has to recover it from prose.
//
// It exists because the Pi facade contract may not name a raw
// `gentle-ai review ` route at all (validPiFacadeLifecycle), so the shipped Pi
// ledger row's "the release command the stop's `continuation` names" is the
// only channel through which that route can reach a Pi maintainer.
//
// The command is the FLAGLESS abandon form, deliberately. The complete
// abandonment needs an actor and an eight-line maintainer binding that no
// producer may invent, and a printed command carrying placeholders for them
// would break the rule that a named command runs exactly as printed. The
// flagless form does run exactly as printed, and its refusal prints the
// binding template plus where every remaining value is read.
//
// Where eligibility says the authority cannot be released, this returns
// nothing rather than naming a command the live operation would refuse --
// the same honesty reviewCorrectionContextBudgetAction already keeps in its
// prose. A stop with no continuation is the truthful "ask a maintainer" case.
func reviewCorrectionReleaseContinuation(repo, agent string, eligibility *reviewtransaction.CompactAbandonEligibility) *ReviewStopContinuation {
	repo = strings.TrimSpace(repo)
	if repo == "" || eligibility == nil || !eligibility.Eligible {
		return nil
	}
	return &ReviewStopContinuation{
		Operation: "abandon",
		Command:   managedAssetsContinuationExecutable() + " review abandon --cwd " + reviewTransitionShellWord(repo),
		Agent:     strings.TrimSpace(agent),
		Detail: "Runs the release this stop requires. Printed flagless on purpose: it refuses with the exact binding template and names where every remaining value is read (" +
			"--lineage, --expected-revision, --actor, --reason and the eight-line --maintainer-authorization), which no producer may invent on a maintainer's behalf.",
	}
}
