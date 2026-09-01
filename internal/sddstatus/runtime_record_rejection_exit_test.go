package sddstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// #3938: a record an older binary admitted (here a finish whose
// evidence_revision is 63 hex digits) is rejected on read by a newer binary.
// That rejection used to be a dead end: status, acquire, settle, and reset all
// stopped at "record rejected" with no way to find the record. The message
// must name the record file, say a maintainer must inspect or remove it, and
// keep the read-only status continuation. No auto-repair exists by design.
func TestRejectedHistoricalRecordNamesItsFileAndMaintainer(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "historical-finish")
	begun, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "historical-begin", WorkUnit: "apply", EvidenceGoal: "prove it",
		MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: begun.Revision,
		Operation: runtimeOperationFinish, RequestID: "historical-finish",
		RequestDigest: "sha256:" + strings.Repeat("a", 64),
		Finish: &runtimeFinishEvent{
			Ordinal: 1, FinishCandidateIdentity: "sha256:" + strings.Repeat("b", 64),
			FinishCandidateTree: strings.Repeat("c", 40), Outcome: AttemptPassed,
			EvidenceRevision: "sha256:" + strings.Repeat("d", 63), HarnessDisposition: HarnessReused,
		},
	}
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(store.Dir, "records", strings.TrimPrefix(revision, "sha256:")+".json")

	_, err = store.Status()
	if err == nil {
		t.Fatal("status read the hand-corrupted finish record as valid authority")
	}
	for _, want := range []string{"invalid_finish_event", recordPath, "maintainer", "gentle-ai sdd-attempt status --cwd <repo> --change <change>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("status rejection lacks %q: %v", want, err)
		}
	}

	result, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "historical-acquire", WorkUnit: "apply", EvidenceGoal: "prove it",
			MaxAttempts: 2, MaxChangedLines: 20,
		},
	})
	if err != nil {
		t.Fatalf("acquire over a rejected record returned a hard error rather than a block: %v", err)
	}
	if result.State != CompactStateBlocked || result.Reason != CompactBlockCorruptAuthority {
		t.Fatalf("result = %#v, want blocked/corrupt_authority", result)
	}
	for _, want := range []string{recordPath, "maintainer"} {
		if !strings.Contains(result.Detail, want) {
			t.Fatalf("corrupt_authority detail lacks %q: %q", want, result.Detail)
		}
	}
}
