package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// reviewContractRequiredForActionEligibilityReason is the single wording
// source for the refusal when --action-eligibility or --next-transition is
// requested without --contract. Both runReviewStatus and
// runReviewFacadeFinalize emit it so the two call sites cannot drift, and it
// names the exact contract value the caller must pass rather than only
// describing the requirement.
const reviewContractRequiredForActionEligibilityReason = "--action-eligibility and --next-transition require --contract " + ReviewIntegrationContractV1

// reviewStatusTargetSelectorsRequireContractReason is the sibling of
// reviewContractRequiredForActionEligibilityReason above: it names the same
// exact contract value for the other set of --contract-gated review status
// flags (target selectors such as --lineage, --base-ref, --base-tree,
// --workspace-overlay, --projection, --gate, and the recovery selectors).
const reviewStatusTargetSelectorsRequireContractReason = "review status target selectors require --contract " + ReviewIntegrationContractV1

// reviewStartTargetRequiresContractReason is the sibling of
// reviewContractRequiredForActionEligibilityReason above, naming the same
// exact contract value for review start --target used without --contract.
const reviewStartTargetRequiresContractReason = "review start --target requires --contract " + ReviewIntegrationContractV1

// reviewStartConsentRequiresContractReason keeps the consent declaration a
// strictly negotiated-form surface: the typed question exists for callers
// that relay envelopes, and the unnegotiated form keeps today's console
// behavior byte for byte. The refusal names the exact runnable rerun.
const reviewStartConsentRequiresContractReason = "review start --consent requires the negotiated form; rerun as gentle-ai review start --contract " +
	ReviewIntegrationContractV1 + " with the bound --target and --projection"

// reviewStartConsentValueReason names the exact allowed-answer domain for the
// consent declaration, mirroring the choice tokens the typed question emits.
const reviewStartConsentValueReason = "review start --consent accepts exactly relay, granted, or declined; rerun gentle-ai review start with one of those values"

// reviewFacadeReceiptNotAvailableReason is the single wording source for the
// refusal when a compact (or legacy) facade lineage was discovered but has
// not been finalized yet, so its receipt does not exist on disk. It names
// the exact continuation — finalize the already-discovered lineage — instead
// of only describing the missing artifact. All three call sites in this file
// (the validate gate path and both terminal-discovery helpers) share it so
// they cannot drift.
func reviewFacadeReceiptNotAvailableReason(lineageID string) string {
	return fmt.Sprintf("facade review receipt is not available; run gentle-ai review finalize --lineage %s to produce one", lineageID)
}

// reviewCompactFacadeLineageNotDiscoverableReason is the single wording
// source for the refusal when selector-free compact lineage discovery finds
// no candidate. This is reached only after every otherwise-loadable leaf has
// already been ruled out (a genuinely quarantined terminal lineage does not
// count as discoverable either), so the message must stay honest for both a
// lineage that was never started here and one started under a different
// --cwd; it never claims nothing was ever attempted.
const reviewCompactFacadeLineageNotDiscoverableReason = "no discoverable compact facade review lineage found; run gentle-ai review start to begin one, or pass --cwd if it was started from a different repository path"

const facadeReviewPolicy = `Gentle AI native bounded review policy.

Only candidate-caused BLOCKER or CRITICAL findings may require correction. Pre-existing and base-only findings are follow-ups. One correction is bounded by the frozen original scope, and delivery gates validate the terminal receipt against live Git evidence.
`

type ReviewFacadeStartResult struct {
	Operation        string                       `json:"operation"`
	Action           string                       `json:"action"`
	LensesRequired   bool                         `json:"lenses_required"`
	LineageID        string                       `json:"lineage_id"`
	State            reviewtransaction.State      `json:"state"`
	RiskLevel        reviewtransaction.RiskLevel  `json:"risk_level"`
	SelectedLenses   []string                     `json:"selected_lenses"`
	LensBindings     []ReviewFacadeLensBinding    `json:"lens_bindings"`
	Projection       reviewtransaction.Projection `json:"projection"`
	TargetMode       reviewtransaction.TargetKind `json:"target_mode,omitempty"`
	TargetIdentity   string                       `json:"target_identity,omitempty"`
	BaseTree         string                       `json:"base_tree,omitempty"`
	CandidateTree    string                       `json:"candidate_tree,omitempty"`
	ChangedFiles     int                          `json:"changed_files"`
	ChangedLines     int                          `json:"changed_lines"`
	CorrectionBudget int                          `json:"correction_budget"`
	// RiskEvidence carries the same human phrases the interactive consent
	// prompt speaks for the frozen candidate, so a headless consumer can relay
	// WHY the tier escalated. Absent when no evidence drove an escalation.
	RiskEvidence []string `json:"risk_evidence,omitempty"`
	// Hint is a purely informational recovery pointer; it never changes START
	// behavior and is absent outside its one scoped case.
	Hint string `json:"hint,omitempty"`
	// Consent reports a user consent choice as a typed outcome instead of an
	// error, in the same additive omitted-by-default spirit as RiskEvidence,
	// Hint, and the gate result's Delivery: every start that already shipped
	// keeps its exact field set, and only a candidate the user declined
	// carries the extra token. A decline is a reported choice, never a veto.
	Consent string `json:"consent,omitempty"`
}

// ReviewStartConsentDeclinedThisCandidate reports that the user answered the
// one-time consent question with "Not now, just this once": this candidate was
// not reviewed, nothing was persisted, and the next candidate asks again. The
// token mirrors the snake_case delivery vocabulary (receipt_governed,
// unmanaged) so agents can distinguish this outcome from every other start.
const ReviewStartConsentDeclinedThisCandidate = "declined_this_candidate"

// reviewStartEmptyCandidateHint makes the committed-work recovery path
// discoverable where it is needed: a clean worktree yields an empty candidate,
// and the fix is to name the base to compare against, not to redo the work.
const reviewStartEmptyCandidateHint = "the candidate has no pending changes; already-committed work can be reviewed by rerunning review start with --base-ref <commit> naming the base to compare against"

// reviewStartNegotiateContractHint makes the negotiated contract path
// discoverable exactly where an unnegotiated caller needs it: this response's
// selected lenses require the frozen candidate_diff, changed_path_manifest,
// and artifact_subjects that only the negotiated form returns, so the hint
// names the exact rerun invocation, reusing this same response's own
// target_identity and projection instead of only describing the requirement.
func reviewStartNegotiateContractHint(targetIdentity string, projection reviewtransaction.Projection) string {
	return fmt.Sprintf(
		"this response's selected lenses require the frozen candidate diff, changed-path manifest, and artifact subjects, which only the negotiated contract form returns; rerun with `gentle-ai review start --contract %s --target %s --projection %s` to receive them",
		ReviewIntegrationContractV1, targetIdentity, projection,
	)
}

// ReviewFacadeLensBinding pairs one selected lens with its frozen zero-based
// order so orchestrators build capture bindings exclusively from START output.
type ReviewFacadeLensBinding struct {
	Lens  string `json:"lens"`
	Order int    `json:"order"`
}

func facadeLensBindings(lenses []string) []ReviewFacadeLensBinding {
	bindings := make([]ReviewFacadeLensBinding, len(lenses))
	for order, lens := range lenses {
		bindings[order] = ReviewFacadeLensBinding{Lens: lens, Order: order}
	}
	return bindings
}

func facadeProjection(projection reviewtransaction.Projection) reviewtransaction.Projection {
	if projection == "" {
		return reviewtransaction.ProjectionWorkspace
	}
	return projection
}

type ReviewFacadeFinalizeResult struct {
	Operation string                  `json:"operation"`
	LineageID string                  `json:"lineage_id"`
	State     reviewtransaction.State `json:"state"`
	Action    string                  `json:"action"`
	// Escalation names the correction-budget accounting behind a terminal
	// escalation, rendered from
	// reviewtransaction.EscalationAccountingReasonTemplate. It is present only
	// when the authority actually escalated with a derivable cause, so every
	// other finalize shape keeps its exact existing output.
	Escalation    string `json:"escalation,omitempty"`
	StoreRevision string `json:"store_revision"`
	ReceiptPath   string `json:"receipt_path,omitempty"`
}

type ReviewReceiptDiscoveryKind string

const (
	ReviewReceiptMissing      ReviewReceiptDiscoveryKind = "receipt_missing"
	ReviewReceiptUnrelated    ReviewReceiptDiscoveryKind = "receipt_unrelated"
	ReviewReceiptScopeChanged ReviewReceiptDiscoveryKind = "receipt_scope_changed"
	ReviewReceiptAmbiguous    ReviewReceiptDiscoveryKind = "receipt_ambiguous"
	ReviewAuthorityCorrupted  ReviewReceiptDiscoveryKind = "authority_corrupted"
	// ReviewReceiptTargetUnresolvable names a deterministic target-resolution
	// failure (issue-1832): the repository has no upstream to derive a
	// publication boundary from. It is not authority damage, so it shares the
	// unmanaged-while-disabled classification with a missing, scope-changed,
	// or unrelated receipt.
	ReviewReceiptTargetUnresolvable ReviewReceiptDiscoveryKind = "target_unresolvable"
)

type ReviewReceiptDiscoveryError struct {
	Kind       ReviewReceiptDiscoveryKind
	Category   string
	Candidates []string
	Context    *reviewtransaction.GateContext
	// Detail preserves the exact typed cause of a deterministic mismatch so
	// the denial states what is actually true instead of a generic projection.
	Detail string
	// DeterministicallyStaleOnly is meaningful only when Kind ==
	// ReviewReceiptAmbiguous. It records the ambiguity's COMPOSITION: true
	// only when every lineage contributing to it classified into a
	// deterministically-typed stale bucket (scope-changed, delivery-shape, or
	// target-resolution) rather than an undecidable one (assessment-unknown
	// or scope-diagnostics-unavailable) or a genuinely competing exact match
	// (len(exact) > 1, which never sets this field). organic-dx Phase 3c: the
	// gate asks exactly one question -- does an approved receipt cover
	// exactly these candidate bytes? A stale receipt must INFORM, never
	// DECIDE. When this field is true, discovery has proven no candidate
	// governs, so the residual ambiguity is only about which stale lineage to
	// name in diagnostics -- it never has a delivery consequence, in either
	// mode. A false value (the default) means the gate genuinely cannot pick
	// -- present-tense competing authority, or an assessment that could not
	// even be completed.
	//
	// That false value keeps failing closed while reviews are ON, where
	// answering requires naming one governing receipt. It no longer decides
	// whether the gate BLOCKS while reviews are off: a switched-off system has
	// no implications, and declining to govern requires choosing nothing. What
	// it decides there is whether the reported disabled/unmanaged result must
	// additionally SAY the gate could not decide -- see
	// reviewDiscoveryLeftTheGateUndecided.
	//
	// A bool field was chosen over a distinct ReviewReceiptDiscoveryKind
	// because Kind is wire-facing (JSON "code", docs continuation table,
	// Phase 4's narration registry); splitting it would ripple through all
	// three surfaces for a distinction that has no separate wire behavior of
	// its own -- it only feeds one internal classification decision. This
	// field is package-internal bookkeeping, never serialized.
	DeterministicallyStaleOnly bool
}

func (err *ReviewReceiptDiscoveryError) Error() string {
	message := "review receipt discovery failed"
	switch err.Kind {
	case ReviewReceiptMissing:
		message = "no terminal review receipt exists for gate validation"
	case ReviewReceiptUnrelated:
		message = "terminal review receipts exist only for unrelated targets"
	case ReviewReceiptScopeChanged:
		message = "terminal review receipts do not exactly match the live gate target"
	case ReviewReceiptAmbiguous:
		if err.DeterministicallyStaleOnly {
			// organic-dx Phase 3c (community blocker + maintainer scope
			// extension): every contributing lineage is proven stale, so
			// nothing governs this candidate. Blocking stays correct where
			// this message is used to deny (reviews enabled): the candidate
			// genuinely was never reviewed. But the caller is told the truth
			// -- review it -- instead of being sent to disambiguate history.
			// Recovering a listed prior lineage remains available, never
			// required.
			message = "no terminal review receipt governs this candidate"
			if len(err.Candidates) > 0 {
				message += "; review it directly with gentle-ai review start, or optionally recover a prior lineage instead: " + strings.Join(err.Candidates, ", ")
			}
		} else {
			// More than one receipt genuinely governs, so the gate must not
			// pick for the caller. That refusal is correct; naming no way to
			// pick was not. A tester driving this from a pre-commit hook with
			// reviews switched OFF had to delete the hook to commit at all,
			// because the message named neither the flag that selects a target
			// nor the lineages it was choosing between -- both of which the
			// error already carries.
			message = "multiple terminal review receipts require explicit target selection"
			if len(err.Candidates) > 0 {
				message += "; select one with gentle-ai review validate --lineage <id>, from: " + strings.Join(err.Candidates, ", ")
			}
		}
	case ReviewAuthorityCorrupted:
		message = "complete review authority inventory is unavailable or corrupted"
	case ReviewReceiptTargetUnresolvable:
		message = "review gate target could not be resolved"
	}
	if err.Detail != "" {
		return message + ": " + err.Detail
	}
	return message
}

// errReviewMixedCompactLegacyAuthority reports that a gate-validate target is
// claimed as an exact governing candidate by BOTH the compact v2 store and a
// terminal legacy v1 chain. organic-dx Phase 3c task 3c.6 ("same-pass
// candidate"): this is the untyped twin of the community-reported
// plural-stale-receipt blocker (3c.1-3c.5 above) -- it too fails
// unconditionally, in every mode, hitting the same upgrader cohort (anyone
// who reviewed with gentle-ai before compact v2 shipped and has since
// reviewed the same target again post-upgrade).
//
// It is typed here (so a caller can match it with errors.Is) and stays OUTSIDE
// the ReviewReceiptDiscoveryError classification the other discovery errors in
// this file flow through, because it is not a discovery outcome: it is computed
// after discovery already proved the compact receipt EXACTLY governs
// (compactErr == nil in the unqualified path is reached ONLY from
// discoverCompactFacadeGateReview's single len(exact) == 1 return).
//
// It keeps failing closed while reviews are ON, and the reason is exact:
// answering the gate at all means naming one governing receipt, and here two
// independent authority systems each claim to be it. Picking would be silent
// and wrong.
//
// It no longer fails closed while reviews are OFF. The earlier argument for
// that -- "reclassifying would mean silently picking one authority system over
// the other" -- protects a choice the disabled path never makes:
// emitDisabledUnmanagedDelivery names no lineage, binds no receipt, reads no
// authority, and reports allowed:false. Declining to govern requires choosing
// nothing, so the contest simply has no delivery consequence until the operator
// switches reviews back on, at which point it is rediscovered and blocks again.
// The contest is still named in the reported reason rather than dropped.
//
// Covered by
// TestUnqualifiedGateDiscoveryOnMixedCompactAndLegacyAuthorityHonorsTheKillSwitch
// in review_receipt_discovery_test.go (negotiated, both modes) and
// TestReviewValidateReportsDisabledUnmanagedDeliveryOverMixedCompactAndLegacyAuthority
// in review_disabled_reach_test.go.
var errReviewMixedCompactLegacyAuthority = errors.New("review authority is ambiguous across compact v2 and legacy v1 stores; specify and clean up the intended lineage")

// ReviewFacadeReceiptPublicationError reports the only safe interpretation of
// a terminal authority whose derived receipt could not be materialized.
type ReviewFacadeReceiptPublicationError struct {
	MutationOutcome string `json:"mutation_outcome"`
	Replayability   string `json:"replayability"`
	LineageID       string `json:"lineage_id"`
	RequestDigest   string `json:"request_digest"`
	Cause           error  `json:"-"`
	// DefectReportClause is the Tier C companion (organic-dx tasks.md 5.6),
	// set only for the confirmed genuine deadlock (3b.8): an already-
	// terminal-committed authority whose derived receipt bytes conflict with
	// an existing immutable receipt.json at the same CAS path.
	DefectReportClause string `json:"-"`
}

func (err *ReviewFacadeReceiptPublicationError) Error() string {
	return fmt.Sprintf(
		"write compact review receipt: %v (mutation_outcome: %s, replayability: %s, lineage: %s, request_digest: %s).%s",
		err.Cause, err.MutationOutcome, err.Replayability, err.LineageID, err.RequestDigest, err.DefectReportClause,
	)
}

func (err *ReviewFacadeReceiptPublicationError) Unwrap() error { return err.Cause }

type reviewFacadeOperationProgressError struct {
	LineageID            string
	StoreRevision        string
	CommittedTransitions int
	Cause                error
	committed            *atomic.Pointer[reviewFacadeOperationProgressError]
}

func (err *reviewFacadeOperationProgressError) Error() string {
	return fmt.Sprintf("review finalize failed after %d committed native transition(s) for lineage %q at revision %s: %v",
		err.CommittedTransitions, err.LineageID, err.StoreRevision, err.Cause)
}

func (err *reviewFacadeOperationProgressError) Unwrap() error { return err.Cause }

func (err *reviewFacadeOperationProgressError) record(lineage, revision string) {
	err.LineageID = lineage
	err.StoreRevision = revision
	err.CommittedTransitions++
	if err.committed != nil {
		snapshot := *err
		err.committed.Store(&snapshot)
	}
}

// assessCompactGateTargetForDiscovery is the hookable seam
// discoverCompactFacadeGateReview calls for the expensive, per-leaf,
// git-subprocess-heavy applicability check. organic-dx Phase 3d tests
// override it to count how many terminal leaves still need the full
// assessment after the genesis-path-disjointness fast path (below) has
// skipped every leaf it can prove cannot govern the live candidate.
var assessCompactGateTargetForDiscovery = reviewtransaction.AssessCompactGateTarget

var writeCompactFacadeReceipt = func(ctx context.Context, store reviewtransaction.CompactStore, receipt reviewtransaction.CompactReceipt) error {
	return store.WriteReceipt(ctx, receipt)
}
var reviewFacadeSyncDirectory = reviewtransaction.SyncReviewDirectory
var reviewRecoverBeforePersist = func() {}

type ReviewInvalidateResult struct {
	Operation     string                  `json:"operation"`
	LineageID     string                  `json:"lineage_id"`
	State         reviewtransaction.State `json:"state"`
	StoreRevision string                  `json:"store_revision"`
}

type ReviewRecoverResult struct {
	Operation      string                                      `json:"operation"`
	LineageID      string                                      `json:"lineage_id"`
	State          reviewtransaction.State                     `json:"state"`
	StoreRevision  string                                      `json:"store_revision"`
	Projection     reviewtransaction.Projection                `json:"projection"`
	TargetIdentity string                                      `json:"target_identity"`
	Recovery       reviewtransaction.CompactRecoveryProvenance `json:"recovery"`
}

type facadeFinding struct {
	ID                string                              `json:"id,omitempty"`
	Lens              string                              `json:"lens,omitempty"`
	Location          string                              `json:"location,omitempty"`
	Severity          string                              `json:"severity,omitempty"`
	Claim             string                              `json:"claim,omitempty"`
	ProofRefs         []string                            `json:"proof_refs,omitempty"`
	EvidenceClass     reviewtransaction.EvidenceClass     `json:"evidence_class,omitempty"`
	CausalDisposition reviewtransaction.CausalDisposition `json:"causal_disposition,omitempty"`
}

type facadeReviewerResult struct {
	SubjectHash string                               `json:"subject_hash"`
	Inspection  reviewtransaction.ArtifactInspection `json:"inspection"`
	Lens        string                               `json:"lens,omitempty"`
	Findings    []facadeFinding                      `json:"findings"`
	Evidence    []string                             `json:"evidence"`
}

type facadeValidationCheck struct {
	Passed   bool     `json:"passed"`
	Evidence []string `json:"evidence"`
}

type facadeValidationResult struct {
	OriginalCriteria     facadeValidationCheck        `json:"original_criteria"`
	CorrectionRegression facadeValidationCheck        `json:"correction_regression"`
	FollowUps            []reviewtransaction.FollowUp `json:"follow_ups"`
}

type facadeRefuterResult struct {
	Results []facadeRefuterOutcome `json:"results"`
}

type facadeRefuterOutcome struct {
	FindingID string                            `json:"finding_id"`
	Outcome   reviewtransaction.EvidenceOutcome `json:"outcome"`
	ProofRefs []string                          `json:"proof_refs"`
}

type facadeArtifacts struct {
	policy, ledger, evidence, fixDelta, receipt string
}

var reviewFacadeOperationTimeout = 25 * time.Second

// reviewFacadeStartOperationTimeout is the deadline for review.start only.
// START performs full candidate construction (snapshot, hashing, frozen
// context) over the real workspace, which can exceed the shared 25s facade
// deadline for a valid large candidate. It is a fixed constant, not a
// configurable timeout framework: every other operation keeps using
// reviewFacadeOperationTimeout unchanged, including the two existing tests
// that mutate that var directly.
const reviewFacadeStartOperationTimeout = 120 * time.Second

// reviewFacadeOperationDeadline selects the operation-scoped deadline.
// review.start uses its own larger constant; every other operation keeps
// the shared reviewFacadeOperationTimeout var byte-identical.
func reviewFacadeOperationDeadline(operation string) time.Duration {
	if operation == "review.start" {
		return reviewFacadeStartOperationTimeout
	}
	return reviewFacadeOperationTimeout
}

var reviewFacadeCommandRunner = runReviewCommandContext
var reviewFacadePlannedTransitionHook = func(context.Context, string, string, string) error { return nil }
var reviewFacadeCommittedTransitionHook = func(context.Context, string, string, string) error { return nil }

// reviewFacadeDiscoverIntendedUntracked is the injectable seam over START's
// untracked-scope discovery, so tests can force an unanticipated internal
// fault at the exact choke point issue #1881 crashed through and prove the
// failure envelope and defect-report treatment both fire.
var reviewFacadeDiscoverIntendedUntracked = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder) ([]string, error) {
	return builder.DiscoverIntendedUntracked(ctx)
}
var renderReviewStartFrozenCandidateContext = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, snapshot reviewtransaction.Snapshot) (reviewtransaction.FrozenCandidateContext, error) {
	return builder.FrozenCandidateContext(ctx, snapshot)
}

type reviewStartContextError struct {
	AuthoritySelected bool
	LineageID         string
	StoreRevision     string
	Cause             error
}

func (err *reviewStartContextError) Error() string {
	base := fmt.Sprintf("render frozen context before START authority creation for %q: %v", err.LineageID, err.Cause)
	if err.AuthoritySelected {
		base = fmt.Sprintf("render frozen context for selected durable START authority %q at revision %s: %v", err.LineageID, err.StoreRevision, err.Cause)
	}
	if remedy := reviewStartContextBoundRemedy(err.Cause); remedy != "" {
		return base + ". " + remedy
	}
	return base
}

// reviewStartContextFailureMessage joins one typed failure headline with the
// optional numbered remedy. Causes that carry no remedy keep their existing
// message byte-for-byte, so only the explainable overflow gains new text.
func reviewStartContextFailureMessage(headline, remedy string) string {
	if remedy == "" {
		return headline
	}
	return headline + " " + remedy
}

// reviewStartContextBoundRemedy explains a frozen-context render that failed
// because the candidate overflowed a bounded Git capture, and returns the empty
// string for every other cause so unrelated failures are not decorated with an
// irrelevant size story.
//
// It names the exact bound and the exact rendered size because "too large" is
// not actionable: only the ratio tells a caller whether one file or half the
// change has to come out. The named way out is to review the change as smaller
// candidates, which is an action rather than a command on purpose. No flag,
// projection, or subcommand makes an oversized candidate renderable -- raising
// the bound would only move the cliff -- and this branch's rule is that a
// message may name a command only if running it resolves the block, so naming
// one here would be a lie.
//
// The remedy states the success criterion ("candidates that each render under
// it") rather than a recipe such as "stage fewer paths", because a recipe would
// be false for the candidate whose single path is itself oversized. Stating the
// criterion tells that caller the truth too: this change is not reviewable as
// one unit until it renders smaller.
func reviewStartContextBoundRemedy(cause error) string {
	var overflow *reviewtransaction.GitOutputLimitError
	if !errors.As(cause, &overflow) {
		return ""
	}
	// The typed negotiated envelope caps a failure message at 240 bytes, and
	// this text has to fit inside it after the longest headline, so it stays
	// terse on purpose: both numbers, then the action, and nothing else.
	measured := fmt.Sprintf("It exceeds the %d-byte", overflow.Limit)
	if overflow.Actual > 0 {
		measured = fmt.Sprintf("It renders %d bytes against a %d-byte", overflow.Actual, overflow.Limit)
	}
	return measured + " reviewer-context bound; review this change as smaller candidates that each render under it."
}

func (err *reviewStartContextError) Unwrap() error { return err.Cause }

func RunReview(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai review <capabilities|start|finalize|validate|status|repair|invalidate|abandon|recover|retry-final-verification|reclaim|inspect-authority|reconcile-authority|reconcile-authority-batch|dispose-result|reopen-results|quarantine-legacy|quarantine-legacy-fix-scope|repair-legacy-alias|schema|bind-sdd> [flags]\n\nOrdinary review facade; repository scope, authority, canonical artifacts, and lifecycle transitions are derived by Go. Use review retry-final-verification only for a provider-proven completed failed final-verification tooling incident. Generic review recover remains unchanged. Use review repair --preflight for provider-owned classified authority repair; repair-legacy-alias is compatibility-only.")
		_, _ = fmt.Fprintln(stdout, "Additive headless capabilities: gentle-ai review capture-result (with --preflight) and gentle-ai review preserve-result.")
		return nil
	}
	operation, negotiated, preflightFailure := reviewIntegrationFailureRoute(args)
	if preflightFailure != nil {
		if err := emitReviewIntegrationFailure(stdout, *preflightFailure); err != nil {
			return err
		}
		return newReviewIntegrationFailureError(*preflightFailure, nil)
	}
	if !negotiated {
		if err := runReviewCommand(args, stdout); err != nil {
			// The plain form has no envelope contract, but an unanticipated
			// internal fault on a mutating operation is the same product defect
			// there (issue #1881 crashed exactly this way): attach the saved
			// defect report to the error the operator is about to read.
			return reviewAppendUnexpectedFaultDefectClause(context.Background(), operation, args[1:], err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), reviewFacadeOperationDeadline(operation))
	defer cancel()
	var committed atomic.Pointer[reviewFacadeOperationProgressError]
	ctx = context.WithValue(ctx, reviewFacadeOperationProgressError{}, &committed)
	metadata, _ := reviewIntegrationOperationByName(operation)
	joinOnTimeout := metadata.JoinOnTimeout && reviewIntegrationOperationMutates(metadata, args[1:])
	var output bytes.Buffer
	result := make(chan error, 1)
	go func(runner func(context.Context, []string, io.Writer) error) { result <- runner(ctx, args, &output) }(reviewFacadeCommandRunner)
	var runErr error
	select {
	case runErr = <-result:
		if runErr == nil && operation != ReviewIntegrationOperationBindSDD {
			runErr = ctx.Err()
		}
	case <-ctx.Done():
		if operation == ReviewIntegrationOperationBindSDD || joinOnTimeout {
			runErr = <-result
			if runErr == nil && operation != ReviewIntegrationOperationBindSDD {
				runErr = ctx.Err()
			}
		} else if progress := committed.Load(); progress != nil {
			progress.Cause = &reviewtransaction.GitCommandTimeoutError{Timeout: reviewFacadeOperationTimeout, Aggregate: true, Cause: ctx.Err()}
			runErr = progress
		} else {
			runErr = ctx.Err()
		}
	}
	if runErr == nil {
		_, err := io.Copy(stdout, &output)
		return err
	}
	failure := newReviewIntegrationFailure(operation, args[1:], runErr)
	// Generate the defect report before emitting the envelope so a stdout
	// write failure cannot suppress the artifact; the clause rides only on the
	// operator-facing error, never inside the schema-bounded envelope.
	clause := reviewUnexpectedFaultDefectReportClause(ctx, operation, args[1:], failure)
	if err := emitReviewIntegrationFailure(stdout, failure); err != nil {
		return err
	}
	typedFailure := newReviewIntegrationFailureError(failure, runErr)
	typedFailure.defectReportClause = clause
	return typedFailure
}

func runReviewCommandContext(ctx context.Context, args []string, stdout io.Writer) error {
	switch args[0] {
	case "start":
		return runReviewFacadeStart(ctx, args[1:], stdout)
	case "status":
		return runReviewStatus(ctx, args[1:], stdout)
	case "repair":
		return runReviewRepair(ctx, args[1:], stdout)
	case "retry-final-verification":
		return runReviewRetryFinalVerification(ctx, args[1:], stdout)
	case "finalize":
		return runReviewFacadeFinalize(ctx, args[1:], stdout)
	case "validate":
		return runReviewFacadeValidate(ctx, args[1:], stdout)
	case "bind-sdd":
		return runReviewBindSDD(ctx, args[1:], stdout)
	case "reconcile-authority-batch":
		return runReviewReconcileAuthorityBatch(ctx, args[1:], stdout)
	default:
		return runReviewCommand(args, stdout)
	}
}

func runReviewCommand(args []string, stdout io.Writer) error {
	switch args[0] {
	case "capture-result":
		return RunReviewCaptureResult(args[1:], stdout)
	case "capture-evidence":
		return RunReviewCaptureEvidence(args[1:], stdout)
	case "preserve-result":
		return RunReviewPreserveResult(args[1:], stdout)
	case "capabilities":
		return RunReviewCapabilities(args[1:], stdout)
	case "start":
		return RunReviewFacadeStart(args[1:], stdout)
	case "finalize":
		return RunReviewFacadeFinalize(args[1:], stdout)
	case "validate":
		return RunReviewFacadeValidate(args[1:], stdout)
	case "status":
		return RunReviewStatus(args[1:], stdout)
	case "repair":
		return RunReviewRepair(args[1:], stdout)
	case "invalidate":
		return RunReviewInvalidate(args[1:], stdout)
	case "abandon":
		return RunReviewAbandon(args[1:], stdout)
	case "recover":
		return RunReviewRecover(args[1:], stdout)
	case "retry-final-verification":
		return RunReviewRetryFinalVerification(args[1:], stdout)
	case "reclaim":
		return RunReviewReclaim(args[1:], stdout)
	case "inspect-authority":
		return RunReviewInspectAuthority(args[1:], stdout)
	case "reconcile-authority":
		return RunReviewReconcileAuthority(args[1:], stdout)
	case "reconcile-authority-batch":
		return RunReviewReconcileAuthorityBatch(args[1:], stdout)
	case "dispose-result":
		return RunReviewDisposeResult(args[1:], stdout)
	case "reopen-results":
		return RunReviewReopenResults(args[1:], stdout)
	case "quarantine-legacy":
		return RunReviewLegacyQuarantine(args[1:], stdout)
	case "quarantine-legacy-fix-scope":
		return RunReviewLegacyFixScopeQuarantine(args[1:], stdout)
	case "repair-legacy-alias":
		return RunReviewLegacyAliasRepair(args[1:], stdout)
	case "schema":
		return RunReviewSchema(args[1:], stdout)
	case "bind-sdd":
		return RunReviewBindSDD(args[1:], stdout)
	default:
		return fmt.Errorf("unknown review command %q", args[0])
	}
}

func RunReviewStatus(args []string, stdout io.Writer) error {
	return runReviewStatus(context.Background(), args, stdout)
}

func runReviewStatus(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review status", stdout, "Read every compact-v2 and shipped legacy-v1 authority from the shared Git common directory without mutation.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	actionEligibility := flags.Bool("action-eligibility", false, "include optional machine-readable review action eligibility in negotiated output")
	nextTransition := flags.Bool("next-transition", false, "include the optional canonical native next transition in negotiated output")
	lineage := flags.String("lineage", "", "optional explicit lineage selector for negotiated target status")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "negotiated target projection: workspace or staged")
	baseRef := flags.String("base-ref", "", "optional negotiated immutable base-to-HEAD target")
	baseTree := flags.String("base-tree", "", "optional negotiated resolved immutable overlay base tree")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "select a negotiated base-ref workspace overlay target")
	gate := flags.String("gate", string(reviewtransaction.GatePreCommit), "lifecycle gate for an approved receipt transition")
	recoverySuccessor := flags.String("recovery-successor-lineage", "", "authorized successor lineage for a recovery transition")
	recoveryReason := flags.String("recovery-reason", "", "authorized recovery reason")
	recoveryActor := flags.String("recovery-actor", "", "authorized recovery actor")
	recoveryAuthorization := flags.String("recovery-authorization", "", "exact authorized recovery binding")
	repairActor := flags.String("repair-actor", "", "authorized classified repair actor")
	repairReason := flags.String("repair-reason", "", "authorized classified repair reason")
	repairAuthorization := flags.String("repair-authorization", "", "exact classified repair authorization binding")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review status argument %q", flags.Arg(0)))
	}
	if *contract != "" {
		if err := validateReviewIntegrationContract(*contract); err != nil {
			return err
		}
		if !validReviewIntegrationGate(reviewtransaction.GateKind(*gate)) {
			return fmt.Errorf("unsupported review lifecycle gate %q", *gate)
		}
		selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projection))
		if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
			return fmt.Errorf("unsupported review projection %q", *projection)
		}
		selectedBaseRef := strings.TrimSpace(*baseRef)
		selectedBaseTree := strings.TrimSpace(*baseTree)
		stagedRecoveryOverlay := *workspaceOverlay && selectedProjection == reviewtransaction.ProjectionStaged
		if *workspaceOverlay && stagedRecoveryOverlay && (selectedBaseRef == "" || selectedBaseTree != "") {
			return errors.New("staged --workspace-overlay requires exactly --base-ref")
		}
		if *workspaceOverlay && !stagedRecoveryOverlay &&
			((selectedBaseRef == "") == (selectedBaseTree == "") || selectedProjection != reviewtransaction.ProjectionWorkspace) {
			return errors.New("--workspace-overlay requires exactly one of --base-ref or --base-tree with workspace projection")
		}
		if !*workspaceOverlay && selectedBaseTree != "" {
			return errors.New("--base-tree requires --workspace-overlay")
		}
		if selectedBaseTree != "" && !validReviewGitTree(selectedBaseTree) {
			return errors.New("--base-tree requires an exact Git tree object ID")
		}
		builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
		root, err := builder.ResolveRepositoryRoot(ctx)
		if err != nil {
			return fmt.Errorf("resolve negotiated review repository root: %w", err)
		}
		intended := []string{}
		if selectedProjection != reviewtransaction.ProjectionStaged {
			intended, err = (reviewtransaction.SnapshotBuilder{Repo: root}).DiscoverIntendedUntracked(ctx)
			if err != nil {
				return fmt.Errorf("discover negotiated review target: %w", err)
			}
		}
		target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: intended}
		if selectedBaseRef != "" {
			target.Kind, target.BaseRef = reviewtransaction.TargetBaseDiff, selectedBaseRef
		}
		if *workspaceOverlay {
			target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
			if selectedBaseTree != "" {
				target.BaseRef = selectedBaseTree
			}
		}
		selector := &reviewTransitionSelector{
			Kind: target.Kind, Projection: selectedProjection, BaseRef: selectedBaseRef,
			BaseTree: selectedBaseTree, WorkspaceOverlay: *workspaceOverlay, PrePRRepresentable: true,
		}
		if reviewtransaction.GateKind(*gate) == reviewtransaction.GatePrePR {
			selector.PrePRRepresentable = reviewtransaction.ValidatePrePRBoundarySelector(ctx, root, selectedBaseRef) == nil
		}
		native, liveSnapshot, err := reviewtransaction.AssessTargetStatusWithSnapshot(ctx, root, reviewtransaction.TargetStatusRequest{
			Target: target, LineageID: *lineage,
		})
		if err != nil {
			return fmt.Errorf("assess negotiated review target: %w", err)
		}
		if selectedBaseTree != "" && native.Projection.BaseTree != selectedBaseTree {
			return errors.New("--base-tree does not identify an exact Git tree object")
		}
		result := newReviewTargetStatusResult(native)
		if native.Applicability == reviewtransaction.TargetApplicabilityCorrupted &&
			native.Action == reviewtransaction.TargetStatusActionRepairAuthority {
			repair, repairErr := reviewtransaction.AssessAuthorityRepairAtRepositoryRoot(ctx, root)
			if repairErr != nil {
				return fmt.Errorf("assess classified authority repair: %w", repairErr)
			}
			result.Repair = repair
		}
		if *actionEligibility {
			result.Eligibility = newReviewActionEligibility(result)
		}
		if *nextTransition {
			artifacts := []ReviewTransitionArtifact{}
			evidenceAvailable := false
			repositoryContext := ""
			var captureContext *reviewCaptureContext
			var validationRequest *reviewtransaction.TargetedValidationRequest
			correctionForecasted := false
			var artifactErr error
			if native.Applicability == reviewtransaction.TargetApplicabilityCurrent && native.AuthorityVersion == reviewtransaction.AuthorityVersionCompact {
				store, storeErr := reviewtransaction.CompactAuthoritativeStore(ctx, root, native.LineageID)
				if storeErr != nil {
					artifactErr = storeErr
				} else {
					record, loadErr := store.LoadContext(ctx)
					if loadErr != nil {
						artifactErr = loadErr
					} else {
						correctionForecasted = record.State.State == reviewtransaction.StateCorrectionRequired && record.State.ProposedCorrectionLines != nil
						stagedScopeRecovery := result.Action == reviewtransaction.TargetStatusActionRecover &&
							result.ActionDisposition == reviewtransaction.RecoveryScopeChanged &&
							record.State.State == reviewtransaction.StateApproved &&
							record.State.InitialSnapshot.Kind == reviewtransaction.TargetBaseDiff &&
							target.Kind == reviewtransaction.TargetBaseWorkspaceOverlay &&
							selector.Projection == reviewtransaction.ProjectionStaged && selector.WorkspaceOverlay
						selector.RecoveryRepresentable = record.State.InitialSnapshot.Kind == target.Kind || stagedScopeRecovery
						predecessorProjection := record.State.InitialSnapshot.Projection
						if predecessorProjection == "" {
							predecessorProjection = reviewtransaction.ProjectionWorkspace
						}
						if stagedScopeRecovery || selector.RecoveryRepresentable && result.ActionDisposition == reviewtransaction.RecoveryInvalidated && target.Kind == reviewtransaction.TargetBaseWorkspaceOverlay && selector.Projection == reviewtransaction.ProjectionStaged {
							selector.RecoveryProjection = reviewtransaction.ProjectionStaged
						} else if selector.RecoveryRepresentable && predecessorProjection != selector.Projection {
							selector.RecoveryRepresentable = result.ActionDisposition == reviewtransaction.RecoveryEscalated
							selector.RecoveryProjection = selector.Projection
						}
						if correctionForecasted {
							request, requestErr := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(ctx, root, record.State, record.Revision, liveSnapshot)
							if requestErr == nil {
								validationRequest = &request
								result.ValidationRequest = validationRequest
							}
						}
						if record.State.State == reviewtransaction.StateReviewing {
							repositoryContext, artifactErr = reviewtransaction.PublishReviewRepositoryContext(ctx, root, reviewtransaction.ReviewRepositoryContextBinding{
								LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.Revision,
							})
							if artifactErr == nil {
								frozen, frozenErr := (reviewtransaction.SnapshotBuilder{Repo: root}).FrozenCandidateContext(ctx, record.State.InitialSnapshot)
								if frozenErr != nil {
									artifactErr = frozenErr
								} else {
									captureContext, artifactErr = newReviewCaptureContext(record.State, record.Revision, frozen)
								}
							}
						}
						if artifactErr == nil {
							artifacts, artifactErr = discoverCapturedReviewerArtifacts(ctx, root, store.Dir, record.State, record.Revision)
						}
						if artifactErr == nil {
							_, evidenceErr := readCapturedFinalEvidence(store.Dir, record.State, record.Revision)
							evidenceAvailable = evidenceErr == nil
						}
					}
				}
			}
			// A start transition must name a lineage the store will accept. The
			// derived name is a function of the target identity alone, so a
			// superseded lineage still occupies the name this target derives, and
			// a start naming nothing would answer blocked-scope-action at exit 0.
			startLineage := strings.TrimSpace(*lineage)
			if startLineage == "" && native.Applicability == reviewtransaction.TargetApplicabilityUnrelated {
				startLineage = reviewAvailableStartLineage(ctx, root, native.TargetIdentity)
			}
			transition := newReviewNextTransition(result, native.SelectedLenses, artifacts, evidenceAvailable, artifactErr, reviewNextTransitionInput{Gate: reviewtransaction.GateKind(*gate), Successor: *recoverySuccessor, Reason: *recoveryReason, Actor: *recoveryActor, Authorization: *recoveryAuthorization, RepairActor: *repairActor, RepairReason: *repairReason, RepairAuthorization: *repairAuthorization, StartLineage: startLineage, RepositoryContext: repositoryContext, ValidationRequest: validationRequest, CorrectionForecasted: correctionForecasted, CaptureContext: captureContext, Selector: selector})
			result.NextTransition = &transition
			// The stdout JSON envelope is the machine surface and stays
			// byte-for-byte unchanged; this is the additive Tier C human
			// surface (spec "Three-Tier Narration Contract"), written to
			// stderr only, never mixed into the parsed stream.
			if transition.Kind == reviewNextTransitionStop {
				reviewNarrateStopReason(transition.ReasonCode)
			}
		}
		if err := result.Validate(); err != nil {
			return fmt.Errorf("validate negotiated review status: %w", err)
		}
		return encodeReviewJSON(stdout, result)
	}
	if *actionEligibility || *nextTransition {
		return errors.New(reviewContractRequiredForActionEligibilityReason)
	}
	if strings.TrimSpace(*lineage) != "" || strings.TrimSpace(*baseRef) != "" || strings.TrimSpace(*baseTree) != "" || *workspaceOverlay || *projection != string(reviewtransaction.ProjectionWorkspace) || *gate != string(reviewtransaction.GatePreCommit) || *recoverySuccessor != "" || *recoveryReason != "" || *recoveryActor != "" || *recoveryAuthorization != "" || *repairActor != "" || *repairReason != "" || *repairAuthorization != "" {
		return errors.New(reviewStatusTargetSelectorsRequireContractReason)
	}
	report, err := reviewtransaction.InventoryAuthority(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("inventory review authority: %w", err)
	}
	return encodeReviewJSON(stdout, report)
}

func RunReviewRecover(args []string, stdout io.Writer) error {
	if err := validateReviewTransitionSelectorFlagCounts(args, "review.recover"); err != nil {
		return err
	}
	flags := newReviewFlagSet("review recover", stdout, "Create an auditable successor authority without changing its predecessor.")
	cwd := flags.String("cwd", ".", "repository path")
	predecessor := flags.String("predecessor-lineage", "", "explicit predecessor lineage")
	expected := flags.String("expected-predecessor-revision", "", "exact predecessor revision")
	successor := flags.String("successor-lineage", "", "identifier for the successor authority this creates; you choose a new one, it only has to differ from every existing lineage")
	disposition := flags.String("disposition", "", "scope_changed, invalidated, or escalated")
	reason := flags.String("reason", "", "recovery reason")
	actor := flags.String("actor", "", "recovery actor")
	projectionFlag := flags.String("projection", "", "successor projection: workspace or staged (default: predecessor projection)")
	authorization := flags.String("maintainer-authorization", "", "exact LF-only gentle-ai.review-recovery-authorization/v1 binding: predecessor_lineage, predecessor_revision, target_identity, optional native successor_lineage, actor, reason")
	policySource := flags.String("policy", "", "optional review policy file")
	focus := flags.String("focus", "reliability", "dominant standard-risk focus; large pure documentation always uses readability")
	baseRef := flags.String("base-ref", "", "optional base revision for immutable base-to-HEAD review")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref excludes dirty tracked changes")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "recover an approved base-diff into the exact staged index over --base-ref")
	releaseScope := flags.Bool("release-scope", false, "recover an approved current-changes review into the immutable HEAD first-parent release scope")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review recover argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*predecessor) == "" || strings.TrimSpace(*expected) == "" || strings.TrimSpace(*successor) == "" || strings.TrimSpace(*disposition) == "" {
		return errors.New("review recover requires --predecessor-lineage, --expected-predecessor-revision, --successor-lineage, and --disposition")
	}
	// Self-derived recovery (organic-dx Duty 2): absence of
	// --maintainer-authorization, not its value, is what triggers
	// derivation, so an explicitly-supplied wrong binding never reaches
	// this branch and still refuses downstream exactly as before
	// self-derivation existed (reviewFlagWasProvided is a flags.Visit
	// presence check, not an empty-value check).
	authorizationProvided := reviewFlagWasProvided(flags, "maintainer-authorization")
	if authorizationProvided && (strings.TrimSpace(*reason) == "" || strings.TrimSpace(*actor) == "") {
		return errors.New("review recover requires --reason and --actor when --maintainer-authorization is supplied")
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
	root, err := resolveReviewMutationRoot(context.Background(), *cwd)
	if err != nil {
		return err
	}
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, *predecessor)
	if err != nil {
		return err
	}
	predecessorRecord, err := predecessorStore.Load()
	if err != nil {
		return fmt.Errorf("load recovery predecessor: %w", err)
	}
	base := strings.TrimSpace(*baseRef)
	baseDiff := predecessorRecord.State.InitialSnapshot.Kind == reviewtransaction.TargetBaseDiff
	overlay := predecessorRecord.State.InitialSnapshot.Kind == reviewtransaction.TargetBaseWorkspaceOverlay
	explicitOverlayBase := overlay && base != "" && !*committedOnly
	stagedScopeOverlay := *workspaceOverlay
	if *releaseScope && (base != "" || *committedOnly || stagedScopeOverlay) {
		return errors.New("--release-scope cannot be combined with --base-ref, --committed-only, or --workspace-overlay")
	}
	if *releaseScope && reviewtransaction.RecoveryDisposition(*disposition) != reviewtransaction.RecoveryScopeChanged {
		return errors.New("--release-scope requires --disposition scope_changed")
	}
	if *releaseScope && predecessorRecord.State.InitialSnapshot.Kind != reviewtransaction.TargetCurrentChanges {
		return errors.New("--release-scope requires a current-changes predecessor")
	}
	if stagedScopeOverlay && (!(baseDiff || overlay && predecessorRecord.State.State == reviewtransaction.StateInvalidated && predecessorRecord.State.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged) || base == "" || *committedOnly || strings.TrimSpace(*projectionFlag) != string(reviewtransaction.ProjectionStaged)) {
		return errors.New("--workspace-overlay recovery requires a base-diff predecessor, --base-ref, and --projection staged without --committed-only")
	}
	if !*releaseScope && !stagedScopeOverlay &&
		(*committedOnly != (base != "")) && !explicitOverlayBase {
		return errors.New("base-diff recovery requires matching --base-ref and --committed-only")
	}
	projection := predecessorRecord.State.InitialSnapshot.Projection
	if selected := strings.TrimSpace(*projectionFlag); selected != "" {
		projection = reviewtransaction.Projection(selected)
		if projection != reviewtransaction.ProjectionWorkspace && projection != reviewtransaction.ProjectionStaged {
			return fmt.Errorf("unsupported review recovery projection %q", selected)
		}
	}
	intended := []string{}
	if projection != reviewtransaction.ProjectionStaged {
		intended, err = builder.DiscoverIntendedUntracked(context.Background())
		if err != nil {
			return err
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: projection, IntendedUntracked: intended}
	if *committedOnly {
		target.Kind, target.BaseRef = reviewtransaction.TargetBaseDiff, base
	} else if stagedScopeOverlay {
		target.Kind, target.BaseRef = reviewtransaction.TargetBaseWorkspaceOverlay, base
	} else if overlay {
		target.Kind, target.BaseRef = reviewtransaction.TargetBaseWorkspaceOverlay, base
		if target.BaseRef == "" {
			target.BaseRef = predecessorRecord.State.InitialSnapshot.BaseTree
		}
	}
	var snapshot reviewtransaction.Snapshot
	if *releaseScope {
		snapshot, err = reviewtransaction.BuildReleaseScopeSnapshot(context.Background(), root)
	} else if stagedScopeOverlay {
		snapshot, err = builder.BuildStagedWorkspaceOverlayRecovery(context.Background(), target)
	} else {
		snapshot, err = builder.Build(context.Background(), target)
	}
	if err != nil {
		return err
	}
	if !*releaseScope && (baseDiff || overlay && base == "" || stagedScopeOverlay) &&
		snapshot.BaseTree != predecessorRecord.State.InitialSnapshot.BaseTree {
		return errors.New("recovery base-ref does not match predecessor base")
	}
	if !*releaseScope && (baseDiff || overlay) && snapshot.Identity == predecessorRecord.State.InitialSnapshot.Identity {
		return errors.New("recovery scope has not changed")
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		return err
	}
	risk, changedLines := assessment.Level, assessment.ChangedLines
	lenses, err := facadeSelectedLenses(assessment, *focus)
	if err != nil {
		return err
	}
	policy, err := facadePolicyBytes(*policySource)
	if err != nil {
		return err
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: *successor, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: predecessorRecord.State.Generation + 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &changedLines,
	})
	if err != nil {
		return err
	}
	// Self-derived recovery (organic-dx Duty 2): for one of the deterministic
	// recovery shapes already proven legal, derive actor (from repository
	// Git identity), reason (a closed machine-generated constant naming the
	// triggering state), and the maintainer-authorization binding itself, so
	// no operator argv is required. This mechanism does not widen authority:
	// reviewtransaction.RecoverCompactAuthority still performs every existing
	// legality check on the derived values exactly as it does on operator-
	// supplied ones, so an illegal recovery (an ACTIVE predecessor, a
	// non-accounting-only escalation over an unchanged target, a no-drift
	// scope-changed reset, and so on) still refuses.
	if !authorizationProvided {
		shape, derivable := reviewSelfRecoveryShapeForRecover(predecessorRecord.State.State, snapshot.Identity != predecessorRecord.State.InitialSnapshot.Identity)
		if !derivable {
			return errors.New("review recover requires --reason, --actor, and --maintainer-authorization for this predecessor state")
		}
		*actor = reviewAuditActor(context.Background(), root)
		*reason = reviewSelfRecoveryReason(shape)
		successorForBinding := ""
		if stagedScopeOverlay && predecessorRecord.State.State == reviewtransaction.StateApproved {
			successorForBinding = *successor
		}
		*authorization = reviewTransitionRecoveryAuthorization(ReviewTransitionBinding{LineageID: *predecessor, Revision: *expected, TargetIdentity: snapshot.Identity}, successorForBinding, *actor, *reason)
	}
	if *releaseScope {
		*authorization = reviewtransaction.ReleaseScopeRecoveryAuthorization
	} else if !stagedScopeOverlay && *authorization == reviewTransitionRecoveryAuthorization(ReviewTransitionBinding{LineageID: *predecessor, Revision: *expected, TargetIdentity: snapshot.Identity}, *successor, *actor, *reason) {
		*authorization = reviewTransitionRecoveryAuthorization(ReviewTransitionBinding{LineageID: *predecessor, Revision: *expected, TargetIdentity: snapshot.Identity}, "", *actor, *reason)
	}
	reviewRecoverBeforePersist()
	record, err := reviewtransaction.RecoverCompactAuthority(context.Background(), root, reviewtransaction.CompactRecoveryRequest{
		PredecessorLineageID: *predecessor, ExpectedPredecessorRevision: *expected, Successor: state,
		Disposition: reviewtransaction.RecoveryDisposition(*disposition), Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		explicitCwd := ""
		if reviewFlagWasProvided(flags, "cwd") {
			explicitCwd = strings.TrimSpace(*cwd)
		}
		switch {
		case reviewtransaction.RecoveryTargetUnchanged(err):
			return reviewUnchangedRecoveryRefusal(err, explicitCwd, *predecessor, *expected, *successor, *disposition)
		case reviewtransaction.ApprovedRecoveryScopeUnchanged(err):
			return reviewUnchangedApprovedScopeRefusal(err, explicitCwd, *predecessor, *expected, *successor)
		case reviewtransaction.RecoveryPredecessorNotInvalidated(err):
			return reviewNotInvalidatedPredecessorRefusal(err, explicitCwd, *predecessor, *expected, *successor, predecessorRecord.State.State)
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewRecoverResult{Operation: "review/recover", LineageID: record.State.LineageID, State: record.State.State,
		StoreRevision: record.Revision, Projection: facadeProjection(snapshot.Projection), TargetIdentity: snapshot.Identity, Recovery: *record.State.Recovery})
}

// reviewUnchangedRecoveryRefusal explains the unchanged-target escalated
// recovery refusal and names what to run once it stops applying.
//
// The refusal itself is correct and stays exactly as strict: a successor whose
// target is byte-identical to the escalated predecessor's would mint fresh
// authority over the very content whose verification failed, which is the
// forgery this authority model exists to prevent. What it never said is that
// the operator has to change the candidate first -- and once they do, the exact
// invocation they just ran becomes the one that works, because a refused
// recovery leaves the predecessor record (and therefore its revision)
// untouched. So the continuation is not a different command, it is this one,
// printed back with the selectors already in hand and nothing left to guess.
func reviewUnchangedRecoveryRefusal(cause error, cwd, predecessor, expected, successor, disposition string) error {
	return fmt.Errorf("%w: the candidate is byte-identical to the escalated predecessor, so this successor would carry the exact content whose verification failed and there is nothing to re-review; change the candidate first (apply the fix that verification asked for, and stage it if you review the staged projection), then re-run: %s",
		cause, reviewRecoverCommand(cwd, predecessor, expected, successor, disposition))
}

// reviewRecoverCommand renders one literal `gentle-ai review recover`
// invocation. Every value is one the caller already holds, so nothing here is
// ever a guess printed at the operator.
func reviewRecoverCommand(cwd, predecessor, expected, successor, disposition string) string {
	command := fmt.Sprintf("%s --predecessor-lineage %s --expected-predecessor-revision %s --successor-lineage %s --disposition %s",
		reviewRunnableCommand("review.recover"), predecessor, expected, successor, disposition)
	if cwd != "" {
		command += " --cwd " + cwd
	}
	return command
}

// reviewChangeTheCandidate is the world action all three healthy-approved
// guard rails ask for, worded once so they cannot drift apart.
const reviewChangeTheCandidate = "change the candidate first (edit the files you want reviewed, and stage them if you review the staged projection)"

// reviewUnchangedApprovedScopeRefusal explains why a scope-changed recovery of
// an approved predecessor whose scope did not move is refused, and names what
// runs once it has.
//
// The refusal stays exactly as strict. A successor here would freeze the very
// bytes the predecessor already approved, minting a second, fresher authority
// over content that was already reviewed — which is the forgery the recovery
// edge exists to prevent, and no flag may buy past it.
//
// What it never said is that this is not a command problem at all: with the
// candidate unchanged there is nothing to review, so the exit is an edit to the
// working tree. Once that edit lands, the invocation the operator just ran is
// the one that works — a refused recovery leaves the predecessor record, and
// therefore its revision, untouched — so the continuation is printed back with
// every selector already in hand.
func reviewUnchangedApprovedScopeRefusal(cause error, cwd, predecessor, expected, successor string) error {
	return fmt.Errorf("%w: lineage %s already approved exactly the candidate in your working tree, so this successor would re-freeze bytes that are already approved and there is nothing to re-review; %s, then re-run: %s",
		cause, predecessor, reviewChangeTheCandidate,
		reviewRecoverCommand(cwd, predecessor, expected, successor, string(reviewtransaction.RecoveryScopeChanged)))
}

// reviewNotInvalidatedPredecessorRefusal explains the invalidated-disposition
// refusal in terms of the predecessor's real state.
//
// This is the leg that closed the reported deadlock (community decode2 report):
// told that recovery needs an invalidated predecessor, the reader reasonably
// went to invalidate the predecessor, and `review invalidate` correctly refused
// a healthy approved authority. Both refusals were right and neither said that
// invalidation was never the way round — so the message must name the state the
// lineage is actually in and route to the disposition that state accepts, and
// it must never point back at `review invalidate`.
//
// Only an approved predecessor gets a printed continuation, because that is the
// state whose exit is proven end to end. Every other state is named honestly
// and nothing is invented for it: naming a command that does not clear the
// block is worse than naming none.
func reviewNotInvalidatedPredecessorRefusal(cause error, cwd, predecessor, expected, successor string, state reviewtransaction.State) error {
	if state != reviewtransaction.StateApproved {
		return fmt.Errorf("%w: --disposition invalidated is only for a lineage that is already invalidated, and lineage %s is %s",
			cause, predecessor, state)
	}
	return fmt.Errorf("%w: --disposition invalidated is only for a lineage that is already invalidated, and lineage %s is %s; it cannot be invalidated on the way either, because its approval still covers the candidate in your working tree, so %s, then re-run with the disposition an approved predecessor accepts: %s",
		cause, predecessor, state, reviewChangeTheCandidate,
		reviewRecoverCommand(cwd, predecessor, expected, successor, string(reviewtransaction.RecoveryScopeChanged)))
}

func RunReviewBindSDD(args []string, stdout io.Writer) error {
	return runReviewBindSDD(context.Background(), args, stdout)
}

func runReviewBindSDD(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review bind-sdd", stdout, "Bind an explicit approved compact lineage to an OpenSpec change.")
	cwd := flags.String("cwd", "", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	change := flags.String("change", "", "OpenSpec change")
	lineage := flags.String("lineage", "", "approved lineage")
	expected := flags.String("expected-binding-revision", "", "binding revision")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review bind-sdd argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	hasExpected := false
	for _, arg := range args {
		hasExpected = hasExpected || arg == "--expected-binding-revision" || strings.HasPrefix(arg, "--expected-binding-revision=")
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*change) == "" || strings.TrimSpace(*lineage) == "" || !hasExpected {
		return errors.New("review bind-sdd requires --cwd, --change, --lineage, and --expected-binding-revision")
	}
	if *expected != "" && !validReviewCapabilitySHA256(*expected) {
		return reviewPreflightError(errors.New("review bind-sdd expected-binding-revision must be empty or sha256"))
	}
	if _, err := resolveReviewMutationRoot(ctx, *cwd); err != nil {
		return err
	}
	binding, err := sddstatus.BindApprovedReview(ctx, *cwd, *change, *lineage, *expected)
	if err != nil {
		return err
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationBindSDD, binding, binding)
}

func RunReviewInvalidate(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review invalidate", stdout, "Terminally invalidate one explicit pristine reviewing authority, or an approved compact authority whose lifecycle gate natively re-derives invalidated under LOCK.")
	cwd := flags.String("cwd", "", "repository path")
	lineage := flags.String("lineage", "", "explicit review lineage identifier")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	reason := flags.String("reason", "", "non-empty terminal invalidation reason for a pristine reviewing authority")
	gate := flags.String("gate", "", "approved-authority lifecycle gate: post-apply, pre-commit, pre-push, pre-pr, or release")
	baseRef := flags.String("base-ref", "", "optional expected remote publication base for pre-push or pre-pr")
	ciAttestation := flags.String("pre-pr-ci-attestation", "", "signed exact-merged-tree CI attestation for a compatible base advance")
	policy := flags.String("policy", "", "explicit custom policy containing compatible-base CI trust")
	releaseConfiguration := flags.String("release-configuration", "", "release configuration artifact")
	releaseGenerated := flags.String("release-generated", "", "generated artifact manifest")
	releaseProvenance := flags.String("release-provenance", "", "release provenance artifact")
	releaseBoundary := flags.String("release-publication-boundary", "", "sealed publication boundary artifact")
	releaseFreshness := flags.String("release-evidence-freshness", "", "current release evidence freshness artifact")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review invalidate argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*expected) == "" {
		return errors.New("review invalidate requires --cwd, --lineage, and --expected-revision")
	}
	root, err := resolveReviewMutationRoot(context.Background(), *cwd)
	if err != nil {
		return err
	}
	compact, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, *lineage)
	if err != nil {
		return err
	}
	record, loadErr := compact.Load()
	if loadErr == nil {
		legacy, legacyErr := reviewtransaction.AuthoritativeStore(context.Background(), root, *lineage)
		if legacyErr == nil {
			if _, legacyLoadErr := legacy.LoadChain(); legacyLoadErr == nil {
				return errors.New("review authority is ambiguous across compact v2 and legacy v1 stores")
			}
		}
		approvedInvalidation := record.State.State == reviewtransaction.StateApproved ||
			record.State.State == reviewtransaction.StateInvalidated && record.State.InvalidationEvidence != nil
		if approvedInvalidation {
			if strings.TrimSpace(*gate) == "" {
				return errors.New("approved review invalidation requires --gate")
			}
			input := reviewtransaction.NativeGateRequestInput{
				Gate: reviewtransaction.GateKind(*gate), LineageID: *lineage, BaseRef: *baseRef,
				PrePRCIAttestation: *ciAttestation, ReleaseConfiguration: *releaseConfiguration,
				ReleaseGenerated: *releaseGenerated, ReleaseProvenance: *releaseProvenance,
				ReleasePublicationBoundary: *releaseBoundary, ReleaseEvidenceFreshness: *releaseFreshness,
			}
			if strings.TrimSpace(*ciAttestation) != "" {
				input.PolicyArtifact = *policy
			}
			invalidated, _, err := reviewtransaction.InvalidateApprovedCompactAuthority(context.Background(), root, reviewtransaction.CompactApprovedInvalidationRequest{
				LineageID: *lineage, ExpectedRevision: *expected, Gate: input,
			})
			if err != nil {
				var healthy *reviewtransaction.HealthyApprovedInvalidationError
				if errors.As(err, &healthy) {
					return reviewHealthyInvalidationRefusal(err, strings.TrimSpace(*cwd), *lineage, *expected, healthy.Result)
				}
				return err
			}
			return encodeReviewJSON(stdout, ReviewInvalidateResult{Operation: "review/invalidate", LineageID: invalidated.State.LineageID, State: invalidated.State.State, StoreRevision: invalidated.Revision})
		}
		if strings.TrimSpace(*reason) == "" {
			return errors.New("pristine review invalidation requires --reason")
		}
		state := record.State
		if state.State != reviewtransaction.StateInvalidated || state.InvalidationReason != strings.TrimSpace(*reason) {
			if err := state.Invalidate(*reason); err != nil {
				return err
			}
		}
		revision, err := compact.Replace(*expected, "review/invalidate", state)
		if err != nil {
			return err
		}
		return encodeReviewJSON(stdout, ReviewInvalidateResult{Operation: "review/invalidate", LineageID: state.LineageID, State: state.State, StoreRevision: revision})
	}
	if !errors.Is(loadErr, os.ErrNotExist) {
		return fmt.Errorf("load explicit compact review lineage: %w", loadErr)
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("legacy review invalidation requires --reason")
	}
	legacy, err := reviewtransaction.AuthoritativeStore(context.Background(), root, *lineage)
	if err != nil {
		return err
	}
	chain, err := legacy.LoadChain()
	if err != nil {
		return fmt.Errorf("load explicit review lineage: %w", err)
	}
	revision, err := legacy.InvalidatePristine(*expected, *reason, chain.Records[len(chain.Records)-1].Transaction.Snapshot)
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, ReviewInvalidateResult{Operation: "review/invalidate", LineageID: *lineage, State: reviewtransaction.StateInvalidated, StoreRevision: revision})
}

// reviewSuccessorLineagePlaceholder is the one value a recovery continuation
// cannot fill in for an operator who has not chosen it yet: the successor's
// name. `review recover --successor-lineage` says as much in its own help, and
// inventing a name on their behalf would be printing a command they never asked
// for under an identifier they now have to live with.
const reviewSuccessorLineagePlaceholder = "<successor-lineage>"

// reviewHealthyInvalidationRefusal adds the continuation to a refusal to
// destroy an approved authority the repository has not made stale.
//
// The refusal is correct and stays: the approval was earned over specific
// bytes, and a command that revoked it on demand would make every approval
// provisional. The situation sentence comes from the authority layer, which
// states it without internal vocabulary; what is added here is what to do.
//
// The two answers are genuinely different, so this branches on the same
// re-derived result the refusal reports rather than printing one continuation
// for both. When the candidate already moved, recovery is available right now.
// When the approval still covers the candidate exactly, there is nothing to
// review and no command can change that — the exit is an edit to the working
// tree, and only then the same recovery.
func reviewHealthyInvalidationRefusal(cause error, cwd, lineage, expected string, result reviewtransaction.GateResult) error {
	command := reviewRecoverCommand(cwd, lineage, expected, reviewSuccessorLineagePlaceholder, string(reviewtransaction.RecoveryScopeChanged))
	if result == reviewtransaction.GateScopeChanged {
		return fmt.Errorf("%w; review the candidate you have now as a successor instead, choosing any name for it that no existing lineage uses: %s",
			cause, command)
	}
	return fmt.Errorf("%w; if you want this candidate reviewed again, %s, then review it as a successor, choosing any name for it that no existing lineage uses: %s",
		cause, reviewChangeTheCandidate, command)
}

func RunReviewFacadeStart(args []string, stdout io.Writer) error {
	return runReviewFacadeStart(context.Background(), args, stdout)
}

func runReviewFacadeStart(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review start", stdout, "Freeze live Git scope and derive the bounded review tier, lenses, and correction budget.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	targetIdentity := flags.String("target", "", "exact frozen target identity for negotiated START")
	lineage := flags.String("lineage", "", "optional explicit review lineage identifier")
	policySource := flags.String("policy", "", "optional review policy file; the native bounded policy is used by default")
	focus := flags.String("focus", "reliability", "dominant standard-risk focus: risk, resilience, readability, or reliability; large pure documentation always uses readability")
	baseRef := flags.String("base-ref", "", "optional base revision for immutable base-to-HEAD review")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "candidate projection: workspace or staged; staged base-diff records post-commit delivery provenance")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref excludes dirty tracked changes")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "include branch commits and the live workspace over --base-ref")
	tracePath := flags.String("trace", "", "optional diagnostic operation metadata trace path")
	consent := flags.String("consent", "", "negotiated consent declaration: relay to receive the typed blocking consent question, granted or declined to answer it for the exact frozen candidate")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review start argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	if err := validateReviewStartBinding(args, negotiated, *targetIdentity, *projection, *baseRef, *lineage, *committedOnly, *workspaceOverlay, *consent); err != nil {
		return reviewPreflightError(err)
	}
	consentMode := reviewStartConsentMode(strings.TrimSpace(*consent))
	builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projection))
	if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported review projection %q", *projection)
	}
	if *workspaceOverlay && (strings.TrimSpace(*baseRef) == "" || *committedOnly || selectedProjection != reviewtransaction.ProjectionWorkspace) {
		return errors.New("--workspace-overlay requires --base-ref with workspace projection and is incompatible with --committed-only")
	}
	if selectedProjection == reviewtransaction.ProjectionStaged && strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		// Combining --projection staged with --base-ref is ambiguous about
		// intent, and this seam cannot guess it: a caller who wants the real
		// staged index reviewed needs plain staged projection, and a caller
		// who wants a base-diff review needs the committed-only escape. Name
		// both verbatim so the refusal is actionable either way.
		return fmt.Errorf("review start with --projection staged and --base-ref is refused because intent is ambiguous: for a staged-index review rerun with %q; for a base-diff review rerun with %q",
			"gentle-ai review start --projection staged",
			fmt.Sprintf("gentle-ai review start --base-ref %s --committed-only", strings.TrimSpace(*baseRef)))
	}
	if strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		dirtyTracked, dirtyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).HasDirtyTrackedChanges(ctx)
		if dirtyErr != nil {
			return fmt.Errorf("detect dirty tracked changes for committed review: %w", dirtyErr)
		}
		if dirtyTracked && !*committedOnly {
			return errors.New("review start with --base-ref omits dirty tracked changes; rerun with --committed-only to acknowledge committed-only review scope")
		}
	}
	intended := []string{}
	if selectedProjection != reviewtransaction.ProjectionStaged {
		intended, err = reviewFacadeDiscoverIntendedUntracked(ctx, builder)
		if err != nil {
			wrapped := fmt.Errorf("discover intended untracked files: %w", err)
			// Discovery runs before Build and before any authority mutation, so
			// its NAMED refusals classify as a not_started preflight in the
			// negotiated envelope. Typed Git subprocess failures keep their own
			// stronger git_command_* classification through this wrapper's
			// chain, and anything else is unanticipated residue that must stay
			// untyped so the defect-report treatment can see it for what it is.
			var refusal *reviewtransaction.UntrackedScopeRefusalError
			if errors.As(err, &refusal) {
				return reviewPreflightRefusal(reviewPreflightUntrackedScopeReason, wrapped)
			}
			return wrapped
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: intended}
	if strings.TrimSpace(*baseRef) != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = strings.TrimSpace(*baseRef)
	}
	if *workspaceOverlay {
		target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: root}).Build(ctx, target)
	if err != nil {
		return fmt.Errorf("build facade review target: %w", err)
	}
	if negotiated && snapshot.Identity != *targetIdentity {
		return reviewPreflightRefusal(reviewPreflightStaleTargetReason,
			errors.New("review start target does not match the freshly built snapshot"))
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("classify facade review target: %w", err)
	}
	risk, changedLines := assessment.Level, assessment.ChangedLines
	lenses, err := facadeSelectedLenses(assessment, *focus)
	if err != nil {
		return err
	}
	// The candidate is frozen and the tier is classified, so this is the one
	// point where the kill switch can stop a start and the one-time question can
	// name the real reason. Nothing has been persisted yet, so refusing here
	// leaves no authority behind.
	if err := authorizeReviewStart(ctx, root, assessment, consentMode); err != nil {
		if errors.Is(err, errReviewConsentQuestionRequired) {
			// The caller declared it can relay a blocking question, so the
			// typed question IS this start's response. Nothing has been
			// persisted; the named follow-up invocations answer for exactly
			// this frozen candidate and nothing else.
			question, questionErr := newReviewIntegrationConsentResult(snapshot, assessment,
				reviewConsentFollowUpBase(*cwd, snapshot.Identity, selectedProjection, strings.TrimSpace(*lineage),
					strings.TrimSpace(*baseRef), strings.TrimSpace(*policySource), strings.TrimSpace(*focus),
					strings.TrimSpace(*tracePath), *committedOnly, *workspaceOverlay))
			if questionErr != nil {
				return questionErr
			}
			return encodeReviewJSON(stdout, question)
		}
		if errors.Is(err, errReviewDeclinedForCandidate) && (!negotiated || consentMode == reviewConsentModeDeclined) {
			// A decline is a reported user choice, not a failure. Nothing has
			// been persisted and no latch was recorded, so the next candidate
			// simply asks again; the typed consent token lets an agent tell
			// "user declined this work unit" apart from every other outcome.
			// The relayed --consent declined answer reuses exactly this typed
			// outcome; an interactive decline inside an undeclared negotiated
			// start keeps its existing failure-envelope projection.
			return encodeReviewJSON(stdout, reviewFacadeStartDeclinedResult(snapshot, assessment))
		}
		return err
	}
	explicitLineage := strings.TrimSpace(*lineage) != ""
	if !explicitLineage {
		*lineage = reviewDerivedStartLineage(snapshot.Identity)
	}
	legacy, err := reviewtransaction.AuthoritativeStore(ctx, root, *lineage)
	if err == nil {
		if _, loadErr := legacy.LoadChain(); loadErr == nil {
			return fmt.Errorf("%w: choose a new lineage for compact authority", reviewtransaction.NewLegacyReadOnlyError("review/start", *lineage))
		}
	}
	policy, err := facadePolicyBytes(*policySource)
	if err != nil {
		return err
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: *lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: risk,
		SelectedLenses: lenses, OriginalChangedLines: &changedLines,
	})
	if err != nil {
		return fmt.Errorf("create compact facade review: %w", err)
	}
	var requestedFrozenContext *reviewtransaction.FrozenCandidateContext
	var requestedRepositoryContext *ReviewRepositoryContextReference
	var requestedContextErr error
	// Selecting lenses is a promise that reviewer work can be handed out, and
	// the frozen candidate context is the whole of what a reviewer is handed.
	// Rendering it here -- for every START that selects lenses, negotiated or
	// not -- is what keeps that promise checkable before any authority exists.
	// Gating this on the negotiated form was the defect: the unnegotiated form
	// only omits the context from its *response*, it does not stop needing the
	// context to exist, and it committed authority for candidates whose context
	// could never be rendered. STATUS then had an active authority and no
	// executable transition, which is the dead end this product refuses to
	// produce. The render is the same bounded Git capture STATUS would run
	// anyway, so a candidate that starts here is a candidate STATUS can answer.
	if len(state.SelectedLenses) > 0 {
		contextResult, contextErr := renderReviewStartFrozenCandidateContext(ctx, reviewtransaction.SnapshotBuilder{Repo: root}, state.InitialSnapshot)
		if contextErr != nil {
			requestedContextErr = &reviewStartContextError{LineageID: state.LineageID, Cause: contextErr}
		} else {
			requestedFrozenContext = &contextResult
		}
	}
	if negotiated && requestedContextErr == nil {
		revision, revisionErr := reviewtransaction.CompactRevisionForState(state)
		if revisionErr != nil {
			requestedContextErr = &reviewStartContextError{LineageID: state.LineageID, Cause: revisionErr}
		} else {
			binding := reviewtransaction.ReviewRepositoryContextBinding{
				LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Revision: revision,
			}
			handle, handleErr := reviewtransaction.DeriveReviewRepositoryContextHandle(ctx, root, binding)
			if handleErr != nil {
				requestedContextErr = &reviewStartContextError{LineageID: state.LineageID, Cause: handleErr}
			} else {
				requestedRepositoryContext = &ReviewRepositoryContextReference{
					Capability: reviewtransaction.ReviewRepositoryContextCapability, Handle: handle, Revision: revision,
					TargetIdentity: state.InitialSnapshot.Identity,
				}
			}
		}
	}
	if negotiated && requestedContextErr == nil {
		preview := reviewFacadeStartResultFor(reviewtransaction.CompactStartCreated, len(state.SelectedLenses) > 0, state)
		if _, previewErr := newReviewIntegrationStartResult(preview, assessment, state.InitialSnapshot.Kind, requestedFrozenContext, requestedRepositoryContext); previewErr != nil {
			requestedContextErr = &reviewStartContextError{LineageID: state.LineageID, Cause: previewErr}
		}
	}
	// BeforeCreate runs under the START lock at the new-authority boundary
	// only, so refusing from it is the one place a START can fail with nothing
	// persisted. It is wired for both forms because both create authority; the
	// live-snapshot recheck stays negotiated-only because only the negotiated
	// form carries the caller-supplied --target whose freshness it defends.
	beforeCreate := func() error {
		if requestedContextErr != nil {
			return requestedContextErr
		}
		if !negotiated {
			return nil
		}
		if err := (reviewtransaction.SnapshotBuilder{Repo: root}).ValidateLiveSnapshot(ctx, snapshot); err != nil {
			return reviewPreflightRefusal(reviewPreflightStaleTargetReason, err)
		}
		return nil
	}
	started, err := reviewtransaction.StartCompactAuthority(ctx, root, reviewtransaction.CompactStartRequest{
		State: state, TracePath: strings.TrimSpace(*tracePath), ExplicitLineage: explicitLineage, BeforeCreate: beforeCreate,
	})
	if err != nil {
		return fmt.Errorf("start compact facade review: %w", err)
	}
	authority := started.Record.State
	legacyResult := reviewFacadeStartResultFor(started.Action, started.LensesRequired, authority)
	if !negotiated {
		// Output-only projections of facts this start already computed. Both
		// describe the frozen candidate, so they attach only when this start
		// froze exactly the snapshot that was assessed above; a blocked start
		// reporting a different frozen authority keeps the previous shape.
		if authority.InitialSnapshot.Identity == snapshot.Identity {
			legacyResult.RiskEvidence = reviewConsentRiskEvidence(assessment)
			switch {
			case legacyResult.ChangedFiles == 0 && target.Kind == reviewtransaction.TargetCurrentChanges:
				legacyResult.Hint = reviewStartEmptyCandidateHint
			case legacyResult.LensesRequired:
				legacyResult.Hint = reviewStartNegotiateContractHint(legacyResult.TargetIdentity, legacyResult.Projection)
			}
		}
		return encodeReviewJSON(stdout, legacyResult)
	}
	if started.Action == reviewtransaction.CompactStartRecover {
		legacyResult.Action = string(reviewtransaction.CompactStartBlocked)
	}
	if authority.InitialSnapshot.Identity != snapshot.Identity {
		assessment, err = (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(ctx, authority.InitialSnapshot)
		if err != nil {
			return fmt.Errorf("classify authoritative negotiated START target: %w", err)
		}
	}
	var frozenContext *reviewtransaction.FrozenCandidateContext
	if len(authority.SelectedLenses) > 0 {
		if authority.InitialSnapshot.Identity == state.InitialSnapshot.Identity && requestedFrozenContext != nil {
			frozenContext = requestedFrozenContext
		} else {
			contextResult, contextErr := renderReviewStartFrozenCandidateContext(ctx, reviewtransaction.SnapshotBuilder{Repo: root}, authority.InitialSnapshot)
			if contextErr != nil {
				return &reviewStartContextError{
					AuthoritySelected: true, LineageID: authority.LineageID, StoreRevision: started.Record.Revision, Cause: contextErr,
				}
			}
			frozenContext = &contextResult
		}
	}
	var repositoryContext *ReviewRepositoryContextReference
	if authority.State == reviewtransaction.StateReviewing &&
		(started.Action == reviewtransaction.CompactStartCreated || started.Action == reviewtransaction.CompactStartResumed) {
		binding := reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: authority.LineageID, TargetIdentity: authority.InitialSnapshot.Identity, Revision: started.Record.Revision,
		}
		handle, contextErr := reviewtransaction.PublishReviewRepositoryContext(ctx, root, binding)
		if contextErr != nil {
			return &reviewStartContextError{
				AuthoritySelected: true, LineageID: authority.LineageID, StoreRevision: started.Record.Revision, Cause: contextErr,
			}
		}
		repositoryContext = &ReviewRepositoryContextReference{
			Capability: reviewtransaction.ReviewRepositoryContextCapability, Handle: handle, Revision: started.Record.Revision,
			TargetIdentity: authority.InitialSnapshot.Identity,
		}
	}
	negotiatedResult, err := newReviewIntegrationStartResult(legacyResult, assessment, authority.InitialSnapshot.Kind, frozenContext, repositoryContext)
	if err != nil {
		return &reviewStartContextError{
			AuthoritySelected: true, LineageID: authority.LineageID, StoreRevision: started.Record.Revision, Cause: err,
		}
	}
	return encodeReviewJSON(stdout, negotiatedResult)
}

func validateReviewStartBinding(args []string, negotiated bool, target, projection, baseRef, lineage string, committedOnly, workspaceOverlay bool, consent string) error {
	counts := reviewStartBindingFlagCounts(args)
	switch reviewStartConsentMode(strings.TrimSpace(consent)) {
	case reviewConsentModeNone, reviewConsentModeRelay, reviewConsentModeGranted, reviewConsentModeDeclined:
	default:
		return errors.New(reviewStartConsentValueReason)
	}
	if !negotiated {
		if counts["target"] != 0 {
			return errors.New(reviewStartTargetRequiresContractReason)
		}
		if counts["consent"] != 0 {
			return errors.New(reviewStartConsentRequiresContractReason)
		}
		return nil
	}
	for _, name := range []string{"contract", "target", "projection", "lineage", "base-ref", "committed-only", "workspace-overlay", "consent"} {
		if counts[name] > 1 {
			return fmt.Errorf("review start repeats --%s", name)
		}
	}
	if counts["contract"] != 1 || counts["target"] != 1 || counts["projection"] != 1 {
		return errors.New("negotiated review start requires exactly one --contract, --target, and --projection")
	}
	if !validReviewCapabilitySHA256(target) {
		return errors.New("negotiated review start requires an exact sha256 --target")
	}
	if projection != string(reviewtransaction.ProjectionWorkspace) && projection != string(reviewtransaction.ProjectionStaged) {
		return errors.New("negotiated review start requires an exact supported --projection")
	}
	if counts["lineage"] == 1 && !validReviewIntegrationLineage(lineage) {
		return errors.New("negotiated review start lineage is not canonical")
	}
	base := strings.TrimSpace(baseRef)
	switch {
	case workspaceOverlay:
		if counts["workspace-overlay"] != 1 || counts["base-ref"] != 1 || base == "" || counts["committed-only"] != 0 {
			return errors.New("negotiated workspace-overlay START requires exactly --base-ref and --workspace-overlay")
		}
	case base != "":
		if counts["base-ref"] != 1 || counts["committed-only"] != 1 || !committedOnly || counts["workspace-overlay"] != 0 {
			return errors.New("negotiated base-diff START requires exactly --base-ref and --committed-only")
		}
	default:
		if counts["base-ref"] != 0 || counts["committed-only"] != 0 || counts["workspace-overlay"] != 0 {
			return errors.New("negotiated current-changes START contains a partial base selector")
		}
	}
	return nil
}

func reviewStartBindingFlagCounts(args []string) map[string]int {
	counts := make(map[string]int)
	shape := reviewIntegrationOperationFlagShape("review.start")
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" || argument == "" || argument == "-" || argument[0] != '-' {
			break
		}
		nameValue := strings.TrimPrefix(strings.TrimPrefix(argument, "-"), "-")
		name, hasValue := nameValue, false
		if separator := strings.IndexByte(nameValue, '='); separator >= 0 {
			name, hasValue = nameValue[:separator], true
		}
		kind, known := shape[name]
		if !known {
			continue
		}
		switch name {
		case "contract", "target", "projection", "lineage", "base-ref", "committed-only", "workspace-overlay", "consent":
			counts[name]++
		}
		if kind != reviewIntegrationBoolFlag && !hasValue {
			index++
		}
	}
	return counts
}

func validateReviewTransitionSelectorFlagCounts(args []string, operation string) error {
	shape := reviewIntegrationOperationFlagShape(operation)
	selectors := []string{"base-ref"}
	if operation == "review.recover" {
		shape = map[string]reviewIntegrationFlagKind{
			"cwd":                           reviewIntegrationValueFlag,
			"predecessor-lineage":           reviewIntegrationValueFlag,
			"expected-predecessor-revision": reviewIntegrationValueFlag,
			"successor-lineage":             reviewIntegrationValueFlag,
			"disposition":                   reviewIntegrationValueFlag,
			"reason":                        reviewIntegrationValueFlag,
			"actor":                         reviewIntegrationValueFlag,
			"projection":                    reviewIntegrationValueFlag,
			"maintainer-authorization":      reviewIntegrationValueFlag,
			"policy":                        reviewIntegrationValueFlag,
			"focus":                         reviewIntegrationValueFlag,
			"base-ref":                      reviewIntegrationValueFlag,
			"committed-only":                reviewIntegrationBoolFlag,
			"workspace-overlay":             reviewIntegrationBoolFlag,
			"release-scope":                 reviewIntegrationBoolFlag,
			"h":                             reviewIntegrationBoolFlag,
			"help":                          reviewIntegrationBoolFlag,
		}
		selectors = []string{"base-ref", "committed-only", "projection", "workspace-overlay"}
	}
	counts := make(map[string]int, len(selectors))
	selected := make(map[string]bool, len(selectors))
	for _, name := range selectors {
		selected[name] = true
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" || argument == "" || argument == "-" || argument[0] != '-' {
			break
		}
		nameValue := strings.TrimPrefix(strings.TrimPrefix(argument, "-"), "-")
		name, hasValue := nameValue, false
		if separator := strings.IndexByte(nameValue, '='); separator >= 0 {
			name, hasValue = nameValue[:separator], true
		}
		kind, known := shape[name]
		if !known {
			continue
		}
		if selected[name] {
			counts[name]++
		}
		if kind != reviewIntegrationBoolFlag && !hasValue {
			index++
		}
	}
	command := strings.TrimPrefix(operation, "review.")
	for _, name := range selectors {
		if counts[name] > 1 {
			return fmt.Errorf("review %s repeats --%s", command, name)
		}
	}
	return nil
}

// reviewConsentFollowUpBase reassembles the caller's own START invocation as
// the runnable answer stem of the typed consent question. Every selector the
// caller bound is reproduced, and --target pins the exact frozen candidate, so
// the follow-up answers for this candidate and nothing else: if the workspace
// moves, the negotiated freshness check refuses the stale target instead of
// silently consenting to different bytes.
func reviewConsentFollowUpBase(
	cwd, target string,
	projection reviewtransaction.Projection,
	lineage, baseRef, policy, focus, trace string,
	committedOnly, workspaceOverlay bool,
) string {
	parts := []string{
		"gentle-ai review start",
		"--contract " + ReviewIntegrationContractV1,
		"--cwd " + cwd,
		"--target " + target,
		"--projection " + string(projection),
	}
	if lineage != "" {
		parts = append(parts, "--lineage "+lineage)
	}
	if baseRef != "" {
		parts = append(parts, "--base-ref "+baseRef)
	}
	if committedOnly {
		parts = append(parts, "--committed-only")
	}
	if workspaceOverlay {
		parts = append(parts, "--workspace-overlay")
	}
	if policy != "" {
		parts = append(parts, "--policy "+policy)
	}
	// The focus default never needs restating; only an explicit non-default
	// focus changes what the answered start would select.
	if focus != "" && focus != "reliability" {
		parts = append(parts, "--focus "+focus)
	}
	if trace != "" {
		parts = append(parts, "--trace "+trace)
	}
	return strings.Join(parts, " ")
}

// reviewFacadeStartDeclinedResult projects a consent decline as a typed START
// outcome. No authority exists, so it carries only facts this start already
// computed about the frozen candidate: no lineage, no state, no lenses, and a
// zero correction budget, because no review was selected and nothing persists.
func reviewFacadeStartDeclinedResult(snapshot reviewtransaction.Snapshot, assessment reviewtransaction.RiskAssessment) ReviewFacadeStartResult {
	return ReviewFacadeStartResult{
		Operation: "review/start", Action: "declined",
		SelectedLenses: []string{}, LensBindings: []ReviewFacadeLensBinding{},
		Projection:     facadeProjection(snapshot.Projection),
		TargetIdentity: snapshot.Identity, ChangedFiles: len(snapshot.Paths),
		ChangedLines: assessment.ChangedLines, RiskLevel: assessment.Level,
		RiskEvidence: reviewConsentRiskEvidence(assessment),
		Consent:      ReviewStartConsentDeclinedThisCandidate,
	}
}

func reviewFacadeStartResultFor(action reviewtransaction.CompactStartAction, lensesRequired bool, authority reviewtransaction.CompactState) ReviewFacadeStartResult {
	result := ReviewFacadeStartResult{
		Operation: "review/start", Action: string(action), LensesRequired: lensesRequired,
		LineageID: authority.LineageID, State: authority.State, RiskLevel: authority.RiskLevel,
		SelectedLenses: append([]string{}, authority.SelectedLenses...), LensBindings: facadeLensBindings(authority.SelectedLenses),
		Projection:   facadeProjection(authority.InitialSnapshot.Projection),
		ChangedFiles: len(authority.InitialSnapshot.Paths), TargetIdentity: authority.InitialSnapshot.Identity,
		ChangedLines: authority.OriginalChangedLines, CorrectionBudget: authority.CorrectionBudget,
	}
	if authority.InitialSnapshot.Kind == reviewtransaction.TargetBaseWorkspaceOverlay {
		result.TargetMode = authority.InitialSnapshot.Kind
		result.BaseTree = authority.InitialSnapshot.BaseTree
		result.CandidateTree = authority.InitialSnapshot.CandidateTree
	}
	return result
}

// reviewUnadmittedResultRefusal retires --result as a reviewer-result source.
//
// The flag read a reviewer result straight off disk and required only that
// findings and evidence were non-nil arrays. facadeReviewerResult already
// carries subject_hash and inspection, but nothing on this path ever checked
// them, so four hand-written files drove a high-risk lineage to an approved
// terminal receipt the delivery gates honoured -- with no lens run and an empty
// reviewer-results directory. That is a fabricated approval: silent, durable,
// and governing delivery.
//
// It is refused rather than admitted in place. Admitting here would need a
// second copy of the subject derivation, causality verification, and
// canonicalization that capture-result already performs -- a second source of
// truth about what admission means -- and it would still publish a receipt with
// nothing persisted to prove which results the approval rested on. Refusing
// makes the stronger invariant hold instead: an approved receipt implies
// admitted artifacts on disk. No workflow is lost, because the subject hash a
// conforming result must echo is only obtainable from the native binding, whose
// documented next step is capture-result and which takes the identical file.
// The capture command name is read from the same helper the collect transition
// uses, so the refusal can never name a command the product does not publish.
var reviewUnadmittedResultRefusal = "review finalize no longer accepts --result: a reviewer result supplied this way carries no provider-owned admission, " +
	"so it cannot prove the lens inspected the frozen candidate. " +
	"Capture each selected lens with `" + reviewCaptureResultCommandName() + "` (see `" + reviewNextTransitionRefreshCommand + "` for the exact lineage/target/lens/order bindings), " +
	"then run `gentle-ai review finalize --captured-results=true`"

func RunReviewFacadeFinalize(args []string, stdout io.Writer) error {
	return runReviewFacadeFinalize(context.Background(), args, stdout)
}

func runReviewFacadeFinalize(ctx context.Context, args []string, stdout io.Writer) (returnErr error) {
	committed, _ := ctx.Value(reviewFacadeOperationProgressError{}).(*atomic.Pointer[reviewFacadeOperationProgressError])
	progress := reviewFacadeOperationProgressError{committed: committed}
	defer func() {
		if returnErr == nil || progress.CommittedTransitions == 0 {
			return
		}
		var alreadyWrapped *reviewFacadeOperationProgressError
		if errors.As(returnErr, &alreadyWrapped) {
			return
		}
		wrapped := progress
		wrapped.Cause = returnErr
		returnErr = &wrapped
	}()
	flags := newReviewFlagSet("review finalize", stdout, "Canonicalize reviewer output and evidence, perform required native transitions, and materialize the terminal receipt.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	actionEligibility := flags.Bool("action-eligibility", false, "include optional machine-readable review action eligibility in negotiated output")
	nextTransition := flags.Bool("next-transition", false, "include the optional canonical native next transition in negotiated output")
	capturedResults := flags.Bool("captured-results", false, "use every natively captured reviewer result in canonical selected-lens order")
	capturedEvidence := flags.Bool("captured-evidence", false, "use the natively captured final verification evidence")
	lineage := flags.String("lineage", "", "optional lineage override when discovery is ambiguous")
	validationPath := flags.String("validation", "", "targeted correction validation JSON file or - for stdin")
	refuterPath := flags.String("refuter", "", "optional refuter outcomes JSON file or - for stdin")
	evidencePath := flags.String("evidence", "", "final test or verification evidence file or - for stdin")
	correctionLines := flags.Int("correction-lines", 0, "positive predicted correction changed lines before editing")
	failed := flags.Bool("failed", false, "bind supplied final evidence as a failed verification")
	tracePath := flags.String("trace", "", "optional diagnostic operation metadata trace path")
	var resultPaths repeatedString
	flags.Var(&resultPaths, "result", "retired and always refused: unadmitted reviewer results cannot bind an approval; capture each lens with `"+reviewCaptureResultCommandName()+"` and finalize with --captured-results")
	var resultArtifacts repeatedString
	flags.Var(&resultArtifacts, "result-artifact", "native reviewer artifact manifest JSON; repeat in selected-lens order")
	var resultArtifactFiles repeatedString
	flags.Var(&resultArtifactFiles, "result-artifact-file", "native reviewer artifact manifest regular file or - for stdin; repeat in selected-lens order")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review finalize argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	if (*actionEligibility || *nextTransition) && !negotiated {
		return errors.New(reviewContractRequiredForActionEligibilityReason)
	}
	stdinPaths := append(append([]string{}, resultPaths...), resultArtifactFiles...)
	if countFacadeStdin(stdinPaths, *validationPath, *refuterPath, *evidencePath) > 1 {
		return reviewPreflightError(errors.New("review finalize accepts stdin for only one input"))
	}
	reviewerResultSources := 0
	for _, supplied := range []bool{len(resultPaths) != 0, len(resultArtifacts) != 0, len(resultArtifactFiles) != 0, *capturedResults} {
		if supplied {
			reviewerResultSources++
		}
	}
	if reviewerResultSources > 1 || (*capturedEvidence && strings.TrimSpace(*evidencePath) != "") {
		return reviewPreflightError(errors.New("review finalize accepts exactly one reviewer-result source and one final-evidence source"))
	}
	// Refused before any repository or authority work, so the rejection cannot
	// advance the lineage or consume a lens slot. reviewUnadmittedResultRefusal
	// records why this route is refused outright rather than admitted in place.
	if len(resultPaths) != 0 {
		return reviewPreflightError(errors.New(reviewUnadmittedResultRefusal))
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	store, record, err := discoverCompactFacadeFinalize(ctx, root, *lineage)
	if err != nil {
		if _, chain, _, legacyErr := discoverFacadeReview(ctx, root, *lineage, false); legacyErr == nil {
			legacyLineage := chain.Records[len(chain.Records)-1].Transaction.LineageID
			return reviewtransaction.NewLegacyReadOnlyError("review/finalize", legacyLineage)
		}
		return err
	}
	store.TracePath = strings.TrimSpace(*tracePath)
	state := record.State
	if strings.TrimSpace(*lineage) != "" {
		leaves, err := reviewtransaction.CompactAuthorityLeaves(ctx, root)
		if err != nil {
			return err
		}
		current := false
		for _, leaf := range leaves {
			current = current || leaf.StatePath() == store.StatePath()
		}
		if !current {
			return fmt.Errorf("review lineage %q is superseded", *lineage)
		}
	}
	terminalAtEntry := facadeTerminalState(state.State)
	// The kill switch reaches FINALIZE here rather than at the router, because
	// only now is it known which FINALIZE this is. A terminal lineage replays
	// its frozen receipt exactly and writes nothing, and reading frozen
	// authority is precisely what freezing it must keep possible. Anything else
	// advances the review and is refused while reviews are off.
	if !terminalAtEntry {
		if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
			return err
		}
	}
	if terminalAtEntry && !facadeFinalizeReplayInputsEmpty(resultPaths, resultArtifacts, resultArtifactFiles, *capturedResults, *capturedEvidence, *validationPath, *refuterPath, *evidencePath, *correctionLines, *failed, *tracePath) {
		return errors.New("terminal review finalize accepts no review inputs; exact replay requires only --lineage")
	}
	// --captured-results is deliberately absent from this guard. The artifact
	// routes carry results in from outside the lineage, so applying them after
	// reviewing has ended would be new input; --captured-results can only name
	// results this lineage already admitted, which makes repeating it an
	// idempotent no-progress replay rather than a late submission.
	if state.State != reviewtransaction.StateReviewing && (len(resultArtifacts) != 0 || len(resultArtifactFiles) != 0 || len(resultPaths) != 0) {
		pending, pendingErr := store.PendingFinalizeAttempt()
		if pendingErr != nil {
			return pendingErr
		}
		if terminalAtEntry || pending == nil {
			return reviewPreflightError(errors.New("reviewer results are accepted only while the authority is reviewing"))
		}
	}
	var terminalReceipt reviewtransaction.CompactReceipt
	terminalReceiptExists := false
	var terminalPending *reviewtransaction.FinalizeAttempt
	terminalComplete := false
	if terminalAtEntry {
		terminalReceipt, err = state.Receipt()
		if err != nil {
			return err
		}
		terminalPending, err = store.PendingFinalizeAttempt()
		if err != nil {
			return err
		}
		terminalReceiptExists, err = inspectCompactFacadeReceipt(store.ReceiptPath(), terminalReceipt)
		if err != nil {
			requestDigest := ""
			if terminalPending != nil {
				requestDigest = terminalPending.Request.RequestDigest
			}
			return newFacadeReceiptPublicationError(ctx, root, state.LineageID, requestDigest, err)
		}
		if terminalReceiptExists {
			if terminalPending == nil {
				terminalComplete = true
			}
		}
		if !terminalReceiptExists {
			if *lineage != state.LineageID || strings.TrimSpace(*lineage) != *lineage {
				return errors.New("receipt publication replay requires the exact explicit --lineage")
			}
		}
	}
	reviewerResults, err := readFacadeReviewerResults(resultPaths)
	if err != nil {
		return reviewPreflightError(err)
	}
	if len(resultArtifacts) != 0 {
		reviewerResults, err = readFacadeReviewerArtifacts(ctx, root, resultArtifacts, store.Dir, state, record.Revision)
		if err != nil {
			return reviewPreflightError(err)
		}
	}
	if len(resultArtifactFiles) != 0 {
		manifests := make([]string, len(resultArtifactFiles))
		for index, path := range resultArtifactFiles {
			payload, readErr := readFacadeArtifactManifest(ctx, path)
			if readErr != nil {
				return reviewPreflightError(fmt.Errorf("read reviewer artifact manifest %d: %w", index+1, readErr))
			}
			payload = bytes.TrimPrefix(payload, []byte("\xef\xbb\xbf"))
			manifests[index] = string(payload)
		}
		reviewerResults, err = readFacadeReviewerArtifacts(ctx, root, manifests, store.Dir, state, record.Revision)
		if err != nil {
			return reviewPreflightError(err)
		}
	}
	if *capturedResults {
		reviewerResults, err = readCapturedReviewerResults(ctx, root, store.Dir, state, record.Revision)
		if err != nil {
			return reviewPreflightError(err)
		}
	}
	var validation *facadeValidationResult
	if strings.TrimSpace(*validationPath) != "" {
		validation = &facadeValidationResult{}
		if err := readFacadeJSON(*validationPath, validation); err != nil {
			return reviewPreflightError(fmt.Errorf("read targeted validation: %w", err))
		}
	}
	var refuter facadeRefuterResult
	if strings.TrimSpace(*refuterPath) != "" {
		if err := readFacadeJSON(*refuterPath, &refuter); err != nil {
			return reviewPreflightError(fmt.Errorf("read refuter outcomes: %w", err))
		}
	}
	var evidence []byte
	if strings.TrimSpace(*evidencePath) != "" {
		evidence, err = readFacadeBytes(*evidencePath)
		if err != nil {
			return reviewPreflightError(fmt.Errorf("read final review evidence: %w", err))
		}
	}
	if *capturedEvidence {
		evidence, err = readCapturedFinalEvidence(store.Dir, state, record.Revision)
		if err != nil {
			return reviewPreflightError(err)
		}
	}
	// A lineage-only finalize call at StateValidating with no request evidence
	// must not silently ignore canonical evidence a separate `review
	// capture-evidence` call already bound to this exact authority. Consume it
	// on the identical bytes path --captured-evidence uses, so the request is
	// a real transition instead of a no-op (1663).
	if len(evidence) == 0 && !*capturedEvidence && state.State == reviewtransaction.StateValidating {
		if captured, captureErr := readCapturedFinalEvidence(store.Dir, state, record.Revision); captureErr == nil {
			evidence = captured
		}
	}
	if terminalComplete {
		if err := reviewFacadeSyncDirectory(filepath.Dir(store.FinalizeAttemptJournalPath())); err != nil {
			return fmt.Errorf("sync completed finalize journal directory: %w", err)
		}
		return encodeCompactFacadeFinalize(stdout, negotiated, *actionEligibility, *nextTransition, state, record.Revision, store, "validate delivery with gentle-ai review validate --gate <gate>", reviewFinalizeOutputContext{Context: ctx, Repo: root})
	}
	var attempt reviewtransaction.FinalizeAttempt
	attemptLoaded := false
	var pendingAtEntry *reviewtransaction.FinalizeAttempt
	if !terminalAtEntry {
		pendingAtEntry, err = store.PendingFinalizeAttempt()
		if err != nil {
			return err
		}
		if index := facadeFinalizeTransitionIndex(pendingAtEntry, record.Revision); index >= 0 {
			replayEvidence := evidence
			if len(replayEvidence) == 0 && facadeNativeLowRiskCandidate(state) {
				replayEvidence, err = prepareFacadeNativeLowRiskVerification(ctx, root, state)
				if err != nil {
					return reviewPreflightError(err)
				}
			}
			replayRequest := facadeFinalizeAttemptRequestForCandidate(record, state.CurrentSnapshot, reviewerResults, validation, refuter, replayEvidence, *correctionLines, *failed)
			attempt, attemptLoaded, err = store.ReconcileFinalizeAttempt(ctx, replayRequest)
			if err != nil {
				return err
			}
			if index == len(attempt.Transitions)-1 {
				if err := store.CompleteFinalizeAttempt(attempt.Request.RequestDigest); err != nil {
					return err
				}
				return encodeCompactFacadeFinalize(stdout, negotiated, *actionEligibility, *nextTransition, state, record.Revision, store, "continue the current review state", reviewFinalizeOutputContext{Context: ctx, Repo: root})
			}
		}
	}
	if !terminalAtEntry && pendingAtEntry == nil {
		if err := (reviewtransaction.SnapshotBuilder{Repo: root}).ValidateEvidence(ctx, state.CurrentSnapshot); err != nil {
			// Keep negotiated Git failures classified as preflight/not_started.
			return reviewPreflightRefusal(reviewPreflightStaleTargetReason,
				fmt.Errorf("validate FINALIZE current snapshot: %v", err))
		}
	}
	plan, err := prepareFacadeFinalizePlan(ctx, root, record.Revision, state, reviewerResults, refuter, validation, evidence, *correctionLines, *failed)
	if err != nil {
		return reviewPreflightError(err)
	}
	// A zero-transition next-transition request is a read-only routing projection;
	// correction routing may intentionally describe a live target not frozen yet.
	if !terminalAtEntry && pendingAtEntry == nil && (len(plan.Transitions) > 0 || !*nextTransition) {
		if err := (reviewtransaction.SnapshotBuilder{Repo: root}).ValidateLiveSnapshot(ctx, plan.Candidate); err != nil {
			return reviewPreflightRefusal(reviewPreflightStaleTargetReason,
				fmt.Errorf("validate FINALIZE live target: %v", err))
		}
	}
	if !terminalAtEntry && pendingAtEntry == nil && len(plan.Transitions) == 0 {
		// A `--next-transition` request is a deliberate read-only routing
		// projection (see the ValidateLiveSnapshot guard above) and must keep
		// reporting the current state plus what to do next, never an error.
		// Without it, a StateValidating call that consumed no evidence at all
		// — neither supplied, captured out of band, nor eligible for native
		// low-risk auto-verification — is the genuine no-op 1663/1788 exist
		// for: it must say so instead of silently reporting success.
		if !*nextTransition && state.State == reviewtransaction.StateValidating && len(plan.Evidence) == 0 {
			return reviewPreflightError(&ErrReviewFinalizeNoTransition{LineageID: state.LineageID})
		}
		return encodeCompactFacadeFinalize(stdout, negotiated, *actionEligibility, *nextTransition, state, record.Revision, store, "continue the current review state", reviewFinalizeOutputContext{Context: ctx, Repo: root})
	}
	request := facadeFinalizeAttemptRequestForCandidate(record, plan.Candidate, reviewerResults, validation, refuter, plan.Evidence, *correctionLines, *failed)
	if !terminalAtEntry && pendingAtEntry != nil && !attemptLoaded {
		attempt, attemptLoaded, err = store.ReconcileFinalizeAttempt(ctx, request)
		if err != nil {
			return err
		}
	}
	if terminalAtEntry {
		if terminalPending != nil {
			attempt = *terminalPending
		} else {
			attempt, err = facadePendingFinalizeAttempt(store, request)
		}
	} else if !attemptLoaded {
		attempt, _, err = store.ReconcileFinalizeAttempt(ctx, request)
	}
	if err != nil {
		return err
	}
	requestDigest := attempt.Request.RequestDigest
	defer func() {
		if returnErr == nil {
			completionErr := store.CompleteFinalizeAttempt(requestDigest)
			if completionErr != nil && facadeTerminalState(state.State) {
				returnErr = newFacadeReceiptPublicationError(ctx, root, state.LineageID, requestDigest, completionErr)
			} else {
				returnErr = completionErr
			}
		}
	}()
	plannedRevisions := make([]string, len(plan.Transitions))
	expectedRevision := record.Revision
	for index, transition := range plan.Transitions {
		planned, err := store.PlanFinalizeAttemptTransition(requestDigest, transition.Operation, expectedRevision, transition.State)
		if err != nil {
			return err
		}
		plannedRevisions[index] = planned
		expectedRevision = planned
	}
	for index, transition := range plan.Transitions {
		planned := plannedRevisions[index]
		if err := reviewFacadePlannedTransitionHook(ctx, root, transition.Operation, planned); err != nil {
			return err
		}
		revision, err := store.ReplaceContext(ctx, record.Revision, transition.Operation, transition.State)
		if err != nil {
			return err
		}
		if revision != planned {
			return errors.New("compact finalize transition did not match its planned revision")
		}
		progress.record(transition.State.LineageID, revision)
		if err := reviewFacadeCommittedTransitionHook(ctx, root, transition.Operation, revision); err != nil {
			return err
		}
		record.Revision, record.State, state = revision, transition.State, transition.State
	}

	if state.State != reviewtransaction.StateApproved && state.State != reviewtransaction.StateEscalated {
		return encodeCompactFacadeFinalize(stdout, negotiated, *actionEligibility, *nextTransition, state, record.Revision, store, "continue the current review state", reviewFinalizeOutputContext{Context: ctx, Repo: root})
	}
	if terminalAtEntry && terminalReceiptExists {
		return encodeCompactFacadeFinalize(stdout, negotiated, *actionEligibility, *nextTransition, state, record.Revision, store, "validate delivery with gentle-ai review validate --gate <gate>", reviewFinalizeOutputContext{Context: ctx, Repo: root})
	}
	receipt := terminalReceipt
	if !terminalAtEntry {
		receipt, err = state.Receipt()
		if err != nil {
			return err
		}
	}
	if err := writeCompactFacadeReceipt(ctx, store, receipt); err != nil {
		return newFacadeReceiptPublicationError(ctx, root, state.LineageID, requestDigest, err)
	}
	published, err := inspectCompactFacadeReceipt(store.ReceiptPath(), receipt)
	if err != nil {
		return newFacadeReceiptPublicationError(ctx, root, state.LineageID, requestDigest, err)
	}
	if !published {
		return newFacadeReceiptPublicationError(ctx, root, state.LineageID, requestDigest, errors.New("receipt writer did not materialize the derived receipt"))
	}
	if err := store.MarkFinalizeAttemptReceiptPublished(requestDigest); err != nil {
		return err
	}
	return encodeCompactFacadeFinalize(stdout, negotiated, *actionEligibility, *nextTransition, state, record.Revision, store, "validate delivery with gentle-ai review validate --gate <gate>", reviewFinalizeOutputContext{Context: ctx, Repo: root})
}

func facadeFinalizeTransitionIndex(attempt *reviewtransaction.FinalizeAttempt, revision string) int {
	if attempt == nil {
		return -1
	}
	for index, transition := range attempt.Transitions {
		if transition.Revision == revision {
			return index
		}
	}
	return -1
}

func facadePendingFinalizeAttempt(store reviewtransaction.CompactStore, request reviewtransaction.FinalizeAttemptRequest) (reviewtransaction.FinalizeAttempt, error) {
	pending, err := store.PendingFinalizeAttempt()
	if err != nil {
		return reviewtransaction.FinalizeAttempt{}, err
	}
	if pending != nil {
		return *pending, nil
	}
	attempt, _, err := store.BeginFinalizeAttempt(context.Background(), request)
	return attempt, err
}

func facadeFinalizeAttemptRequest(record reviewtransaction.CompactRecord, results []facadeReviewerResult, validation *facadeValidationResult, refuter facadeRefuterResult, evidence []byte, correctionLines int, failed bool) reviewtransaction.FinalizeAttemptRequest {
	return facadeFinalizeAttemptRequestForCandidate(record, record.State.CurrentSnapshot, results, validation, refuter, evidence, correctionLines, failed)
}

func facadeFinalizeAttemptRequestForCandidate(record reviewtransaction.CompactRecord, candidate reviewtransaction.Snapshot, results []facadeReviewerResult, validation *facadeValidationResult, refuter facadeRefuterResult, evidence []byte, correctionLines int, failed bool) reviewtransaction.FinalizeAttemptRequest {
	request := reviewtransaction.FinalizeAttemptRequest{
		LineageID: record.State.LineageID, ExpectedRevision: record.Revision,
		CandidateDigest:          reviewtransaction.FinalizeAttemptValueDigest("candidate", candidate),
		ReviewerResultsDigest:    reviewtransaction.FinalizeAttemptValueDigest("reviewer-results", results),
		CorrectionForecastDigest: reviewtransaction.FinalizeAttemptValueDigest("correction-forecast", correctionLines),
		ValidationDigest:         reviewtransaction.FinalizeAttemptValueDigest("validation", validation),
		RefuterDigest:            reviewtransaction.FinalizeAttemptValueDigest("refuter", refuter),
		EvidenceDigest:           reviewtransaction.FinalizeAttemptValueDigest("evidence", evidence),
		FailedDigest:             reviewtransaction.FinalizeAttemptValueDigest("failed", failed),
	}
	request.RequestDigest = reviewtransaction.FinalizeAttemptRequestDigest(request)
	return request
}

type facadeFinalizeTransition struct {
	Operation string
	State     reviewtransaction.CompactState
}
type facadeFinalizePlan struct {
	Transitions []facadeFinalizeTransition
	Candidate   reviewtransaction.Snapshot
	Evidence    []byte
}

// prepareFacadeFinalizePlan performs every deterministic validation before the
// attempt journal exists. Its states are the only states later admitted and
// written through the write-ahead journal.
func prepareFacadeFinalizePlan(ctx context.Context, repo, revision string, state reviewtransaction.CompactState, results []facadeReviewerResult, refuter facadeRefuterResult, validation *facadeValidationResult, evidence []byte, correctionLines int, failed bool) (facadeFinalizePlan, error) {
	entryState, entryProposed := state.State, state.ProposedCorrectionLines != nil
	plan := facadeFinalizePlan{Transitions: []facadeFinalizeTransition{}, Candidate: state.CurrentSnapshot, Evidence: evidence}
	appendState := func(operation string) {
		plan.Transitions = append(plan.Transitions, facadeFinalizeTransition{Operation: operation, State: state})
	}
	if state.State == reviewtransaction.StateReviewing {
		input, err := prepareCompactReviewerResults(state, results, refuter, facadeRepositoryEvidence{ctx: ctx, repo: repo})
		if err != nil {
			return plan, err
		}
		if err := state.CompleteReview(input); err != nil {
			return plan, err
		}
		appendState("review/complete-review")
	}
	if state.State == reviewtransaction.StateCorrectionRequired && state.ProposedCorrectionLines == nil && correctionLines > 0 {
		if err := state.BeginCorrection(correctionLines); err != nil {
			return plan, err
		}
		appendState("review/begin-fix")
	}
	if state.State == reviewtransaction.StateCorrectionRequired && validation != nil && entryState == reviewtransaction.StateCorrectionRequired && entryProposed {
		if err := rejectFacadeCorrectionUntracked(ctx, repo, state); err != nil {
			return plan, err
		}
		fix, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{Kind: reviewtransaction.TargetFixDiff, Projection: state.InitialSnapshot.Projection, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
		if err != nil {
			return plan, err
		}
		actual, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(ctx, fix)
		if err != nil {
			return plan, err
		}
		request, err := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(ctx, repo, state, revision, fix)
		if err != nil {
			return plan, err
		}
		native, err := validation.compact(reviewtransaction.FixDeltaHashForSnapshot(fix), state.FixFindingIDs, request)
		if err != nil {
			return plan, err
		}
		if err := state.CompleteCorrection(fix, actual, native); err != nil {
			return plan, err
		}
		plan.Candidate = fix
		appendState("review/complete-fix")
	}
	if state.State == reviewtransaction.StateValidating {
		if len(plan.Evidence) == 0 && facadeNativeLowRiskCandidate(state) {
			generated, err := prepareFacadeNativeLowRiskVerification(ctx, repo, state)
			if err != nil {
				return plan, err
			}
			plan.Evidence = generated
		}
		if len(plan.Evidence) > 0 {
			if err := state.CompleteVerification(plan.Evidence, !failed); err != nil {
				return plan, err
			}
			appendState("review/complete-verification")
		}
	}
	if state.State == reviewtransaction.StateValidating && len(plan.Evidence) == 0 {
		return plan, nil
	}
	return plan, nil
}

// ErrReviewFinalizeNoTransition reports that a StateValidating finalize call
// had no evidence to consume — neither supplied on the request, captured out
// of band, nor eligible for native low-risk auto-verification — and
// therefore produced no transition at all. It replaces the old silent
// success ("continue the current review state") for exactly this shape,
// covering both the case where verification evidence was never captured and
// the case where a prior finalize already consumed it. The message names the
// concrete two-step escape verbatim rather than a prose description.
type ErrReviewFinalizeNoTransition struct {
	LineageID string
}

func (err *ErrReviewFinalizeNoTransition) Error() string {
	return fmt.Sprintf(
		"finalize for lineage %q had no verification evidence to consume and made no transition; capture it first with `gentle-ai review capture-evidence`, then run `gentle-ai review finalize --lineage %s --captured-evidence`",
		err.LineageID, err.LineageID,
	)
}

func facadeNativeLowRiskCandidate(state reviewtransaction.CompactState) bool {
	return (state.State == reviewtransaction.StateReviewing || state.State == reviewtransaction.StateValidating) &&
		state.RiskLevel == reviewtransaction.RiskLow && len(state.SelectedLenses) == 0
}

func prepareFacadeNativeLowRiskVerification(ctx context.Context, repo string, state reviewtransaction.CompactState) ([]byte, error) {
	if err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, state.InitialSnapshot); err != nil {
		return nil, fmt.Errorf("revalidate frozen low-risk snapshot: %w", err)
	}
	target := reviewtransaction.Target{
		Kind: state.InitialSnapshot.Kind, Projection: state.InitialSnapshot.Projection,
		IntendedUntracked: append([]string{}, state.InitialSnapshot.IntendedUntracked...),
	}
	switch target.Kind {
	case reviewtransaction.TargetCurrentChanges:
	case reviewtransaction.TargetBaseDiff, reviewtransaction.TargetBaseWorkspaceOverlay:
		target.BaseRef = state.InitialSnapshot.BaseTree
	default:
		return nil, fmt.Errorf("native low-risk verification does not support target kind %q", target.Kind)
	}
	live, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("rebuild frozen low-risk projection: %w", err)
	}
	if live.Identity != state.InitialSnapshot.Identity || live.Identity != state.CurrentSnapshot.Identity {
		return nil, errors.New("live low-risk projection no longer matches the frozen authority")
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(ctx, live)
	if err != nil {
		return nil, fmt.Errorf("reclassify frozen low-risk projection: %w", err)
	}
	return reviewtransaction.NativeLowRiskVerificationEvidence(state, assessment)
}

func facadeTerminalState(state reviewtransaction.State) bool {
	return state == reviewtransaction.StateApproved || state == reviewtransaction.StateEscalated
}

func facadeFinalizeReplayInputsEmpty(results, artifacts, artifactFiles []string, capturedResults, capturedEvidence bool, validation, refuter, evidence string, correctionLines int, failed bool, trace string) bool {
	return len(results) == 0 && len(artifacts) == 0 && len(artifactFiles) == 0 && !capturedResults && !capturedEvidence && strings.TrimSpace(validation) == "" && strings.TrimSpace(refuter) == "" &&
		strings.TrimSpace(evidence) == "" && correctionLines == 0 && !failed && strings.TrimSpace(trace) == ""
}

func inspectCompactFacadeReceipt(path string, expected reviewtransaction.CompactReceipt) (bool, error) {
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, &reviewtransaction.ImmutablePublicationConflictError{Cause: errors.New("existing receipt cannot be read")}
	}
	existing, err := reviewtransaction.ParseCompactReceipt(payload)
	if err != nil {
		return false, &reviewtransaction.ImmutablePublicationConflictError{Cause: errors.New("existing receipt is invalid")}
	}
	if !reflect.DeepEqual(existing, expected) {
		return false, &reviewtransaction.ImmutablePublicationConflictError{Cause: errors.New("existing receipt differs from terminal authority")}
	}
	return true, nil
}

func newFacadeReceiptPublicationError(ctx context.Context, root, lineage, requestDigest string, cause error) error {
	replayability := string(reviewtransaction.ReplayabilityExactReplaySafe)
	clause := ""
	var conflict *reviewtransaction.ImmutablePublicationConflictError
	if errors.As(cause, &conflict) {
		replayability = string(reviewtransaction.ReplayabilityManualActionRequired)
		// Confirmed genuine deadlock (organic-dx tasks.md 3b.8): none of
		// reclaim (incomplete entries only), reconcile-authority (invalid
		// recovery-successor edges only), or repair's AuthorityRepairConflicting
		// (a different, multi-lineage-alias conflict class) resolves this
		// shape. A defect report is this fault's designed exit for this release.
		clause = reviewGenerateToolFaultDefectReport(ctx, root, reviewDefectReportInput{
			Operation:            "review finalize --lineage",
			ReasonCode:           "receipt_publication_conflict",
			ErrorMessage:         cause.Error(),
			TerminalPrecondition: "an already-terminal-committed review authority's derived receipt differs from an existing immutable receipt at the same storage path; there is no command to reconcile it",
			StateIdentifiers:     map[string]string{"lineage": lineage, "request_digest": requestDigest},
		})
	}
	return &ReviewFacadeReceiptPublicationError{
		MutationOutcome: "committed", Replayability: replayability,
		LineageID: lineage, RequestDigest: requestDigest, Cause: cause, DefectReportClause: clause,
	}
}

func facadeFinalizeReplayRequestDigest(lineage, revision string, receipt reviewtransaction.CompactReceipt) string {
	return facadeValueHash("finalize-replay-request", struct {
		Schema        string                           `json:"schema"`
		Operation     string                           `json:"operation"`
		LineageID     string                           `json:"lineage_id"`
		StoreRevision string                           `json:"store_revision"`
		Receipt       reviewtransaction.CompactReceipt `json:"receipt"`
	}{
		Schema: "gentle-ai.review-finalize-replay-request/v1", Operation: "review/finalize",
		LineageID: lineage, StoreRevision: revision, Receipt: receipt,
	})
}

func RunReviewFacadeValidate(args []string, stdout io.Writer) error {
	return runReviewFacadeValidate(context.Background(), args, stdout)
}

func runReviewFacadeValidate(ctx context.Context, args []string, stdout io.Writer) error {
	if err := validateReviewTransitionSelectorFlagCounts(args, ReviewIntegrationOperationValidate); err != nil {
		return err
	}
	flags := newReviewFlagSet("review validate", stdout, "Auto-discover authoritative review state and receipt, then validate them against live Git evidence.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	lineage := flags.String("lineage", "", "optional lineage override when discovery is ambiguous")
	gate := flags.String("gate", "", "lifecycle gate: post-apply, pre-commit, pre-push, pre-pr, or release")
	baseRef := flags.String("base-ref", "", "optional expected remote publication base for pre-pr")
	ciAttestation := flags.String("pre-pr-ci-attestation", "", "signed exact-merged-tree CI attestation for a compatible base advance")
	policy := flags.String("policy", "", "explicit custom policy containing compatible-base CI trust")
	releaseConfiguration := flags.String("release-configuration", "", "release configuration artifact")
	releaseGenerated := flags.String("release-generated", "", "generated artifact manifest")
	releaseProvenance := flags.String("release-provenance", "", "release provenance artifact")
	releaseBoundary := flags.String("release-publication-boundary", "", "sealed publication boundary artifact")
	releaseFreshness := flags.String("release-evidence-freshness", "", "current release evidence freshness artifact")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review validate argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*gate) == "" {
		return fmt.Errorf("review validate requires --gate: one of %s", strings.Join(reviewIntegrationGateNames(), ", "))
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	gateInput := reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GateKind(*gate), BaseRef: *baseRef, PrePRCIAttestation: *ciAttestation,
		ReleaseConfiguration: *releaseConfiguration, ReleaseGenerated: *releaseGenerated,
		ReleaseProvenance: *releaseProvenance, ReleasePublicationBoundary: *releaseBoundary,
		ReleaseEvidenceFreshness: *releaseFreshness,
	}
	if strings.TrimSpace(*ciAttestation) != "" {
		gateInput.PolicyArtifact = *policy
	}
	// A pre-push event that transfers no commit is evaluated BEFORE receipt
	// discovery, because "nothing is being delivered" is a fact about the
	// repository and the push destination, not about any receipt. Deriving it
	// first is also what keeps the allow honest: no receipt is read, so none can
	// be reported as governing, and the emitted context binds no lineage, no
	// trees and no artifact hashes. Only the complete derivation in
	// PrePushDeliversNothing can produce it; every failure keeps the ordinary
	// path and its denials.
	if gateInput.Gate == reviewtransaction.GatePrePush {
		nothingDelivered, deliveryErr := reviewtransaction.PrePushDeliversNothing(ctx, root, *baseRef)
		if deliveryErr == nil && nothingDelivered {
			return emitFacadeGateEvaluationNegotiated(stdout, reviewtransaction.NativeGateEvaluation{
				Result:  reviewtransaction.GateAllow,
				Reason:  reviewEmptyPublicationRangeReason,
				Context: reviewtransaction.GateContext{Gate: gateInput.Gate},
			}, negotiated)
		}
	}
	compactStore, compactRecord, compactErr := discoverCompactFacadeGateReview(ctx, root, *lineage, gateInput)
	if compactErr == nil {
		contested := false
		if strings.TrimSpace(*lineage) != "" {
			if _, _, _, legacyErr := discoverFacadeReview(ctx, root, *lineage, true); legacyErr == nil {
				contested = true
			}
		} else if legacyExactFacadeGateLineages(ctx, root, gateInput) > 0 {
			contested = true
		}
		if contested {
			// Two independent authority systems both claim to govern this exact
			// candidate. With reviews ON that refusal is correct: answering would
			// mean silently picking one store over the other. With reviews OFF
			// nothing is picked — the gate names neither store as governing,
			// emits no lineage and no receipt, and defers to ordinary repository
			// policy, so the contest has no delivery consequence until the
			// operator turns reviews back on and resolves it.
			mixed := &ReviewReceiptDiscoveryError{Kind: ReviewReceiptAmbiguous, Detail: errReviewMixedCompactLegacyAuthority.Error()}
			if reviewDeliveryDisposition(ctx, root, false) == reviewtransaction.RDDDeliveryDisabledUnmanaged {
				return emitDisabledUnmanagedDelivery(stdout, gateInput.Gate, mixed, negotiated)
			}
			return errReviewMixedCompactLegacyAuthority
		}
		payload, err := os.ReadFile(compactStore.ReceiptPath())
		if err != nil {
			return errors.New(reviewFacadeReceiptNotAvailableReason(compactRecord.State.LineageID))
		}
		receipt, err := reviewtransaction.ParseCompactReceipt(payload)
		if err != nil {
			return fmt.Errorf("parse compact review receipt: %w", err)
		}
		input := gateInput
		input.LineageID = compactRecord.State.LineageID
		input.IntendedUntracked = append([]string(nil), compactRecord.State.InitialSnapshot.IntendedUntracked...)
		evaluation := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, input)
		if gateInput.Gate == reviewtransaction.GatePrePR && strings.TrimSpace(*lineage) == "" &&
			evaluation.Context.Denial != nil && evaluation.Context.Denial.Stage == "receipt-binding" && evaluation.Context.Denial.Code == "base-mismatch" {
			if composed, attempted := reviewtransaction.EvaluateCompactPrePRChain(ctx, root, gateInput); attempted {
				return emitFacadeGateEvaluationNegotiated(stdout, composed, negotiated)
			}
		}
		return emitFacadeGateEvaluationNegotiated(stdout, evaluation, negotiated)
	}
	var compactDiscovery *ReviewReceiptDiscoveryError
	if gateInput.Gate == reviewtransaction.GatePrePR && strings.TrimSpace(*lineage) == "" &&
		errors.As(compactErr, &compactDiscovery) && compactDiscovery.Kind != ReviewAuthorityCorrupted && compactDiscovery.Kind != ReviewReceiptMissing {
		if evaluation, attempted := reviewtransaction.EvaluateCompactPrePRChain(ctx, root, gateInput); attempted {
			return emitFacadeGateEvaluationNegotiated(stdout, evaluation, negotiated)
		}
	}
	// The kill switch is consulted BEFORE the negotiation branch and for every
	// discovery outcome. While reviews are off, review-driven development does
	// not exist, so nothing it discovered — a missing receipt, a stale one, an
	// unresolvable target, competing authority, or a damaged inventory — governs
	// this candidate, and the gate reports rather than vetoes. It never
	// approves: `allowed` stays false, no receipt or authority is invented, and
	// the command exits successfully only because ordinary repository policy —
	// hooks, tests, CI — decides instead. Fencing this behind `!negotiated` used
	// to mean the identical repository exited 0 for a human and 1 for any agent
	// driving the negotiated contract.
	var targetResolution *reviewtransaction.GateTargetResolutionError
	var disabledDiscovery *ReviewReceiptDiscoveryError
	if errors.As(compactErr, &targetResolution) {
		// issue-1832: a repository with no upstream has no publication boundary
		// to derive at all. It has no receipt to lose here, only a target it
		// cannot compute while off.
		disabledDiscovery = &ReviewReceiptDiscoveryError{Kind: ReviewReceiptTargetUnresolvable, Detail: targetResolution.Error()}
	} else {
		_ = errors.As(compactErr, &disabledDiscovery)
	}
	if disabledDiscovery != nil &&
		reviewDeliveryDisposition(ctx, root, false) == reviewtransaction.RDDDeliveryDisabledUnmanaged {
		return emitDisabledUnmanagedDelivery(stdout, gateInput.Gate, disabledDiscovery, negotiated)
	}
	if !negotiated {
		if targetResolution != nil {
			return emitFacadeGateEvaluationNegotiated(stdout, reviewtransaction.NativeGateEvaluation{
				Result: reviewtransaction.GateInvalidated, Reason: targetResolution.Error(), Cause: compactErr,
				Context: reviewtransaction.GateContext{
					Gate: gateInput.Gate, Denial: &reviewtransaction.GateDenial{Stage: "target-resolution", Code: "target_resolution_failed"},
				},
			}, false)
		}
		var discovery *ReviewReceiptDiscoveryError
		if errors.As(compactErr, &discovery) {
			// Reviews are on, so the gate holds its authority: no receipt
			// governs this candidate and that is a denial, unchanged.
			result := reviewtransaction.GateInvalidated
			reason := discovery.Error()
			context := reviewtransaction.GateContext{
				Gate: gateInput.Gate, Denial: &reviewtransaction.GateDenial{Stage: "receipt-discovery", Code: string(discovery.Kind)},
			}
			if discovery.Kind == ReviewReceiptScopeChanged {
				result = reviewtransaction.GateScopeChanged
				if discovery.Context != nil {
					context = *discovery.Context
				}
			}
			return emitFacadeGateEvaluationNegotiated(stdout, reviewtransaction.NativeGateEvaluation{
				Result: result, Reason: reason, Context: context,
			}, false)
		}
	}

	_, chain, artifacts, legacyErr := discoverFacadeReview(ctx, root, *lineage, true)
	if legacyErr != nil {
		return compactErr
	}
	tx := chain.Records[len(chain.Records)-1].Transaction
	validateArgs := []string{"--cwd", root, "--receipt", artifacts.receipt, "--lineage", tx.LineageID, "--gate", *gate}
	if strings.TrimSpace(*baseRef) != "" {
		validateArgs = append(validateArgs, "--base-ref", *baseRef)
	}
	if strings.TrimSpace(*ciAttestation) != "" {
		validateArgs = append(validateArgs, "--pre-pr-ci-attestation", *ciAttestation)
		if _, err := os.Stat(artifacts.policy); err == nil {
			validateArgs = append(validateArgs, "--policy", artifacts.policy)
		}
	}
	for _, item := range [][2]string{{"--release-configuration", *releaseConfiguration}, {"--release-generated", *releaseGenerated}, {"--release-provenance", *releaseProvenance}, {"--release-publication-boundary", *releaseBoundary}, {"--release-evidence-freshness", *releaseFreshness}} {
		if strings.TrimSpace(item[1]) != "" {
			validateArgs = append(validateArgs, item[0], item[1])
		}
	}
	for _, path := range tx.Snapshot.IntendedUntracked {
		validateArgs = append(validateArgs, "--intended-untracked", path)
	}
	return runFacadeLegacyValidateNegotiated(ctx, validateArgs, stdout, negotiated)
}

func discoverCompactFacadeGateReview(ctx context.Context, repo, lineage string, input reviewtransaction.NativeGateRequestInput) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
	if strings.TrimSpace(lineage) != "" {
		return discoverCompactFacadeReview(ctx, repo, lineage, true)
	}
	report, err := reviewtransaction.InventoryAuthority(ctx, repo)
	if (err != nil || !report.Complete || !report.Authoritative) && !reviewAuthorityCorruptionConfinedToLegacyEntries(report, err) {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{
			Kind: ReviewAuthorityCorrupted, Category: reviewAuthorityCauseCategory(report, err),
			Detail: reviewAuthorityCorruptionDetail(ctx, repo),
		}
	}
	stores, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{
			Kind: ReviewAuthorityCorrupted, Category: "record_or_graph_invalid",
			Detail: reviewAuthorityCorruptionDetail(ctx, repo),
		}
	}
	type candidate struct {
		store      reviewtransaction.CompactStore
		record     reviewtransaction.CompactRecord
		assessment reviewtransaction.CompactGateTargetAssessment
		context    reviewtransaction.GateContext
	}
	exact := []candidate{}
	scopeChanged := []candidate{}
	scopeWithoutContext := []string{}
	assessmentUnknown := []string{}
	// deliveryShape collects receipts whose assessment failed only on the
	// deterministic pre-push one-commit delivery rule: a typed statement about
	// candidate shape versus the reviewed receipt, made over an inventory the
	// authority check above already proved healthy. It classifies with the
	// scope-changed family; every other assessment failure stays fail-closed
	// as corruption.
	type deliveryShapeMismatch struct {
		lineage string
		context reviewtransaction.GateContext
		detail  string
	}
	deliveryShape := []deliveryShapeMismatch{}
	type targetResolutionFailure struct {
		lineage string
		err     error
	}
	targetResolution := []targetResolutionFailure{}
	terminalCount := 0
	allLineages := []string{}
	// organic-dx Phase 3d: as terminal lineages accumulate, most of them can
	// never govern the live candidate again, yet every gate call re-assessed
	// every one of them with AssessCompactGateTarget's several git
	// subprocesses. For GatePreCommit only (its staged projection makes the
	// live resolution provably identical across every ordinary leaf --
	// see CompactPreCommitDiscoveryBaseline's doc comment), resolve that
	// live candidate ONCE and use it to skip the expensive per-leaf call for
	// any leaf whose genesis paths are disjoint from it. A leaf that fails
	// the cheap, exact eligibility check still gets the full, unmodified
	// assessment below -- this can only ever SKIP work whose outcome is
	// already provably CompactGateTargetUnrelated (contributes to no
	// discovery bucket), never change which bucket a leaf lands in.
	var (
		preCommitBaseline      reviewtransaction.CompactPreCommitDiscoveryBaseline
		preCommitBaselineOK    bool
		preCommitBaselineTried bool
	)
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
		}
		if !facadeTerminalState(record.State.State) {
			continue
		}
		payload, readErr := os.ReadFile(store.ReceiptPath())
		if readErr != nil {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
		}
		receipt, parseErr := reviewtransaction.ParseCompactReceipt(payload)
		derived, deriveErr := record.State.Receipt()
		if parseErr != nil || deriveErr != nil || !reviewtransaction.CompactReceiptEqual(receipt, derived) {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
		}
		terminalCount++
		allLineages = append(allLineages, record.State.LineageID)
		if input.Gate == reviewtransaction.GatePreCommit {
			if !preCommitBaselineTried {
				preCommitBaselineTried = true
				if baseline, baselineErr := reviewtransaction.BuildCompactPreCommitDiscoveryBaseline(ctx, repo); baselineErr == nil {
					preCommitBaseline, preCommitBaselineOK = baseline, true
				}
			}
			if preCommitBaselineOK && reviewtransaction.CompactLeafProvablyUnrelatedToPreCommitBaseline(record.State, preCommitBaseline) {
				continue
			}
		}
		candidateInput := input
		candidateInput.LineageID = record.State.LineageID
		candidateInput.IntendedUntracked = append([]string(nil), record.State.CurrentSnapshot.IntendedUntracked...)
		assessment, assessErr := assessCompactGateTargetForDiscovery(ctx, repo, record.State, candidateInput)
		if assessErr != nil {
			var targetErr *reviewtransaction.GateTargetResolutionError
			if errors.As(assessErr, &targetErr) {
				targetResolution = append(targetResolution, targetResolutionFailure{lineage: record.State.LineageID, err: assessErr})
				continue
			}
			// deliveryContext binds the frozen receipt values — the expected
			// side of the mismatch — exactly as evidence-bearing denials do,
			// so the real cause stays discoverable regardless of which
			// deterministic delivery-shape producer failed.
			deliveryContext := func(code string) reviewtransaction.GateContext {
				context := reviewtransaction.GateContext{
					Gate: input.Gate, LineageID: record.State.LineageID, Generation: record.State.Generation,
					StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
					BaseTree: record.State.CurrentSnapshot.BaseTree, CandidateTree: record.State.CurrentSnapshot.CandidateTree, PathsDigest: record.State.CurrentSnapshot.PathsDigest,
					FixDeltaHash: record.State.FixDeltaHash, PolicyHash: record.State.PolicyHash,
					LedgerHash: record.State.LedgerHash(), EvidenceHash: record.State.EvidenceHash,
					Denial: &reviewtransaction.GateDenial{Stage: "delivery-derivation", Code: code},
				}
				// Both producers below fail BEFORE an actual snapshot exists,
				// so the receipt-binding derivation is not callable here. The
				// delivery-shape derivation builds its own evidence from the
				// publication boundary; when it cannot -- an already fully
				// published delivery, an unresolvable boundary, a receipt that
				// never armed the one-commit rule -- ScopeChange stays nil and
				// the denial keeps the honest terminal fallback instead of
				// naming a recovery that would dead-end.
				if diagnostics, diagnosticsErr := reviewtransaction.CompactDeliveryShapeScopeChangeDiagnostics(
					ctx, repo, record.State, record.Revision, input.Gate,
				); diagnosticsErr == nil {
					context.ScopeChange = &diagnostics
				}
				return context
			}
			if errors.Is(assessErr, reviewtransaction.ErrReviewedDeliveryNotOneCommit) {
				deliveryShape = append(deliveryShape, deliveryShapeMismatch{
					lineage: record.State.LineageID, context: deliveryContext("delivery-shape-mismatch"),
					detail: reviewtransaction.ErrReviewedDeliveryNotOneCommit.Error(),
				})
				continue
			}
			// The published-delivery release blocker: the reviewed candidate's
			// base commit could not be uniquely located in the publication
			// range, because it was already published. This is a receipt that
			// stopped governing a candidate that already moved, not authority
			// damage, so it routes into the same scope-changed family as the
			// one-commit delivery-shape mismatch above.
			var deliveryBaseErr *reviewtransaction.GateDeliveryBaseResolutionError
			if errors.As(assessErr, &deliveryBaseErr) {
				deliveryShape = append(deliveryShape, deliveryShapeMismatch{
					lineage: record.State.LineageID, context: deliveryContext("delivery-base-ambiguous"),
					detail: deliveryBaseErr.Error(),
				})
				continue
			}
			assessmentUnknown = append(assessmentUnknown, record.State.LineageID)
			continue
		}
		switch assessment.Applicability {
		case reviewtransaction.CompactGateTargetExact:
			exact = append(exact, candidate{store: store, record: record, assessment: assessment})
		case reviewtransaction.CompactGateTargetScopeChanged:
			diagnostics, diagnosticsErr := reviewtransaction.CompactScopeChangeDiagnostics(ctx, repo, record.State, record.Revision, assessment.Actual, input.Gate)
			if diagnosticsErr != nil {
				scopeWithoutContext = append(scopeWithoutContext, record.State.LineageID)
				continue
			}
			scopeChanged = append(scopeChanged, candidate{
				store: store, record: record, assessment: assessment,
				context: reviewtransaction.GateContext{
					Gate: input.Gate, LineageID: record.State.LineageID, Generation: record.State.Generation,
					StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
					BaseTree: assessment.Actual.BaseTree, CandidateTree: assessment.Actual.CandidateTree, PathsDigest: assessment.Actual.PathsDigest,
					FixDeltaHash: record.State.FixDeltaHash, PolicyHash: record.State.PolicyHash,
					LedgerHash: record.State.LedgerHash(), EvidenceHash: record.State.EvidenceHash,
					Denial: &reviewtransaction.GateDenial{Stage: "receipt-binding", Code: "candidate-or-paths-mismatch"}, ScopeChange: &diagnostics,
				},
			})
		}
	}
	if len(exact) == 1 {
		return exact[0].store, exact[0].record, nil
	}
	if len(exact) > 1 {
		lineages := make([]string, len(exact))
		for index := range exact {
			lineages[index] = exact[index].record.State.LineageID
		}
		sort.Strings(lineages)
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptAmbiguous, Candidates: lineages}
	}
	if terminalCount == 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptMissing}
	}
	scopeCandidateCount := len(scopeChanged) + len(scopeWithoutContext) + len(deliveryShape)
	if scopeCandidateCount > 0 && scopeCandidateCount+len(assessmentUnknown)+len(targetResolution) > 1 {
		lineages := make([]string, 0, scopeCandidateCount+len(assessmentUnknown)+len(targetResolution))
		for index := range scopeChanged {
			lineages = append(lineages, scopeChanged[index].record.State.LineageID)
		}
		lineages = append(lineages, scopeWithoutContext...)
		for index := range deliveryShape {
			lineages = append(lineages, deliveryShape[index].lineage)
		}
		lineages = append(lineages, assessmentUnknown...)
		for _, failure := range targetResolution {
			lineages = append(lineages, failure.lineage)
		}
		sort.Strings(lineages)
		// organic-dx Phase 3c: assessmentUnknown and scopeWithoutContext mean
		// an assessment could not even be COMPLETED for that lineage -- that
		// is undecidable, not proven-stale, so their presence keeps the
		// composition NOT deterministically-stale-only. scopeChanged,
		// deliveryShape, and targetResolution are all deterministic
		// statements that the lineage does not govern this candidate.
		staleOnly := len(scopeWithoutContext) == 0 && len(assessmentUnknown) == 0
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{
			Kind: ReviewReceiptAmbiguous, Candidates: lineages, DeterministicallyStaleOnly: staleOnly,
		}
	}
	if len(scopeChanged) == 1 && len(deliveryShape) == 0 && len(scopeWithoutContext) == 0 && len(assessmentUnknown) == 0 && len(targetResolution) == 0 {
		context := scopeChanged[0].context
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{
			Kind: ReviewReceiptScopeChanged, Candidates: []string{scopeChanged[0].record.State.LineageID}, Context: &context,
		}
	}
	if len(deliveryShape) == 1 && len(scopeChanged) == 0 && len(scopeWithoutContext) == 0 && len(assessmentUnknown) == 0 && len(targetResolution) == 0 {
		context := deliveryShape[0].context
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{
			Kind: ReviewReceiptScopeChanged, Detail: deliveryShape[0].detail,
			Candidates: []string{deliveryShape[0].lineage}, Context: &context,
		}
	}
	if len(scopeWithoutContext) > 0 || len(assessmentUnknown) > 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
	}
	if len(targetResolution) == terminalCount {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, targetResolution[0].err
	}
	if len(targetResolution) > 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
	}
	sort.Strings(allLineages)
	return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptUnrelated, Candidates: allLineages}
}

// reviewAuthorityCorruptionConfinedToLegacyEntries reports whether every cause
// of a non-authoritative inventory is an invalid legacy-v1 entry, which can
// never resolve as a compact discovery candidate. Ambiguous lock residue is
// tolerated only when it belongs to such an already-invalid legacy entry;
// inventory IO/layout diagnostics, shared or live-entry ambiguous locks, reset
// residue, mixed-store collisions, and any compact-v2 problem keep
// lineage-less discovery fail-closed.
func reviewAuthorityCorruptionConfinedToLegacyEntries(report reviewtransaction.AuthorityStatusReport, inventoryErr error) bool {
	if inventoryErr != nil || len(report.Diagnostics) > 0 {
		return false
	}
	for _, lock := range report.Locks {
		if lock.Status == reviewtransaction.AuthorityLockAmbiguous && !reviewAmbiguousLockConfinedToInvalidLegacyEntry(report, lock) {
			return false
		}
	}
	confined := false
	for _, entry := range report.Entries {
		switch entry.Status {
		case reviewtransaction.AuthorityStatusInvalid:
			if entry.Version != reviewtransaction.AuthorityVersionLegacy {
				return false
			}
			confined = true
		case reviewtransaction.AuthorityStatusIncomplete, reviewtransaction.AuthorityStatusReset, reviewtransaction.AuthorityStatusCollision:
			return false
		}
	}
	return confined
}

// reviewAmbiguousLockConfinedToInvalidLegacyEntry reports whether ambiguous
// lock evidence is part of the corruption of a legacy-v1 lineage entry that
// the inventory has already classified invalid. Only that residue is confined:
// the shared compact-v2 store lock carries no owning lineage, and any lock
// attached to a live, historical, collided, or missing entry stays a
// fail-closed corruption cause because it may still guard real authority.
func reviewAmbiguousLockConfinedToInvalidLegacyEntry(report reviewtransaction.AuthorityStatusReport, lock reviewtransaction.AuthorityLockEvidence) bool {
	if lock.Version != reviewtransaction.AuthorityVersionLegacy || strings.TrimSpace(lock.LineageID) == "" {
		return false
	}
	for _, entry := range report.Entries {
		if entry.Version == reviewtransaction.AuthorityVersionLegacy && entry.LineageID == lock.LineageID {
			return entry.Status == reviewtransaction.AuthorityStatusInvalid
		}
	}
	return false
}

func reviewAuthorityCauseCategory(report reviewtransaction.AuthorityStatusReport, inventoryErr error) string {
	if inventoryErr != nil || len(report.Diagnostics) > 0 {
		return "inventory_io_or_layout"
	}
	for _, lock := range report.Locks {
		if lock.Status == reviewtransaction.AuthorityLockAmbiguous && !reviewAmbiguousLockConfinedToInvalidLegacyEntry(report, lock) {
			return "lock_ambiguous"
		}
	}
	for _, entry := range report.Entries {
		switch entry.Status {
		case reviewtransaction.AuthorityStatusReset:
			return "reset_residue"
		case reviewtransaction.AuthorityStatusInvalid, reviewtransaction.AuthorityStatusCollision:
			return "record_or_graph_invalid"
		}
	}
	for _, entry := range report.Entries {
		if entry.Status == reviewtransaction.AuthorityStatusIncomplete {
			return "incomplete_store_entry"
		}
	}
	return "inventory_incomplete"
}

func legacyExactFacadeGateLineages(ctx context.Context, repo string, input reviewtransaction.NativeGateRequestInput) int {
	stores, err := reviewtransaction.DiscoverAuthoritativeStores(ctx, repo)
	if err != nil {
		return 0
	}
	exact := 0
	for _, store := range stores {
		chain, loadErr := store.LoadChain()
		if loadErr != nil {
			continue
		}
		tx := chain.Records[len(chain.Records)-1].Transaction
		if !facadeTerminalState(tx.State) {
			continue
		}
		candidateInput := input
		candidateInput.LineageID = tx.LineageID
		candidateInput.IntendedUntracked = append([]string(nil), tx.Snapshot.IntendedUntracked...)
		request, requestErr := reviewtransaction.BuildNativeGateRequest(ctx, repo, candidateInput)
		if requestErr != nil {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(store.Dir, "artifacts", "receipt.json"))
		if readErr != nil {
			continue
		}
		receipt, parseErr := reviewtransaction.ParseReceipt(payload)
		if parseErr != nil {
			continue
		}
		authoritative, deriveErr := tx.Receipt()
		if deriveErr != nil || !reflect.DeepEqual(receipt, authoritative) {
			continue
		}
		evaluation := reviewtransaction.EvaluateNativeGate(ctx, repo, receipt, request)
		if evaluation.Result == reviewtransaction.GateAllow ||
			evaluation.Context.LineageID == receipt.LineageID && evaluation.Context.CandidateTree == receipt.FinalCandidateTree && evaluation.Result != reviewtransaction.GateScopeChanged {
			exact++
		}
	}
	return exact
}

func facadeSelectedLenses(assessment reviewtransaction.RiskAssessment, focus string) ([]string, error) {
	return reviewtransaction.SelectReviewLenses(assessment, focus)
}

func (result facadeReviewerResult) nativeLensResult() reviewtransaction.LensResult {
	findings := make([]reviewtransaction.Finding, len(result.Findings))
	for index, finding := range result.Findings {
		findings[index] = reviewtransaction.Finding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location, Severity: finding.Severity,
			Claim: finding.Claim, ProofRefs: append([]string(nil), finding.ProofRefs...),
			EvidenceClass: finding.EvidenceClass, CausalDisposition: finding.CausalDisposition,
		}
	}
	return reviewtransaction.LensResult{Lens: result.Lens, Findings: findings, Evidence: result.Evidence}
}

func (result facadeValidationResult) native(tx reviewtransaction.Transaction) (reviewtransaction.ScopedValidationResult, error) {
	if len(result.OriginalCriteria.Evidence) == 0 || len(result.CorrectionRegression.Evidence) == 0 {
		return reviewtransaction.ScopedValidationResult{}, errors.New("targeted validation requires original_criteria and correction_regression evidence")
	}
	if result.FollowUps == nil {
		result.FollowUps = []reviewtransaction.FollowUp{}
	}
	return reviewtransaction.ScopedValidationResult{
		LedgerIDs: tx.FixFindingIDs, FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: result.FollowUps,
		OriginalCriteria: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("original-criteria", result.OriginalCriteria), FixDeltaHash: tx.FixDeltaHash, Passed: result.OriginalCriteria.Passed,
		},
		CorrectionRegression: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("correction-regression", result.CorrectionRegression), FixDeltaHash: tx.FixDeltaHash, Passed: result.CorrectionRegression.Passed,
		},
	}, nil
}

func (result facadeValidationResult) compact(fixDeltaHash string, findingIDs []string, request reviewtransaction.TargetedValidationRequest) (reviewtransaction.ScopedValidationResult, error) {
	if len(result.OriginalCriteria.Evidence) == 0 || len(result.CorrectionRegression.Evidence) == 0 {
		return reviewtransaction.ScopedValidationResult{}, errors.New("targeted validation requires original_criteria and correction_regression evidence")
	}
	if result.FollowUps == nil {
		result.FollowUps = []reviewtransaction.FollowUp{}
	}
	return reviewtransaction.ScopedValidationResult{
		LedgerIDs: append([]string(nil), findingIDs...), FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: result.FollowUps,
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("original-criteria", result.OriginalCriteria), FixDeltaHash: fixDeltaHash, Passed: result.OriginalCriteria.Passed,
		},
		CorrectionRegression: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("correction-regression", result.CorrectionRegression), FixDeltaHash: fixDeltaHash, Passed: result.CorrectionRegression.Passed,
		},
	}, nil
}

func (result facadeRefuterResult) native() []reviewtransaction.EvidenceResult {
	outcomes := make([]reviewtransaction.EvidenceResult, len(result.Results))
	for index, item := range result.Results {
		outcomes[index] = reviewtransaction.EvidenceResult{
			FindingID: item.FindingID, Outcome: item.Outcome, Proof: strings.Join(item.ProofRefs, "; "),
		}
	}
	return outcomes
}

type facadeRepositoryEvidence struct {
	ctx  context.Context
	repo string
}

func prepareCompactReviewerResults(state reviewtransaction.CompactState, results []facadeReviewerResult, refuter facadeRefuterResult, repository ...facadeRepositoryEvidence) (reviewtransaction.CompactReviewInput, error) {
	if len(results) != len(state.SelectedLenses) {
		return reviewtransaction.CompactReviewInput{}, fmt.Errorf("review finalize requires all %d original reviewer result(s); capture each missing one with `%s` (see `%s` for the exact lineage/target/lens/order bindings)", len(state.SelectedLenses), reviewCaptureResultCommandName(), reviewNextTransitionRefreshCommand)
	}
	lensResults := make([]reviewtransaction.LensResult, len(results))
	classifications := make([]reviewtransaction.FindingEvidence, 0)
	for index, reviewer := range results {
		lensResult := reviewer.nativeLensResult()
		expectedLens := state.SelectedLenses[index]
		if reviewer.Lens != "" {
			providedLens, err := nativeFacadeReviewerLens(reviewer.Lens)
			if err != nil {
				return reviewtransaction.CompactReviewInput{}, fmt.Errorf("reviewer result %d: %w", index+1, err)
			}
			if providedLens != expectedLens {
				return reviewtransaction.CompactReviewInput{}, fmt.Errorf(
					"reviewer result %d lens %q does not match selected lens %q",
					index+1, reviewer.Lens, expectedLens,
				)
			}
		}
		lensResult.Lens = expectedLens
		canonical, err := reviewtransaction.CanonicalCompactLensResult(lensResult)
		if err != nil {
			return reviewtransaction.CompactReviewInput{}, fmt.Errorf("canonicalize reviewer result %d: %w", index+1, err)
		}
		causalityChanged := false
		for findingIndex, finding := range canonical.Findings {
			if !facadeSevere(finding.Severity) {
				continue
			}
			switch finding.CausalDisposition {
			case reviewtransaction.CausalIntroduced, reviewtransaction.CausalBehaviorActivated, reviewtransaction.CausalWorsened:
				if len(repository) == 1 {
					changed, err := (reviewtransaction.SnapshotBuilder{Repo: repository[0].repo}).CandidateLocationSupportsCausality(repository[0].ctx, state.InitialSnapshot, finding.Location, finding.CausalDisposition)
					if err != nil {
						return reviewtransaction.CompactReviewInput{}, fmt.Errorf("verify candidate causality for finding %q: %w", finding.ID, err)
					}
					if !changed {
						finding.CausalDisposition = reviewtransaction.CausalUnknown
						canonical.Findings[findingIndex] = finding
						causalityChanged = true
					}
				}
			}
		}
		if causalityChanged {
			canonical.ResultHash = ""
			canonical, err = reviewtransaction.CanonicalCompactLensResult(canonical)
			if err != nil {
				return reviewtransaction.CompactReviewInput{}, fmt.Errorf("canonicalize reviewer result %d after causal admission: %w", index+1, err)
			}
		}
		lensResults[index] = canonical
		for _, finding := range canonical.Findings {
			if !facadeSevere(finding.Severity) {
				continue
			}
			classifications = append(classifications, reviewtransaction.FindingEvidence{
				FindingID: finding.ID, Class: finding.EvidenceClass, Causality: finding.CausalDisposition,
				Proof: strings.Join(finding.ProofRefs, "; "),
			})
		}
	}
	return reviewtransaction.CompactReviewInput{
		LensResults: lensResults, Classifications: classifications, RefuterOutcomes: refuter.native(),
	}, nil
}

func nativeFacadeReviewerLens(lens string) (string, error) {
	switch lens {
	case "risk", reviewtransaction.LensRisk:
		return reviewtransaction.LensRisk, nil
	case "resilience", reviewtransaction.LensResilience:
		return reviewtransaction.LensResilience, nil
	case "readability", reviewtransaction.LensReadability:
		return reviewtransaction.LensReadability, nil
	case "reliability", reviewtransaction.LensReliability:
		return reviewtransaction.LensReliability, nil
	default:
		return "", fmt.Errorf("unsupported reviewer lens %q", lens)
	}
}

func discoverCompactFacadeFinalize(ctx context.Context, repo, lineage string) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
	if strings.TrimSpace(lineage) != "" {
		return discoverCompactFacadeReview(ctx, repo, lineage, false)
	}
	stores, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, err
	}
	type candidate struct {
		store  reviewtransaction.CompactStore
		record reviewtransaction.CompactRecord
	}
	candidates := []candidate{}
	for _, store := range stores {
		if record, loadErr := store.Load(); loadErr == nil {
			candidates = append(candidates, candidate{store: store, record: record})
		}
	}
	if len(candidates) > 1 {
		active := candidates[:0]
		for _, candidate := range candidates {
			if !facadeTerminalState(candidate.record.State.State) {
				active = append(active, candidate)
			}
		}
		if len(active) > 0 {
			candidates = active
		}
	}
	if len(candidates) > 1 && facadeTerminalState(candidates[0].record.State.State) {
		return discoverCompactFacadeReview(ctx, repo, "", false)
	}
	if len(candidates) > 1 {
		exact := candidates[:0]
		for _, candidate := range candidates {
			if (reviewtransaction.SnapshotBuilder{Repo: repo}).ValidateLiveSnapshot(ctx, candidate.record.State.CurrentSnapshot) == nil {
				exact = append(exact, candidate)
			}
		}
		if len(exact) == 0 {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, reviewPreflightRefusal(
				reviewPreflightStaleTargetReason, errors.New("no compact FINALIZE authority matches the live target"))
		}
		candidates = exact
	}
	if len(candidates) == 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New(reviewCompactFacadeLineageNotDiscoverableReason)
	}
	if len(candidates) != 1 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New("multiple compact facade review lineages found; specify --lineage")
	}
	return candidates[0].store, candidates[0].record, nil
}

func discoverCompactFacadeReview(ctx context.Context, repo, lineage string, terminal bool) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
	if strings.TrimSpace(lineage) != "" {
		store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, lineage)
		if err != nil {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, err
		}
		record, err := store.Load()
		if err != nil {
			legacy, legacyErr := reviewtransaction.AuthoritativeStore(ctx, repo, lineage)
			if legacyErr == nil {
				if _, loadErr := legacy.LoadChain(); loadErr == nil {
					return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, reviewtransaction.ErrLegacyReadOnly
				}
			}
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, fmt.Errorf("load compact facade review lineage: %w", err)
		}
		if terminal {
			if _, err := os.Stat(store.ReceiptPath()); err != nil {
				return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New(reviewFacadeReceiptNotAvailableReason(lineage))
			}
		}
		return store, record, nil
	}
	stores, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, err
	}
	type candidate struct {
		store  reviewtransaction.CompactStore
		record reviewtransaction.CompactRecord
	}
	candidates := []candidate{}
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			continue
		}
		isTerminal := record.State.State == reviewtransaction.StateApproved || record.State.State == reviewtransaction.StateEscalated
		if terminal {
			if !isTerminal {
				continue
			}
			if _, statErr := os.Stat(store.ReceiptPath()); statErr != nil {
				continue
			}
		}
		candidates = append(candidates, candidate{store: store, record: record})
	}
	if !terminal && len(candidates) > 1 {
		active := candidates[:0]
		for _, candidate := range candidates {
			if candidate.record.State.State != reviewtransaction.StateApproved && candidate.record.State.State != reviewtransaction.StateEscalated {
				active = append(active, candidate)
			}
		}
		if len(active) > 0 {
			candidates = active
		}
	}
	if len(candidates) > 1 {
		matching := candidates[:0]
		for _, candidate := range candidates {
			snapshot, buildErr := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
				Kind: reviewtransaction.TargetCurrentChanges, Projection: candidate.record.State.InitialSnapshot.Projection,
				IntendedUntracked: candidate.record.State.InitialSnapshot.IntendedUntracked,
			})
			if buildErr == nil && snapshot.CandidateTree == candidate.record.State.CurrentSnapshot.CandidateTree {
				matching = append(matching, candidate)
			}
		}
		if len(matching) > 0 {
			candidates = matching
		}
	}
	if len(candidates) == 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New(reviewCompactFacadeLineageNotDiscoverableReason)
	}
	if len(candidates) != 1 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New("multiple compact facade review lineages found; specify --lineage")
	}
	return candidates[0].store, candidates[0].record, nil
}

func discoverFacadeReview(ctx context.Context, repo, lineage string, terminal bool) (reviewtransaction.Store, reviewtransaction.ValidatedChain, facadeArtifacts, error) {
	if strings.TrimSpace(lineage) != "" {
		store, err := reviewtransaction.AuthoritativeStore(ctx, repo, lineage)
		if err != nil {
			return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, err
		}
		chain, err := store.LoadChain()
		if err != nil {
			return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, fmt.Errorf("load facade review lineage: %w", err)
		}
		artifacts := facadeArtifactPaths(store)
		if terminal {
			if _, err := os.Stat(artifacts.receipt); err != nil {
				return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, errors.New(reviewFacadeReceiptNotAvailableReason(lineage))
			}
		}
		return store, chain, artifacts, nil
	}
	stores, err := reviewtransaction.DiscoverAuthoritativeStores(ctx, repo)
	if err != nil {
		return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, fmt.Errorf("discover authoritative review stores: %w", err)
	}
	type candidate struct {
		store     reviewtransaction.Store
		chain     reviewtransaction.ValidatedChain
		artifacts facadeArtifacts
	}
	candidates := []candidate{}
	for _, store := range stores {
		artifacts := facadeArtifactPaths(store)
		if terminal {
			if _, err := os.Stat(artifacts.receipt); err != nil {
				continue
			}
		}
		chain, err := store.LoadChain()
		if err != nil {
			continue
		}
		tx := chain.Records[len(chain.Records)-1].Transaction
		isTerminal := tx.State == reviewtransaction.StateApproved || tx.State == reviewtransaction.StateEscalated
		if terminal && !isTerminal {
			continue
		}
		candidates = append(candidates, candidate{store: store, chain: chain, artifacts: artifacts})
	}
	if len(candidates) == 0 {
		return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, errors.New("no discoverable facade review lineage found")
	}
	if !terminal && len(candidates) > 1 {
		nonterminal := candidates[:0]
		for _, candidate := range candidates {
			tx := candidate.chain.Records[len(candidate.chain.Records)-1].Transaction
			if tx.State != reviewtransaction.StateApproved && tx.State != reviewtransaction.StateEscalated {
				nonterminal = append(nonterminal, candidate)
			}
		}
		if len(nonterminal) > 0 {
			candidates = nonterminal
		}
	}
	if len(candidates) > 1 {
		matching := candidates[:0]
		for _, candidate := range candidates {
			tx := candidate.chain.Records[len(candidate.chain.Records)-1].Transaction
			snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
				Kind: reviewtransaction.TargetCurrentChanges, Projection: tx.Snapshot.Projection,
				IntendedUntracked: tx.Snapshot.IntendedUntracked,
			})
			if err == nil && snapshot.CandidateTree == tx.FinalCandidateTree {
				matching = append(matching, candidate)
			}
		}
		if len(matching) > 0 {
			candidates = matching
		}
	}
	if len(candidates) != 1 {
		return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, errors.New("multiple facade review lineages found; specify --lineage")
	}
	selected := candidates[0]
	return selected.store, selected.chain, selected.artifacts, nil
}

func facadeArtifactPaths(store reviewtransaction.Store) facadeArtifacts {
	dir := filepath.Join(store.Dir, "artifacts")
	return facadeArtifacts{
		policy: filepath.Join(dir, "policy.md"), ledger: filepath.Join(dir, "ledger.json"),
		evidence: filepath.Join(dir, "evidence"), fixDelta: filepath.Join(dir, "fix-delta.json"),
		receipt: filepath.Join(dir, "receipt.json"),
	}
}

type reviewFinalizeOutputContext struct {
	Context context.Context
	Repo    string
}

func encodeCompactFacadeFinalize(stdout io.Writer, negotiated, actionEligibility, nextTransition bool, state reviewtransaction.CompactState, revision string, store reviewtransaction.CompactStore, action string, contexts ...reviewFinalizeOutputContext) error {
	var validationRequest *reviewtransaction.TargetedValidationRequest
	var captureContext *reviewCaptureContext
	repositoryContext := ""
	var transitionErr error
	if negotiated && len(contexts) > 0 && contexts[0].Context != nil && strings.TrimSpace(contexts[0].Repo) != "" {
		outputContext := contexts[0]
		if state.State == reviewtransaction.StateCorrectionRequired && state.ProposedCorrectionLines != nil {
			request, err := reviewtransaction.BuildTargetedValidationRequest(outputContext.Context, outputContext.Repo, state, revision)
			if err == nil {
				validationRequest = &request
			}
		}
		if state.State == reviewtransaction.StateReviewing {
			repositoryContext, transitionErr = reviewtransaction.PublishReviewRepositoryContext(outputContext.Context, outputContext.Repo, reviewtransaction.ReviewRepositoryContextBinding{
				LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Revision: revision,
			})
			if transitionErr == nil {
				frozen, err := (reviewtransaction.SnapshotBuilder{Repo: outputContext.Repo}).FrozenCandidateContext(outputContext.Context, state.InitialSnapshot)
				if err != nil {
					transitionErr = err
				} else {
					captureContext, transitionErr = newReviewCaptureContext(state, revision, frozen)
				}
			}
		}
	}
	var eligibility *ReviewActionEligibility
	if actionEligibility {
		eligibility = reviewStopEligibility(reviewActionForbiddenFinalizeStatus, []string{"target_scoped_status"})
	}
	var transition *ReviewNextTransition
	if nextTransition {
		artifacts := []ReviewTransitionArtifact{}
		var artifactErr error
		if state.State == reviewtransaction.StateReviewing {
			if len(contexts) == 0 || contexts[0].Context == nil || strings.TrimSpace(contexts[0].Repo) == "" {
				artifactErr = errors.New("reviewer artifact context is unavailable")
			} else {
				artifacts, artifactErr = discoverCapturedReviewerArtifacts(contexts[0].Context, contexts[0].Repo, store.Dir, state, revision)
			}
		}
		if transitionErr != nil {
			artifactErr = transitionErr
		}
		value := reviewFinalizeNextTransition(state, revision, artifacts, artifactErr, reviewFinalizeTransitionContext{
			RepositoryContext: repositoryContext, ValidationRequest: validationRequest, CaptureContext: captureContext,
		})
		transition = &value
	}
	result := ReviewFacadeFinalizeResult{
		Operation: "review/finalize", LineageID: state.LineageID, State: state.State, Action: action, StoreRevision: revision,
	}
	if state.State == reviewtransaction.StateApproved || state.State == reviewtransaction.StateEscalated {
		result.ReceiptPath = store.ReceiptPath()
	}
	if accounting := state.EscalationAccounting(); accounting.Cause != "" {
		result.Escalation = fmt.Sprintf(reviewtransaction.EscalationAccountingReasonTemplate,
			accounting.Cause, accounting.Spent, accounting.Remaining, accounting.Total)
	}
	public := ReviewIntegrationFinalizeResult{
		Operation: result.Operation, LineageID: result.LineageID, State: result.State,
		Action: result.Action, Escalation: result.Escalation, StoreRevision: result.StoreRevision,
		Eligibility: eligibility, NextTransition: transition, ValidationRequest: validationRequest,
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationFinalize, result, public)
}

func rejectFacadeCorrectionUntracked(ctx context.Context, repo string, state reviewtransaction.CompactState) error {
	if state.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged {
		return nil
	}
	live, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).DiscoverIntendedUntracked(ctx)
	if err != nil {
		return fmt.Errorf("discover correction untracked paths: %w", err)
	}
	allowed := make(map[string]struct{}, len(state.CurrentSnapshot.IntendedUntracked))
	for _, path := range state.CurrentSnapshot.IntendedUntracked {
		allowed[path] = struct{}{}
	}
	unexpected := make([]string, 0)
	for _, path := range live {
		if _, ok := allowed[path]; !ok {
			unexpected = append(unexpected, path)
		}
	}
	if len(unexpected) != 0 {
		return fmt.Errorf("correction contains untracked paths outside the frozen review scope: %s", strings.Join(unexpected, ", "))
	}
	return nil
}

// reviewDisabledUnmanagedReason is the shipped disposition sentence, unchanged
// for every discovery outcome that already reported disabled/unmanaged.
const reviewDisabledUnmanagedReason = "review-driven development is disabled and no receipt governs this candidate, so delivery follows ordinary repository policy"

// reviewEmptyPublicationRangeReason states what the empty-range allow does and
// does not mean. It is an allow because there is no delivery to gate, never
// because anything was reviewed, so it says so in the same sentence.
const reviewEmptyPublicationRangeReason = "the publication range is empty: every commit reachable from HEAD is already published on the push destination, so this push delivers nothing and no review receipt governs it"

// reviewDiscoveryLeftTheGateUndecided reports whether discovery ended without
// being able to say which authority applies, as opposed to proving that none
// does.
//
// This no longer decides whether the gate blocks — while the kill switch is off
// nothing does — it decides whether the emitted result must additionally SAY
// that the gate could not decide. Not blocking is not the same as pretending
// nothing was ambiguous, so these outcomes carry their full typed cause into
// the reported reason instead of being flattened into the plain disposition
// sentence.
//
// Two outcomes qualify. Ambiguous authority that is not provably stale-only:
// either several receipts each EXACTLY govern (len(exact) > 1), or the mixture
// contains a lineage whose assessment could not even be completed
// (assessmentUnknown or scopeWithoutContext). And corrupted authority: the
// inventory itself is unreadable.
func reviewDiscoveryLeftTheGateUndecided(discovery *ReviewReceiptDiscoveryError) bool {
	switch discovery.Kind {
	case ReviewAuthorityCorrupted:
		return true
	case ReviewReceiptAmbiguous:
		return !discovery.DeterministicallyStaleOnly
	default:
		return false
	}
}

// reviewDisabledUnmanagedDeliveryReason states what governs delivery and, when
// the gate could not decide which authority applies, exactly what it could not
// decide. The damage or the contest is named rather than dropped, so a user who
// wants to know still learns about it — they simply are not stopped by it.
func reviewDisabledUnmanagedDeliveryReason(discovery *ReviewReceiptDiscoveryError) string {
	if !reviewDiscoveryLeftTheGateUndecided(discovery) {
		return reviewDisabledUnmanagedReason
	}
	detail := discovery.Error()
	if discovery.Kind == ReviewAuthorityCorrupted && strings.TrimSpace(discovery.Category) != "" {
		detail += " (" + discovery.Category + ")"
	}
	return reviewDisabledUnmanagedReason + "; the gate could not decide which review authority applies: " + detail
}

// emitDisabledUnmanagedDelivery reports a candidate no receipt governs under a
// user-disabled kill switch.
//
// It never approves: `allowed` stays false, the result is not an allow, and no
// receipt, PASS, or authority is invented. It also never vetoes, because a
// disabled switch defers delivery to ordinary repository policy — hooks, tests,
// and CI stay active and decide — so the command exits successfully and the
// typed result names `disabled/unmanaged` as what governs. The discovery
// context is preserved so the reason no receipt governs stays discoverable —
// including the full scope-change diagnostics when a stale receipt stopped
// matching the candidate, and the typed cause when the gate could not decide at
// all — and the whole result is derived from the same frozen authority so
// replaying the same request returns the same bytes.
//
// Damage is deferred, never forgiven: a corrupted inventory reported here is
// rediscovered and blocks again the moment reviews are switched back on. What
// still fails closed is an unreadable KILL SWITCH — reviewDeliveryDisposition
// treats an unresolvable mode as managed — so a tampered or broken mode record
// can never manufacture this disposition.
func emitDisabledUnmanagedDelivery(stdout io.Writer, gate reviewtransaction.GateKind, discovery *ReviewReceiptDiscoveryError, negotiated bool) error {
	context := reviewtransaction.GateContext{
		Gate:   gate,
		Denial: &reviewtransaction.GateDenial{Stage: "receipt-discovery", Code: string(discovery.Kind)},
	}
	if discovery.Kind == ReviewReceiptScopeChanged && discovery.Context != nil {
		context = *discovery.Context
	}
	result := ReviewValidateResult{
		Schema: ReviewValidateSchema, Result: reviewtransaction.GateInvalidated, Allowed: false,
		Action:   reviewDeliveryPolicyAction,
		Reason:   reviewDisabledUnmanagedDeliveryReason(discovery),
		Delivery: reviewtransaction.RDDDeliveryDisabledUnmanaged,
		Context:  context,
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationValidate, result, result)
}

func emitFacadeGateEvaluation(stdout io.Writer, evaluation reviewtransaction.NativeGateEvaluation) error {
	return emitFacadeGateEvaluationNegotiated(stdout, evaluation, false)
}

func emitFacadeGateEvaluationNegotiated(stdout io.Writer, evaluation reviewtransaction.NativeGateEvaluation, negotiated bool) error {
	if err := reviewGateContentionError(evaluation); err != nil {
		return err
	}
	result := ReviewValidateResult{
		Schema: ReviewValidateSchema, Result: evaluation.Result, Allowed: evaluation.Result == reviewtransaction.GateAllow,
		Action: reviewGateAction(evaluation.Result), Reason: evaluation.Reason, Context: evaluation.Context,
	}
	if err := encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationValidate, result, result); err != nil {
		return err
	}
	if !result.Allowed {
		return ReviewGateDeniedError{Result: result.Result, Reason: result.Reason, Context: result.Context, Cause: evaluation.Cause}
	}
	return nil
}

func runFacadeLegacyValidateNegotiated(ctx context.Context, args []string, stdout io.Writer, negotiated bool) error {
	if !negotiated {
		return runReviewValidate(ctx, args, stdout)
	}
	var output bytes.Buffer
	runErr := runReviewValidate(ctx, args, &output)
	if output.Len() == 0 {
		return runErr
	}
	var result ReviewValidateResult
	if err := decodeStrictReviewIntegrationResult(output.Bytes(), &result); err != nil {
		return err
	}
	if err := encodeReviewIntegrationOperation(stdout, true, ReviewIntegrationOperationValidate, result, result); err != nil {
		return err
	}
	return runErr
}

func facadePolicyBytes(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return []byte(facadeReviewPolicy), nil
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read facade review policy: %w", err)
	}
	return payload, nil
}

func readFacadeReviewerResults(paths []string) ([]facadeReviewerResult, error) {
	results := make([]facadeReviewerResult, len(paths))
	for index, path := range paths {
		if err := readFacadeJSON(path, &results[index]); err != nil {
			return nil, fmt.Errorf("read reviewer result %d: %w", index+1, err)
		}
		if results[index].Findings == nil || results[index].Evidence == nil {
			return nil, fmt.Errorf("reviewer result %d requires explicit findings and evidence arrays", index+1)
		}
	}
	return results, nil
}

func readFacadeJSON(path string, value any) error {
	payload, err := readFacadeBytes(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("input contains multiple JSON values")
	}
	return nil
}

func readFacadeBytes(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

var errFacadeArtifactManifestInputNotRegular = errors.New("artifact manifest input must be a regular file")

func readFacadeArtifactManifest(ctx context.Context, path string) ([]byte, error) {
	file, restore, err := openFacadeArtifactManifestInput(ctx, path)
	if err != nil {
		return nil, err
	}
	interrupted := make(chan struct{})
	stopInterrupt := context.AfterFunc(ctx, func() {
		cancelFacadeArtifactManifestInput(file)
		_ = file.SetReadDeadline(time.Now())
		_ = file.Close()
		close(interrupted)
	})
	payload, readErr := io.ReadAll(io.LimitReader(file, reviewResultArtifactLimit+1))
	if stopInterrupt() {
		_ = file.Close()
	} else {
		<-interrupted
	}
	restoreErr := restore()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if readErr != nil {
		return nil, readErr
	}
	if restoreErr != nil {
		return nil, fmt.Errorf("restore facade input mode: %w", restoreErr)
	}
	if len(payload) > reviewResultArtifactLimit {
		return nil, errors.New("artifact exceeds the native result size limit")
	}
	return payload, nil
}

func countFacadeStdin(resultPaths []string, paths ...string) int {
	count := 0
	for _, path := range append(append([]string{}, resultPaths...), paths...) {
		if path == "-" {
			count++
		}
	}
	return count
}

func facadeValueHash(domain string, value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(append([]byte("gentle-ai.facade-"+domain+"/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func facadePayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func facadeSevere(severity string) bool {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "BLOCKER", "CRITICAL":
		return true
	default:
		return false
	}
}
