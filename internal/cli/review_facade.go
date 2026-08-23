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
	"strings"
	"sync/atomic"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// reviewContractRequiredForActionEligibilityReason is the single wording
// source for the refusal when --action-eligibility or --next-transition is
// requested without --contract. Both runReviewStatus and
// runReviewFacadeFinalize emit it so the two call sites cannot drift, and it
// names every accepted contract value rather than only describing the
// requirement.
const reviewContractRequiredForActionEligibilityReason = "--action-eligibility and --next-transition require --contract " +
	ReviewIntegrationContractV1 + " or " + ReviewIntegrationContractV2

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
// refusal when a compact facade lineage was discovered but has not been
// finalized yet, so its receipt does not exist on disk. It names the exact
// continuation — finalize the already-discovered lineage — instead of only
// describing the missing artifact.
func reviewFacadeReceiptNotAvailableReason(lineageID string) string {
	return fmt.Sprintf("facade review receipt is not available; run gentle-ai review finalize --lineage %s to produce one", lineageID)
}

// reviewCompactFacadeLineageAbsentError distinguishes an ordinary compact
// absence from malformed compact authority. Ordinary lifecycle commands never
// probe historical stores: callers either start a fresh compact review or use
// the explicit read-only compatibility command for a historical lineage.
type reviewCompactFacadeLineageAbsentError struct {
	LineageID string
}

func (err *reviewCompactFacadeLineageAbsentError) Error() string {
	if strings.TrimSpace(err.LineageID) == "" {
		return "no discoverable compact facade review lineage found; run gentle-ai review start to begin one, or use gentle-ai review-resume --cwd <repo> --lineage <lineage> for explicit read-only historical compatibility"
	}
	return fmt.Sprintf("no compact facade review lineage %q was found; run gentle-ai review start --lineage %s to begin a fresh compact review, or use gentle-ai review-resume --cwd <repo> --lineage %s for explicit read-only historical compatibility", err.LineageID, err.LineageID, err.LineageID)
}

func reviewCompactFacadeLineageAbsent(lineageID string) error {
	return &reviewCompactFacadeLineageAbsentError{LineageID: strings.TrimSpace(lineageID)}
}

// reviewApprovedCompactBurnedFinalizeAction is the terminal result after FINALIZE
// has approved the compact lineage and irreversibly burned its authority and
// receipt. With no review authority remaining, delivery is governed only by the
// repository's ordinary policy; this action must not advertise review validation
// or any further lifecycle operation.
const reviewApprovedCompactBurnedFinalizeAction = "the approved review completed and burned; delivery follows ordinary repository policy"

const facadeReviewPolicy = `Gentle AI native bounded review policy.

Only candidate-caused BLOCKER or CRITICAL findings may require correction. Pre-existing and base-only findings are follow-ups. One correction is bounded by the frozen original scope. Gates are informational and unmanaged; ordinary repository policy decides delivery.
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

// reviewUndeclaredRuntimeIdentitySlot marks the optional agent segment in Tier C
// narration until bindNarrationRuntimeIdentity either binds or removes it.
const reviewUndeclaredRuntimeIdentitySlot = "<your-runtime-identity>"

// reviewNegotiatedStartCommand builds the exact negotiated `review start`
// invocation for a frozen snapshot. It is the single source of the runnable
// continuation the direct route names when it refuses (issue #2447): every
// direct-route refusal must name a command that actually runs, so the
// command shape lives here once instead of being reconstructed per call site.
//
// runtimeAgent is the identity the caller itself declared, never a constant.
// This message is printed at the exact moment a reader is most likely to copy
// it verbatim, so a baked-in `claude-code` would invite every other runtime to
// declare a false identity and pass the transport admission check in
// review_transport_capability.go under Claude Code's capability profile
// (issue #2440). Naming the caller's own runtime keeps that check meaningful:
// a runtime whose transport is unsupported is then refused, which is the
// correct outcome for this build.
func reviewNegotiatedStartCommand(snapshot reviewtransaction.Snapshot, runtimeAgent string) string {
	identity := strings.TrimSpace(runtimeAgent)
	command := fmt.Sprintf("gentle-ai review start --contract %s", ReviewIntegrationContractV2)
	if identity != "" {
		command += " --agent " + identity
	}
	command += fmt.Sprintf(" --target %s --projection %s", snapshot.Identity, facadeProjection(snapshot.Projection))
	switch snapshot.Kind {
	case reviewtransaction.TargetBaseDiff:
		command += " --base-ref " + snapshot.BaseTree + " --committed-only"
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		command += " --base-ref " + snapshot.BaseTree + " --workspace-overlay"
	}
	return command
}

// ReviewFacadeLensBinding pairs one selected lens with its frozen zero-based
// order so orchestrators build capture bindings exclusively from START output.
type ReviewFacadeLensBinding struct {
	Lens  string `json:"lens"`
	Order int    `json:"order"`
	// FindingIDPrefix is the prefix admission requires for explicit finding
	// IDs bound to this lens. It follows the lens, not the selection order:
	// high-risk START selects review-resilience at order 1, but its explicit
	// IDs must still carry R4-.
	FindingIDPrefix string `json:"finding_id_prefix"`
}

func facadeLensBindings(lenses []string) []ReviewFacadeLensBinding {
	bindings := make([]ReviewFacadeLensBinding, len(lenses))
	for order, lens := range lenses {
		bindings[order] = ReviewFacadeLensBinding{
			Lens: lens, Order: order,
			FindingIDPrefix: reviewtransaction.FindingIDPrefixForLens(lens),
		}
	}
	return bindings
}

func facadeProjection(projection reviewtransaction.Projection) reviewtransaction.Projection {
	if projection == "" {
		return reviewtransaction.ProjectionWorkspace
	}
	return projection
}

func facadeCorrectionEvidenceTargetFromRequest(state reviewtransaction.CompactState, live reviewtransaction.Snapshot, request reviewtransaction.TargetedValidationRequest) reviewtransaction.Snapshot {
	return reviewtransaction.Snapshot{
		Kind: reviewtransaction.TargetFixDiff, Projection: request.Projection, UnbornHead: live.UnbornHead,
		BaseTree: state.CurrentSnapshot.CandidateTree, CandidateTree: request.CorrectionCandidateTree,
		PathsDigest: request.CorrectionPathsDigest, Paths: append([]string(nil), request.CorrectionPaths...),
		IntendedUntracked:      append([]string(nil), state.InitialSnapshot.IntendedUntracked...),
		IntendedUntrackedProof: live.IntendedUntrackedProof,
		LedgerIDs:              append([]string(nil), request.FixFindingIDs...), Identity: request.CorrectionTargetIdentity,
	}
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
	Escalation string `json:"escalation,omitempty"`
	// AdvisoryFindings names the disposition of every non-blocking frozen
	// finding on a terminally approved lineage, plus the sentence saying the
	// approval stands. It is present only when an approved lineage actually
	// froze such a finding, so a clean approval's bytes are unchanged.
	AdvisoryFindings *reviewtransaction.AdvisoryFindingSet `json:"advisory_findings,omitempty"`
	StoreRevision    string                                `json:"store_revision"`
	ReceiptPath      string                                `json:"receipt_path,omitempty"`
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
	// ReviewGateRemoteFetchRequired (issue #3342) names the one assessment
	// failure class the local repository resolves by itself: every unknown
	// assessment failed because the advertised publication tip is not in the
	// local object store. The authority store is healthy; `git fetch
	// <remote>` followed by the identical gate resolves it. The gate stays
	// denied (fail-closed), but the denial is retry-safe, never maintainer
	// escalation.
	ReviewGateRemoteFetchRequired ReviewReceiptDiscoveryKind = "remote_fetch_required"
	// ReviewAuthorityInventoryBusy (issue #3342) names transient shared-store
	// coordination instead of damage: the shared compact store lock is held
	// by a live concurrent review operation, a store read lost a bounded lock
	// wait, or the lock file carries readable interrupted-holder residue. The
	// gate stays denied (fail-closed), but the denial names the lock, its
	// recorded holder, and the retry continuation, never maintainer
	// escalation for a condition that clears itself.
	ReviewAuthorityInventoryBusy ReviewReceiptDiscoveryKind = "inventory_busy"
	// ReviewAuthorityLockUnverifiable names shared store-lock residue whose
	// recorded holder is neither flock-held nor verifiably dead on this
	// host; no event terminates a retry, so it is NOT retry-safe.
	ReviewAuthorityLockUnverifiable ReviewReceiptDiscoveryKind = "lock_holder_unverifiable"
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
		// issue #3408: the old wording ("terminal review receipts exist only
		// for unrelated targets") reported OTHER lineages' existence as
		// though one of them were the obstacle, so an operator went looking
		// for what those receipts had to do with their work and found
		// nothing, because there is nothing. Discovery has proven the exact
		// opposite: every terminal receipt on file was assessed against this
		// candidate and none of them governs it. That is the candidate's own
		// situation, and it has one route, so the denial states both.
		message = "no approved review receipt covers this candidate; review it with gentle-ai review start"
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
	case ReviewGateRemoteFetchRequired:
		message = "advertised publication base commit is not available locally"
	case ReviewAuthorityInventoryBusy:
		message = "review authority store is busy"
	case ReviewAuthorityLockUnverifiable:
		message = "review authority store lock records a holder this host cannot verify"
	}
	if err.Detail != "" {
		return message + ": " + err.Detail
	}
	return message
}

// ReviewFacadeReceiptPublicationError reports the only safe interpretation of
// a terminal authority whose derived receipt could not be materialized.
type ReviewFacadeReceiptPublicationError struct {
	MutationOutcome string `json:"mutation_outcome"`
	Replayability   string `json:"replayability"`
	LineageID       string `json:"lineage_id"`
	TargetIdentity  string `json:"target_identity"`
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
	TargetedValidationRequestHash string                       `json:"targeted_validation_request_hash"`
	CorrectionTargetIdentity      string                       `json:"correction_target_identity"`
	OriginalCriteria              facadeValidationCheck        `json:"original_criteria"`
	CorrectionRegression          facadeValidationCheck        `json:"correction_regression"`
	FollowUps                     []reviewtransaction.FollowUp `json:"follow_ups"`
}

// errReviewTargetedValidationInconclusive marks the one admission failure
// that is neither corruption nor an inadmissible slot: a captured targeted
// validation that produced no verdict because the validator could not reach
// the frozen trees. Callers separate it from every other read failure with
// errors.Is, because its continuation is the opposite of theirs -- nothing was
// consumed and the same validation can simply be run again (issue #3378).
var errReviewTargetedValidationInconclusive = errors.New("targeted validation is inconclusive") // refusal:by-design world-action: restore the validator's read-only access to the frozen trees and run the same targeted validation again

// conclusive rejects validation checks whose evidence reports the immutable
// candidate could not be inspected. Such a check is not a verdict: admitted as
// failed it consumes the single correction attempt on a non-observation
// (issue #1309 follow-up), and admitted as passed it approves uninspected
// bytes. No state transitions, so the same validation can be captured again
// once the validator regains access to the frozen trees.
func (result facadeValidationResult) conclusive() error {
	for _, check := range []struct {
		name     string
		evidence []string
	}{
		{name: "original_criteria", evidence: result.OriginalCriteria.Evidence},
		{name: "correction_regression", evidence: result.CorrectionRegression.Evidence},
	} {
		if reviewtransaction.InconclusiveValidationEvidence(check.evidence) {
			// The exit is named on the sentinel this wraps: restore the
			// validator's access to the frozen trees and run the same
			// targeted validation again.
			return fmt.Errorf("%w: %s evidence reports the immutable candidate could not be inspected, so no verdict was produced and the correction attempt was not consumed; restore validator access to the frozen trees and run the same targeted validation again", errReviewTargetedValidationInconclusive, check.name)
		}
	}
	return nil
}

type facadeRefuterResult struct {
	RequestHash string                 `json:"refuter_request_hash,omitempty"`
	Results     []facadeRefuterOutcome `json:"results"`
}

type facadeRefuterOutcome struct {
	FindingID string                            `json:"finding_id"`
	Outcome   reviewtransaction.EvidenceOutcome `json:"outcome"`
	ProofRefs []string                          `json:"proof_refs"`
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

// reviewFacadeFinalizeProviderOperationTimeout is the deadline for
// review.finalize only when a compiled runtime is bound with --agent: that
// finalize launches the provider refuter (and, on the correction path, the
// targeted validator) as a real model process, whose ordinary duration is
// minutes — categorically outside the shared 25s budget, for the same
// reason review.start owns its own constant.
const reviewFacadeFinalizeProviderOperationTimeout = 600 * time.Second

// reviewFacadeOperationDeadline selects the operation-scoped deadline.
// review.start uses its own larger constant, review.finalize with a bound
// compiled runtime uses the provider constant, and every other operation
// keeps the shared reviewFacadeOperationTimeout var byte-identical.
func reviewFacadeOperationDeadline(operation string, args []string) time.Duration {
	if operation == "review.start" {
		return reviewFacadeStartOperationTimeout
	}
	if operation == "review.finalize" && reviewRuntimeAgentCount(args) > 0 {
		return reviewFacadeFinalizeProviderOperationTimeout
	}
	return reviewFacadeOperationTimeout
}

var reviewFacadeCommandRunner = runReviewCommandContext
var reviewFacadePlannedTransitionHook = func(context.Context, string, string, string) error { return nil }
var reviewFacadeCommittedTransitionHook = func(context.Context, string, string, string) error { return nil }

// reviewFacadeBuildStartSnapshot is the injectable seam over START's candidate
// freeze, so tests can force an unanticipated internal fault at a real choke
// point on the mutating path -- before any authority exists -- and prove the
// failure envelope and defect-report treatment both fire. It replaced the
// untracked-scope discovery seam that #2394 deleted from START.
var reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
	return builder.Build(ctx, target)
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
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai review <capture-result|capture-refuter|capture-validation|lens-context|capture-evidence|preserve-result|capabilities|start|finalize|validate|status|repair|invalidate|abandon|recover|retry-final-verification|reclaim|store-reset|inspect-authority|inspect-candidate|dispose-result|reopen-results|schema|opencode-transport|bind-sdd> [flags]\n\nOrdinary review facade; repository scope, authority, canonical artifacts, and lifecycle transitions are derived by Go. Provider transports relay opaque bytes only; Go materializes, admits, captures, and decides delivery. Use review retry-final-verification only for a provider-proven completed failed final-verification tooling incident. Generic review recover remains unchanged. Use review repair --preflight for provider-owned classified authority repair.")
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
		if capture, collectCapture := reviewCollectCaptureOperationByCommand(args[0]); collectCapture {
			return runReviewCollectCaptureCommand(capture.Operation, args, stdout)
		}
		if err := runReviewCommandContext(context.Background(), args, stdout); err != nil {
			// The plain form has no envelope contract, but an unanticipated
			// internal fault on a mutating operation is the same product defect
			// there (issue #1881 crashed exactly this way): attach the saved
			// defect report to the error the operator is about to read.
			return reviewAppendUnexpectedFaultDefectClause(context.Background(), operation, args[1:], err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), reviewFacadeOperationDeadline(operation, args[1:]))
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

// runReviewCollectCaptureCommand dispatches one collect-satisfying capture
// verb (capture-result, capture-evidence, capture-refuter, capture-validation)
// and, on refusal, emits the typed failure/v2 envelope on stdout.
//
// DECISION (finalize-ambiguity diagnosis): emission is unconditional, not
// gated on a contract flag, because these verbs cannot know they have a
// machine caller -- orchestrators invoke them WITHOUT --contract, exactly as
// the negotiated collect transitions render their submission argv. Their
// success paths already print one JSON document on stdout unconditionally, so
// refusals printing one JSON document is symmetric; without it a machine
// caller that parses stdout (the gentle-pi runtime's shared invoke path) can
// only classify the refusal as empty-output with mutation outcome unknown --
// a false ambiguity for a refusal that provably never started. The
// operator-facing error still returns unchanged, so stderr keeps its
// human-readable line and the exit code stays non-zero.
//
// Output is buffered like the negotiated route so stdout carries exactly one
// document: a flag-parse refusal writes usage prose before failing, and those
// bytes must never precede the envelope a machine caller will decode.
func runReviewCollectCaptureCommand(operation string, args []string, stdout io.Writer) error {
	ctx := context.Background()
	var output bytes.Buffer
	runErr := runReviewCommand(args, &output)
	if runErr == nil {
		_, err := io.Copy(stdout, &output)
		return err
	}
	failure := newReviewIntegrationFailure(operation, args[1:], runErr)
	// The capture verbs exist only in the v2.1 negotiated lifecycle and the
	// published v1 failure schema does not admit their operation names, so
	// the envelope publishes under the v2 identity unconditionally.
	failure.Schema, failure.Contract = ReviewIntegrationFailureSchemaV2, ReviewIntegrationContractV2
	// Generate the defect report before emitting the envelope so a stdout
	// write failure cannot suppress the artifact, exactly as the negotiated
	// route does.
	clause := reviewUnexpectedFaultDefectReportClause(ctx, operation, args[1:], failure)
	emitErr := emitReviewIntegrationFailure(stdout, failure)
	typedFailure := newReviewIntegrationFailureError(failure, runErr)
	typedFailure.defectReportClause = clause
	// The plain route always printed the native refusal text on stderr, and
	// operators (and the sweep of refusal-message tests) depend on it; only
	// the machine-readable code is new on this line.
	typedFailure.operatorMessage = runErr.Error()
	if emitErr != nil {
		// A dead stdout (closed pipe, halted host relay) must never cost the
		// caller the native refusal: keep the refusal primary so errors.Is/As
		// dispatch survives, with the emit failure attached to the chain.
		return errors.Join(typedFailure, emitErr)
	}
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
		return runReviewFacadeValidateNonDeciding(ctx, args[1:], stdout)
	case "bind-sdd":
		return runReviewBindSDD(ctx, args[1:], stdout)
	default:
		return runReviewCommand(args, stdout)
	}
}

func runReviewCommand(args []string, stdout io.Writer) error {
	switch args[0] {
	case "capture-result":
		return RunReviewCaptureResult(args[1:], stdout)
	case "capture-refuter":
		return RunReviewCaptureRefuter(args[1:], stdout)
	case "capture-validation":
		return RunReviewCaptureValidation(args[1:], stdout)
	case "inspect-candidate":
		return RunReviewInspectCandidate(args[1:], stdout)
	case "lens-context":
		return RunReviewLensContext(args[1:], stdout)
	case "capture-evidence":
		return RunReviewCaptureEvidence(args[1:], stdout)
	case "preserve-result":
		return RunReviewPreserveResult(args[1:], stdout)
	case "capabilities":
		return RunReviewCapabilities(args[1:], stdout)
	case "invalidate":
		return RunReviewInvalidate(args[1:], stdout)
	case "abandon":
		return RunReviewAbandon(args[1:], stdout)
	case "recover":
		return RunReviewRecover(args[1:], stdout)
	case "reclaim":
		return RunReviewReclaim(args[1:], stdout)
	case "store-reset":
		return RunReviewStoreReset(args[1:], stdout)
	case "inspect-authority":
		return RunReviewInspectAuthority(args[1:], stdout)
	case "dispose-result":
		return RunReviewDisposeResult(args[1:], stdout)
	case "reopen-results":
		return RunReviewReopenResults(args[1:], stdout)
	case "schema":
		return RunReviewSchema(args[1:], stdout)
	case "opencode-transport":
		return RunReviewOpenCodeTransport(args[1:], stdout)
	default:
		return fmt.Errorf("unknown review command %q", args[0])
	}
}

func runReviewStatus(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review status", stdout, "Read every compact-v2 and shipped legacy-v1 authority from the shared Git common directory without mutation.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	runtimeAgent := flags.String("agent", "", "generated active runtime identity for negotiated lifecycle routing")
	actionEligibility := flags.Bool("action-eligibility", false, "include optional machine-readable review action eligibility in negotiated output")
	nextTransition := flags.Bool("next-transition", false, "include the optional canonical native next transition in negotiated output")
	lineage := flags.String("lineage", "", "optional explicit lineage selector for negotiated target status")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "negotiated target projection: workspace or staged")
	baseRef := flags.String("base-ref", "", "optional negotiated immutable base-to-HEAD target")
	baseTree := flags.String("base-tree", "", "optional negotiated resolved immutable overlay base tree")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref selects committed-only review scope")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "select a negotiated base-ref workspace overlay target")
	gate := flags.String("gate", string(reviewtransaction.GatePreCommit), "lifecycle gate for an approved receipt transition")
	recoverySuccessor := flags.String("recovery-successor-lineage", "", "authorized successor lineage for a recovery transition")
	recoveryReason := flags.String("recovery-reason", "", "authorized recovery reason")
	recoveryActor := flags.String("recovery-actor", "", "authorized recovery actor")
	recoveryAuthorization := flags.String("recovery-authorization", "", "exact authorized recovery binding")
	repairActor := flags.String("repair-actor", "", "authorized classified repair actor")
	repairReason := flags.String("repair-reason", "", "authorized classified repair reason")
	repairAuthorization := flags.String("repair-authorization", "", "exact classified repair authorization binding")
	untrackedScope := reviewSingleValueFlag{}
	intendedUntracked := reviewRepeatedPathFlag{}
	expectedUntrackedInventory := reviewSingleValueFlag{}
	flags.Var(&untrackedScope, "untracked-scope", "explicit untracked scope: exclude or select")
	flags.Var(&intendedUntracked, "intended-untracked", "repo-relative untracked path to include; repeat for each path")
	flags.Var(&expectedUntrackedInventory, "expected-untracked-inventory", "sha256 inventory digest from review status")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review status argument %q", flags.Arg(0)))
	}
	committedOnlyProvided := reviewFlagWasProvided(flags, "committed-only")
	if *contract != "" {
		if err := validateReviewIntegrationContract(*contract); err != nil {
			return err
		}
		var runtime model.AgentID
		// A declared runtime identity is validated exactly as before, so an
		// unsupported transport still stops here. An undeclared one is the
		// manual/non-agent compatibility path runReviewFacadeStart already
		// names: a read-only STATUS creates no authority, tier, budget, or
		// collection state, so it has no state to fail closed over, and the
		// documented route never declares an identity.
		if *contract == ReviewIntegrationContractV2 && reviewRuntimeAgentCount(args) != 0 {
			if reviewRuntimeAgentCount(args) != 1 {
				// refusal:by-design world-action: an ambiguous runtime identity cannot safely select a review transport
				return reviewPreflightRefusal(reviewImmutableTransportUnsupportedReason, errors.New("negotiated lifecycle STATUS requires exactly one generated runtime identity"))
			}
			var runtimeErr error
			runtime, runtimeErr = reviewRuntimeWithImmutableTransport(*runtimeAgent)
			if runtimeErr != nil {
				return reviewPreflightRefusal(reviewImmutableTransportUnsupportedReason, runtimeErr)
			}
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
		if *committedOnly && (selectedBaseRef == "" || *workspaceOverlay) {
			return errors.New("review status --committed-only requires --base-ref without --workspace-overlay; rerun `gentle-ai review status --base-ref <ref> --committed-only`")
		}
		if selectedBaseRef != "" && committedOnlyProvided && !*committedOnly && !*workspaceOverlay {
			return errors.New("review status --base-ref requires --committed-only; rerun `gentle-ai review status --base-ref <ref> --committed-only`")
		}
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
		// STATUS accepts a nested selector, but every subsequent snapshot
		// operation must use the canonical worktree root it resolved. Leaving
		// the nested selector here would make fresh target freezing reject a
		// valid foreign repository before START can bind its lifecycle root.
		builder.Repo = root
		intendedScope := reviewIntendedUntrackedScope{Intended: []string{}}
		if selectedProjection == reviewtransaction.ProjectionStaged {
			if reviewIntendedUntrackedDeclared(untrackedScope, intendedUntracked, expectedUntrackedInventory) {
				return reviewPreflightError(errors.New("staged projection does not accept intended-untracked selection; remove those flags and rerun `gentle-ai review status --projection staged`"))
			}
		} else {
			intendedScope, err = reviewIntendedUntrackedScopeForTarget(ctx, reviewtransaction.SnapshotBuilder{Repo: root}, untrackedScope, intendedUntracked, expectedUntrackedInventory)
			if err != nil {
				return reviewPreflightError(err)
			}
		}
		target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: []string{}}
		if selectedBaseRef != "" {
			target.Kind, target.BaseRef = reviewtransaction.TargetBaseDiff, selectedBaseRef
		}
		if *workspaceOverlay {
			target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
			if selectedBaseTree != "" {
				target.BaseRef = selectedBaseTree
			}
		}
		target.IntendedUntracked = intendedScope.Intended
		var prePR *reviewtransaction.PrePRRequest
		prePRRepresentable := true
		if reviewtransaction.GateKind(*gate) == reviewtransaction.GatePrePR {
			prePRTarget := target
			target, prePR, err = reviewtransaction.BuildPrePRTarget(ctx, root, selectedBaseRef, "", intendedScope.Intended)
			if err != nil {
				var resolutionErr *reviewtransaction.GateTargetResolutionError
				if !errors.As(err, &resolutionErr) {
					return fmt.Errorf("resolve negotiated pre-PR target: %w", err)
				}
				target = prePRTarget
				prePRRepresentable = false
			}
		}
		selector := &reviewTransitionSelector{
			Kind: target.Kind, Projection: selectedProjection, BaseRef: selectedBaseRef,
			BaseTree: selectedBaseTree, WorkspaceOverlay: *workspaceOverlay, PrePRRepresentable: prePRRepresentable,
		}
		var native reviewtransaction.TargetStatusResult
		var liveSnapshot reviewtransaction.Snapshot
		requestedLineage := strings.TrimSpace(*lineage)
		requestedLineageOccupied := false
		if *nextTransition && requestedLineage != "" {
			requestedLineageOccupied, err = reviewtransaction.ExactReviewLineageOccupied(ctx, root, requestedLineage)
			if err != nil {
				return fmt.Errorf("inspect negotiated START lineage occupancy: %w", err)
			}
			if !requestedLineageOccupied {
				_, staged, stageErr := reviewtransaction.InspectStagedApprovedCompactBurn(ctx, root, requestedLineage)
				if stageErr != nil {
					return fmt.Errorf("inspect negotiated staged compact lineage: %w", stageErr)
				}
				requestedLineageOccupied = staged
			}
		}
		if *nextTransition && (requestedLineage == "" || !requestedLineageOccupied) {
			// A free exact selector starts independently. Freeze its target without
			// consulting sibling authority; START then creates or replays only this
			// explicitly rendered lineage.
			liveSnapshot, err = builder.BuildStoredSnapshot(ctx, target)
			if err != nil {
				return fmt.Errorf("freeze negotiated fresh review target: %w", err)
			}
			native = reviewFreshAtomicTargetStatus(target, liveSnapshot)
		} else {
			native, liveSnapshot, err = reviewtransaction.AssessTargetStatusWithSnapshot(ctx, root, reviewtransaction.TargetStatusRequest{
				Target: target, LineageID: *lineage, PrePR: prePR,
			})
			if err != nil {
				return fmt.Errorf("assess negotiated review target: %w", err)
			}
		}
		if selectedBaseTree != "" && native.Projection.BaseTree != selectedBaseTree {
			return errors.New("--base-tree does not identify an exact Git tree object")
		}
		result := newReviewTargetStatusResultForContract(native, *contract)
		result.intendedUntracked = intendedScope
		if native.Decision.FrozenReviewing {
			// Explicit reviewing resume keeps the immutable scope that the reviewer
			// artifacts bind, rather than asking live workspace drift to select one.
			intendedScope = reviewIntendedUntrackedScope{
				Intended: append([]string{}, native.Projection.IntendedUntracked...), Declared: true,
			}
			result.intendedUntracked = intendedScope
		}
		// STATUS renders the core's executable decision, never the raw spelling
		// the CLI parsed.
		selector.Kind, selector.Projection = result.decision.Selector.Kind, result.decision.Selector.Projection
		if result.decision.Selector.BaseRef != "" {
			selector.BaseRef = result.decision.Selector.BaseRef
		} else if native.Projection.Kind == reviewtransaction.TargetBaseDiff {
			selector.BaseRef = native.Projection.BaseTree
		}
		selector.WorkspaceOverlay = result.decision.Selector.Kind == reviewtransaction.TargetBaseWorkspaceOverlay
		selector.Recovery = result.decision.RecoverySelector
		selector.SelectorFreeAccountingOnlyRecovery = result.decision.SelectorFreeAccountingOnlyRecovery
		if *actionEligibility || *nextTransition {
			mode, modeErr := reviewModeStatus(ctx, root)
			if modeErr != nil {
				return modeErr
			}
			result.repositoryRoot, result.rddMode, result.rddModeResolved = root, mode, true
		}
		if native.Applicability == reviewtransaction.TargetApplicabilityCorrupted &&
			native.Action == reviewtransaction.TargetStatusActionRepairAuthority {
			repair, repairErr := reviewtransaction.AssessAuthorityRepairAtRepositoryRoot(ctx, root)
			if repairErr != nil {
				return fmt.Errorf("assess classified authority repair: %w", repairErr)
			}
			result.Repair = repair
			// Wave 6 (rdd-closure-disposition-execution / "Reachable Through
			// the Negotiated Transition Route"): only when the classified
			// vocabulary above has nothing to offer, mirroring `review repair
			// --preflight`'s own read-only prediction (review_repair.go) —
			// actor/reason stay empty so nothing maintainer-specific is ever
			// derived here, and a derivation refusal is never propagated (no
			// eligible edge this round is not a status-assembly error).
			if repair.Status != reviewtransaction.AuthorityRepairEligible || repair.Candidate == nil {
				if plan, planErr := reviewtransaction.DeriveAuthorityDispositionPlanAtRepo(ctx, root, "", ""); planErr == nil && reviewtransaction.AdmitAuthorityDispositionClosure(plan) == nil {
					result.Disposition = &ReviewRepairDispositionProviderInputs{
						PlanDigest: plan.PlanDigest, AuthorityInventoryRevision: plan.AuthorityInventoryRevision,
						SeedLineageID: plan.SeedSet[0], SeedExpectedRevision: plan.ExpectedRevisions[plan.SeedSet[0]],
					}
				}
			}
		}
		if *actionEligibility {
			result.Eligibility = newReviewActionEligibility(result)
		}
		var compactAuthority *reviewStatusCompactAuthority
		if *nextTransition {
			artifacts := []ReviewTransitionArtifact{}
			var capturedEvidence *reviewtransaction.VerificationEvidenceRecord
			var evidenceErr error
			repositoryContext := ""
			var captureContext *reviewCaptureContext
			var validationRequest *reviewtransaction.TargetedValidationRequest
			var correctionRequest *reviewtransaction.CorrectionPlanRequest
			providerRole := reviewProviderRole("")
			var preCommitDeliveryAssessment *reviewtransaction.CompactGateTargetApplicability
			capturedProviderTargetedValidator := false
			capturedProviderTargetedValidatorInconclusive := false
			correctionForecasted := false
			lensContextBudgetExceeded := false
			var artifactErr error
			if native.Applicability == reviewtransaction.TargetApplicabilityCurrent && native.AuthorityVersion == reviewtransaction.AuthorityVersionCompact {
				store, storeErr := reviewtransaction.CompactStatusStore(ctx, root, native.LineageID)
				if storeErr != nil {
					artifactErr = storeErr
				} else {
					record, loadErr := store.LoadContext(ctx)
					if loadErr != nil {
						artifactErr = loadErr
					} else {
						compactAuthority = &reviewStatusCompactAuthority{
							OriginalChangedLines:   record.State.OriginalChangedLines,
							CorrectionBudget:       record.State.CorrectionBudget,
							CorrectionBudgetPolicy: record.State.CorrectionBudgetPolicy,
						}
						correctionForecasted = record.State.State == reviewtransaction.StateCorrectionRequired && record.State.ProposedCorrectionLines != nil
						if record.State.State == reviewtransaction.StateApproved && result.Receipt.Status == ReviewReceiptPresent &&
							reviewtransaction.GateKind(*gate) == reviewtransaction.GatePreCommit {
							assessment, assessmentErr := reviewtransaction.AssessCompactGateTarget(ctx, root, record.State, reviewtransaction.NativeGateRequestInput{
								Gate: reviewtransaction.GatePreCommit, LineageID: record.State.LineageID,
								IntendedUntracked: intendedScope.Intended,
							})
							if assessmentErr != nil {
								return fmt.Errorf("assess negotiated staged delivery candidate: %w", assessmentErr)
							}
							applicability := assessment.Applicability
							preCommitDeliveryAssessment = &applicability
						}
						if record.State.State == reviewtransaction.StateCorrectionRequired && !record.State.CorrectionAttemptConsumed() {
							request, requestErr := reviewtransaction.BuildCorrectionPlanRequest(record.State, record.Revision)
							if requestErr == nil {
								correctionRequest = &request
							}
						}
						if correctionForecasted {
							request, requestErr := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(ctx, root, record.State, record.Revision, liveSnapshot)
							if requestErr == nil {
								validationRequest = &request
								result.ValidationRequest = validationRequest
							}
						}
						if *contract == ReviewIntegrationContractV2 && (record.State.State == reviewtransaction.StateCorrectionRequired || record.State.State == reviewtransaction.StateValidating) {
							contextTarget := record.State.CurrentSnapshot.Identity
							if validationRequest != nil {
								contextTarget = validationRequest.CorrectionTargetIdentity
							}
							repositoryContext, artifactErr = reviewtransaction.PublishReviewRepositoryContext(ctx, root, reviewtransaction.ReviewRepositoryContextBinding{
								LineageID: record.State.LineageID, TargetIdentity: contextTarget, Revision: record.Revision,
							})
							if artifactErr == nil {
								result.RepositoryContext = &ReviewRepositoryContextReference{
									Capability: reviewtransaction.ReviewRepositoryContextCapability, Handle: repositoryContext,
									Revision: record.Revision, TargetIdentity: contextTarget,
								}
							}
						}
						if record.State.State == reviewtransaction.StateReviewing {
							artifacts, artifactErr = discoverCapturedReviewerArtifacts(ctx, root, store.Dir, record.State, record.Revision)
							if artifactErr == nil && len(artifacts) != len(record.State.SelectedLenses) {
								// Only the probe's deterministic verdict stops STATUS: an unproven
								// probe says nothing about artifacts that just verified, so it must
								// never become a terminal captured-artifact failure (issue #3367).
								lensContextBudgetExceeded = reviewLensContextStatusBudgetExhausted(ctx, root, record.State, record.Revision)
							}
							if artifactErr == nil && !lensContextBudgetExceeded {
								if hasRepositoryContextIntent(record.EffectIntents) {
									var reconciled reviewtransaction.CompactRepositoryContextResult
									reconciled, artifactErr = reviewtransaction.ReconcileCompactRepositoryContext(ctx, store, record)
									if artifactErr == nil {
										repositoryContext = reconciled.Handle
										result.RepositoryContext = &ReviewRepositoryContextReference{
											Capability: reviewtransaction.ReviewRepositoryContextCapability, Handle: reconciled.Handle,
											Revision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity,
											EventID: reconciled.EventID, Outcome: reconciled.Outcome,
										}
									}
								} else {
									repositoryContext, artifactErr = reviewtransaction.PublishReviewRepositoryContext(ctx, root, reviewtransaction.ReviewRepositoryContextBinding{
										LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.Revision,
									})
									if artifactErr == nil && *contract == ReviewIntegrationContractV2 {
										result.RepositoryContext = &ReviewRepositoryContextReference{
											Capability: reviewtransaction.ReviewRepositoryContextCapability, Handle: repositoryContext,
											Revision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity,
										}
									}
								}
							}
							if artifactErr == nil && !lensContextBudgetExceeded {
								contextBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
								frozen, frozenErr := contextBuilder.FrozenCandidateContext(ctx, record.State.InitialSnapshot)
								if frozenErr == nil && *contract == ReviewIntegrationContractV1 {
									frozen, frozenErr = contextBuilder.WithLegacyCandidateDiff(ctx, record.State.InitialSnapshot, frozen)
								}
								if frozenErr != nil {
									artifactErr = frozenErr
								} else {
									captureContext, artifactErr = newReviewCaptureContext(record.State, record.Revision, frozen)
								}
							}
						}
						if artifactErr == nil && record.State.State != reviewtransaction.StateReviewing {
							artifacts, artifactErr = discoverCapturedReviewerArtifacts(ctx, root, store.Dir, record.State, record.Revision)
						}
						if artifactErr == nil {
							evidenceTarget := record.State.CurrentSnapshot
							if validationRequest != nil {
								evidenceTarget = facadeCorrectionEvidenceTargetFromRequest(record.State, liveSnapshot, *validationRequest)
							}
							if record.State.State == reviewtransaction.StateValidating || validationRequest != nil {
								captured, readErr := reviewtransaction.ReadCapturedVerificationEvidence(store.Dir, record.State.LineageID, record.Revision, evidenceTarget)
								evidenceErr = readErr
								if readErr == nil {
									recordCopy := captured.Record
									capturedEvidence = &recordCopy
								}
							}
							// OpenCode relays Go-issued role tasks through its live
							// transport; the pi host relay collects the same roles
							// through the printed materialize + submission route.
							// Both discover pending roles identically here.
							providerRoleHost := runtime == model.AgentOpenCode || reviewProviderHostRelayMaterializeRuntime(runtime)
							if providerRoleHost && record.State.State == reviewtransaction.StateReviewing && len(artifacts) == len(record.State.SelectedLenses) {
								slot, slotErr := reviewtransaction.ReadCompactRefuterResultSlot(store.Dir)
								if slotErr != nil {
									artifactErr = slotErr
								} else if !slot.Occupied {
									_, requestErr := reviewProviderNewRefuterRequest(ctx, root, store.Dir, record.State, record.Revision)
									switch {
									case requestErr == nil:
										providerRole = reviewerprovider.RoleRefuter
									case errors.Is(requestErr, errReviewProviderRefuterNotRequired):
									default:
										artifactErr = requestErr
									}
								}
							}
							if validationRequest != nil && capturedEvidence != nil && capturedEvidence.Outcome == reviewtransaction.VerificationOutcomePassed {
								_, readErr := readCapturedProviderTargetedValidatorResult(ctx, root, store.Dir, record.State, record.Revision)
								switch {
								case readErr == nil:
									capturedProviderTargetedValidator = true
								case errors.Is(readErr, errReviewProviderTargetedValidatorResultNotCaptured):
									// OpenCode and the pi host relay are the
									// host-mediated providers. When no current slot
									// exists, they receive the Go-issued task that can
									// fill one; a readable occupied slot is
									// provider-generic.
									if providerRoleHost {
										providerRole = reviewerprovider.RoleTargetedValidator
									}
								case errors.Is(readErr, errReviewTargetedValidationInconclusive):
									// A host-mediated runtime must never be handed the
									// generic "author the validation yourself" form: only
									// Go may run its validator. It gets the same Go-issued
									// role task an unoccupied slot gets, and the capture
									// vacates the non-verdict occupant before publishing.
									if providerRoleHost {
										providerRole = reviewerprovider.RoleTargetedValidator
									}
									// A captured result whose evidence reports the
									// frozen trees could not be inspected is not a
									// verdict and not corruption: the bytes are exactly
									// what the provider produced, no state transitioned,
									// and the correction attempt was never consumed.
									// Its continuation is the opposite of an
									// unverifiable slot's -- restore validator access
									// and run the same validation again -- so it must
									// not be routed as unverifiable evidence (#3378).
									capturedProviderTargetedValidatorInconclusive = true
								default:
									// An unreadable or inadmissible captured slot is
									// unverifiable evidence, never silent continuation.
									artifactErr = readErr
								}
							}
						}
					}
				}
			}
			startLineage := strings.TrimSpace(*lineage)
			if native.Action == reviewtransaction.TargetStatusActionStart {
				startLineage, err = reviewStatusStartLineage(ctx, root, native.TargetIdentity, *lineage, *recoverySuccessor)
				if err != nil {
					return fmt.Errorf("select STATUS atomic START lineage: %w", err)
				}
			}
			if lensContextBudgetExceeded {
				result.Action = reviewtransaction.TargetStatusActionStop
				result.Replayability = reviewtransaction.ReplayabilityManualActionRequired
				if *actionEligibility {
					result.Eligibility = newReviewActionEligibility(result)
				}
			}
			input := reviewNextTransitionInput{Gate: reviewtransaction.GateKind(*gate), Successor: *recoverySuccessor, Reason: *recoveryReason, Actor: *recoveryActor, Authorization: *recoveryAuthorization, RepairActor: *repairActor, RepairReason: *repairReason, RepairAuthorization: *repairAuthorization, StartLineage: startLineage, RuntimeAgent: runtime, ProviderRole: providerRole, CapturedProviderTargetedValidator: capturedProviderTargetedValidator, CapturedProviderTargetedValidatorInconclusive: capturedProviderTargetedValidatorInconclusive, Contract: *contract, RepositoryContext: repositoryContext, ValidationRequest: validationRequest, CorrectionRequest: correctionRequest, EvidenceErr: evidenceErr, CorrectionForecasted: correctionForecasted, CaptureContext: captureContext, Selector: selector, IntendedUntracked: intendedScope, RDDMode: result.rddMode, RDDModeResolved: result.rddModeResolved, LensContextBudgetExceeded: lensContextBudgetExceeded, PreCommitDeliveryAssessment: preCommitDeliveryAssessment}
			var transition ReviewNextTransition
			if !reviewStatusOmitsApprovedReceiptTransition(result) {
				transition = newReviewNextTransition(result, native.SelectedLenses, artifacts, capturedEvidence, artifactErr, input)
				result.NextTransition = &transition
			}
			providerTargetedValidation := (transition.ReasonCode == "targeted_validation_required" || transition.ReasonCode == reviewInconclusiveTargetedValidationReason) &&
				transition.Collect != nil && len(transition.Collect.Inputs) == 1 && transition.Collect.Inputs[0].ProviderTask != nil
			providerCapturedTargetedValidation := transition.ReasonCode == "captured_provider_targeted_validation_ready" && transition.Execute != nil &&
				transition.Execute.Operation == "review.finalize"
			if reviewTransitionValidationRequest(&transition) == nil && transition.ReasonCode != "correction_repository_verification_required" &&
				transition.ReasonCode != "correction_repository_tooling_failed" && !providerTargetedValidation && !providerCapturedTargetedValidation {
				result.ValidationRequest = nil
			}
			// A negotiated invocation is the machine surface end to end: the
			// stdout JSON envelope carries every routing fact (kind,
			// reason_code, forecast), and a successful operation writes zero
			// bytes to stderr. gentle-pi fails closed (UNEXPECTED_STDERR) on
			// any stderr a successful native process writes, so the Tier C
			// stop narration that used to print here was removed rather than
			// allowlisted downstream. The registered Tier C statements remain
			// in review_narration.go as the human-surface vocabulary source.
		}
		if intendedScope.NeedsSelection && !reviewStatusOmitsApprovedReceiptTransition(result) &&
			(result.NextTransition == nil || result.NextTransition.Kind != reviewNextTransitionStop || result.NextTransition.ReasonCode != "rdd_disabled") {
			transition := reviewIntendedUntrackedCollection(result, intendedScope)
			result.NextTransition = &transition
		}
		if *contract == ReviewIntegrationContractV2 && result.NextTransition != nil {
			// The forecast is structural only: it rides the v2 envelope's
			// `forecast` field and is never narrated to stderr, because a
			// successful negotiated operation must stay byte-silent there.
			forecast := newReviewForecast(*result.NextTransition)
			result.Forecast = &forecast
		}
		var validationErr error
		if compactAuthority != nil {
			validationErr = result.validateWithCompactAuthority(compactAuthority)
		} else {
			validationErr = result.Validate()
		}
		if validationErr != nil {
			return fmt.Errorf("validate negotiated review status: %w", validationErr)
		}
		return encodeReviewJSON(stdout, result)
	}
	if *actionEligibility || *nextTransition {
		return errors.New(reviewContractRequiredForActionEligibilityReason)
	}
	if strings.TrimSpace(*runtimeAgent) != "" || strings.TrimSpace(*lineage) != "" || strings.TrimSpace(*baseRef) != "" || strings.TrimSpace(*baseTree) != "" || committedOnlyProvided || *workspaceOverlay || *projection != string(reviewtransaction.ProjectionWorkspace) || *gate != string(reviewtransaction.GatePreCommit) || *recoverySuccessor != "" || *recoveryReason != "" || *recoveryActor != "" || *recoveryAuthorization != "" || *repairActor != "" || *repairReason != "" || *repairAuthorization != "" || reviewIntendedUntrackedDeclared(untrackedScope, intendedUntracked, expectedUntrackedInventory) {
		return errors.New(reviewStatusTargetSelectorsRequireContractReason)
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	report, err := reviewtransaction.InventoryAuthority(ctx, root)
	if err != nil {
		return fmt.Errorf("inventory review authority: %w", err)
	}
	return encodeReviewJSON(stdout, report)
}

// reviewFreshAtomicTargetStatus projects a frozen target into the only status
// decision a selector-free fresh compact START needs. It deliberately has no
// authority view: CreateOrReplayAtomicStart owns the exact-lineage decision.
func reviewFreshAtomicTargetStatus(target reviewtransaction.Target, snapshot reviewtransaction.Snapshot) reviewtransaction.TargetStatusResult {
	action, replayability := reviewFreshStatusPreflight(snapshot)
	projection := reviewtransaction.TargetProjectionStatus{
		Kind: snapshot.Kind, Projection: facadeProjection(snapshot.Projection), BaseTree: snapshot.BaseTree,
		InitialReviewTree: snapshot.CandidateTree, CurrentCandidateTree: snapshot.CandidateTree,
		PathsDigest: snapshot.PathsDigest, Paths: append([]string{}, snapshot.Paths...),
		IntendedUntracked: append([]string{}, snapshot.IntendedUntracked...), IntendedUntrackedProof: snapshot.IntendedUntrackedProof,
		InitialSnapshotIdentity: snapshot.Identity, CurrentSnapshotIdentity: snapshot.Identity,
	}
	return reviewtransaction.TargetStatusResult{
		Applicability: reviewtransaction.TargetApplicabilityUnrelated,
		Action:        action, Replayability: replayability,
		TargetIdentity: snapshot.Identity, Projection: projection, CandidateLineageIDs: []string{},
		Decision: reviewtransaction.TargetStatusDecision{
			CandidateRelation:  reviewtransaction.TargetApplicabilityUnrelated,
			SemanticTransition: action,
			TargetIdentity:     snapshot.Identity, Selector: target,
		},
	}
}

// reviewFreshStatusPreflight makes the fresh, store-free STATUS classification
// agree with START before a review.start transition is emitted. The transition
// builder continues to own the published stop reason codes.
func reviewFreshStatusPreflight(snapshot reviewtransaction.Snapshot) (reviewtransaction.TargetStatusAction, reviewtransaction.Replayability) {
	if snapshot.Kind == reviewtransaction.TargetBaseWorkspaceOverlay && snapshot.Projection == reviewtransaction.ProjectionStaged ||
		snapshot.Kind == reviewtransaction.TargetBaseDiff && reviewStartEmptyCandidateScope(snapshot) {
		return reviewtransaction.TargetStatusActionStop, reviewtransaction.ReplayabilityManualActionRequired
	}
	return reviewtransaction.TargetStatusActionStart, reviewtransaction.ReplayabilityNotReplayable
}

// reviewStartEmptyCandidateScope is shared by fresh STATUS and START so neither
// surface can offer or create an empty current-changes or base-diff review.
func reviewStartEmptyCandidateScope(snapshot reviewtransaction.Snapshot) bool {
	return len(snapshot.Paths) == 0 &&
		(snapshot.Kind == reviewtransaction.TargetCurrentChanges || snapshot.Kind == reviewtransaction.TargetBaseDiff)
}

func hasRepositoryContextIntent(intents []reviewtransaction.CompactEffectIntent) bool {
	for _, intent := range intents {
		if intent.Class == reviewtransaction.CompactEffectClassRepositoryContext {
			return true
		}
	}
	return false
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
	var untrackedScope, expectedUntrackedInventory reviewSingleValueFlag
	var intendedUntracked reviewRepeatedPathFlag
	flags.Var(&untrackedScope, "untracked-scope", "explicit untracked scope for a current-changes successor: exclude or select (default: inherit the predecessor's frozen declaration)")
	flags.Var(&intendedUntracked, "intended-untracked", "repo-relative untracked path to include; repeat for each path")
	flags.Var(&expectedUntrackedInventory, "expected-untracked-inventory", "sha256 inventory digest from review status")
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
	// Issue #2394: a recovery successor declares scope exactly the way a fresh
	// START does, so it never re-sweeps the worktree either. Issue #3159:
	// inheriting the predecessor's frozen, explicitly authorized declaration
	// is not a sweep — those exact paths were human-selected and frozen into
	// the authority being recovered, and dropping them silently rebinds the
	// successor to a partial candidate. Only the current-changes successor
	// carries the declaration forward; base-diff, overlay, and release
	// scopes never hold intended-untracked paths. Issue #1972: an explicit
	// recovery-time declaration (the same flags START and STATUS accept)
	// overrides inheritance, so the successor target derives from the exact
	// selection STATUS authorized rather than from the predecessor alone.
	declaredSelection := reviewIntendedUntrackedDeclared(untrackedScope, intendedUntracked, expectedUntrackedInventory)
	currentChangesSuccessor := !*releaseScope && !*committedOnly && !stagedScopeOverlay && !overlay
	if declaredSelection && !currentChangesSuccessor {
		return errors.New("intended-untracked selection requires a current-changes recovery; rerun `gentle-ai review recover` without --untracked-scope, --intended-untracked, and --expected-untracked-inventory")
	}
	if declaredSelection && projection == reviewtransaction.ProjectionStaged {
		return errors.New("staged projection does not accept intended-untracked selection; remove those flags and rerun `gentle-ai review recover --projection staged`")
	}
	intended := []string{}
	switch {
	case declaredSelection:
		scope, scopeErr := reviewIntendedUntrackedScopeForTarget(context.Background(), reviewtransaction.SnapshotBuilder{Repo: root}, untrackedScope, intendedUntracked, expectedUntrackedInventory)
		if scopeErr != nil {
			return scopeErr
		}
		intended = append(intended, scope.Intended...)
	case currentChangesSuccessor && predecessorRecord.State.InitialSnapshot.Kind == reviewtransaction.TargetCurrentChanges:
		intended = append(intended, predecessorRecord.State.InitialSnapshot.IntendedUntracked...)
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
	policyContent := string(policy)
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: *successor, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: predecessorRecord.State.Generation + 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), PolicyContent: &policyContent,
		RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &changedLines,
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
		if stagedScopeOverlay && (predecessorRecord.State.State == reviewtransaction.StateApproved || predecessorRecord.State.State == reviewtransaction.StateCorrectionRequired) {
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
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationBindSDD, binding, binding, *contract)
}

func RunReviewInvalidate(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review invalidate", stdout, "Terminally invalidate one explicit pristine reviewing authority. An approved compact authority is no longer invalidated by this verb -- invalidated is a derived verdict `review validate --gate <gate>` reports directly, never a write.")
	cwd := flags.String("cwd", "", "repository path")
	lineage := flags.String("lineage", "", "explicit review lineage identifier")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	reason := flags.String("reason", "", "non-empty terminal invalidation reason for a pristine reviewing authority")
	gate := flags.String("gate", "", "retained only to name the runnable `review validate --gate <gate>` alternative for an approved compact authority; performs no gate-derived invalidation")
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
			// Wave 5 (Gate Cutover) Slice 7, design decision 2: `invalidated`
			// is now a derived verdict (relation in {changed, unrelated} =>
			// GateInvalidated), not a write this verb performs.
			// InvalidateApprovedCompactAuthority is deleted
			// (TestNoGateWritesAuthority_CallAbsenceGuard,
			// internal/reviewtransaction, proves it by call-absence): a
			// drifted approved candidate already denies at `review validate
			// --gate <gate>` through the SAME gate evaluation this verb used
			// to re-derive and then persist, and the receipt is never
			// removed.
			gateName := strings.TrimSpace(*gate)
			if gateName == "" {
				gateName = "<gate>"
			}
			return fmt.Errorf("review invalidate no longer performs gate-derived invalidation for lineage %q; invalidated is now a derived verdict, never a write; see it instead: gentle-ai review validate --cwd %s --lineage %s --gate %s", *lineage, strings.TrimSpace(*cwd), *lineage, gateName)
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

func runReviewFacadeStart(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review start", stdout, "Freeze live Git scope and derive the bounded review tier, lenses, and correction budget.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	runtimeAgent := flags.String("agent", "", "generated active runtime identity for negotiated lifecycle routing")
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
	locale := flags.String("locale", "", "optional consent-envelope locale: en or es")
	untrackedScope := reviewSingleValueFlag{}
	intendedUntracked := reviewRepeatedPathFlag{}
	expectedUntrackedInventory := reviewSingleValueFlag{}
	flags.Var(&untrackedScope, "untracked-scope", "explicit untracked scope: exclude or select")
	flags.Var(&intendedUntracked, "intended-untracked", "repo-relative untracked path to include; repeat for each path")
	flags.Var(&expectedUntrackedInventory, "expected-untracked-inventory", "sha256 inventory digest from review status")
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
	runtimeRequested := reviewRuntimeAgentCount(args) > 0
	// Wave 4 S4 (design.md decision 5): transport capability admission is
	// the broadest precondition — "can this runtime participate in review
	// at all" — so it runs before the narrower immutable-receipt-transport
	// eligibility check below, and before any authority, tier, lens,
	// budget, or collection slot exists. An absent agent identity is the
	// manual/non-agent compatibility path and is not gated.
	if runtimeRequested && reviewRuntimeAgentCount(args) == 1 {
		if err := authorizeReviewTransportCapability(*runtimeAgent); err != nil {
			return reviewPreflightRefusal(reviewTransportCapabilityUnsupportedReason, err)
		}
	}
	// The same admission as the capability gate above: an absent agent
	// identity is the manual/non-agent compatibility path and is not gated,
	// so the provider-returned START of an undeclared route stays runnable.
	// Every declared identity is still proven before authority is created.
	if negotiated && runtimeRequested {
		if reviewRuntimeAgentCount(args) != 1 {
			// refusal:by-design world-action: an ambiguous runtime identity cannot safely create review authority
			return reviewPreflightRefusal(reviewImmutableTransportUnsupportedReason, errors.New("negotiated START requires exactly one generated runtime identity"))
		}
		if _, err := reviewRuntimeWithImmutableTransport(*runtimeAgent); err != nil {
			return reviewPreflightRefusal(reviewImmutableTransportUnsupportedReason, err)
		}
	}
	if err := validateReviewStartBinding(args, negotiated, *targetIdentity, *projection, *baseRef, *lineage, *committedOnly, *workspaceOverlay, *consent, *locale); err != nil {
		return reviewPreflightError(err)
	}
	consentMode := reviewStartConsentMode(strings.TrimSpace(*consent))
	consentLocale, err := normalizeReviewConsentLocale(*locale)
	if err != nil {
		return reviewPreflightError(err)
	}
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
	if strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		dirtyTracked, dirtyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).HasDirtyTrackedChanges(ctx)
		if dirtyErr != nil {
			return fmt.Errorf("detect dirty tracked changes for committed review: %w", dirtyErr)
		}
		if dirtyTracked && !*committedOnly {
			return errors.New("review start with --base-ref omits dirty tracked changes; rerun with --committed-only to acknowledge committed-only review scope")
		}
	}
	intendedScope := reviewIntendedUntrackedScope{Intended: []string{}}
	if selectedProjection == reviewtransaction.ProjectionStaged {
		if reviewIntendedUntrackedDeclared(untrackedScope, intendedUntracked, expectedUntrackedInventory) {
			return reviewPreflightError(errors.New("staged projection does not accept intended-untracked selection; remove those flags and rerun `gentle-ai review start --projection staged`"))
		}
	} else {
		intendedScope, err = reviewIntendedUntrackedScopeForTarget(ctx, reviewtransaction.SnapshotBuilder{Repo: root}, untrackedScope, intendedUntracked, expectedUntrackedInventory)
		if err != nil {
			return reviewPreflightError(err)
		}
		if intendedScope.NeedsSelection {
			return reviewPreflightError(reviewIntendedUntrackedSelectionRequired(intendedScope))
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: intendedScope.Intended}
	if strings.TrimSpace(*baseRef) != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = strings.TrimSpace(*baseRef)
	}
	if *workspaceOverlay {
		target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
	}
	snapshot, err := reviewFacadeBuildStartSnapshot(ctx, reviewtransaction.SnapshotBuilder{Repo: root}, target)
	if err != nil {
		return fmt.Errorf("build facade review target: %w", err)
	}
	if negotiated && snapshot.Identity != *targetIdentity {
		return reviewPreflightRefusal(reviewPreflightStaleTargetReason,
			errors.New("review start target does not match the freshly built snapshot"))
	}
	// Issue #2586: a TargetCurrentChanges candidate with zero changed paths
	// (a clean, fully-committed worktree) or a TargetBaseDiff candidate
	// whose named base nets zero changed paths (a mode-only or truly-empty
	// diff) would otherwise freeze as an empty candidate, pass risk
	// assessment, and mint an approved receipt that inspected nothing. Left
	// alive on ANY route, that receipt's base_tree/final_candidate_tree can
	// coincide with a genuinely unreviewed candidate's tree and get
	// discovered ahead of it. This guard used to fire only on the negotiated
	// route and only for TargetCurrentChanges, so a plain (non-negotiated)
	// `review start` on a clean worktree minted exactly this receipt
	// (reported in #2586) and a `--base-ref` netting no manifest entries was
	// unguarded on either route. Refusing here, before any authority is
	// created, for both routes and both target kinds, and naming --base-ref
	// as the way to name a real comparison base instead of silently deriving
	// one, is cheaper and safer than the two narrower guards it replaces.
	// This refusal already spells out its own runnable resolution
	// (reviewStartEmptyCandidateHint), exactly as it did before this change,
	// so no additional exemption marker is needed here.
	if reviewStartEmptyCandidateScope(snapshot) {
		return reviewPreflightRefusal(reviewPreflightEmptyCandidateReason,
			errors.New(reviewStartEmptyCandidateHint))
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("classify facade review target: %w", err)
	}
	changedLines := assessment.ChangedLines
	lenses, err := facadeSelectedLenses(assessment, *focus)
	if err != nil {
		return err
	}
	// Issue #2447. The direct route's own response type
	// (ReviewFacadeStartResult) never carries repository_context -- only the
	// negotiated ReviewIntegrationStartResult does -- so no reviewer lens can
	// ever capture a result against a lineage this call would create, and
	// the negotiated facade does not rediscover a lineage the direct route
	// created. Refusing here, before the candidate is frozen into any
	// lineage, authority, tier freeze, or budget, leaves nothing behind to
	// strand. The maintainer decision recorded on #2447 made this the
	// permanent shape of the direct route, not a stopgap: carrying
	// repository_context on this path was considered and cancelled.
	// Lineages the direct route created before this fix landed are a
	// separate, still-open recovery/visibility gap tracked on #2447 and
	// rdd-single-lifecycle-cutover; this refusal only stops new ones. The
	// error below names the exact runnable negotiated continuation, so the
	// refusal-resolution ratchet resolves it by naming, needing no by-design
	// annotation.
	if !negotiated && target.Kind != reviewtransaction.TargetCurrentChanges && len(lenses) > 0 {
		return reviewPreflightRefusal(reviewPreflightDirectRouteUncompletableReason,
			fmt.Errorf("review start without --contract cannot produce a completable review because its %d selected lens(es) require repository_context, which only the negotiated contract form publishes; rerun with `gentle-ai review start %s` instead",
				len(lenses), strings.TrimPrefix(reviewNegotiatedStartCommand(snapshot, *runtimeAgent), "gentle-ai review start ")))
	}
	// The candidate is frozen and the tier is classified, so this is the one
	// point where the kill switch can stop a start and consent can name the real
	// reason. Nothing has been persisted yet, so refusing here leaves no
	// authority behind.
	if err := authorizeReviewStart(ctx, root, assessment, consentMode, negotiated); err != nil {
		if errors.Is(err, errReviewConsentQuestionRequired) {
			// The caller declared it can relay a blocking question, so the
			// typed question IS this start's response. Nothing has been
			// persisted; the named follow-up invocations answer for exactly
			// this frozen candidate and nothing else.
			question, questionErr := newReviewIntegrationConsentResult(snapshot, assessment,
				reviewConsentFollowUpBase(*cwd, snapshot.Identity, selectedProjection, strings.TrimSpace(*lineage),
					strings.TrimSpace(*baseRef), strings.TrimSpace(*policySource), strings.TrimSpace(*focus),
					strings.TrimSpace(*tracePath), *committedOnly, *workspaceOverlay, *contract, *runtimeAgent, strings.TrimSpace(*locale), intendedScope), *contract, *runtimeAgent, consentLocale)
			if questionErr != nil {
				return questionErr
			}
			return encodeReviewJSON(stdout, question)
		}
		if errors.Is(err, errReviewDeclinedForCandidate) {
			// Wave 5 (Gate Cutover) Slice 6, design decision 6: a candidate
			// decline is no longer durably recorded at all -- consistent with
			// Wave 4 decision 4 ("decline = unmanaged proceed: nothing
			// recorded"), an inert authority-shaped file is exactly the mirror
			// pattern that decision removed on the SDD side. Declining here
			// governs only this exact `review start` call: it never creates a
			// review lineage, a receipt, or any durable authorization a later
			// gate could read.
			if negotiated && consentMode != reviewConsentModeDeclined {
				return err
			}
			// The relayed --consent declined answer reports a typed choice, but
			// does not create review authority or consume the legacy consent latch.
			return encodeReviewJSON(stdout, reviewFacadeStartDeclinedResult(snapshot, assessment))
		}
		return err
	}
	// Every enabled START creates or replays only its exact compact authority.
	// Historical v1/v3 records never participate in ordinary RDD lifecycle
	// routing; explicit compatibility owners remain responsible for any manual
	// inspection or disposition.
	{
		request, err := prepareReviewFacadeCompactAtomicStart(ctx, root, strings.TrimSpace(*lineage), strings.TrimSpace(*policySource), target, snapshot, assessment, changedLines, lenses)
		if err != nil {
			return reviewPreflightError(fmt.Errorf("prepare compact atomic facade review: %w", err))
		}
		if err := reviewLensContextCompactAtomicStartBudgetRefusal(ctx, root, request); err != nil {
			return err
		}

		// Compact budget/context preflight completes before the live snapshot is
		// revalidated and before the one exact create-or-replay call.
		var frozenContext *reviewtransaction.FrozenCandidateContext
		if negotiated && len(request.Binding.SelectedLenses) > 0 {
			contextBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
			contextResult, contextErr := renderReviewStartFrozenCandidateContext(ctx, contextBuilder, snapshot)
			if contextErr == nil && *contract == ReviewIntegrationContractV1 {
				contextResult, contextErr = contextBuilder.WithLegacyCandidateDiff(ctx, snapshot, contextResult)
			}
			if contextErr != nil {
				return &reviewStartContextError{LineageID: request.Binding.LineageID, Cause: contextErr}
			}
			frozenContext = &contextResult
		}
		if err := (reviewtransaction.SnapshotBuilder{Repo: root}).ValidateLiveSnapshot(ctx, snapshot); err != nil {
			return reviewPreflightRefusal(reviewPreflightStaleTargetReason,
				fmt.Errorf("frozen review target changed before authority creation; refresh it with `gentle-ai review status --cwd <repo> --contract %s --next-transition`: %w", ReviewIntegrationContractV2, err))
		}

		started, err := runReviewFacadeCompactAtomicStart(ctx, root, request)
		if err != nil {
			return fmt.Errorf("start compact atomic facade review: %w", err)
		}
		record := started.Record
		action := "created"
		if started.Replayed {
			action = "resumed"
		}
		legacyResult := reviewFacadeStartResultFor(action, len(record.State.SelectedLenses) > 0, record.State)
		if started.Replayed && (!negotiated || *contract == ReviewIntegrationContractV2) {
			legacyResult.Action = "replayed"
		}
		legacyResult.RiskEvidence = reviewConsentRiskEvidence(assessment)
		if !negotiated {
			return encodeReviewJSON(stdout, legacyResult)
		}
		repositoryContextHandle, contextErr := reviewtransaction.PublishReviewRepositoryContext(ctx, root, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: record.State.LineageID, TargetIdentity: snapshot.Identity, Revision: record.Revision,
		})
		if contextErr != nil {
			return &reviewStartContextError{AuthoritySelected: true, LineageID: record.State.LineageID, StoreRevision: record.Revision, Cause: contextErr}
		}
		repositoryContext := &ReviewRepositoryContextReference{
			Capability: reviewtransaction.ReviewRepositoryContextCapability, Handle: repositoryContextHandle,
			Revision: record.Revision, TargetIdentity: snapshot.Identity,
		}
		negotiatedResult, err := newReviewIntegrationStartResult(legacyResult, assessment, snapshot.Kind, frozenContext, repositoryContext, *contract)
		if err != nil {
			return &reviewStartContextError{AuthoritySelected: true, LineageID: record.State.LineageID, StoreRevision: record.Revision, Cause: err}
		}
		return encodeReviewJSON(stdout, negotiatedResult)
	}
}
func validateReviewStartBinding(args []string, negotiated bool, target, projection, baseRef, lineage string, committedOnly, workspaceOverlay bool, consent, locale string) error {
	counts := reviewStartBindingFlagCounts(args)
	switch reviewStartConsentMode(strings.TrimSpace(consent)) {
	case reviewConsentModeNone, reviewConsentModeRelay, reviewConsentModeGranted, reviewConsentModeDeclined:
	default:
		return errors.New(reviewStartConsentValueReason)
	}
	if !negotiated {
		if counts["agent"] != 0 {
			// refusal:by-design operator-knowledge: only the generated negotiated route owns an active runtime identity
			return errors.New("review start --agent requires a negotiated --contract")
		}
		if counts["target"] != 0 {
			return errors.New(reviewStartTargetRequiresContractReason)
		}
		if counts["consent"] != 0 {
			return errors.New(reviewStartConsentRequiresContractReason)
		}
		if counts["locale"] != 0 {
			// refusal:by-design operator-knowledge: only the caller can supply the complete candidate-bound negotiated START invocation
			return errors.New("review start --locale requires a negotiated --contract")
		}
		return nil
	}
	for _, name := range []string{"contract", "agent", "target", "projection", "lineage", "base-ref", "committed-only", "workspace-overlay", "consent", "locale"} {
		if counts[name] > 1 {
			return fmt.Errorf("review start repeats --%s", name)
		}
	}
	if _, err := normalizeReviewConsentLocale(locale); err != nil {
		return err
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
		case "contract", "agent", "target", "projection", "lineage", "base-ref", "committed-only", "workspace-overlay", "consent", "locale":
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
			"untracked-scope":               reviewIntegrationValueFlag,
			"intended-untracked":            reviewIntegrationValueFlag,
			"expected-untracked-inventory":  reviewIntegrationValueFlag,
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
	contract, runtimeAgent string,
	locale string, intendedScope reviewIntendedUntrackedScope,
) string {
	parts := []string{
		"gentle-ai review start",
		"--contract " + contract,
		"--cwd " + reviewTransitionShellWord(cwd),
		"--target " + target,
		"--projection " + string(projection),
	}
	if runtimeAgent != "" {
		parts = append(parts, "--agent "+runtimeAgent)
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
		parts = append(parts, "--policy "+reviewTransitionShellWord(policy))
	}
	// The focus default never needs restating; only an explicit non-default
	// focus changes what the answered start would select.
	if focus != "" && focus != "reliability" {
		parts = append(parts, "--focus "+focus)
	}
	if trace != "" {
		parts = append(parts, "--trace "+reviewTransitionShellWord(trace))
	}
	if locale != "" {
		parts = append(parts, "--locale "+locale)
	}
	for _, argument := range reviewStartIntendedUntrackedArguments(intendedScope) {
		parts = append(parts, reviewTransitionShellWord("--"+argument.Name+"="+argument.Value))
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

func reviewFacadeStartResultFor(action string, lensesRequired bool, authority reviewtransaction.CompactState) ReviewFacadeStartResult {
	legacyBudget, _ := reviewtransaction.CorrectionBudget(authority.OriginalChangedLines)
	result := ReviewFacadeStartResult{
		Operation: "review/start", Action: action, LensesRequired: lensesRequired,
		LineageID: authority.LineageID, State: authority.State, RiskLevel: authority.RiskLevel,
		SelectedLenses: append([]string{}, authority.SelectedLenses...), LensBindings: facadeLensBindings(authority.SelectedLenses),
		Projection:   facadeProjection(authority.InitialSnapshot.Projection),
		ChangedFiles: len(authority.InitialSnapshot.Paths), TargetIdentity: authority.InitialSnapshot.Identity,
		ChangedLines: authority.OriginalChangedLines, CorrectionBudget: legacyBudget,
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

func reviewFinalizeFlagProvided(args []string, name string) bool {
	for _, argument := range args {
		trimmed := strings.TrimLeft(argument, "-")
		if trimmed == name || strings.HasPrefix(trimmed, name+"=") {
			return true
		}
	}
	return false
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
	runtimeAgent := flags.String("agent", "", "generated registered runtime identity for native provider-role execution")
	expectedSubmissionRevision := flags.String("expected-revision", "", "provider-issued compact authority revision for a submission descriptor")
	targetIdentity := flags.String("target", "", "provider-issued target identity for a submission descriptor")
	requestHash := flags.String("request-hash", "", "provider-issued correction or validation request hash")
	repositoryContext := flags.String("repository-context", "", "provider-issued opaque repository context for a submission descriptor")
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
	failedProvided := reviewFinalizeFlagProvided(args, "failed")
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	if (*actionEligibility || *nextTransition) && !negotiated {
		return reviewPreflightError(errors.New(reviewContractRequiredForActionEligibilityReason))
	}
	providerRuntime := model.AgentID("")
	if reviewRuntimeAgentCount(args) != 0 {
		if !negotiated || *contract != ReviewIntegrationContractV2 || reviewRuntimeAgentCount(args) != 1 {
			return reviewPreflightError(errors.New("review finalize --agent requires exactly one negotiated v2 runtime identity; refresh with `gentle-ai review status --contract gentle-ai.review-integration/v2 --next-transition`"))
		}
		resolved, runtimeErr := reviewRuntimeWithImmutableTransport(*runtimeAgent)
		if runtimeErr != nil {
			return reviewPreflightError(runtimeErr)
		}
		if !reviewProviderCaptureRuntime(resolved) {
			return reviewPreflightError(fmt.Errorf("review finalize provider runtime %q is host-mediated; use its live transport collection", resolved)) // refusal:by-design world-action: host-mediated runtimes do not use subprocess finalization
		}
		providerRuntime = resolved
	}
	submissionBindingProvided := reviewFinalizeFlagProvided(args, "target") || reviewFinalizeFlagProvided(args, "request-hash") ||
		reviewFinalizeFlagProvided(args, "repository-context")
	expectedRevisionOnly := reviewFinalizeFlagProvided(args, "expected-revision") && !submissionBindingProvided
	if submissionBindingProvided && (!negotiated || *contract != ReviewIntegrationContractV2 || strings.TrimSpace(*expectedSubmissionRevision) == "" ||
		strings.TrimSpace(*targetIdentity) == "" || strings.TrimSpace(*requestHash) == "" || strings.TrimSpace(*repositoryContext) == "") {
		return reviewPreflightError(errors.New("review finalize submission descriptors require the complete v2 revision, target, request hash, and repository context binding; refresh with gentle-ai review status --next-transition"))
	}
	if reviewFinalizeFlagProvided(args, "repository-context") && reviewFinalizeFlagProvided(args, "cwd") {
		return reviewPreflightError(errors.New("review finalize submission descriptors cannot combine --repository-context with --cwd; refresh with gentle-ai review status --next-transition"))
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
	if providerRuntime != "" && (strings.TrimSpace(*refuterPath) != "" || strings.TrimSpace(*validationPath) != "") {
		return reviewPreflightError(errors.New("review finalize --agent materializes provider refuter and validator requests; omit --refuter and --validation")) // refusal:by-design operator-knowledge: a provider runtime must receive the Go-materialized request rather than caller-authored role output
	}
	// Refused before any repository or authority work, so the rejection cannot
	// advance the lineage or consume a lens slot. reviewUnadmittedResultRefusal
	// records why this route is refused outright rather than admitted in place.
	// guard:population finalize-result-admission too-loose: legitimate finalize results are provider-admitted artifacts or captured results; raw --result files are excluded
	if len(resultPaths) != 0 {
		return reviewPreflightError(errors.New(reviewUnadmittedResultRefusal))
	}
	var root string
	if submissionBindingProvided {
		root, err = resolveOpaqueReviewRepositoryRoot(ctx, *repositoryContext, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *targetIdentity, Revision: *expectedSubmissionRevision,
		})
	} else {
		root, err = (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
		if err != nil {
			return fmt.Errorf("resolve review repository root: %w", err)
		}
	}
	if err != nil {
		return err
	}
	// An interrupted approved burn has no live v2 directory to discover. Its
	// exact --lineage retry is the only mutation that may recover staged bytes,
	// so it runs before every ordinary authority lookup and never guesses a
	// selector-free lineage.
	if exactLineage := strings.TrimSpace(*lineage); exactLineage != "" {
		staged, stagedFound, inspectErr := reviewtransaction.InspectStagedApprovedCompactBurn(ctx, root, exactLineage)
		recoveredDeleting := false
		if inspectErr != nil {
			recoveredDeleting, err = reviewtransaction.RecoverDeletingCompactBurnWithoutAuthority(ctx, root, exactLineage, strings.TrimSpace(*expectedSubmissionRevision))
			if err != nil {
				return fmt.Errorf("recover deleting staged approved compact review authority: %w", err)
			}
			if !recoveredDeleting {
				return fmt.Errorf("inspect staged approved compact review authority: %w", inspectErr)
			}
			staged, stagedFound = reviewtransaction.CompactRecord{
				Revision: *expectedSubmissionRevision,
				State:    reviewtransaction.CompactState{LineageID: exactLineage, State: reviewtransaction.StateApproved},
			}, true
		}
		if stagedFound {
			if expectedRevisionOnly && !recoveredDeleting {
				return reviewPreflightError(errors.New("review finalize submission descriptors require the complete v2 revision, target, request hash, and repository context binding; refresh with gentle-ai review status --next-transition"))
			}
			if !facadeFinalizeReplayInputsEmpty(resultPaths, resultArtifacts, resultArtifactFiles, *capturedResults, *capturedEvidence, *validationPath, *refuterPath, *evidencePath, *correctionLines, *failed, *tracePath, providerRuntime) {
				return errors.New("terminal review finalize accepts no review inputs; exact replay requires only --lineage")
			}
			if !recoveredDeleting {
				if _, recovered, recoveryErr := reviewtransaction.RecoverStagedApprovedCompactBurn(ctx, root, exactLineage); recoveryErr != nil {
					return fmt.Errorf("recover staged approved compact review authority: %w", recoveryErr)
				} else if !recovered {
					return errors.New("staged approved compact review authority disappeared before exact FINALIZE recovery; run gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition before retrying")
				}
			}
			store, storeErr := reviewtransaction.CompactAuthoritativeStore(ctx, root, exactLineage)
			if storeErr != nil {
				return storeErr
			}
			return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, staged.State, staged.Revision, store, reviewApprovedCompactBurnedFinalizeAction, reviewFinalizeOutputContext{Context: ctx, Repo: root, AuthorityBurned: true})
		}
	}
	if expectedRevisionOnly {
		return reviewPreflightError(errors.New("review finalize submission descriptors require the complete v2 revision, target, request hash, and repository context binding; refresh with gentle-ai review status --next-transition"))
	}
	store, record, err := discoverCompactFacadeFinalize(ctx, root, *lineage)
	if err != nil {
		var absent *reviewCompactFacadeLineageAbsentError
		if errors.As(err, &absent) {
			return reviewPreflightError(err)
		}
		return err
	}
	store.TracePath = strings.TrimSpace(*tracePath)
	state := record.State
	if err := validateReviewFinalizeSubmission(ctx, root, state, record.Revision, args, submissionBindingProvided,
		reviewFinalizeFlagProvided(args, "correction-lines"), *correctionLines, *validationPath, *capturedEvidence,
		*expectedSubmissionRevision, *targetIdentity, *requestHash, *repositoryContext); err != nil {
		return reviewPreflightError(err)
	}
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
			if blocked := reviewtransaction.CompactAuthorityLineageBlocked(ctx, root, *lineage); blocked != nil {
				return blocked
			}
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
	if terminalAtEntry && !facadeFinalizeReplayInputsEmpty(resultPaths, resultArtifacts, resultArtifactFiles, *capturedResults, *capturedEvidence, *validationPath, *refuterPath, *evidencePath, *correctionLines, *failed, *tracePath, providerRuntime) {
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
			return newFacadeReceiptPublicationError(ctx, root, state.LineageID, state.CurrentSnapshot.Identity, requestDigest, err)
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
	} else if *capturedResults && state.State == reviewtransaction.StateReviewing {
		// The slot READ is deliberately not gated on a compiled --agent: a
		// host-mediated finalize (OpenCode's transport or the pi host relay's
		// capture-refuter submission) occupies the same compact slot, and the
		// ordinary `--captured-results` finalize must discover it identically.
		// Only the in-process provider CAPTURE below stays compiled-only.
		slot, slotErr := reviewtransaction.ReadCompactRefuterResultSlot(store.Dir)
		if slotErr != nil {
			return reviewPreflightError(fmt.Errorf("read captured provider refuter result: %w", slotErr))
		}
		switch {
		case slot.Occupied:
			refuter, err = readCapturedProviderRefuterResult(ctx, root, store.Dir, state, record.Revision)
		case providerRuntime != "":
			var captured bool
			_, captured, err = reviewProviderCaptureRefuter(ctx, root, store, state, record.Revision, providerRuntime)
			if err == nil && captured {
				refuter, err = readCapturedProviderRefuterResult(ctx, root, store.Dir, state, record.Revision)
			}
		}
		if err != nil {
			return reviewPreflightError(err)
		}
	}
	// Observed only after every consumer that re-derives the revision from
	// the state it is handed (the provider refuter materialization above)
	// has run: CompactRevisionForState hashes ReviewerContextLevel, so
	// setting it earlier made every compiled-runtime finalize diverge from
	// the frozen record revision. The completion plan below still carries
	// the observation onto the receipt.
	if *capturedResults && state.State == reviewtransaction.StateReviewing {
		state.ReviewerContextLevel = discoverReviewerContextLevel(ctx, root, store.Dir, state, record.Revision)
	}
	var evidence []byte
	var capturedVerification *reviewtransaction.CapturedVerificationEvidence
	effectiveFailed := *failed
	if strings.TrimSpace(*evidencePath) != "" {
		evidence, err = readFacadeBytes(*evidencePath)
		if err != nil {
			return reviewPreflightError(fmt.Errorf("read final review evidence: %w", err))
		}
	}
	if *capturedEvidence {
		evidenceTarget, targetErr := facadeVerificationEvidenceTarget(ctx, root, state, record.Revision)
		if targetErr != nil {
			return reviewPreflightError(targetErr)
		}
		captured, captureErr := reviewtransaction.ReadCapturedVerificationEvidence(store.Dir, state.LineageID, record.Revision, evidenceTarget)
		if captureErr != nil {
			return reviewPreflightError(captureErr)
		}
		capturedVerification = &captured
		evidence = captured.Payload
		derivedFailed := captured.Record.Outcome != reviewtransaction.VerificationOutcomePassed
		if failedProvided && *failed != derivedFailed {
			return reviewPreflightError(errors.New("--failed conflicts with captured verification evidence outcome")) // refusal:by-design operator-knowledge: callers must omit the legacy boolean or make it agree with the immutable captured outcome
		}
		effectiveFailed = derivedFailed
	}
	if submissionBindingProvided && !reviewFinalizeFlagProvided(args, "correction-lines") && strings.TrimSpace(*validationPath) == "" &&
		*capturedEvidence && state.State == reviewtransaction.StateCorrectionRequired && capturedVerification != nil &&
		capturedVerification.Record.Outcome == reviewtransaction.VerificationOutcomePassed && validation == nil {
		// The Go-issued slot-consumption transition promises an occupied,
		// readable validator slot; an unreadable slot must fail this exact
		// submission instead of silently launching another provider.
		capturedValidation, readErr := readCapturedProviderTargetedValidatorResult(ctx, root, store.Dir, state, record.Revision)
		if readErr != nil {
			return reviewPreflightError(fmt.Errorf("read captured provider targeted validator result: %w", readErr))
		}
		validation = &capturedValidation
	} else if state.State == reviewtransaction.StateCorrectionRequired && capturedVerification != nil &&
		capturedVerification.Record.Outcome == reviewtransaction.VerificationOutcomePassed && validation == nil {
		// Discovery of an occupied validator slot is host-mediated-safe: the
		// pi host relay's capture-validation submission occupies the same
		// compact slot the compiled path does, so the ordinary finalize (no
		// --agent) reads it back here. Only the in-process provider CAPTURE
		// stays compiled-only, and a host-mediated finalize with no captured
		// slot keeps its ordinary --validation route.
		capturedValidation, readErr := readCapturedProviderTargetedValidatorResult(ctx, root, store.Dir, state, record.Revision)
		switch {
		case readErr == nil:
			validation = &capturedValidation
		case providerRuntime != "":
			var captureErr error
			capturedValidation, _, captureErr = reviewProviderCaptureTargetedValidator(ctx, root, store, state, record.Revision, providerRuntime)
			if captureErr != nil {
				return reviewPreflightError(captureErr)
			}
			validation = &capturedValidation
		case errors.Is(readErr, errReviewProviderTargetedValidatorResultNotCaptured):
		default:
			return reviewPreflightError(readErr)
		}
	}
	// A lineage-only finalize call at StateValidating with no request evidence
	// must not silently ignore canonical evidence a separate `review
	// capture-evidence` call already bound to this exact authority. Consume it
	// on the identical bytes path --captured-evidence uses, so the request is
	// a real transition instead of a no-op (1663).
	if len(evidence) == 0 && strings.TrimSpace(*evidencePath) == "" && !*capturedEvidence && state.State == reviewtransaction.StateValidating {
		captured, captureErr := readCapturedFinalEvidence(store.Dir, state, record.Revision)
		switch {
		case captureErr == nil:
			capturedVerification = &captured
			evidence = captured.Payload
			effectiveFailed = captured.Record.Outcome != reviewtransaction.VerificationOutcomePassed
		case !errors.Is(captureErr, errCapturedFinalEvidenceMissing):
			return reviewPreflightError(captureErr)
		}
	}
	if terminalComplete {
		if err := reviewFacadeSyncDirectory(filepath.Dir(store.FinalizeAttemptJournalPath())); err != nil {
			return fmt.Errorf("sync completed finalize journal directory: %w", err)
		}
		if state.State == reviewtransaction.StateApproved {
			if err := reviewtransaction.BurnApprovedCompactAuthority(ctx, root, state.LineageID, record.Revision); err != nil {
				return fmt.Errorf("burn approved compact review authority: %w", err)
			}
			return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, reviewApprovedCompactBurnedFinalizeAction, reviewFinalizeOutputContext{Context: ctx, Repo: root, AuthorityBurned: true})
		}
		return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, "delivery is informational and unmanaged; ordinary repository policy decides delivery", reviewFinalizeOutputContext{Context: ctx, Repo: root})
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
			replayRequest := facadeFinalizeAttemptRequestForCandidate(record, state.CurrentSnapshot, reviewerResults, validation, refuter, replayEvidence, *correctionLines, effectiveFailed, capturedVerification)
			attempt, attemptLoaded, err = store.ReconcileFinalizeAttempt(ctx, replayRequest)
			if err != nil {
				return err
			}
			if index == len(attempt.Transitions)-1 {
				if err := store.CompleteFinalizeAttempt(attempt.Request.RequestDigest); err != nil {
					return err
				}
				return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, "continue the current review state", reviewFinalizeOutputContext{Context: ctx, Repo: root})
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
	plan, err := prepareFacadeFinalizePlan(ctx, root, record.Revision, store.Dir, state, reviewerResults, refuter, validation, evidence, *correctionLines, effectiveFailed, capturedVerification)
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
		outputContext := reviewFinalizeOutputContext{Context: ctx, Repo: root}
		if plan.CapturedEvidence != nil {
			outputContext.CapturedEvidence = &plan.CapturedEvidence.Record
		}
		return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, "continue the current review state", outputContext)
	}
	request := facadeFinalizeAttemptRequestForCandidate(record, plan.Candidate, reviewerResults, validation, refuter, plan.Evidence, *correctionLines, effectiveFailed, plan.CapturedEvidence)
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
	attemptCompleted := false
	defer func() {
		if returnErr == nil && !attemptCompleted {
			completionErr := store.CompleteFinalizeAttempt(requestDigest)
			if completionErr != nil && facadeTerminalState(state.State) {
				returnErr = newFacadeReceiptPublicationError(ctx, root, state.LineageID, state.CurrentSnapshot.Identity, requestDigest, completionErr)
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
		return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, "continue the current review state", reviewFinalizeOutputContext{Context: ctx, Repo: root})
	}
	if terminalAtEntry && terminalReceiptExists {
		if state.State == reviewtransaction.StateApproved {
			if err := store.MarkFinalizeAttemptReceiptPublished(requestDigest); err != nil {
				return err
			}
			if err := store.CompleteFinalizeAttempt(requestDigest); err != nil {
				return newFacadeReceiptPublicationError(ctx, root, state.LineageID, state.CurrentSnapshot.Identity, requestDigest, err)
			}
			attemptCompleted = true
			if err := reviewtransaction.BurnApprovedCompactAuthority(ctx, root, state.LineageID, record.Revision); err != nil {
				return fmt.Errorf("burn approved compact review authority: %w", err)
			}
			return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, reviewApprovedCompactBurnedFinalizeAction, reviewFinalizeOutputContext{Context: ctx, Repo: root, AuthorityBurned: true})
		}
		return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, "delivery is informational and unmanaged; ordinary repository policy decides delivery", reviewFinalizeOutputContext{Context: ctx, Repo: root})
	}
	receipt := terminalReceipt
	if !terminalAtEntry {
		receipt, err = state.Receipt()
		if err != nil {
			return err
		}
	}
	if err := writeCompactFacadeReceipt(ctx, store, receipt); err != nil {
		return newFacadeReceiptPublicationError(ctx, root, state.LineageID, state.CurrentSnapshot.Identity, requestDigest, err)
	}
	published, err := inspectCompactFacadeReceipt(store.ReceiptPath(), receipt)
	if err != nil {
		return newFacadeReceiptPublicationError(ctx, root, state.LineageID, state.CurrentSnapshot.Identity, requestDigest, err)
	}
	if !published {
		return newFacadeReceiptPublicationError(ctx, root, state.LineageID, state.CurrentSnapshot.Identity, requestDigest, errors.New("receipt writer did not materialize the derived receipt"))
	}
	if err := store.MarkFinalizeAttemptReceiptPublished(requestDigest); err != nil {
		return err
	}
	if state.State == reviewtransaction.StateApproved {
		if err := store.CompleteFinalizeAttempt(requestDigest); err != nil {
			return newFacadeReceiptPublicationError(ctx, root, state.LineageID, state.CurrentSnapshot.Identity, requestDigest, err)
		}
		attemptCompleted = true
		if err := reviewtransaction.BurnApprovedCompactAuthority(ctx, root, state.LineageID, record.Revision); err != nil {
			return fmt.Errorf("burn approved compact review authority: %w", err)
		}
		return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, reviewApprovedCompactBurnedFinalizeAction, reviewFinalizeOutputContext{Context: ctx, Repo: root, AuthorityBurned: true})
	}
	return encodeCompactFacadeFinalize(stdout, negotiated, *contract, *actionEligibility, *nextTransition, state, record.Revision, store, "delivery is informational and unmanaged; ordinary repository policy decides delivery", reviewFinalizeOutputContext{Context: ctx, Repo: root})
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

func validateReviewFinalizeSubmission(ctx context.Context, repo string, state reviewtransaction.CompactState, revision string, args []string, descriptor, correctionLinesProvided bool, correctionLines int, validationPath string, capturedEvidence bool, expectedRevision, targetIdentity, requestHash, repositoryContext string) error {
	if !descriptor {
		return nil
	}
	if expectedRevision != revision {
		return errors.New("review finalize submission expected revision is stale; refresh with gentle-ai review status --next-transition")
	}
	hasValidation := strings.TrimSpace(validationPath) != ""
	if !correctionLinesProvided && !hasValidation && capturedEvidence {
		request, err := reviewtransaction.BuildTargetedValidationRequest(ctx, repo, state, revision)
		if err != nil {
			return err
		}
		if targetIdentity != request.CorrectionTargetIdentity || requestHash != request.RequestHash {
			return errors.New("captured provider validator submission does not match the provider-owned targeted validation request; refresh with gentle-ai review status --next-transition")
		}
		expected := []string{
			"--contract=" + ReviewIntegrationContractV2,
			"--lineage=" + state.LineageID,
			"--expected-revision=" + revision,
			"--target=" + request.CorrectionTargetIdentity,
			"--request-hash=" + request.RequestHash,
			"--repository-context=" + repositoryContext,
			"--captured-evidence=true",
		}
		if !reflect.DeepEqual(args, expected) {
			return errors.New("captured provider validator submission differs from the provider-issued transition; refresh with gentle-ai review status --next-transition")
		}
		return nil
	}
	if correctionLinesProvided == hasValidation {
		return errors.New("review finalize submission requires exactly one descriptor value; refresh with gentle-ai review status --next-transition")
	}
	binding := ReviewTransitionBinding{LineageID: state.LineageID, Revision: revision, TargetIdentity: targetIdentity, RepositoryContext: repositoryContext}
	var submission *ReviewTransitionSubmission
	value := ""
	if correctionLinesProvided {
		request, err := reviewtransaction.BuildCorrectionPlanRequest(state, revision)
		if err != nil {
			return err
		}
		if correctionLines < 1 || correctionLines > request.CorrectionBudget || targetIdentity != request.TargetIdentity || requestHash != request.RequestHash {
			return errors.New("review finalize submission does not match the provider-owned correction request; refresh with gentle-ai review status --next-transition")
		}
		submission = reviewCorrectionPlanSubmission(ReviewIntegrationContractV2, binding, request)
		value = fmt.Sprint(correctionLines)
	} else {
		if !capturedEvidence {
			return errors.New("review finalize validation submission requires captured evidence; refresh with gentle-ai review status --next-transition")
		}
		request, err := reviewtransaction.BuildTargetedValidationRequest(ctx, repo, state, revision)
		if err != nil {
			return err
		}
		if targetIdentity != request.CorrectionTargetIdentity || requestHash != request.RequestHash {
			return errors.New("review finalize submission does not match the provider-owned targeted validation request; refresh with gentle-ai review status --next-transition")
		}
		binding.TargetIdentity = request.CorrectionTargetIdentity
		submission = reviewTargetedValidationSubmission(ReviewIntegrationContractV2, binding, request)
		value = validationPath
	}
	if submission == nil {
		return errors.New("review finalize submission is unavailable") // refusal:by-design world-action: the provider must issue a complete descriptor before a submission can be executed
	}
	if submission.Value == nil {
		return errors.New("review finalize submission has no value slot") // refusal:by-design world-action: a finalize request cannot execute a capture-evidence descriptor
	}
	expected := append([]string{}, submission.ArgumentTokens...)
	slot := submission.Value.SubstitutionLocation
	expected[slot] = strings.Replace(expected[slot], reviewSubmissionValuePlaceholder, value, 1)
	if !reflect.DeepEqual(args, expected) {
		return errors.New("review finalize submission differs from the provider-issued descriptor; refresh with gentle-ai review status --next-transition")
	}
	return nil
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
	return facadeFinalizeAttemptRequestForCandidate(record, record.State.CurrentSnapshot, results, validation, refuter, evidence, correctionLines, failed, nil)
}

func facadeFinalizeAttemptRequestForCandidate(record reviewtransaction.CompactRecord, candidate reviewtransaction.Snapshot, results []facadeReviewerResult, validation *facadeValidationResult, refuter facadeRefuterResult, evidence []byte, correctionLines int, failed bool, captured *reviewtransaction.CapturedVerificationEvidence) reviewtransaction.FinalizeAttemptRequest {
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
	if captured != nil {
		request.EvidenceRecordDigest = captured.Record.RecordDigest
		request.VerificationOutcome = captured.Record.Outcome
	}
	request.RequestDigest = reviewtransaction.FinalizeAttemptRequestDigest(request)
	return request
}

type facadeFinalizeTransition struct {
	Operation string
	State     reviewtransaction.CompactState
}
type facadeFinalizePlan struct {
	Transitions      []facadeFinalizeTransition
	Candidate        reviewtransaction.Snapshot
	Evidence         []byte
	CapturedEvidence *reviewtransaction.CapturedVerificationEvidence
}

// prepareFacadeFinalizePlan performs every deterministic validation before the
// attempt journal exists. Its states are the only states later admitted and
// written through the write-ahead journal.
func prepareFacadeFinalizePlan(ctx context.Context, repo, revision, storeDir string, state reviewtransaction.CompactState, results []facadeReviewerResult, refuter facadeRefuterResult, validation *facadeValidationResult, evidence []byte, correctionLines int, failed bool, captured *reviewtransaction.CapturedVerificationEvidence) (facadeFinalizePlan, error) {
	entryState, entryProposed := state.State, state.ProposedCorrectionLines != nil
	plan := facadeFinalizePlan{Transitions: []facadeFinalizeTransition{}, Candidate: state.CurrentSnapshot, Evidence: evidence, CapturedEvidence: captured}
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
	if state.State == reviewtransaction.StateCorrectionRequired && entryState == reviewtransaction.StateCorrectionRequired && entryProposed &&
		captured != nil && captured.Record.Outcome == reviewtransaction.VerificationOutcomeProceduralFailure {
		fix, err := facadeVerificationEvidenceTarget(ctx, repo, state, revision)
		if err != nil {
			return plan, err
		}
		if err := state.EscalateCorrectionVerification(fix, captured.Record, captured.Payload); err != nil {
			return plan, err
		}
		plan.Candidate = fix
		appendState("review/escalate-correction-verification")
	}
	if state.State == reviewtransaction.StateCorrectionRequired && validation != nil && entryState == reviewtransaction.StateCorrectionRequired && entryProposed {
		if captured == nil {
			return plan, errors.New("compact correction acceptance requires captured repository verification evidence") // refusal:by-design operator-knowledge: repository verification is an external prerequisite captured from the current STATUS transition
		}
		if captured.Record.Outcome != reviewtransaction.VerificationOutcomePassed || failed {
			return plan, errors.New("compact correction repository verification must pass before acceptance") // refusal:by-design world-action: the candidate must change and pass repository verification before the open correction can be accepted
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
		complete, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildCorrectedCandidate(ctx, state.InitialSnapshot, fix)
		if err != nil {
			return plan, err
		}
		native, err := validation.compact(reviewtransaction.FixDeltaHashForSnapshot(fix), state.FixFindingIDs, request)
		if err != nil {
			// The caller-authored recapture never reaches provider capture: it
			// submits straight to here, so this is where its attempt is spent.
			payload, _ := canonicalProviderRoleResult(*validation) // marshalling an already-decoded result cannot fail
			return plan, reviewRecordInconclusiveTargetedValidation(storeDir, request, payload, err)
		}
		if err := state.CompleteCorrectionVerification(fix, actual, native, captured.Record, captured.Payload, complete); err != nil {
			return plan, err
		}
		if state.State == reviewtransaction.StateEscalated {
			plan.Candidate = fix
		} else {
			plan.Candidate = complete
		}
		appendState("review/complete-correction-verification")
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
			if captured != nil {
				if err := state.CompleteVerificationRecord(captured.Record, captured.Payload); err != nil {
					return plan, err
				}
			} else if err := state.CompleteVerification(plan.Evidence, !failed); err != nil {
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

func facadeFinalizeReplayInputsEmpty(results, artifacts, artifactFiles []string, capturedResults, capturedEvidence bool, validation, refuter, evidence string, correctionLines int, failed bool, trace string, providerRuntime model.AgentID) bool {
	return len(results) == 0 && len(artifacts) == 0 && len(artifactFiles) == 0 && !capturedResults && !capturedEvidence && strings.TrimSpace(validation) == "" && strings.TrimSpace(refuter) == "" &&
		strings.TrimSpace(evidence) == "" && correctionLines == 0 && !failed && strings.TrimSpace(trace) == "" && providerRuntime == ""
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

func newFacadeReceiptPublicationError(ctx context.Context, root, lineage, targetIdentity, requestDigest string, cause error) error {
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
		LineageID: lineage, TargetIdentity: targetIdentity, RequestDigest: requestDigest, Cause: cause, DefectReportClause: clause,
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

// runReviewFacadeValidateNonDeciding is the shipped delivery-gate route. Gates
// remain syntax-checked commands, but they no longer discover authority, read
// receipts, derive candidates, or decide delivery. The effective RDD mode is the
// only governing read: disabled uses the established disabled/unmanaged report;
// enabled reports the equally non-deciding unmanaged disposition.
func runReviewFacadeValidateNonDeciding(ctx context.Context, args []string, stdout io.Writer) error {
	if err := validateReviewTransitionSelectorFlagCounts(args, ReviewIntegrationOperationValidate); err != nil {
		return err
	}
	flags := newReviewFlagSet("review validate", stdout, "Validate delivery-gate syntax and report the repository policy that governs delivery.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	_ = flags.String("lineage", "", "accepted historical lineage selector; gates do not read authority")
	gate := flags.String("gate", "", "lifecycle gate: post-apply, pre-commit, pre-push, pre-pr, or release")
	_ = flags.String("base-ref", "", "accepted historical pre-pr publication base")
	_ = flags.String("pre-pr-ci-attestation", "", "accepted historical pre-pr CI attestation")
	_ = flags.String("policy", "", "accepted historical policy artifact")
	_ = flags.String("release-configuration", "", "accepted historical release configuration artifact")
	_ = flags.String("release-generated", "", "accepted historical generated artifact manifest")
	_ = flags.String("release-provenance", "", "accepted historical release provenance artifact")
	_ = flags.String("release-publication-boundary", "", "accepted historical sealed publication boundary artifact")
	_ = flags.String("release-evidence-freshness", "", "accepted historical current release evidence artifact")
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
	if strings.TrimSpace(*gate) == "" || !validReviewIntegrationGate(reviewtransaction.GateKind(*gate)) {
		return reviewPreflightError(fmt.Errorf("review validate requires --gate: one of %s", strings.Join(reviewIntegrationGateNames(), ", ")))
	}
	return runNonDecidingReviewGate(ctx, *cwd, reviewtransaction.GateKind(*gate), negotiated, *contract, stdout)
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
	if err := result.conclusive(); err != nil {
		return reviewtransaction.ScopedValidationResult{}, err
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
	if err := result.conclusive(); err != nil {
		return reviewtransaction.ScopedValidationResult{}, err
	}
	if result.TargetedValidationRequestHash != request.RequestHash || result.CorrectionTargetIdentity != request.CorrectionTargetIdentity {
		return reviewtransaction.ScopedValidationResult{}, errors.New("targeted validation result does not bind the provider-owned correction request") // refusal:by-design operator-knowledge: the external validator must echo both bindings from the provider-owned request
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
			validationErr := (reviewtransaction.SnapshotBuilder{Repo: repo}).ValidateLiveSnapshot(ctx, candidate.record.State.CurrentSnapshot)
			if validationErr != nil && reviewtransaction.IsCompactAuthorityOperationalFailure(validationErr) {
				return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, validationErr
			}
			matches := validationErr == nil
			if !matches {
				_, rebuildErr := reviewtransaction.RebuildCommittedBaseDiffCorrectionCandidate(ctx, repo, candidate.record.State)
				if rebuildErr != nil && (reviewtransaction.IsCompactAuthorityOperationalFailure(rebuildErr) || reviewtransaction.IsCorrectionBudgetExceeded(rebuildErr)) {
					return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, rebuildErr
				}
				matches = rebuildErr == nil
			}
			if matches {
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
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, reviewCompactFacadeLineageAbsent("")
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
			if errors.Is(err, os.ErrNotExist) {
				return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, reviewCompactFacadeLineageAbsent(lineage)
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
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, reviewCompactFacadeLineageAbsent("")
	}
	if len(candidates) != 1 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New("multiple compact facade review lineages found; specify --lineage")
	}
	return candidates[0].store, candidates[0].record, nil
}

type reviewFinalizeOutputContext struct {
	Context          context.Context
	Repo             string
	CapturedEvidence *reviewtransaction.VerificationEvidenceRecord
	// AuthorityBurned is set only after compact approved authority deletion has
	// proven no source or staging residue remains. The immediate response keeps
	// the completed verdict but cannot advertise the now-deleted receipt path.
	AuthorityBurned bool
}

func encodeCompactFacadeFinalize(stdout io.Writer, negotiated bool, contract string, actionEligibility, nextTransition bool, state reviewtransaction.CompactState, revision string, store reviewtransaction.CompactStore, action string, contexts ...reviewFinalizeOutputContext) error {
	var validationRequest *reviewtransaction.TargetedValidationRequest
	var captureContext *reviewCaptureContext
	var capturedEvidence *reviewtransaction.VerificationEvidenceRecord
	var evidenceErr error
	repositoryContext := ""
	var transitionErr error
	authorityBurned := false
	if len(contexts) > 0 {
		capturedEvidence = contexts[0].CapturedEvidence
		authorityBurned = contexts[0].AuthorityBurned
	}
	if !authorityBurned && negotiated && len(contexts) > 0 && contexts[0].Context != nil && strings.TrimSpace(contexts[0].Repo) != "" {
		outputContext := contexts[0]
		if state.State == reviewtransaction.StateCorrectionRequired && state.ProposedCorrectionLines != nil {
			if nextTransition {
				target, targetErr := facadeVerificationEvidenceTarget(outputContext.Context, outputContext.Repo, state, revision)
				if targetErr == nil {
					request, requestErr := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(outputContext.Context, outputContext.Repo, state, revision, target)
					if requestErr == nil {
						validationRequest = &request
						if capturedEvidence == nil {
							captured, readErr := reviewtransaction.ReadCapturedVerificationEvidence(store.Dir, state.LineageID, revision, target)
							evidenceErr = readErr
							if readErr == nil {
								record := captured.Record
								capturedEvidence = &record
							}
						}
					}
				}
			} else {
				request, err := reviewtransaction.BuildTargetedValidationRequest(outputContext.Context, outputContext.Repo, state, revision)
				if err == nil {
					validationRequest = &request
				}
			}
		}
		if nextTransition && contract == ReviewIntegrationContractV2 && (state.State == reviewtransaction.StateCorrectionRequired || state.State == reviewtransaction.StateValidating) {
			contextTarget := state.CurrentSnapshot.Identity
			if validationRequest != nil {
				contextTarget = validationRequest.CorrectionTargetIdentity
			}
			repositoryContext, transitionErr = reviewtransaction.PublishReviewRepositoryContext(outputContext.Context, outputContext.Repo, reviewtransaction.ReviewRepositoryContextBinding{
				LineageID: state.LineageID, TargetIdentity: contextTarget, Revision: revision,
			})
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
	if nextTransition && !authorityBurned {
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
			Contract: contract, RepositoryContext: repositoryContext, ValidationRequest: validationRequest, CaptureContext: captureContext,
			CapturedEvidence: capturedEvidence, EvidenceErr: evidenceErr,
		})
		if value.Kind != "" {
			transition = &value
			if reviewTransitionValidationRequest(&value) == nil && value.ReasonCode != "correction_repository_verification_required" &&
				value.ReasonCode != "correction_repository_tooling_failed" {
				validationRequest = nil
			}
		}
	}
	result := ReviewFacadeFinalizeResult{
		Operation: "review/finalize", LineageID: state.LineageID, State: state.State, Action: action, StoreRevision: revision,
	}
	if !authorityBurned && (state.State == reviewtransaction.StateApproved || state.State == reviewtransaction.StateEscalated) {
		result.ReceiptPath = store.ReceiptPath()
	}
	if accounting := state.EscalationAccounting(); accounting.Cause != "" {
		result.Escalation = fmt.Sprintf(reviewtransaction.EscalationAccountingReasonTemplate,
			accounting.Cause, accounting.Spent, accounting.Remaining, accounting.Total)
	}
	// The disposition of a non-blocking finding is already decided by the time
	// this lineage is approved -- it lives in state.Outcomes and its absence
	// from state.FixFindingIDs. Saying it out loud changes no routing and
	// blocks nothing new; it removes the inference a consumer previously had
	// to make from a bare severity string, which is what let an approved
	// WARNING be "fixed" and re-reviewed in the field.
	result.AdvisoryFindings = reviewtransaction.AdvisoryFindingSetFor(state)
	public := ReviewIntegrationFinalizeResult{
		Operation: result.Operation, LineageID: result.LineageID, State: result.State,
		Action: result.Action, Escalation: result.Escalation, AdvisoryFindings: result.AdvisoryFindings,
		StoreRevision: result.StoreRevision,
		Eligibility:   eligibility, NextTransition: transition, ValidationRequest: validationRequest,
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationFinalize, result, public, contract)
}

// reviewDisabledUnmanagedReason is the shipped disposition sentence, unchanged
// for every discovery outcome that already reported disabled/unmanaged.
const reviewDisabledUnmanagedReason = "receipt-driven development is disabled and no receipt governs this candidate, so delivery follows ordinary repository policy"

// reviewEnabledUnmanagedReason describes the enabled-mode counterpart. Receipt-
// driven development remains enabled, but shipped gates no longer inspect or
// select authority, so no review authority governs the candidate at this command
// boundary and ordinary repository policy decides delivery.
const reviewEnabledUnmanagedReason = "review enforcement is enabled but no review authority governs this candidate, so delivery follows ordinary repository policy"

type reviewUnmanagedGateContext struct {
	Gate reviewtransaction.GateKind `json:"gate"`
}

// reviewUnmanagedGateResult is intentionally narrower than ReviewValidateResult:
// it serializes context as only the requested gate. No receipt, lineage, denial,
// relation, or next-step data exists because the non-deciding route never reads
// the authority or candidate evidence that could truthfully supply it.
type reviewUnmanagedGateResult struct {
	Schema   string                        `json:"schema"`
	Result   reviewtransaction.GateResult  `json:"result"`
	Allowed  bool                          `json:"allowed"`
	Action   string                        `json:"action"`
	Reason   string                        `json:"reason"`
	Context  reviewUnmanagedGateContext    `json:"context"`
	Delivery reviewtransaction.RDDDelivery `json:"delivery"`
}

// runNonDecidingReviewGate resolves a repository only to determine the effective
// user-owned RDD mode. It intentionally returns before authority discovery,
// receipt reads, candidate construction, pre-push range derivation, managed-asset
// authorization, or any other gate evidence evaluation.
func runNonDecidingReviewGate(ctx context.Context, cwd string, gate reviewtransaction.GateKind, negotiated bool, contract string, stdout io.Writer) error {
	root, err := (reviewtransaction.SnapshotBuilder{Repo: cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	disabled, err := reviewDrivenDevelopmentDisabled(ctx, root)
	if err != nil {
		return err
	}
	if disabled {
		return emitDisabledUnmanagedDelivery(stdout, gate, negotiated, contract)
	}
	return emitEnabledUnmanagedDelivery(stdout, gate, negotiated, contract)
}

// reviewEmptyPublicationRangeReason states what the empty-range allow does and
// does not mean. It is an allow because there is no delivery to gate, never
// because anything was reviewed, so it says so in the same sentence.
const reviewEmptyPublicationRangeReason = "the publication range is empty: every commit reachable from HEAD is already published on the push destination, so this push delivers nothing and no review receipt governs it"

// emitDisabledUnmanagedDelivery reports a candidate no receipt governs under a
// user-disabled kill switch.
//
// It never approves: `allowed` stays false, the result is not an allow, and no
// receipt, PASS, or authority is invented. It also never vetoes, because a
// disabled switch defers delivery to ordinary repository policy — hooks, tests,
// and CI stay active and decide — so the command exits successfully and the
// typed result names `disabled/unmanaged` as what governs.
//
// Wave 5 (Gate Cutover) Slice 2, design decision 4: the caller consults the
// kill switch BEFORE any authority read, so this always reports the same
// fixed, generic reason and carries no discovery-kind detail (no Denial, no
// Context beyond the gate) — there is no discovery outcome to describe,
// because discovery never runs. This is an intentional behavior change from
// the pre-Slice-2 shape, which read authority first and then decorated the
// disabled report with what it found (a missing receipt, a stale one, an
// unresolvable target, competing authority, or a damaged inventory). That
// detail-visibility property is still true and still tested — while reviews
// are ON — in review_disabled_reach_test.go / review_disabled_delivery_test.go's
// "...WhileEnabled" siblings; it never applied while disabled, because while
// disabled receipt-driven development does not exist at all.
//
// The whole result is a fixed constant for a given gate, so replaying the
// same request always returns the same bytes, and a decoy/corrupted store
// contributes nothing to it — proven by
// TestDisabledOutputIsByteIdenticalRegardlessOfAuthorityStoreContent.
func emitDisabledUnmanagedDelivery(stdout io.Writer, gate reviewtransaction.GateKind, negotiated bool, contract string) error {
	result := reviewUnmanagedGateResult{
		Schema: ReviewValidateSchema, Result: reviewtransaction.GateInvalidated, Allowed: false,
		Action:   reviewDeliveryPolicyAction,
		Reason:   reviewDisabledUnmanagedReason,
		Delivery: reviewtransaction.RDDDeliveryDisabledUnmanaged,
		Context:  reviewUnmanagedGateContext{Gate: gate},
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationValidate, result, result, contract)
}

// emitEnabledUnmanagedDelivery reports enabled-mode delivery without assigning
// receipt authority. It is a successful repository-policy disposition, never a
// denied gate result: callers must continue through their ordinary hooks, tests,
// and CI rather than interpreting this command as an approval or veto.
func emitEnabledUnmanagedDelivery(stdout io.Writer, gate reviewtransaction.GateKind, negotiated bool, contract string) error {
	result := reviewUnmanagedGateResult{
		Schema: ReviewValidateSchema, Result: reviewtransaction.GateInvalidated, Allowed: false,
		Action: reviewDeliveryPolicyAction, Reason: reviewEnabledUnmanagedReason,
		Context: reviewUnmanagedGateContext{Gate: gate}, Delivery: reviewtransaction.RDDDeliveryUnmanaged,
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationValidate, result, result, contract)
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
