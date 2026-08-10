package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// ReviewFacadeFinalizeNewLineageResult is finalize's v3 activation-branch
// response shape (task C1). It is deliberately not ReviewFacadeFinalizeResult:
// that type is keyed to legacy CompactState's Mode/State vocabulary and a
// compact receipt shape, neither of which NewLineageAuthority/NewLineageReceipt
// use — mirroring ReviewFacadeStartNewLineageResult's own precedent
// (review_facade_new_lineage.go) of not reusing a legacy-shaped type.
type ReviewFacadeFinalizeNewLineageResult struct {
	Operation          string                            `json:"operation"`
	LineageID          string                            `json:"lineage_id"`
	State              reviewtransaction.NewLineageState `json:"state"`
	AdmittedFindingIDs []string                          `json:"admitted_finding_ids,omitempty"`
	Receipt            *reviewtransaction.ReceiptRef     `json:"receipt,omitempty"`
}

// runReviewFacadeFinalizeNewLineage is the missing caller CRITICAL C1
// identified: design decision 5 says finalize routes on discovered lineage
// kind, but no production code ever called ReviewCore.finalize +
// AuthorityStore.WriteReceipt from the CLI — a v3 lineage the shipped
// `review start` creates could never be finalized through the product. This
// supplies that caller, reusing the exact receipt-issuance path S2/S3 already
// built and tested (TestReviewCoreFinalizeApprovedIssuesReceiptRef,
// authority_store_test.go's receipt-immutability suite) rather than adding
// new receipt logic.
//
// It is the tier-0 minimum-viable finalize path (no reviewer-lens execution,
// no correction plan — those are Wave 4+ territory per this wave's own
// documented scope, apply-progress task 6.7): a lineage with no admitted
// candidate-causal blocker (task C2, reviewtransaction.AdmitCandidateCausalFindings)
// and no --failed evidence advances reviewing/validating straight through
// validating to approved; an admitted blocker or --failed advances it to
// escalated instead. Both routes issue the terminal receipt through the
// identical ReviewCore.Next(finalize) + AuthorityStore.WriteReceipt call the
// rollback-safety test (review_new_lineage_rollback_safety_test.go) already
// drives directly against the Go API — this function is the CLI wiring that
// test's own comment noted was still missing.
//
// A lineage already `correcting` now finalizes through the identical
// AdvanceRequest path as reviewing/validating (W14 remediation): the one
// bounded correction it already spent (ReviewCore.validate's
// ErrOneCorrectionOnly boundary, enforced at validate time, not here) is
// consumed by this finalize the same way a plain reviewing lineage's review
// is — refusing it outright, as this function previously did, left
// `correcting` unreachable and unfinalizable through the product, breaking
// spec scenario "In-flight new lineage still finalizes after rollback"
// (whose GIVEN is literally a `correcting` lineage) at the product surface.
//
// C6 remediation (verify-report CRITICAL, "ReviewCore-owned transitions in
// finalize"): this function no longer sets NewLineageState itself. It
// classifies findings (AdmitCandidateCausalFindings — pure, not a state
// decision) and hands the result to ReviewCore.Next(finalize) via
// CoreRequest.AdvanceRequest; ReviewCore alone decides the resulting
// authority, and the ONE Mutate call below stores it verbatim
// (`*next = *transition.Authority`), mirroring runReviewFacadeStartNewLineage's
// own precedent (review_facade_new_lineage.go:73-78) exactly.
func runReviewFacadeFinalizeNewLineage(
	ctx context.Context, stdout io.Writer, root, lineage string, record reviewtransaction.NewLineageRecord,
	findings []reviewtransaction.FindingEvidence, failed, capturedResults bool,
) error {
	store, err := reviewtransaction.NewLineageAuthorityStore(ctx, root, lineage)
	if err != nil {
		return err
	}
	authority := record.Authority
	revision := record.Revision

	// C-A (Wave 5 fix cycle 2): --captured-results names the lens results the
	// minimal v3 capture primitive already persisted onto this authority
	// (AuthorityStore.CaptureLensResult) -- never a caller-supplied list, the
	// same "only the caller's own already-admitted results, never new input"
	// shape v2's --captured-results has. A tier-low authority (SelectedLenses
	// empty) passes hasCapturedAllSelectedLenses trivially either way.
	var capturedLensResults []string
	if capturedResults {
		capturedLensResults = authority.CapturedLensNames()
	}
	// W-9 (Wave 7 S1, design decision 3c): an unresolved (unknown or unset)
	// causal_disposition on a severe finding has no confirmed candidate
	// cause AND no known non-candidate cause either -- it must terminally
	// escalate, matching v2's own transaction.go validate logic, rather than
	// silently approve because it happened not to land in admittedIDs.
	// unresolvedIDs is never merged into AdmittedFindingIDs itself: that
	// field's contract is "candidate-caused blocker admitted for
	// correction", and an unresolved cause is not confirmed candidate-caused.
	admitted, _, unresolved := reviewtransaction.AdmitCandidateCausalFindings(findings)
	transition, err := (reviewtransaction.ReviewCore{}).Next(ctx, authority, reviewtransaction.CoreRequest{
		Kind: reviewtransaction.CoreRequestFinalize,
		AdvanceRequest: &reviewtransaction.FinalizeAdvanceRequest{
			Failed: failed || len(unresolved) > 0, AdmittedFindingIDs: admitted, CapturedLensResults: capturedLensResults,
		},
	})
	if err != nil {
		// W-8 (Wave 5 fix cycle 3, verify-report #10186 cycle 2): a partial
		// capture (some, not all, frozen selected lenses captured) is an
		// ORDINARY incomplete state, not a tool-internal fault -- an
		// unclassified fmt.Errorf wrap left it falling through to the outer
		// dispatcher's unexpected-fault/defect-report path. Classified here as
		// an ordinary preflight refusal (matching every other operator-
		// actionable refusal in this package) that names exactly which
		// lens(es) are still missing and the runnable continuation.
		if errors.Is(err, reviewtransaction.ErrFinalizeRequiresLensResults) {
			missing := authority.MissingCapturedLensNames()
			return reviewPreflightError(fmt.Errorf(
				"new-lineage finalize requires captured results for every frozen selected lens before approving: missing %s; capture each with `gentle-ai review capture-result --cwd <repo> --lineage %s --target <target> --lens <lens> --order <order> --input <result.json>` (run with --preflight first to discover the exact subject hash to echo back), then retry `gentle-ai review finalize --cwd <repo> --lineage %s --captured-results=true`",
				strings.Join(missing, ", "), lineage, lineage,
			))
		}
		return fmt.Errorf("review core finalize: %w", err)
	}
	if transition.Authority != nil {
		revision, err = store.Mutate(ctx, revision, func(next *reviewtransaction.NewLineageAuthority) error {
			*next = *transition.Authority
			return nil
		})
		if err != nil {
			return fmt.Errorf("advance new-lineage authority to %s: %w", transition.Authority.State, err)
		}
	}
	if transition.Receipt == nil {
		return fmt.Errorf("review core finalize returned no receipt for terminal state %q", authority.State) // refusal:by-design world-action: ReviewCore.finalize always returns a non-nil Receipt for an approved/escalated authority or an explicit AdvanceRequest (review_core.go); anything else reaching here is a bug in that code, fixed by editing it, not an operator command
	}
	record, err = store.Load()
	if err != nil {
		return err
	}
	if err := store.WriteReceipt(ctx, reviewtransaction.NewLineageReceipt{
		Schema: reviewtransaction.NewLineageReceiptSchema, LineageID: record.Authority.LineageID,
		TerminalState: record.Authority.State, AuthorityRevision: transition.Receipt.AuthorityRevision,
		CandidateIdentity: record.Authority.CandidateIdentity,
	}); err != nil {
		return fmt.Errorf("write new-lineage terminal receipt: %w", err)
	}
	result := ReviewFacadeFinalizeNewLineageResult{
		Operation: "review/finalize", LineageID: lineage, State: record.Authority.State,
		AdmittedFindingIDs: append([]string{}, record.Authority.AdmittedFindingIDs...), Receipt: transition.Receipt,
	}
	return encodeReviewJSON(stdout, result)
}
