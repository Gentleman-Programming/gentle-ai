package sddstatus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// runtime_receipt.go is Wave 4 S5b (design.md decision 1, task 6.5):
// RuntimeStatus.Receipt, recorded through the exact same CAS/locking and
// one-time legacy-import idempotency pattern bindPreparedReview already
// proved for Binding — mirrored here for reviewtransaction.SDDReceiptRef,
// importing the legacy compatibility artifact through legacy_binding_read.
// go's parseLegacyBinding projection instead of the full ReviewBinding
// shape.
//
// This is additive alongside Binding/BindingRevision, not a replacement:
// see apply-progress for the confirmed ~40-call-site blast radius
// (validateRuntimeRecordShape's per-operation state machine for every OTHER
// operation, runtime_compact.go's remediation-successor CAS, review_binding
// .go's writer functions) that removing those fields outright would touch —
// a materially larger, separate migration than this slice's safe budget.

const runtimeOperationReceipt = "receipt/set"

type runtimeReceiptEvent struct {
	ExpectedRevision string                          `json:"expected_revision"`
	Current          reviewtransaction.SDDReceiptRef `json:"current"`
	LegacyImport     *runtimeLegacyReceiptImport     `json:"legacy_import,omitempty"`
}

type runtimeLegacyReceiptImport struct {
	SourceDigest string                          `json:"source_digest"`
	Receipt      reviewtransaction.SDDReceiptRef `json:"receipt"`
}

// RecordReceiptRequest is recordPreparedReceipt's CAS request, mirroring
// BindReviewRequest's shape for the receipt event.
type RecordReceiptRequest struct {
	ExpectedReceiptRevision string
	RequestID               string
	Lineage                 string
}

// ReceiptRevisionConflictError mirrors BindingRevisionConflictError for the
// receipt CAS anchor.
type ReceiptRevisionConflictError struct{ Expected, Current string }

func (err *ReceiptRevisionConflictError) Error() string {
	return fmt.Sprintf(
		"SDD runtime receipt revision conflict: expected %q, current %q; retry with --expected-receipt-revision %q",
		err.Expected, err.Current, err.Current,
	)
}

// receiptRefDigest is the CAS anchor for a SDDReceiptRef, mirroring
// bindingDigest's role for ReviewBinding. SDDReceiptRef carries no Revision
// field of its own (design.md decision 1: exactly two fields), so this
// digest is what BindingRevision's role is played by for the receipt event.
func receiptRefDigest(ref reviewtransaction.SDDReceiptRef) string {
	payload, err := json.Marshal(ref)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeRecordReceiptRequest(request RecordReceiptRequest) (RecordReceiptRequest, error) {
	if len(request.ExpectedReceiptRevision) > 128 || strings.ContainsAny(request.ExpectedReceiptRevision, "\r\n\x00") {
		return RecordReceiptRequest{}, errors.New("expected receipt revision is not a bounded single-line value") // refusal:by-design operator-knowledge: only the caller knows the intended expected-revision token; retry with a bounded single-line value
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return RecordReceiptRequest{}, errors.New("request_id must be a canonical lowercase identifier") // refusal:by-design operator-knowledge: only the caller can supply a canonical lowercase request identifier
	}
	if !validReviewBindingLineage(request.Lineage) {
		return RecordReceiptRequest{}, errors.New("lineage must be a canonical lowercase lineage") // refusal:by-design operator-knowledge: only the caller knows the intended lineage; retry with a canonical lowercase lineage identifier
	}
	return request, nil
}

func applyRuntimeReceiptEvent(replay *runtimeReplay, event *runtimeReceiptEvent) error {
	current := ""
	if replay.Status.Receipt != nil {
		if event.LegacyImport != nil {
			return errors.New("native receipt successor cannot import legacy authority again") // refusal:by-design world-action: a populated native receipt makes legacy import structurally impossible, so this is a mutated record and the exit is restoring the store
		}
		current = receiptRefDigest(*replay.Status.Receipt)
	} else if event.LegacyImport != nil {
		current = receiptRefDigest(event.LegacyImport.Receipt)
	}
	if current != event.ExpectedRevision {
		return errors.New("receipt record expected revision does not equal replay state") // refusal:by-design world-action: this record was already validated at commit time, so a replay mismatch is a mutated record and the exit is restoring the store
	}
	receipt := event.Current
	replay.Status.Receipt = &receipt
	return nil
}

// readLegacyReceiptRef mirrors readLegacyBinding's exact read pattern but
// projects through legacy_binding_read.go's parseLegacyBinding instead of
// parseBinding, so the receipt path never carries GateContext or
// AuthorityRevision — SDDReceiptRef's own two fields are all it needs.
func (store RuntimeStore) readLegacyReceiptRef() (*reviewtransaction.SDDReceiptRef, string, error) {
	legacy, digest, err := store.readLegacyBinding()
	if err != nil || legacy == nil {
		return nil, digest, err
	}
	ref := reviewtransaction.SDDReceiptRef{Lineage: legacy.Lineage, ReceiptHash: legacy.ReceiptHash}
	return &ref, digest, nil
}

// recordPreparedReceipt mirrors bindPreparedReview's exact CAS/locking and
// one-time legacy-import idempotency semantics for a SDDReceiptRef instead
// of a ReviewBinding.
func (store RuntimeStore) recordPreparedReceipt(
	ctx context.Context,
	request RecordReceiptRequest,
	prepare func() (reviewtransaction.SDDReceiptRef, error),
) (RuntimeStatus, error) {
	request, err := normalizeRecordReceiptRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	requestDigest := runtimeValueHash("gentle-ai.sdd-runtime-receipt-request/v1", request)
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, err
	}
	if err := store.ensureDirectories(); err != nil {
		return RuntimeStatus{}, err
	}
	lock, err := store.acquireLock()
	if err != nil {
		return RuntimeStatus{}, err
	}
	defer lock.Release()

	replay, err := store.load()
	if err != nil {
		return RuntimeStatus{}, err
	}
	if receipt, ok := replay.Requests[request.RequestID]; ok {
		if receipt.Digest != requestDigest {
			return RuntimeStatus{}, ErrRuntimeRequestConflict
		}
		if err := store.syncReplay(); err != nil {
			return RuntimeStatus{}, &RuntimePublicationError{Revision: receipt.Revision, Committed: true, Cause: err}
		}
		return replay.Status, nil
	}

	var legacy *reviewtransaction.SDDReceiptRef
	var legacyDigest string
	if replay.Status.Receipt == nil {
		legacy, legacyDigest, err = store.readLegacyReceiptRef()
		if err != nil {
			return RuntimeStatus{}, fmt.Errorf("read legacy SDD review binding as a receipt ref: %w", err)
		}
	}

	prepared, err := prepare()
	if err != nil {
		return RuntimeStatus{}, err
	}
	if prepared.Lineage != request.Lineage {
		return RuntimeStatus{}, errors.New("prepared SDD runtime receipt does not match selected lineage") // refusal:by-design operator-knowledge: only the caller's prepare callback can supply a receipt that agrees with the requested lineage
	}
	if replay.Status.Receipt == nil {
		finalLegacy, finalDigest, finalErr := store.readLegacyReceiptRef()
		if finalErr != nil {
			return RuntimeStatus{}, fmt.Errorf("reopen legacy SDD review binding as a receipt ref: %w", finalErr)
		}
		if (legacy == nil) != (finalLegacy == nil) || legacyDigest != finalDigest {
			return RuntimeStatus{}, errors.New("legacy SDD review binding changed before native receipt import") // refusal:by-design world-action: the legacy artifact was mutated between the two reads inside this call; retry once the concurrent external write settles
		}
	}

	// A populated native receipt is authoritative. Identical-candidate
	// retries are no-ops even when the caller repeats the original expected
	// revision — the same idempotent bind contract bindPreparedReview keeps,
	// without another mutable request journal.
	if replay.Status.Receipt != nil {
		if *replay.Status.Receipt == prepared {
			if err := store.syncReplay(); err != nil {
				return RuntimeStatus{}, &RuntimePublicationError{Revision: replay.Status.Revision, Committed: true, Cause: err}
			}
			return replay.Status, nil
		}
		currentRevision := receiptRefDigest(*replay.Status.Receipt)
		if request.ExpectedReceiptRevision != currentRevision {
			return RuntimeStatus{}, &ReceiptRevisionConflictError{Expected: request.ExpectedReceiptRevision, Current: currentRevision}
		}
	} else {
		current := ""
		if legacy != nil {
			current = receiptRefDigest(*legacy)
		}
		if request.ExpectedReceiptRevision != current {
			return RuntimeStatus{}, &ReceiptRevisionConflictError{Expected: request.ExpectedReceiptRevision, Current: current}
		}
	}

	event := &runtimeReceiptEvent{ExpectedRevision: request.ExpectedReceiptRevision, Current: prepared}
	if replay.Status.Receipt == nil && legacy != nil {
		event.LegacyImport = &runtimeLegacyReceiptImport{SourceDigest: legacyDigest, Receipt: *legacy}
	}
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: replay.Status.Revision,
		Operation: runtimeOperationReceipt, RequestID: request.RequestID, RequestDigest: requestDigest, Receipt: event,
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		return RuntimeStatus{}, err
	}
	return store.commitRecordLocked(record)
}
