package reviewtransaction

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
)

const finalizeAttemptJournalSchema = "gentle-ai.review-finalize-attempt-journal/v1"

var writeFinalizeAttemptAtomic = writeAtomic

// FinalizeAttemptRequest is a content-bound FINALIZE invocation. Input paths
// intentionally do not appear here: paths are transport, their payloads are
// the authority-relevant request.
type FinalizeAttemptRequest struct {
	LineageID                string `json:"lineage_id"`
	ExpectedRevision         string `json:"expected_revision"`
	CandidateDigest          string `json:"candidate_digest"`
	ReviewerResultsDigest    string `json:"reviewer_results_digest"`
	CorrectionForecastDigest string `json:"correction_forecast_digest"`
	ValidationDigest         string `json:"validation_digest"`
	RefuterDigest            string `json:"refuter_digest"`
	EvidenceDigest           string `json:"evidence_digest"`
	FailedDigest             string `json:"failed_digest"`
	RequestDigest            string `json:"request_digest"`
}

type FinalizeAttemptTransition struct {
	Operation        string `json:"operation"`
	ExpectedRevision string `json:"expected_revision"`
	Revision         string `json:"revision"`
}

type FinalizeAttempt struct {
	Request          FinalizeAttemptRequest      `json:"request"`
	Transitions      []FinalizeAttemptTransition `json:"transitions"`
	ReceiptPublished bool                        `json:"receipt_published"`
	Completed        bool                        `json:"completed"`
}

type finalizeAttemptJournal struct {
	Schema   string            `json:"schema"`
	Attempts []FinalizeAttempt `json:"attempts"`
}

type FinalizeAttemptReplayMismatchError struct{ LineageID string }

func (err *FinalizeAttemptReplayMismatchError) Error() string {
	return fmt.Sprintf("finalize request does not match the incomplete attempt for lineage %q", err.LineageID)
}

func FinalizeAttemptValueDigest(domain string, value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(append([]byte("gentle-ai.finalize-input/"+domain+"\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func FinalizeAttemptRequestDigest(request FinalizeAttemptRequest) string {
	request.RequestDigest = ""
	return FinalizeAttemptValueDigest("request", request)
}

func (store CompactStore) FinalizeAttemptJournalPath() string {
	return filepath.Join(store.Dir, compactFinalizeJournalFileName)
}

// PendingFinalizeAttempt returns the one unresolved request for this lineage.
// It is used only to complete a terminal receipt publication; the terminal
// authority itself remains the proof that no native transition is replayed.
func (store CompactStore) PendingFinalizeAttempt() (*FinalizeAttempt, error) {
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return nil, err
	}
	defer lock.release()
	journal, err := store.loadFinalizeAttemptJournalLocked()
	if err != nil {
		return nil, err
	}
	for index := len(journal.Attempts) - 1; index >= 0; index-- {
		if !journal.Attempts[index].Completed {
			attempt := journal.Attempts[index]
			return &attempt, nil
		}
	}
	return nil, nil
}

// PendingFinalizeAttemptReadOnly never acquires or rewrites LOCK, so status
// remains observational even while a writer owns the compact authority.
func (store CompactStore) PendingFinalizeAttemptReadOnly() (*FinalizeAttempt, error) {
	payload, err := os.ReadFile(store.FinalizeAttemptJournalPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	journal, err := parseFinalizeAttemptJournal(payload, store.lineageID)
	if err != nil {
		return nil, err
	}
	for index := len(journal.Attempts) - 1; index >= 0; index-- {
		if !journal.Attempts[index].Completed {
			attempt := journal.Attempts[index]
			return &attempt, nil
		}
	}
	return nil, nil
}

// BeginFinalizeAttempt durably records the request before its first native
// transition. A later replay may only resume the same content-bound request.
func (store CompactStore) BeginFinalizeAttempt(ctx context.Context, request FinalizeAttemptRequest) (FinalizeAttempt, bool, error) {
	return store.ReconcileFinalizeAttempt(ctx, request)
}

// ReconcileFinalizeAttempt is the only FINALIZE replay admission point. It
// accepts an exact request at its entry revision, or the same payload after a
// journaled (or inferable immediately post-commit) native transition. A new
// revision or candidate is never enough by itself to replay an attempt.
func (store CompactStore) ReconcileFinalizeAttempt(ctx context.Context, request FinalizeAttemptRequest) (FinalizeAttempt, bool, error) {
	if err := ctx.Err(); err != nil {
		return FinalizeAttempt{}, false, err
	}
	if err := validateFinalizeAttemptRequest(store.lineageID, request); err != nil {
		return FinalizeAttempt{}, false, err
	}
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return FinalizeAttempt{}, false, err
	}
	defer lock.release()
	journal, err := store.loadFinalizeAttemptJournalLocked()
	if err != nil {
		return FinalizeAttempt{}, false, err
	}
	for index := range journal.Attempts {
		attempt := journal.Attempts[index]
		if attempt.Request.RequestDigest == request.RequestDigest {
			if err := SyncReviewDirectory(filepath.Dir(store.FinalizeAttemptJournalPath())); err != nil {
				return FinalizeAttempt{}, false, &directorySyncError{path: store.FinalizeAttemptJournalPath(), cause: err}
			}
			return attempt, true, nil
		}
		if attempt.Completed {
			continue
		}
		if sameFinalizeAttemptPayload(attempt.Request, request) {
			current, loadErr := store.loadCompactRecordLocked()
			if loadErr == nil && finalizeAttemptMayResume(attempt, request, current) {
				return attempt, true, nil
			}
		}
		if attempt.Request.LineageID == request.LineageID {
			return FinalizeAttempt{}, false, &FinalizeAttemptReplayMismatchError{LineageID: request.LineageID}
		}
	}
	attempt := FinalizeAttempt{Request: request, Transitions: []FinalizeAttemptTransition{}}
	journal.Attempts = append(journal.Attempts, attempt)
	if err := store.writeFinalizeAttemptJournalLocked(journal); err != nil {
		return FinalizeAttempt{}, false, err
	}
	return attempt, false, nil
}

func sameFinalizeAttemptPayload(left, right FinalizeAttemptRequest) bool {
	return left.LineageID == right.LineageID && left.CandidateDigest == right.CandidateDigest &&
		left.ReviewerResultsDigest == right.ReviewerResultsDigest && left.CorrectionForecastDigest == right.CorrectionForecastDigest &&
		left.ValidationDigest == right.ValidationDigest && left.RefuterDigest == right.RefuterDigest &&
		left.EvidenceDigest == right.EvidenceDigest && left.FailedDigest == right.FailedDigest
}

func (store CompactStore) loadCompactRecordLocked() (CompactRecord, error) {
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		return CompactRecord{}, err
	}
	return parseCompactRecord(payload, store.lineageID)
}

func finalizeAttemptMayResume(attempt FinalizeAttempt, request FinalizeAttemptRequest, current CompactRecord) bool {
	if current.Revision == attempt.Request.ExpectedRevision {
		return request.ExpectedRevision == attempt.Request.ExpectedRevision
	}
	for _, transition := range attempt.Transitions {
		if transition.Revision == current.Revision && request.ExpectedRevision == current.Revision {
			return true
		}
	}
	return false
}

func (store CompactStore) RecordFinalizeAttemptTransition(requestDigest, operation, revision string) error {
	return store.updateFinalizeAttempt(requestDigest, func(attempt *FinalizeAttempt) error {
		if len(attempt.Transitions) > 0 && attempt.Transitions[len(attempt.Transitions)-1].Operation == operation && attempt.Transitions[len(attempt.Transitions)-1].Revision == revision {
			return nil
		}
		expected := attempt.Request.ExpectedRevision
		if len(attempt.Transitions) > 0 {
			expected = attempt.Transitions[len(attempt.Transitions)-1].Revision
		}
		attempt.Transitions = append(attempt.Transitions, FinalizeAttemptTransition{Operation: operation, ExpectedRevision: expected, Revision: revision})
		return nil
	})
}

// PlanFinalizeAttemptTransition is write-ahead: the exact successor revision
// is durable before ReplaceContext can make that successor authoritative.
func (store CompactStore) PlanFinalizeAttemptTransition(requestDigest, operation, expectedRevision string, next CompactState) (string, error) {
	revision, err := CompactRevisionForState(next)
	if err != nil {
		return "", err
	}
	err = store.updateFinalizeAttempt(requestDigest, func(attempt *FinalizeAttempt) error {
		for _, transition := range attempt.Transitions {
			if transition.Operation == operation && transition.ExpectedRevision == expectedRevision && transition.Revision == revision {
				return nil
			}
		}
		attempt.Transitions = append(attempt.Transitions, FinalizeAttemptTransition{Operation: operation, ExpectedRevision: expectedRevision, Revision: revision})
		return nil
	})
	return revision, err
}

func (store CompactStore) MarkFinalizeAttemptReceiptPublished(requestDigest string) error {
	return store.updateFinalizeAttempt(requestDigest, func(attempt *FinalizeAttempt) error {
		attempt.ReceiptPublished = true
		return nil
	})
}

func (store CompactStore) CompleteFinalizeAttempt(requestDigest string) error {
	return store.updateFinalizeAttempt(requestDigest, func(attempt *FinalizeAttempt) error {
		attempt.Completed = true
		return nil
	})
}

func (store CompactStore) updateFinalizeAttempt(requestDigest string, update func(*FinalizeAttempt) error) error {
	if !validSHA256(requestDigest) {
		return errors.New("invalid finalize attempt request digest")
	}
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return err
	}
	defer lock.release()
	journal, err := store.loadFinalizeAttemptJournalLocked()
	if err != nil {
		return err
	}
	for index := range journal.Attempts {
		if journal.Attempts[index].Request.RequestDigest != requestDigest {
			continue
		}
		if err := update(&journal.Attempts[index]); err != nil {
			return err
		}
		return store.writeFinalizeAttemptJournalLocked(journal)
	}
	return errors.New("finalize attempt journal entry is missing")
}

func (store CompactStore) loadFinalizeAttemptJournalLocked() (finalizeAttemptJournal, error) {
	payload, err := os.ReadFile(store.FinalizeAttemptJournalPath())
	if errors.Is(err, os.ErrNotExist) {
		return finalizeAttemptJournal{Schema: finalizeAttemptJournalSchema, Attempts: []FinalizeAttempt{}}, nil
	}
	if err != nil {
		return finalizeAttemptJournal{}, err
	}
	return parseFinalizeAttemptJournal(payload, store.lineageID)
}

// writeFinalizeAttemptJournalLocked rereads and parses the exact replacement
// after rename. This turns an ambiguous post-rename error into a durable fact.
func (store CompactStore) writeFinalizeAttemptJournalLocked(journal finalizeAttemptJournal) error {
	payload, err := marshalFinalizeAttemptJournal(journal)
	if err != nil {
		return err
	}
	writeErr := writeFinalizeAttemptAtomic(store.FinalizeAttemptJournalPath(), payload, 0o644)
	var syncErr *directorySyncError
	if errors.As(writeErr, &syncErr) {
		return writeErr
	}
	reloaded, err := os.ReadFile(store.FinalizeAttemptJournalPath())
	if err != nil {
		return fmt.Errorf("reread finalize attempt journal after replacement: %w", err)
	}
	got, err := parseFinalizeAttemptJournal(reloaded, store.lineageID)
	if err != nil || !reflect.DeepEqual(got, journal) {
		if writeErr != nil {
			return writeErr
		}
		return errors.New("finalize attempt journal replacement is ambiguous")
	}
	return nil
}

func marshalFinalizeAttemptJournal(journal finalizeAttemptJournal) ([]byte, error) {
	if journal.Schema == "" {
		journal.Schema = finalizeAttemptJournalSchema
	}
	if err := validateFinalizeAttemptJournal(journal, ""); err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(journal, "", "  ")
	return append(payload, '\n'), err
}

func parseFinalizeAttemptJournal(payload []byte, lineageID string) (finalizeAttemptJournal, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var journal finalizeAttemptJournal
	if err := decoder.Decode(&journal); err != nil {
		return finalizeAttemptJournal{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return finalizeAttemptJournal{}, errors.New("multiple JSON values in finalize attempt journal")
	}
	if err := validateFinalizeAttemptJournal(journal, lineageID); err != nil {
		return finalizeAttemptJournal{}, err
	}
	return journal, nil
}

func validateFinalizeAttemptJournal(journal finalizeAttemptJournal, lineageID string) error {
	if journal.Schema != finalizeAttemptJournalSchema || journal.Attempts == nil {
		return errors.New("invalid finalize attempt journal")
	}
	seen := map[string]struct{}{}
	for _, attempt := range journal.Attempts {
		if err := validateFinalizeAttemptRequest(lineageID, attempt.Request); err != nil {
			return err
		}
		if _, ok := seen[attempt.Request.RequestDigest]; ok {
			return errors.New("duplicate finalize attempt request digest")
		}
		seen[attempt.Request.RequestDigest] = struct{}{}
		for _, transition := range attempt.Transitions {
			if strings.TrimSpace(transition.Operation) == "" || !validSHA256(transition.ExpectedRevision) || !validSHA256(transition.Revision) {
				return errors.New("invalid finalize attempt transition")
			}
		}
	}
	return nil
}

func validateFinalizeAttemptRequest(lineageID string, request FinalizeAttemptRequest) error {
	if validateLineageID(request.LineageID) != nil || lineageID != "" && request.LineageID != lineageID ||
		!validSHA256(request.ExpectedRevision) || !validSHA256(request.RequestDigest) || request.RequestDigest != FinalizeAttemptRequestDigest(request) {
		return errors.New("invalid finalize attempt request")
	}
	for _, digest := range []string{request.CandidateDigest, request.ReviewerResultsDigest, request.CorrectionForecastDigest, request.ValidationDigest, request.RefuterDigest, request.EvidenceDigest, request.FailedDigest} {
		if !validSHA256(digest) {
			return errors.New("invalid finalize attempt input digest")
		}
	}
	return nil
}
