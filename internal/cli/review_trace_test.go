package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestZeroLensStartCommitsAndReportsUnwritableTraceOutcome is issue #1854,
// reworked to the issue's own accepted scope: `review start
// --trace=<existing-empty-directory>` on a zero-lens candidate must commit
// exactly as it would without a requested trace -- the transition genuinely
// succeeded -- and represent the failed trace as a typed, committed-but-
// degraded outcome instead of a stderr-only warning. Zero lenses (ordinary
// low-risk documentation) is what routes review start through the exact
// CommitApprovedCompactAcknowledgement call this fix guards.
func TestZeroLensStartCommitsAndReportsUnwritableTraceOutcome(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = fmt.Sprintf("ordinary documentation line %03d", index+1)
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", strings.Join(lines, "\n")+"\n", 0o644)

	traceDir := t.TempDir() // an existing directory: never openable as a file

	var startOutput bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--trace", traceDir,
	}), &startOutput); err != nil {
		t.Fatalf("RunReview() with an unwritable requested trace: %v\n%s", err, startOutput.String())
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &started)
	if started.Action != "closed" || started.Acknowledgement == nil {
		t.Fatalf("zero-lens START did not commit despite the trace failure: %#v", started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)

	if started.Trace == nil {
		t.Fatal("no trace outcome reported for a requested --trace")
	}
	if started.Trace.Persisted {
		t.Fatalf("trace outcome = %#v, want persisted=false for an unwritable target", started.Trace)
	}
	if started.Trace.ErrorClass == "" {
		t.Fatal("trace outcome has no error class")
	}
	if started.Trace.CommittedRevision == "" || started.Trace.CommittedRevision != started.Acknowledgement.Binding.Revision {
		t.Fatalf("trace outcome committed revision = %q, want the exact committed revision %q", started.Trace.CommittedRevision, started.Acknowledgement.Binding.Revision)
	}
	if started.Trace.EventIdentity == "" {
		t.Fatal("trace outcome has no event identity")
	}
	if started.Trace.Retry == "" {
		t.Fatal("a failed trace outcome must name the retry guidance")
	}
	if started.Trace.RequestedPath == "" || strings.Contains(started.Trace.RequestedPath, traceDir) {
		t.Fatalf("trace outcome requested_path = %q, want a redacted identity, not the raw path", started.Trace.RequestedPath)
	}

	traceEntries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(traceEntries) != 0 {
		t.Fatalf("unwritable trace target unexpectedly gained entries: %v", traceEntries)
	}
}

// TestZeroLensStartRecordsTraceWhenTracePathIsWritable is the positive-path
// sibling: a writable --trace path commits normally and reports
// persisted=true with the committed revision and no error class.
func TestZeroLensStartRecordsTraceWhenTracePathIsWritable(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = fmt.Sprintf("ordinary documentation line %03d", index+1)
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", strings.Join(lines, "\n")+"\n", 0o644)

	tracePath := filepath.Join(t.TempDir(), "trace.jsonl")

	var startOutput bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--trace", tracePath,
	}), &startOutput); err != nil {
		t.Fatalf("RunReview() with a writable trace path: %v\n%s", err, startOutput.String())
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &started)
	if started.Action != "closed" {
		t.Fatalf("zero-lens START with a writable trace = %#v", started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)

	if started.Trace == nil {
		t.Fatal("no trace outcome reported for a requested --trace")
	}
	if !started.Trace.Persisted || started.Trace.ErrorClass != "" || started.Trace.Retry != "" {
		t.Fatalf("trace outcome = %#v, want persisted=true, no error class, no retry guidance", started.Trace)
	}
	if started.Trace.CommittedRevision == "" || started.Trace.EventIdentity == "" {
		t.Fatalf("trace outcome missing identity fields: %#v", started.Trace)
	}
	payload, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("requested trace was not written: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"review/complete-review"`)) {
		t.Fatalf("trace payload missing the committed operation: %s", payload)
	}
}
