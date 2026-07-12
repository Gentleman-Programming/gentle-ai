package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
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
	transaction, err := reviewtransaction.ParseTransaction(payload)
	if err != nil {
		return nil, fmt.Sprintf("bounded review transaction is invalid: %v", err)
	}
	return &transaction, ""
}

func resolveBoundedRemediation(required bool, verify verifyResultEvaluation, transaction *reviewtransaction.Transaction, transactionReason, applyProgress string) RemediationState {
	if !required {
		return RemediationState{}
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

func applyReviewGate(
	status *Status,
	repo string,
	receiptPath, receiptContent string,
) {
	if status.Dependencies.Verify != DependencyAllDone || !status.TaskProgress.AllComplete {
		return
	}
	receiptPayload, ok := readReviewArtifact(receiptPath, receiptContent)
	if !ok {
		var err error
		receiptPayload, err = discoverNativeReceipt(context.Background(), repo)
		if err != nil {
			blockReviewGate(status, reviewtransaction.GateInvalidated, err.Error())
			return
		}
	}
	var evaluation reviewtransaction.NativeGateEvaluation
	if reviewtransaction.CompactReceiptSchemaOf(receiptPayload) == reviewtransaction.CompactReceiptSchema {
		receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
		if err != nil {
			blockReviewGate(status, reviewtransaction.GateInvalidated, fmt.Sprintf("compact review receipt is invalid or non-terminal: %v", err))
			return
		}
		evaluation = reviewtransaction.EvaluateCompactGate(context.Background(), repo, receipt, reviewtransaction.NativeGateRequestInput{
			Gate: reviewtransaction.GatePostApply, LineageID: receipt.LineageID,
		})
	} else {
		receipt, err := reviewtransaction.ParseReceipt(receiptPayload)
		if err != nil {
			blockReviewGate(status, reviewtransaction.GateInvalidated, fmt.Sprintf("review receipt is invalid or non-terminal: %v", err))
			return
		}
		request, err := reviewtransaction.BuildNativeGateRequest(context.Background(), repo, reviewtransaction.NativeGateRequestInput{
			Gate: reviewtransaction.GatePostApply, LineageID: receipt.LineageID,
		})
		if err != nil {
			blockReviewGate(status, reviewtransaction.GateInvalidated, fmt.Sprintf("native review gate request cannot be derived: %v", err))
			return
		}
		evaluation = evaluateNativeReviewGate(context.Background(), repo, receipt, request)
	}
	result := evaluation.Result
	switch result {
	case reviewtransaction.GateAllow:
		status.ReviewGate = &ReviewGateState{Result: result, Reason: "approved receipt exactly matches authoritative native state and the current repository"}
	case reviewtransaction.GateScopeChanged:
		blockReviewGate(status, result, "review scope changed; maintainer must create an explicit new lineage without reusing this budget")
	case reviewtransaction.GateEscalated:
		blockReviewGate(status, result, "new external evidence or terminal transaction state escalated the receipt without reopening review")
	default:
		blockReviewGate(status, reviewtransaction.GateInvalidated, "review receipt was invalidated by content relationship, policy, ledger, evidence, or publication state; explicit maintainer action is required and no budget resets")
	}
}

var evaluateNativeReviewGate = reviewtransaction.EvaluateNativeGate

func discoverNativeReceipt(ctx context.Context, repo string) ([]byte, error) {
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
		return nil, errors.New("terminal review receipt is missing")
	}
	if len(matches) != 1 {
		return nil, errors.New("multiple terminal native review receipts found; restore the change-local reviews/receipt.json mirror or remove stale terminal authority")
	}
	return matches[0], nil
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
