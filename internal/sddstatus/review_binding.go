package sddstatus

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
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewBindingSchema = "gentle-ai.sdd-review-binding/v1"

var reviewBindingChange = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var reviewBindingLineage = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var reviewBindingHash = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var bindingFinalAuthorizationHook = func() {}

type ReviewBindingPublicationError struct{ Cause error }

func (err *ReviewBindingPublicationError) Error() string {
	return fmt.Sprintf("SDD review binding publication requires exact replay: %v", err.Cause)
}

func (err *ReviewBindingPublicationError) Unwrap() error { return err.Cause }

type ReviewBinding struct {
	Schema            string                        `json:"schema"`
	Revision          string                        `json:"revision"`
	Change            string                        `json:"change"`
	Lineage           string                        `json:"lineage"`
	AuthorityRevision string                        `json:"authority_revision"`
	ReceiptHash       string                        `json:"receipt_hash"`
	GateContext       reviewtransaction.GateContext `json:"gate_context"`
}

func BindApprovedReview(ctx context.Context, repo, change, lineage, expected string) (ReviewBinding, error) {
	if !validReviewBindingChange(change) {
		return ReviewBinding{}, errors.New("invalid OpenSpec change name")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return ReviewBinding{}, err
	}
	if _, err := resolveBindingChangeRoot(ctx, root, repo, change); err != nil {
		return ReviewBinding{}, err
	}
	if err := rejectHistoricalLegacyBinding(ctx, root, lineage); err != nil {
		return ReviewBinding{}, err
	}
	runtimeStore, err := OpenRuntimeStore(ctx, root, change)
	if err != nil {
		return ReviewBinding{}, err
	}
	requestID := "bind-" + strings.TrimPrefix(runtimeValueHash("gentle-ai.sdd-review-binding-request-id/v1", struct {
		Change   string `json:"change"`
		Lineage  string `json:"lineage"`
		Expected string `json:"expected"`
	}{Change: change, Lineage: lineage, Expected: expected}), "sha256:")
	status, err := runtimeStore.bindPreparedReview(ctx, BindReviewRequest{
		ExpectedBindingRevision: expected, RequestID: requestID, LineageID: lineage,
	}, func() (ReviewBinding, error) {
		return prepareApprovedReviewBinding(ctx, root, repo, change, lineage)
	})
	if err != nil {
		var publication *RuntimePublicationError
		if errors.As(err, &publication) {
			return ReviewBinding{}, &ReviewBindingPublicationError{Cause: err}
		}
		return ReviewBinding{}, err
	}
	if status.Binding == nil {
		return ReviewBinding{}, errors.New("native SDD runtime binding commit returned no binding")
	}
	return *status.Binding, nil
}

func rejectHistoricalLegacyBinding(ctx context.Context, root, lineage string) error {
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, lineage)
	if err != nil {
		return err
	}
	if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	legacy, legacyErr := reviewtransaction.AuthoritativeStore(ctx, root, lineage)
	if legacyErr != nil {
		return nil
	}
	if _, legacyErr = legacy.LoadChain(); legacyErr == nil {
		return reviewtransaction.NewLegacyReadOnlyError("review/bind-sdd", lineage)
	}
	return nil
}

func prepareApprovedReviewBinding(ctx context.Context, root, workspace, change, lineage string) (ReviewBinding, error) {
	changeRoot, err := resolveBindingChangeRoot(ctx, root, workspace, change)
	if err != nil {
		return ReviewBinding{}, err
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, lineage)
	if err != nil {
		return ReviewBinding{}, err
	}
	record, err := store.Load()
	if errors.Is(err, os.ErrNotExist) {
		legacy, legacyErr := reviewtransaction.AuthoritativeStore(ctx, root, lineage)
		if legacyErr == nil {
			if _, legacyLoadErr := legacy.LoadChain(); legacyLoadErr == nil {
				return ReviewBinding{}, reviewtransaction.NewLegacyReadOnlyError("review/bind-sdd", lineage)
			}
		}
	}
	if err != nil || record.State.State != reviewtransaction.StateApproved {
		return ReviewBinding{}, errors.New("explicit compact authority is not approved")
	}
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		return ReviewBinding{}, err
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(payload)
	authoritative, receiptErr := record.State.Receipt()
	if err != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
		return ReviewBinding{}, errors.New("compact receipt does not match approved authority")
	}
	if err := verifyBindingLedger(changeRoot, record.State.Findings); err != nil {
		return ReviewBinding{}, err
	}
	input := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: lineage}
	gate := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, input)
	if gate.Result != reviewtransaction.GateAllow {
		return ReviewBinding{}, errors.New("compact post-apply gate is not allow")
	}
	binding := ReviewBinding{Schema: reviewBindingSchema, Change: change, Lineage: lineage, AuthorityRevision: record.Revision, ReceiptHash: bindingHash(payload), GateContext: gate.Context}
	binding.Revision = bindingDigest(binding)
	final, finalErr := store.Load()
	finalPayload, readErr := os.ReadFile(store.ReceiptPath())
	finalGate := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, input)
	finalChangeRoot, changeErr := resolveBindingChangeRoot(ctx, root, workspace, change)
	if finalErr != nil || readErr != nil || changeErr != nil || finalChangeRoot != changeRoot || final.Revision != record.Revision || !bytes.Equal(payload, finalPayload) || finalGate.Result != reviewtransaction.GateAllow || !reflect.DeepEqual(gate.Context, finalGate.Context) {
		return ReviewBinding{}, errors.New("authority or live gate changed before binding publish")
	}
	return binding, nil
}

// prepareApprovedRuntimeSuccessorBinding preserves the OpenSpec ledger check
// whenever the selected change has a file-backed root. Pure Engram changes do
// not have such a root, so their already-approved compact authority and live
// post-apply gate are the complete native successor provenance.
func prepareApprovedRuntimeSuccessorBinding(ctx context.Context, root, workspace, change, lineage string) (ReviewBinding, error) {
	matches, err := bindingChangeRoots(ctx, root, change)
	if err != nil {
		return ReviewBinding{}, err
	}
	if len(matches) != 0 {
		return prepareApprovedReviewBinding(ctx, root, workspace, change, lineage)
	}
	return prepareApprovedCompactBinding(ctx, root, change, lineage)
}

func prepareApprovedCompactBinding(ctx context.Context, root, change, lineage string) (ReviewBinding, error) {
	if !validReviewBindingChange(change) || !validReviewBindingLineage(lineage) {
		return ReviewBinding{}, errors.New("invalid compact SDD review binding identity")
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, lineage)
	if err != nil {
		return ReviewBinding{}, err
	}
	record, err := store.Load()
	if err != nil || record.State.State != reviewtransaction.StateApproved {
		return ReviewBinding{}, errors.New("explicit compact authority is not approved")
	}
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		return ReviewBinding{}, err
	}
	receipt, parseErr := reviewtransaction.ParseCompactReceipt(payload)
	authoritative, receiptErr := record.State.Receipt()
	if parseErr != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
		return ReviewBinding{}, errors.New("compact receipt does not match approved authority")
	}
	input := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: lineage}
	evaluation := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, input)
	if evaluation.Result != reviewtransaction.GateAllow {
		return ReviewBinding{}, errors.New("compact post-apply gate is not allow")
	}
	binding := ReviewBinding{
		Schema: reviewBindingSchema, Change: change, Lineage: lineage,
		AuthorityRevision: record.Revision, ReceiptHash: bindingHash(payload), GateContext: evaluation.Context,
	}
	binding.Revision = bindingDigest(binding)
	finalRecord, finalErr := loadRuntimeBoundCompactArtifacts(ctx, root, binding)
	if finalErr != nil || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalRecord.State, record.State) {
		return ReviewBinding{}, errors.New("authority or receipt changed before compact binding selection")
	}
	finalReceipt, finalReceiptErr := finalRecord.State.Receipt()
	if finalReceiptErr != nil {
		return ReviewBinding{}, errors.New("approved compact authority cannot produce its final receipt")
	}
	finalGate := reviewtransaction.EvaluateCompactGate(ctx, root, finalReceipt, input)
	if finalGate.Result != reviewtransaction.GateAllow || !reflect.DeepEqual(evaluation.Context, finalGate.Context) {
		return ReviewBinding{}, errors.New("compact post-apply gate changed before binding selection")
	}
	return binding, nil
}

// validateRuntimeRemediationSuccessor proves that an already approved binding
// is the current leaf of the native compact recovery graph rooted at the
// populated binding. It deliberately does not mutate compact authority: the
// runtime ledger selects the independently approved successor and records the
// attempt charge, evidence transition, and binding swap in its own one-HEAD
// compare-and-swap.
func validateRuntimeRemediationSuccessor(ctx context.Context, repo string, current, successor ReviewBinding) error {
	if current.Lineage == successor.Lineage {
		return errors.New("atomic SDD remediation requires a distinct recovery descendant")
	}
	currentRecord, err := loadRuntimeBoundCompactArtifacts(ctx, repo, current)
	if err != nil {
		return fmt.Errorf("validate current remediation binding: %w", err)
	}
	leaves, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return fmt.Errorf("validate compact recovery graph: %w", err)
	}
	leaf := false
	for _, store := range leaves {
		record, loadErr := store.Load()
		if loadErr != nil {
			return fmt.Errorf("load compact recovery leaf: %w", loadErr)
		}
		if record.State.LineageID == successor.Lineage && record.Revision == successor.AuthorityRevision {
			leaf = true
			break
		}
	}
	if !leaf {
		return errors.New("approved SDD remediation authority is not the current compact recovery leaf")
	}

	successorStore, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, successor.Lineage)
	if err != nil {
		return err
	}
	cursor, err := successorStore.Load()
	if err != nil || cursor.Revision != successor.AuthorityRevision {
		return errors.New("approved SDD remediation successor authority changed before native commit")
	}
	seen := map[string]struct{}{}
	for steps := 0; steps < 10_000; steps++ {
		if _, duplicate := seen[cursor.State.LineageID]; duplicate {
			return errors.New("compact remediation recovery chain contains a cycle")
		}
		seen[cursor.State.LineageID] = struct{}{}
		recovery := cursor.State.Recovery
		if recovery == nil || recovery.Disposition != reviewtransaction.RecoveryScopeChanged {
			return errors.New("approved SDD remediation authority is not a scope-changed recovery descendant of the populated binding")
		}
		predecessorStore, storeErr := reviewtransaction.CompactAuthoritativeStore(ctx, repo, recovery.PredecessorLineageID)
		if storeErr != nil {
			return storeErr
		}
		predecessor, loadErr := predecessorStore.Load()
		if loadErr != nil || predecessor.Revision != recovery.PredecessorRevision {
			return errors.New("compact remediation predecessor revision changed before native commit")
		}
		if predecessor.State.LineageID == current.Lineage {
			if predecessor.Revision != currentRecord.Revision {
				return errors.New("compact remediation recovery chain does not reach the populated binding revision")
			}
			return nil
		}
		cursor = predecessor
	}
	return errors.New("compact remediation recovery chain exceeds the bounded lineage count")
}

// validateRuntimeRemediationSelfSuccessor proves that an approved
// self-successor — the corrected authority of the very lineage the runtime
// binding already names — is healthy, current, content-bound, and still the
// leaf of the compact recovery graph. A same-lineage remediation never mutates
// compact authority and never demands the impossible invalidation of a healthy
// approved predecessor: the corrected approved receipt itself is the successor
// provenance. A lineage that has since gained a true recovery successor is no
// longer the leaf and must be refused.
func validateRuntimeRemediationSelfSuccessor(ctx context.Context, repo string, current, successor ReviewBinding) error {
	if current.Lineage != successor.Lineage {
		return errors.New("approved SDD self-remediation requires the bound lineage")
	}
	if _, err := loadRuntimeBoundCompactArtifacts(ctx, repo, successor); err != nil {
		return fmt.Errorf("validate approved self-remediation authority: %w", err)
	}
	leaves, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return fmt.Errorf("validate compact recovery graph: %w", err)
	}
	for _, store := range leaves {
		record, loadErr := store.Load()
		if loadErr != nil {
			return fmt.Errorf("load compact recovery leaf: %w", loadErr)
		}
		if record.State.LineageID == successor.Lineage && record.Revision == successor.AuthorityRevision {
			return nil
		}
	}
	return errors.New("approved SDD self-remediation authority is not the current compact recovery leaf")
}

// runtimeSelfSuccessorAvailable answers exactly one question: would the
// self-successor finish be ACCEPTED right now? It runs the same preparation
// and the same validator the finish itself would run, against the same
// candidate tree the finish already charged, so a refusal that consults it
// can only name the self-successor exit when running that exit works.
//
// It is deliberately read-only and fail-closed: any preparation error, any
// gate that is not allow, an approved authority that binds different bytes, or
// a lineage that is no longer the compact recovery leaf all answer false, and
// the caller then names the review router instead of a command that would be
// refused one layer deeper.
func runtimeSelfSuccessorAvailable(ctx context.Context, repo, workspace, change string, binding ReviewBinding, candidateTree string) bool {
	prepared, err := prepareApprovedRuntimeSuccessorBinding(ctx, repo, workspace, change, binding.Lineage)
	if err != nil || prepared.Lineage != binding.Lineage || prepared.GateContext.CandidateTree != candidateTree {
		return false
	}
	return validateRuntimeRemediationSelfSuccessor(ctx, repo, binding, prepared) == nil
}

// RuntimeStrandedSuccessor identifies the one recovery successor that stands
// between a bound lineage and its own approved review, together with the two
// persisted values `review abandon` demands in its authorization binding.
type RuntimeStrandedSuccessor struct {
	Lineage          string
	Revision         string
	SnapshotIdentity string
}

// runtimeStrandedSuccessor answers exactly one question: is the ONLY thing
// standing between the bound lineage and the self-successor finish a recovery
// successor that can never be finalized?
//
// A recovery successor freezes its own review target when it is minted. If the
// candidate then moves away from that frozen target — the widened scope is
// taken back out, the corrected file is reverted — the successor's live
// projection will never match its own authority again, so it can never be
// reviewed, never be approved, and never become a legal --successor-lineage.
// While it exists the bound lineage is not the compact recovery leaf and its
// post-apply gate does not allow, so every exit through the review router leads
// to a review the runtime then refuses to accept. Quarantining the stranded
// successor is the one operation that resolves it, and it destroys nothing the
// operator still wants: a stranded successor is by definition pristine, and the
// approved predecessor it superseded — which holds the operator's corrected
// work — is exactly what quarantining it restores.
//
// It is deliberately read-only and fail-closed. The bound authority must be
// approved for the very bytes this finish charged, the successor must be the
// bound lineage's own direct recovery successor, it must be the current leaf,
// its frozen candidate must NOT be the charged candidate, and `review abandon`
// must already accept it. Anything else answers false and the caller names the
// review router instead of an abandonment that would be refused one layer in —
// most importantly a successor that DOES govern the charged candidate, which is
// a live review the operator still wants and must never be sent to quarantine.
func runtimeStrandedSuccessor(ctx context.Context, repo string, binding ReviewBinding, candidateTree string) (RuntimeStrandedSuccessor, bool) {
	bound, err := loadRuntimeBoundCompactArtifacts(ctx, repo, binding)
	if err != nil || bound.State.CurrentSnapshot.CandidateTree != candidateTree {
		return RuntimeStrandedSuccessor{}, false
	}
	leaves, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return RuntimeStrandedSuccessor{}, false
	}
	found := RuntimeStrandedSuccessor{}
	for _, store := range leaves {
		record, loadErr := store.Load()
		if loadErr != nil {
			return RuntimeStrandedSuccessor{}, false
		}
		recovery := record.State.Recovery
		if recovery == nil || recovery.PredecessorLineageID != binding.Lineage ||
			recovery.PredecessorRevision != binding.AuthorityRevision {
			continue
		}
		if found.Lineage != "" {
			// A fork is structurally refused elsewhere; seeing one here means
			// the graph is not what this probe can reason about.
			return RuntimeStrandedSuccessor{}, false
		}
		if record.State.InitialSnapshot.CandidateTree == candidateTree {
			// Not stranded: this successor governs the charged candidate and
			// can still be reviewed, approved and named as the successor.
			return RuntimeStrandedSuccessor{}, false
		}
		found.Lineage = record.State.LineageID
	}
	if found.Lineage == "" {
		return RuntimeStrandedSuccessor{}, false
	}
	eligibility, err := reviewtransaction.InspectCompactPristineAbandonment(ctx, repo, found.Lineage)
	if err != nil || !eligibility.Eligible {
		return RuntimeStrandedSuccessor{}, false
	}
	found.Revision, found.SnapshotIdentity = eligibility.Revision, eligibility.SnapshotIdentity
	return found, true
}

// loadRuntimeBoundCompactArtifacts validates immutable authority and receipt
// identity without evaluating the live post-apply gate. The old binding is
// expected to be live-stale after remediation; its exact approved bytes remain
// the predecessor provenance that the successor recovery edge must name.
func loadRuntimeBoundCompactArtifacts(ctx context.Context, repo string, binding ReviewBinding) (reviewtransaction.CompactRecord, error) {
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, binding.Lineage)
	if err != nil {
		return reviewtransaction.CompactRecord{}, err
	}
	record, err := store.Load()
	if err != nil || record.Revision != binding.AuthorityRevision || record.State.State != reviewtransaction.StateApproved {
		return reviewtransaction.CompactRecord{}, errors.New("bound compact predecessor is stale or not approved")
	}
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil || bindingHash(payload) != binding.ReceiptHash {
		return reviewtransaction.CompactRecord{}, errors.New("bound compact predecessor receipt changed")
	}
	receipt, parseErr := reviewtransaction.ParseCompactReceipt(payload)
	authoritative, receiptErr := record.State.Receipt()
	if parseErr != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
		return reviewtransaction.CompactRecord{}, errors.New("bound compact predecessor receipt does not match authority")
	}
	return record, nil
}

// validateRuntimeBoundCandidate proves that an unchanged runtime attempt still
// has the exact approved compact authority, receipt, live gate context, and
// candidate tree recorded by its existing binding. It deliberately does not
// resolve an OpenSpec change root, so Engram-backed changes use the same native
// compact authority path.
func validateRuntimeBoundCandidate(ctx context.Context, repo string, binding ReviewBinding, candidateTree string) error {
	record, err := loadRuntimeBoundCompactArtifacts(ctx, repo, binding)
	if err != nil {
		return err
	}
	receipt, err := record.State.Receipt()
	if err != nil {
		return errors.New("bound compact authority cannot produce its receipt")
	}
	evaluation := reviewtransaction.EvaluateCompactGate(ctx, repo, receipt, reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GatePostApply, LineageID: binding.Lineage,
	})
	if evaluation.Result != reviewtransaction.GateAllow || evaluation.Context.CandidateTree != candidateTree ||
		!boundGateContextMatches(binding.GateContext, evaluation.Context) {
		return errors.New("bound compact post-apply gate context changed")
	}
	return nil
}

func validateBoundReview(ctx context.Context, repo, change string) (ReviewBinding, reviewtransaction.NativeGateEvaluation, error) {
	if !validReviewBindingChange(change) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("invalid OpenSpec change name")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	changeRoot, err := resolveBindingChangeRoot(ctx, root, repo, change)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	binding, err := loadEffectiveReviewBinding(ctx, root, change)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, fmt.Errorf("approved review binding is missing or invalid: %w", err)
	}
	if binding.Change != change {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("approved review binding change does not match selected change")
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, binding.Lineage)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	record, err := store.Load()
	if err != nil || record.Revision != binding.AuthorityRevision || record.State.State != reviewtransaction.StateApproved {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact authority is stale or not approved")
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil || bindingHash(receiptPayload) != binding.ReceiptHash {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact receipt changed")
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	authoritative, receiptErr := record.State.Receipt()
	if err != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact receipt does not match approved authority")
	}
	if err := verifyBindingLedger(changeRoot, record.State.Findings); err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	evaluation := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: binding.Lineage})
	if evaluation.Result != reviewtransaction.GateAllow || !boundGateContextMatches(binding.GateContext, evaluation.Context) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact post-apply gate context changed")
	}
	bindingFinalAuthorizationHook()
	finalBinding, bindingErr := loadEffectiveReviewBinding(ctx, root, change)
	finalRecord, recordErr := store.Load()
	finalReceipt, receiptErr := os.ReadFile(store.ReceiptPath())
	finalChangeRoot, changeErr := resolveBindingChangeRoot(ctx, root, repo, change)
	if bindingErr != nil || recordErr != nil || receiptErr != nil || changeErr != nil || finalChangeRoot != changeRoot || !reflect.DeepEqual(finalBinding, binding) || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalRecord.State, record.State) || finalRecord.State.State != reviewtransaction.StateApproved || !bytes.Equal(finalReceipt, receiptPayload) || bindingHash(finalReceipt) != binding.ReceiptHash {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound authority, receipt, or binding changed during final read")
	}
	finalReceiptValue, parseErr := reviewtransaction.ParseCompactReceipt(finalReceipt)
	finalAuthoritative, authorityErr := finalRecord.State.Receipt()
	if parseErr != nil || authorityErr != nil || !reflect.DeepEqual(finalReceiptValue, finalAuthoritative) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact receipt does not match final authority")
	}
	finalGate := reviewtransaction.EvaluateCompactGate(ctx, root, finalReceiptValue, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: binding.Lineage})
	if finalGate.Result != reviewtransaction.GateAllow || !boundGateContextMatches(binding.GateContext, finalGate.Context) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact post-apply gate changed during final authorization")
	}
	return binding, finalGate, nil
}

// boundGateContextMatches compares a persisted binding gate context against
// the live post-apply evaluation. Bindings persisted before compact gate
// contexts bound the frozen findings ledger recorded the empty-input hash in
// ledger_hash; they stay valid against a now-populated live context because
// the findings ledger itself is still pinned by the authority state through
// AuthorityRevision and ReceiptHash. Every other divergence — including any
// non-empty ledger hash that does not match the live binding — still fails.
// The reverse mix is a known residual this side cannot repair: a binding
// written by a ledger-binding binary but validated by an older binary
// deterministically fails closed here ("bound compact post-apply gate context
// changed"), because the older binary hardcodes the empty hash in its live
// context; the remedy is upgrading that older binary.
func boundGateContextMatches(bound, live reviewtransaction.GateContext) bool {
	if reflect.DeepEqual(bound, live) {
		return true
	}
	if bound.LedgerHash != reviewtransaction.EmptyFixDeltaHash {
		return false
	}
	bound.LedgerHash = live.LedgerHash
	return reflect.DeepEqual(bound, live)
}

func bindingExists(ctx context.Context, repo, change string) (bool, error) {
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return false, nil
	}
	store, err := OpenRuntimeStore(ctx, root, change)
	if err != nil {
		return false, err
	}
	status, err := store.Status()
	if err != nil {
		return false, err
	}
	if status.Binding != nil {
		return true, nil
	}
	legacyPath := filepath.Join(store.commonDir, "gentle-ai", "sdd-review-bindings", "v1", change, "binding.json")
	_, err = os.Lstat(legacyPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func loadEffectiveReviewBinding(ctx context.Context, repo, change string) (ReviewBinding, error) {
	store, err := OpenRuntimeStore(ctx, repo, change)
	if err != nil {
		return ReviewBinding{}, err
	}
	status, err := store.Status()
	if err != nil {
		return ReviewBinding{}, err
	}
	if status.Binding != nil {
		return *status.Binding, nil
	}
	legacy, _, err := store.readLegacyBinding()
	if err != nil {
		return ReviewBinding{}, err
	}
	if legacy == nil {
		return ReviewBinding{}, os.ErrNotExist
	}
	return *legacy, nil
}

func resolveBindingChangeRoot(ctx context.Context, root, workspace, change string) (string, error) {
	// Both operands are canonicalized the same way before any containment or
	// equality decision below. Resolving only the workspace was 1773 boundary
	// 1: on macOS the same repository spelled through /var and through
	// /private/var compared unequal, and the planning workspace was reported
	// outside its own repository.
	workspace, err := canonicalBindingPath(workspace)
	if err != nil {
		return "", err
	}
	root, err = canonicalBindingPath(root)
	if err != nil {
		return "", err
	}
	if !pathWithinBindingRoot(root, workspace) {
		return "", errors.New("planning workspace is outside selected repository")
	}

	planningRoot := ""
	for current := workspace; pathWithinBindingRoot(root, current); current = filepath.Dir(current) {
		openspecRoot := filepath.Join(current, "openspec")
		info, statErr := os.Stat(openspecRoot)
		if statErr == nil {
			if !info.IsDir() {
				return "", errors.New("selected OpenSpec root is not a directory")
			}
			resolved, resolveErr := filepath.EvalSymlinks(openspecRoot)
			if resolveErr != nil {
				return "", resolveErr
			}
			resolved = filepath.Clean(resolved)
			if !pathWithinBindingRoot(root, resolved) {
				return "", errors.New("selected OpenSpec root resolves outside repository")
			}
			if resolved != filepath.Clean(openspecRoot) {
				return "", errors.New("selected OpenSpec root uses a symlinked path")
			}
			planningRoot = current
			break
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		if pathidentity.SameDirectory(current, root) {
			break
		}
	}
	if planningRoot == "" {
		return "", errors.New("selected OpenSpec change does not exist")
	}
	candidate := filepath.Join(planningRoot, "openspec", "changes", change)
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("selected OpenSpec change does not exist")
		}
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("selected OpenSpec change is not a directory")
	}
	selected, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	selected = filepath.Clean(selected)
	if !pathWithinBindingRoot(root, selected) {
		return "", errors.New("selected OpenSpec change resolves outside repository")
	}
	if selected != filepath.Clean(candidate) {
		return "", errors.New("selected OpenSpec change uses a symlinked path")
	}

	matches, err := bindingChangeRoots(ctx, root, change)
	if err != nil {
		return "", err
	}
	if len(matches) != 1 || matches[0] != selected {
		return "", errors.New("selected OpenSpec change is ambiguous within repository")
	}
	return selected, nil
}

func bindingChangeRoots(ctx context.Context, root, change string) ([]string, error) {
	paths, err := (reviewtransaction.SnapshotBuilder{Repo: root}).DiscoverTrackedAndUnignoredPaths(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	matches := []string{}
	for _, logicalPath := range paths {
		parts := strings.Split(logicalPath, "/")
		if parts[len(parts)-1] == "openspec" {
			parts = append(parts, "changes", change)
		}
		for index := 0; index+2 < len(parts); index++ {
			if parts[index] != "openspec" || parts[index+1] != "changes" || parts[index+2] != change {
				continue
			}
			rootPath := strings.Join(parts[:index+3], "/")
			if _, duplicate := seen[rootPath]; duplicate {
				break
			}
			seen[rootPath] = struct{}{}
			candidate := filepath.Join(root, filepath.FromSlash(rootPath))
			info, statErr := os.Lstat(candidate)
			if os.IsNotExist(statErr) {
				break
			}
			if statErr != nil {
				return nil, statErr
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				matches = append(matches, filepath.Clean(candidate))
			}
			break
		}
	}
	return matches, nil
}

// canonicalBindingPath is the single canonicalization every binding path goes
// through before it is compared with another. Having one of these, used on
// both operands, is what keeps a second spelling of one repository from
// looking like a different repository.
func canonicalBindingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// pathWithinBindingRoot defers containment to the filesystem identity policy
// in internal/pathidentity, so alternate spellings that the operating system
// resolves to one directory -- symlinked ancestors, case-insensitive volumes,
// Unicode-equivalent names -- are one directory here too. Callers still
// resolve a candidate with filepath.EvalSymlinks before asking, because this
// answers "is it inside", never "did it get there through a symlink".
func pathWithinBindingRoot(root, path string) bool {
	return pathidentity.Contains(root, path)
}

func verifyBindingLedger(changeRoot string, findings []reviewtransaction.Finding) error {
	payload, err := os.ReadFile(filepath.Join(changeRoot, "reviews", "ledger.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	want, err := reviewtransaction.CanonicalLedger(findings)
	if err != nil || !bytes.Equal(payload, want) {
		return errors.New("SDD review ledger does not equal compact findings")
	}
	return nil
}
func bindingPath(store reviewtransaction.CompactStore, change string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(store.Dir)))), "gentle-ai", "sdd-review-bindings", "v1", change, "binding.json")
}
func bindingHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func bindingDigest(b ReviewBinding) string {
	b.Revision = ""
	payload, _ := json.Marshal(b)
	return bindingHash(payload)
}

func validReviewBindingChange(change string) bool {
	return len(change) <= 96 && reviewBindingChange.MatchString(change)
}

func validReviewBindingLineage(lineage string) bool {
	return len(lineage) <= 128 && reviewBindingLineage.MatchString(lineage)
}

func bindingBytes(binding ReviewBinding) ([]byte, error) {
	payload, err := json.Marshal(binding)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func parseBinding(payload []byte) (ReviewBinding, error) {
	var binding ReviewBinding
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&binding); err != nil {
		return ReviewBinding{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ReviewBinding{}, errors.New("multiple binding values")
	}
	canonical, err := bindingBytes(binding)
	if err != nil || !bytes.Equal(payload, canonical) || binding.Schema != reviewBindingSchema || !validReviewBindingChange(binding.Change) || !validReviewBindingLineage(binding.Lineage) || !reviewBindingHash.MatchString(binding.Revision) || !reviewBindingHash.MatchString(binding.AuthorityRevision) || !reviewBindingHash.MatchString(binding.ReceiptHash) || binding.Revision != bindingDigest(binding) || binding.GateContext.Gate != reviewtransaction.GatePostApply || binding.GateContext.LineageID != binding.Lineage || binding.GateContext.StoreRevision != binding.AuthorityRevision {
		return ReviewBinding{}, errors.New("invalid binding")
	}
	return binding, nil
}
