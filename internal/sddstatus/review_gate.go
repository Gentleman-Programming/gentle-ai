package sddstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

type compactPreVerifyBridge struct {
	Eligible bool
	Relevant bool
	Reason   string
	Revision string
}

func discoverCompactPreVerifyAuthority(ctx context.Context, repo, changeName, observedRevision string) compactPreVerifyBridge {
	stores, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return compactPreVerifyBridge{Reason: "no eligible path-bound compact authority found"}
	}
	eligible := 0
	candidateRevision := ""
	for _, store := range stores {
		record, err := store.Load()
		if err != nil {
			return compactPreVerifyBridge{Relevant: true, Reason: "compact authority record is malformed"}
		}
		bound, pathReason := compactAuthorityPathsBound(record.State, changeName)
		if !bound {
			if pathReason != "" {
				return compactPreVerifyBridge{Relevant: true, Reason: pathReason}
			}
			continue
		}
		if record.State.State != reviewtransaction.StateApproved {
			return compactPreVerifyBridge{Relevant: true, Reason: "path-bound compact authority is not approved"}
		}
		payload, err := os.ReadFile(store.ReceiptPath())
		if err != nil {
			return compactPreVerifyBridge{Relevant: true, Reason: "path-bound compact authority receipt is missing"}
		}
		receipt, err := reviewtransaction.ParseCompactReceipt(payload)
		authoritative, receiptErr := record.State.Receipt()
		if err != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
			return compactPreVerifyBridge{Relevant: true, Reason: "path-bound compact authority receipt does not equal approved state"}
		}
		evaluation := reviewtransaction.EvaluateCompactGate(ctx, repo, receipt, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: receipt.LineageID})
		finalRecord, finalErr := store.Load()
		finalPayload, finalReadErr := os.ReadFile(store.ReceiptPath())
		if finalErr != nil || finalReadErr != nil || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalPayload, payload) {
			return compactPreVerifyBridge{Relevant: true, Reason: "compact authority changed during discovery"}
		}
		if evaluation.Result != reviewtransaction.GateAllow {
			if skipsObservedStalePredecessor(observedRevision, record.Revision, evaluation.Result) {
				continue
			}
			return compactPreVerifyBridge{Relevant: true, Reason: "path-bound compact authority post-apply gate is not allow"}
		}
		eligible++
		if eligible == 1 {
			// The revision is immutable store evidence used only for retry routing.
			// It never authorizes a receipt by itself.
			candidateRevision = record.Revision
		}
	}
	switch eligible {
	case 1:
		return compactPreVerifyBridge{Eligible: true, Revision: candidateRevision}
	case 0:
		return compactPreVerifyBridge{Reason: "no eligible path-bound compact authority found"}
	default:
		return compactPreVerifyBridge{Relevant: true, Reason: "multiple eligible path-bound compact authorities found"}
	}
}

func skipsObservedStalePredecessor(observedRevision, revision string, result reviewtransaction.GateResult) bool {
	return observedRevision != "" && observedRevision == revision && result == reviewtransaction.GateScopeChanged
}

func compactAuthorityPathsBound(state reviewtransaction.CompactState, changeName string) (bool, string) {
	if !sameCanonicalPaths(state.GenesisPaths, state.InitialSnapshot.Paths) {
		return false, "path-bound compact authority has inconsistent immutable paths"
	}
	prefix := "openspec/changes/" + changeName + "/"
	matched := false
	foreignOpenSpec := false
	for _, raw := range state.GenesisPaths {
		path := filepath.ToSlash(raw)
		if filepath.IsAbs(raw) || path != filepath.ToSlash(filepath.Clean(raw)) {
			return false, "path-bound compact authority contains a non-canonical OpenSpec path"
		}
		if strings.HasPrefix(path, "openspec/") {
			if !strings.HasPrefix(path, prefix) {
				foreignOpenSpec = true
				continue
			}
			matched = true
		}
	}
	if matched && foreignOpenSpec {
		return false, "path-bound compact authority contains a foreign OpenSpec path"
	}
	return matched, ""
}

func sameCanonicalPaths(left, right []string) bool {
	left, right = append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	return reflect.DeepEqual(left, right)
}

func readSpecCounts(paths []string) (SpecCounts, error) {
	contents := make([]string, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return SpecCounts{}, err
		}
		contents = append(contents, string(content))
	}
	return countSpecRequirementsAndScenarios(contents), nil
}

func readVerifyResult(path string, counts SpecCounts) (verifyResultEvaluation, error) {
	if path == "" {
		return verifyResultEvaluation{Reason: "verify result is missing"}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return verifyResultEvaluation{}, err
	}
	return parseVerifyResult(string(content), counts), nil
}

func readText(path string) string {
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

const incompatibleReviewTransactionReason = "bounded review transaction artifact is not a native JSON review transaction; regenerate it from native review authority"

func readReviewTransaction(path, content string) (*reviewtransaction.Transaction, string) {
	if path == "" && strings.TrimSpace(content) == "" {
		return nil, "bounded review transaction is missing"
	}
	payload := []byte(content)
	if path != "" {
		read, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Sprintf("bounded review transaction cannot be read: %v", err)
		}
		payload = read
	}
	if !strings.HasPrefix(strings.TrimSpace(string(payload)), "{") {
		if json.Valid(payload) {
			return nil, "bounded review transaction is invalid: native review transaction must be a JSON object"
		}
		return nil, incompatibleReviewTransactionReason
	}
	transaction, err := reviewtransaction.ParseTransaction(payload)
	if err != nil {
		return nil, fmt.Sprintf("bounded review transaction is invalid: %v", err)
	}
	return &transaction, ""
}

// EscalationAccountingReasonTemplate is the single source of truth for the
// escalation accounting sentence: this SDD-bound call site, the organic gate
// surface (reviewtransaction.compactEscalatedGateReason) and the organic-dx
// Tier A narration registry (internal/cli/review_narration.go) all render from
// this exact template, so no two surfaces can drift.
//
// The definition lives in reviewtransaction because the organic gate is the
// lower layer and cannot import sddstatus. This alias keeps the existing
// sddstatus-qualified name working unchanged for both remaining consumers.
const EscalationAccountingReasonTemplate = reviewtransaction.EscalationAccountingReasonTemplate

func resolveBoundedRemediation(required bool, verify verifyResultEvaluation, transaction *reviewtransaction.Transaction, compact *reviewtransaction.CompactState, transactionReason, applyProgress string) RemediationState {
	if !required {
		return RemediationState{}
	}
	if verify.EvidenceRevision == "" && strings.Contains(verify.Reason, "evidence_revision") {
		return RemediationState{Reason: fmt.Sprintf("verify evidence cannot enter remediation: %s", verify.Reason)}
	}
	if transaction == nil && compact != nil {
		if compact.State == reviewtransaction.StateEscalated {
			accounting := compact.EscalationAccounting()
			return RemediationState{Reason: fmt.Sprintf(
				EscalationAccountingReasonTemplate,
				accounting.Cause, accounting.Spent, accounting.Remaining, accounting.Total,
			)}
		}
		remainingBudget := compact.CorrectionBudget - compact.CumulativeCorrectionLines
		if remainingBudget <= 0 {
			return RemediationState{Reason: "compact review authority has no correction budget remaining"}
		}
		if compact.CorrectionAttemptConsumed() {
			return RemediationState{Reason: "compact review authority has exhausted its correction attempts"}
		}
		state := RemediationState{
			Required:                  true,
			FailedEvidenceRevision:    verify.EvidenceRevision,
			LineageID:                 compact.LineageID,
			Generation:                compact.Generation,
			FixBatch:                  len(compact.CorrectionAttempts) + 1,
			CorrectionBudgetRemaining: remainingBudget,
			CorrectionBudgetTotal:     compact.CorrectionBudget,
			Reason:                    fmt.Sprintf("verify evidence requires bounded compact remediation for %s: %s", verify.EvidenceRevision, verify.Reason),
		}
		binding := RemediationBinding{LineageID: state.LineageID, Generation: state.Generation, FixBatch: state.FixBatch}
		evaluation := parseRemediationResult(applyProgress, verify.EvidenceRevision, binding)
		state.Complete = evaluation.Complete
		state.Required = !evaluation.Complete
		if evaluation.Complete {
			state.Reason = ""
		}
		return state
	}
	if transaction == nil {
		return RemediationState{Reason: fmt.Sprintf("verify evidence cannot enter remediation: %s; %s", verify.Reason, transactionReason)}
	}
	if transaction.State == reviewtransaction.StateEscalated {
		return RemediationState{Reason: "review transaction is escalated; remediation cannot reopen an exhausted lineage"}
	}
	if verify.EvidenceRevision == "" || transaction.FailedEvidenceRevision != verify.EvidenceRevision {
		return RemediationState{Reason: fmt.Sprintf("transaction failed evidence revision %q does not match failed evidence revision %q", transaction.FailedEvidenceRevision, verify.EvidenceRevision)}
	}

	fixBatch := transaction.Counters.FixBatches
	switch transaction.Mode {
	case reviewtransaction.ModeOrdinary4R:
		if fixBatch != 1 {
			return RemediationState{Reason: "ordinary remediation requires its single persisted fix batch"}
		}
	case reviewtransaction.ModeJudgmentDay:
		fixBatch = transaction.Counters.FixRounds
		if fixBatch < 1 || fixBatch > 2 {
			return RemediationState{Reason: "Judgment Day remediation requires a persisted fix round within its two-round budget"}
		}
	default:
		return RemediationState{Reason: "unsupported remediation transaction mode"}
	}

	state := RemediationState{
		FailedEvidenceRevision: verify.EvidenceRevision,
		LineageID:              transaction.LineageID,
		Generation:             transaction.Generation,
		FixBatch:               fixBatch,
	}
	binding := RemediationBinding{LineageID: state.LineageID, Generation: state.Generation, FixBatch: state.FixBatch}
	evaluation := parseRemediationResult(applyProgress, verify.EvidenceRevision, binding)
	switch transaction.State {
	case reviewtransaction.StateFixing:
		state.Required = true
		state.Reason = fmt.Sprintf("verify evidence requires bounded remediation for %s: %s", verify.EvidenceRevision, verify.Reason)
	case reviewtransaction.StateFixValidating:
		state.Reason = "fix evidence exists but scoped fix-delta validation is still pending"
	case reviewtransaction.StateReadyFinalVerification:
		state.Complete = evaluation.Complete
		state.Required = !evaluation.Complete
		if !evaluation.Complete {
			state.Reason = "scoped fix validation passed but concrete remediation evidence is missing, stale, or not transaction-bound"
		}
	default:
		state.Reason = fmt.Sprintf("transaction state %q does not permit remediation", transaction.State)
	}
	return state
}

type reviewAuthorityEvaluation struct {
	Result       reviewtransaction.GateResult
	Reason       string
	CompactState *reviewtransaction.CompactState
	// Absent reports that no review authority GOVERNS this change: the change
	// supplied no review artifact, and discovery found either no terminal
	// native receipt at all or only receipts that verifiably stopped covering
	// the current repository state. That is the implicit demand the kill
	// switch removes — issue #1877's disabled window delivers under ordinary
	// repository policy and is recorded as unmanaged, never blocked on a
	// review the switch refuses to run. An explicit change-bound artifact that
	// failed validation keeps Absent false and stays a blocker whatever the
	// kill switch says.
	Absent bool
}

func resolveCompactRemediationAuthority(ctx context.Context, repo, change string, bindingPresent, required bool, receiptPath, receiptContent string) *reviewtransaction.CompactState {
	if !required {
		return nil
	}
	if bindingPresent {
		binding, _, err := validateBoundReview(ctx, repo, change)
		if err != nil {
			return nil
		}
		store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, binding.Lineage)
		if err != nil {
			return nil
		}
		record, err := store.Load()
		if err != nil || record.Revision != binding.AuthorityRevision || record.State.State != reviewtransaction.StateApproved {
			return nil
		}
		return &record.State
	}
	evaluation := resolveReviewAuthority(ctx, repo, receiptPath, receiptContent, change)
	if evaluation.Result != reviewtransaction.GateAllow {
		return nil
	}
	return evaluation.CompactState
}

// errTerminalReceiptMissing is the one discovery outcome that means "no review
// authority exists", as opposed to "a review authority exists and does not
// govern these bytes". Only the first is the implicit demand the kill switch
// removes, so it is a sentinel rather than a message to match on.
var errTerminalReceiptMissing = errors.New("terminal review receipt is missing")

// reviewGateDisabledUnmanagedReason names the situation before the mechanism:
// what governs this change comes first, and the reason the gate could not
// govern stays appended behind it so no information is destroyed.
const reviewGateDisabledUnmanagedReason = "receipt-driven development is disabled, so no review governs this change; it closes under ordinary repository policy rather than under a review receipt"

func applyReviewGate(
	status *Status,
	repo string,
	receiptPath, receiptContent string,
	reviewDisabled bool,
) {
	if status.Dependencies.Verify != DependencyAllDone || !status.TaskProgress.AllComplete {
		return
	}
	applyReviewGateEvaluation(status, resolveReviewAuthority(context.Background(), repo, receiptPath, receiptContent, ""), reviewDisabled)
}

func applyReviewGateEvaluation(status *Status, evaluation reviewAuthorityEvaluation, reviewDisabled bool) {
	if evaluation.Result == reviewtransaction.GateAllow {
		status.ReviewGate = &ReviewGateState{Result: evaluation.Result, Reason: evaluation.Reason}
		return
	}
	// With the kill switch off the archive gate has no say in whether this
	// change may close: it would demand a terminal receipt that review/start is
	// refused from producing, which is a deadlock, not a safeguard. Only the
	// implicit demand goes away — an explicit review artifact that failed
	// validation still blocks below. Re-enabling re-validates from the current
	// state, because the next enforcement point rediscovers it on its own.
	if reviewDisabled && evaluation.Absent {
		status.ReviewGate = &ReviewGateState{
			Result:   evaluation.Result,
			Reason:   fmt.Sprintf("%s: %s", reviewGateDisabledUnmanagedReason, evaluation.Reason),
			Delivery: reviewtransaction.RDDDeliveryDisabledUnmanaged,
		}
		return
	}
	blockReviewGate(status, evaluation.Result, evaluation.Reason)
}

// reviewGateFreshReviewContinuation is the runnable exit every archive stop
// over unreviewed content names (issue #1877): the fresh full review of the
// current state. It stays LAST in each reason so the operator's read and the
// mechanical continuation extraction see the same command.
const reviewGateFreshReviewContinuation = "run the fresh full review of the current state with gentle-ai review start"

// reviewGateEmptyReceiptReason refuses the one laundering shape a clean tree
// permits: an approved receipt that froze no content cannot count as coverage
// of delivered history, so the continuation it names carries the base-ref
// selector whose commit value only the operator knows.
const reviewGateEmptyReceiptReason = "the terminal review receipt froze an empty candidate and covers no delivered content; " +
	reviewGateFreshReviewContinuation + " --base-ref <commit>"

// reviewGateAmbiguousGovernanceReason is the one multi-receipt shape that
// still blocks: several terminal receipts each exactly govern the identical
// current state, so the archive cannot know which one to record. Binding one
// to the change resolves it without opening any review.
const reviewGateAmbiguousGovernanceReason = "multiple terminal native review receipts govern the current repository state; " +
	"bind the governing one to the change with gentle-ai review bind-sdd"

func resolveReviewAuthority(ctx context.Context, repo, receiptPath, receiptContent, changeName string) reviewAuthorityEvaluation {
	receiptPayload, explicit := readReviewArtifact(receiptPath, receiptContent)
	payloads := [][]byte{receiptPayload}
	if !explicit {
		var err error
		payloads, err = discoverNativeReceipts(ctx, repo)
		if err != nil {
			reason := err.Error()
			absent := errors.Is(err, errTerminalReceiptMissing)
			if absent {
				reason += "; " + reviewGateFreshReviewContinuation
			}
			return reviewAuthorityEvaluation{
				Result: reviewtransaction.GateInvalidated,
				Reason: reason,
				Absent: absent,
			}
		}
	}
	var allows, blockers, stale []reviewAuthorityEvaluation
	for _, payload := range payloads {
		evaluation := evaluateReceiptPayload(ctx, repo, payload, changeName)
		switch {
		case evaluation.Result == reviewtransaction.GateAllow:
			allows = append(allows, evaluation)
		case evaluation.Result == reviewtransaction.GateScopeChanged:
			stale = append(stale, evaluation)
		default:
			blockers = append(blockers, evaluation)
		}
	}
	// Exactly one receipt governing the current repository state governs the
	// archive; stale terminal receipts left behind by earlier deliveries are
	// history, not blockers, so they never smother a live governing receipt.
	if len(allows) == 1 {
		return allows[0]
	}
	if len(allows) > 1 {
		return reviewAuthorityEvaluation{
			Result: reviewtransaction.GateInvalidated,
			Reason: reviewGateAmbiguousGovernanceReason,
			Absent: !explicit,
		}
	}
	// Escalations and validation failures block regardless of provenance: an
	// artifact that asked receipt-driven development to act is validated in
	// full even while the switch is off.
	if len(blockers) > 0 {
		return blockers[0]
	}
	selected := stale[0]
	// An empty-candidate receipt means the operator already ran the plain
	// start on a clean tree; naming it again would loop, so the base-ref
	// continuation it carries wins over any generic scope-changed reason,
	// independent of discovery order.
	for _, evaluation := range stale {
		if evaluation.Reason == reviewGateEmptyReceiptReason {
			selected = evaluation
			break
		}
	}
	// A receipt whose scope verifiably moved on governs nothing. Unless the
	// change explicitly bound it, that is absent governance: while the switch
	// is off the change closes unmanaged under ordinary repository policy, and
	// once re-enabled the stop names the fresh full review that subsumes the
	// unmanaged history (issue #1877 — no retroactive reconciliation).
	selected.Absent = !explicit
	return selected
}

func reviewReceiptCoversDeliveredContent(baseTree, finalCandidateTree string) bool {
	// guard:population receipt-content-governance too-loose: legitimate governing receipts must cover delivered content; empty-candidate receipts are excluded
	return baseTree != finalCandidateTree
}

func evaluateReceiptPayload(ctx context.Context, repo string, receiptPayload []byte, changeName string) reviewAuthorityEvaluation {
	var evaluation reviewtransaction.NativeGateEvaluation
	var compactState *reviewtransaction.CompactState
	if reviewtransaction.CompactReceiptSchemaOf(receiptPayload) == reviewtransaction.CompactReceiptSchema {
		receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
		if err != nil {
			return reviewAuthorityEvaluation{Result: reviewtransaction.GateInvalidated, Reason: fmt.Sprintf("compact review receipt is invalid or non-terminal: %v", err)}
		}
		if !reviewReceiptCoversDeliveredContent(receipt.BaseTree, receipt.FinalCandidateTree) {
			return reviewAuthorityEvaluation{Result: reviewtransaction.GateScopeChanged, Reason: reviewGateEmptyReceiptReason}
		}
		if changeName != "" {
			store, storeErr := reviewtransaction.CompactAuthoritativeStore(ctx, repo, receipt.LineageID)
			if storeErr != nil {
				return reviewAuthorityEvaluation{Result: reviewtransaction.GateInvalidated, Reason: "selected change compact authority cannot be loaded"}
			}
			record, loadErr := store.Load()
			if loadErr != nil {
				return reviewAuthorityEvaluation{Result: reviewtransaction.GateInvalidated, Reason: "selected change compact authority cannot be loaded"}
			}
			bound, reason := compactAuthorityPathsBound(record.State, changeName)
			if !bound {
				if reason == "" {
					reason = fmt.Sprintf("compact review authority is not bound to selected change %q", changeName)
				}
				return reviewAuthorityEvaluation{Result: reviewtransaction.GateInvalidated, Reason: reason}
			}
			compactState = &record.State
		}
		evaluation = reviewtransaction.EvaluateCompactGate(ctx, repo, receipt, reviewtransaction.NativeGateRequestInput{
			Gate: reviewtransaction.GatePostApply, LineageID: receipt.LineageID,
		})
	} else {
		receipt, err := reviewtransaction.ParseReceipt(receiptPayload)
		if err != nil {
			return reviewAuthorityEvaluation{Result: reviewtransaction.GateInvalidated, Reason: fmt.Sprintf("review receipt is invalid or non-terminal: %v", err)}
		}
		if !reviewReceiptCoversDeliveredContent(receipt.BaseTree, receipt.FinalCandidateTree) {
			return reviewAuthorityEvaluation{Result: reviewtransaction.GateScopeChanged, Reason: reviewGateEmptyReceiptReason}
		}
		request, err := reviewtransaction.BuildNativeGateRequest(ctx, repo, reviewtransaction.NativeGateRequestInput{
			Gate: reviewtransaction.GatePostApply, LineageID: receipt.LineageID,
		})
		if err != nil {
			return reviewAuthorityEvaluation{Result: reviewtransaction.GateInvalidated, Reason: fmt.Sprintf("native review gate request cannot be derived: %v", err)}
		}
		evaluation = evaluateNativeReviewGate(ctx, repo, receipt, request)
	}
	switch evaluation.Result {
	case reviewtransaction.GateAllow:
		return reviewAuthorityEvaluation{Result: evaluation.Result, Reason: "approved receipt exactly matches authoritative native state and the current repository", CompactState: compactState}
	case reviewtransaction.GateScopeChanged:
		return reviewAuthorityEvaluation{Result: evaluation.Result, Reason: "review scope changed; maintainer must create an explicit new lineage without reusing this budget: " + reviewGateFreshReviewContinuation}
	case reviewtransaction.GateEscalated:
		return reviewAuthorityEvaluation{Result: evaluation.Result, Reason: "new external evidence or terminal transaction state escalated the receipt without reopening review"}
	default:
		return reviewAuthorityEvaluation{Result: reviewtransaction.GateInvalidated, Reason: "review receipt was invalidated by content relationship, policy, ledger, evidence, or publication state; explicit maintainer action is required and no budget resets"}
	}
}

var evaluateNativeReviewGate = reviewtransaction.EvaluateNativeGate

// discoverNativeReceipts returns every valid terminal native receipt in
// discovery order. Multiplicity is not an error here: normal reviewed use
// leaves one terminal receipt per delivered candidate behind, so the caller
// selects the receipt that governs the current repository state and treats the
// rest as history.
func discoverNativeReceipts(ctx context.Context, repo string) ([][]byte, error) {
	var matches [][]byte
	compactStores, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("discover compact review stores: %w", err)
	}
	for _, store := range compactStores {
		record, err := store.Load()
		if err != nil || record.State.State != reviewtransaction.StateApproved && record.State.State != reviewtransaction.StateEscalated {
			continue
		}
		payload, err := os.ReadFile(store.ReceiptPath())
		if err != nil {
			continue
		}
		receipt, err := reviewtransaction.ParseCompactReceipt(payload)
		authoritative, receiptErr := record.State.Receipt()
		if err != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
			continue
		}
		matches = append(matches, payload)
	}
	stores, err := reviewtransaction.DiscoverAuthoritativeStores(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("discover authoritative review stores: %w", err)
	}
	for _, store := range stores {
		chain, err := store.LoadChain()
		if err != nil {
			continue
		}
		transaction := chain.Records[len(chain.Records)-1].Transaction
		if transaction.State != reviewtransaction.StateApproved && transaction.State != reviewtransaction.StateEscalated {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(store.Dir, "artifacts", "receipt.json"))
		if err != nil {
			continue
		}
		receipt, err := reviewtransaction.ParseReceipt(payload)
		authoritative, receiptErr := transaction.Receipt()
		if err != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
			continue
		}
		matches = append(matches, payload)
	}
	if len(matches) == 0 {
		return nil, errTerminalReceiptMissing
	}
	return matches, nil
}

func readReviewArtifact(path, content string) ([]byte, bool) {
	if path != "" {
		payload, err := os.ReadFile(path)
		return payload, err == nil && len(strings.TrimSpace(string(payload))) > 0
	}
	if strings.TrimSpace(content) == "" {
		return nil, false
	}
	return []byte(content), true
}

func blockReviewGate(status *Status, result reviewtransaction.GateResult, reason string) {
	status.ReviewGate = &ReviewGateState{Result: result, Reason: reason}
	status.Dependencies.Archive = DependencyBlocked
	status.NextRecommended = "resolve-review"
	status.BlockedReasons = append(status.BlockedReasons, reason)
}
