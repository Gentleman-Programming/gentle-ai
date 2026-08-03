package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewStartTraceOpenFailureReturnsCommittedDegradationAndStatus(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	tracePath := t.TempDir() // A directory cannot be opened as the requested JSONL file.
	args := []string{"--cwd", repo, "--lineage", "trace-gap-start", "--trace", tracePath}
	var output bytes.Buffer
	if err := RunReviewFacadeStart(args, &output); err != nil {
		t.Fatalf("START must preserve its committed authority: %v", err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	degradation := started.TraceDegradation
	if degradation == nil || degradation.MutationOutcome != "committed" || degradation.LineageID != started.LineageID ||
		degradation.Event.Operation != "review/start" || !degradation.TraceRequested || !degradation.TraceFailed ||
		degradation.FailureCode != reviewtransaction.CompactTraceFailureOpen || degradation.RetrySafe || degradation.NextAction != "review.status" {
		t.Fatalf("START trace degradation = %#v", degradation)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != degradation.StoreRevision || record.State.LineageID != degradation.LineageID {
		t.Fatalf("committed authority = %#v, degradation = %#v", record, degradation)
	}
	var statusOutput bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo}, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var status reviewtransaction.AuthorityStatusReport
	if err := json.Unmarshal(statusOutput.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Entries) != 1 || status.Entries[0].TraceDegradation == nil ||
		status.Entries[0].TraceDegradation.StoreRevision != record.Revision ||
		status.Entries[0].TraceDegradation.Event.Operation != "review/start" {
		t.Fatalf("STATUS trace gap = %#v", status.Entries)
	}
	output.Reset()
	if err := RunReviewFacadeStart(args, &output); err != nil {
		t.Fatalf("exact START replay must remain successful: %v", err)
	}
	var replay ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &replay); err != nil {
		t.Fatal(err)
	}
	if replay.TraceDegradation != nil {
		t.Fatalf("replay invented a new trace degradation: %#v", replay.TraceDegradation)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != record.Revision {
		t.Fatalf("replay changed authority revision from %q to %q", record.Revision, after.Revision)
	}
	entries, err := os.ReadDir(tracePath)
	if err != nil || len(entries) != 0 {
		t.Fatalf("directory trace path gained a false JSONL trace: entries=%v err=%v", entries, err)
	}
}

func TestEncodeCompactFacadeFinalizeReportsTraceDegradationReadFailure(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	var startedOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "trace-read-failure"}, &startedOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(startedOutput.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Dir, "trace-gaps", strings.TrimPrefix(record.Revision, "sha256:")+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := encodeCompactFacadeFinalize(&output, false, "", false, false, record.State, record.Revision, store, "unchanged"); err != nil {
		t.Fatalf("committed FINALIZE output must remain successful: %v", err)
	}
	var result ReviewFacadeFinalizeResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	problem := result.TraceDegradationProblem
	if problem == nil || problem.MutationOutcome != "committed" || problem.LineageID != started.LineageID ||
		problem.StoreRevision != record.Revision || problem.FailureCode != reviewtransaction.CompactTraceDegradationReadFailureCode ||
		problem.RetrySafe || problem.NextAction != "review.status" || !strings.Contains(problem.Message, "could not be read") {
		t.Fatalf("FINALIZE trace degradation read problem = %#v", problem)
	}
	after, err := store.Load()
	if err != nil || after.Revision != record.Revision {
		t.Fatalf("FINALIZE read failure changed authority: record=%#v err=%v", after, err)
	}
}
