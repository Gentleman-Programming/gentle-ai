package sddstatus

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

// TestRuntimeLedgerRefusesFinishFromADifferentLinkedWorktreeThanBegin is the
// RED reproduction for #2296 part 1: OpenRuntimeStore resolves the shared
// Git-common-dir ledger identically from every linked worktree of the same
// repository, but Begin pinned its base candidate tree to store.Repo -- the
// worktree-LOCAL toplevel of the exact --cwd it ran under -- while Finish,
// called from a DIFFERENT --cwd, rebuilt store.Repo against that OTHER
// worktree and diffed its working tree against the pinned base. Before this
// fix that diff silently measured the distance between two unrelated trees:
// worktree B's own clean checkout matched the pinned base exactly, so the
// authorized apply that really ran in worktree A was charged changed_lines: 0.
func TestRuntimeLedgerRefusesFinishFromADifferentLinkedWorktreeThanBegin(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	worktree := filepath.Join(t.TempDir(), "sibling-worktree")
	runRuntimeLedgerGit(t, repo, "worktree", "add", "-q", "-b", "worktree-b", worktree)

	change := "worktree-binding"
	storeA, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	began, err := storeA.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-a", WorkUnit: "cross-worktree-binding",
		EvidenceGoal: "prove worktree-pinned candidate accounting", MaxAttempts: 2, MaxChangedLines: 200,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The authorized apply actually runs in worktree A.
	appendRuntimeLedgerFile(t, repo, "changed-in-a\n")

	storeB, err := OpenRuntimeStore(context.Background(), worktree, change)
	if err != nil {
		t.Fatal(err)
	}
	_, finishErr := storeB.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: began.Revision, RequestID: "finish-b", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('9'), Diagnosis: "cross-worktree finish must refuse before capturing any candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "no cleanup needed for the refused finish",
		ProcessEvidence: "no process evidence needed for the refused finish",
	})
	if !errors.Is(finishErr, ErrRuntimeWorktreeMismatch) {
		t.Fatalf("cross-worktree finish error = %v, want ErrRuntimeWorktreeMismatch", finishErr)
	}
	message := finishErr.Error()
	beginWorktree := canonicalRuntimeLedgerWorktreePath(t, repo)
	if !strings.Contains(message, beginWorktree) {
		t.Fatalf("refusal %q does not name the begin worktree %q to rerun finish from", message, beginWorktree)
	}
	currentWorktree := canonicalRuntimeLedgerWorktreePath(t, worktree)
	if !strings.Contains(message, currentWorktree) {
		t.Fatalf("refusal %q does not name the current mismatched worktree %q", message, currentWorktree)
	}

	// Mutation proof: the refusal fires before any candidate capture/diff, so
	// the ledger revision is unchanged and no new record was published.
	status, statusErr := storeA.Status()
	if statusErr != nil || status.Revision != began.Revision || countRuntimeRecords(t, storeA.Dir) != 1 {
		t.Fatalf("cross-worktree refusal mutated the ledger: status=%#v err=%v records=%d",
			status, statusErr, countRuntimeRecords(t, storeA.Dir))
	}
	if status.ActiveAttempt == nil || status.ActiveAttempt.Outcome != AttemptRunning {
		t.Fatalf("cross-worktree refusal closed the active attempt: %#v", status.ActiveAttempt)
	}

	// The ordinary continuation the refusal names: finishing from the exact
	// worktree Begin recorded works and measures the real change.
	finished, err := storeA.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: began.Revision, RequestID: "finish-a", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('9'), Diagnosis: "same-worktree finish measures the real change",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.Attempts) != 1 || finished.Attempts[0].ChangedLines != 1 {
		t.Fatalf("same-worktree finish attempts = %#v, want exactly one changed line", finished.Attempts)
	}
}

func canonicalRuntimeLedgerWorktreePath(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		t.Fatal(err)
	}
	return pathquote.Quote(filepath.Clean(canonical))
}

// TestRuntimeLedgerBeginRecordsTheCanonicalBeginWorktreeEndToEnd is the
// matching-worktree happy path: Begin and Finish both run from the same
// --cwd, so no refusal fires, and begin_worktree is populated end to end from
// the begin record through the replayed active attempt and the terminal one.
func TestRuntimeLedgerBeginRecordsTheCanonicalBeginWorktreeEndToEnd(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "worktree-identity")
	if err != nil {
		t.Fatal(err)
	}
	began, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-identity", WorkUnit: "worktree-identity",
		EvidenceGoal: "prove begin_worktree is recorded end to end", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if began.ActiveAttempt == nil || began.ActiveAttempt.BeginWorktree != store.Workspace {
		t.Fatalf("active attempt begin worktree = %#v, want %q", began.ActiveAttempt, store.Workspace)
	}

	finished, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: began.Revision, RequestID: "finish-identity", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('8'), Diagnosis: "same-worktree finish still records begin_worktree on the terminal attempt",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.Attempts) != 1 || finished.Attempts[0].BeginWorktree != store.Workspace {
		t.Fatalf("terminal attempt begin worktree = %#v, want %q", finished.Attempts, store.Workspace)
	}
}

// TestRuntimeLedgerLegacyBeginRecordReplaysWithoutWorktreeEnforcement is the
// single most important regression guard in this slice: a chain recorded
// before #2296 part 1's fix has no begin_worktree on its begin record, and
// replaying it must stay byte-identical -- no invented value, and Finish from
// a distinct linked worktree is NOT enforced, exactly as it behaved before
// this slice existed.
func TestRuntimeLedgerLegacyBeginRecordReplaysWithoutWorktreeEnforcement(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	worktree := filepath.Join(t.TempDir(), "legacy-sibling-worktree")
	runRuntimeLedgerGit(t, repo, "worktree", "add", "-q", "-b", "legacy-worktree-b", worktree)

	change := "legacy-worktree-replay"
	store, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := captureRuntimeCandidate(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "legacy-begin-worktree", WorkUnit: "legacy-work-unit",
		EvidenceGoal: "replay a chain recorded before this field existed", MaxAttempts: 2, MaxChangedLines: 20,
	}
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, Operation: runtimeOperationBegin,
		RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request),
		Begin: &runtimeBeginEvent{
			ObjectiveID: legacyRuntimeObjectiveID(store.Change, request.EvidenceGoal), WorkUnit: request.WorkUnit,
			EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Ordinal: 1, BeginCandidateIdentity: snapshot.Identity, BeginCandidateTree: snapshot.CandidateTree,
			// BeginWorktree deliberately left unset: this is the exact shape of
			// every chain recorded before #2296 part 1's fix.
		},
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		t.Fatalf("legacy-shaped record (no begin_worktree) rejected: %v", err)
	}
	payload, marshalErr := json.Marshal(record)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(payload), `"begin_worktree"`) {
		t.Fatalf("legacy-shaped record unexpectedly serializes the begin_worktree key: %s", payload)
	}
	revision, recordPayload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, recordPayload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveAttempt == nil || status.ActiveAttempt.BeginWorktree != "" {
		t.Fatalf("legacy replay invented a begin_worktree: %#v", status.ActiveAttempt)
	}

	// The regression guard itself: finishing from a DIFFERENT linked worktree
	// than any recorded value must NOT be enforced against a legacy record.
	otherStore, err := OpenRuntimeStore(context.Background(), worktree, change)
	if err != nil {
		t.Fatal(err)
	}
	finished, err := otherStore.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: "legacy-finish-worktree", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('7'), Diagnosis: "legacy chain finish from a distinct worktree is not enforced",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatalf("legacy chain finish from a distinct worktree was refused: %v", err)
	}
	if finished.ActiveAttempt != nil || len(finished.Attempts) != 1 || finished.Attempts[0].BeginWorktree != "" {
		t.Fatalf("legacy chain finish status = %#v, want a terminal attempt still carrying no begin_worktree", finished)
	}
}

// TestRuntimeLedgerRejectsGarbageBeginWorktreeShape is the light shape-
// validation guard requested alongside the additive field: a PRESENT
// begin_worktree is still an identity string, not free user input, so a
// multi-line value is rejected the same way every other recorded text field
// already is via validateRuntimeText.
func TestRuntimeLedgerRejectsGarbageBeginWorktreeShape(t *testing.T) {
	request := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "shape-check", WorkUnit: "work", EvidenceGoal: "goal",
		MaxAttempts: 1, MaxChangedLines: 20,
	}
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: "shape-check-change", Operation: runtimeOperationBegin,
		RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request),
		Begin: &runtimeBeginEvent{
			ObjectiveID: legacyRuntimeObjectiveID("shape-check-change", request.EvidenceGoal), WorkUnit: request.WorkUnit,
			EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Ordinal: 1, BeginCandidateIdentity: runtimeTestHash('a'), BeginCandidateTree: strings.Repeat("a", 40),
			BeginWorktree: "not\na-clean-path",
		},
	}
	if err := validateRuntimeRecordShape(record); err == nil {
		t.Fatal("validateRuntimeRecordShape admitted a multi-line begin_worktree value")
	}
}
