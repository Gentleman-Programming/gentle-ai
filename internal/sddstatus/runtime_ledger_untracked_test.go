package sddstatus

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeIntendedUntrackedRejectsInvalidPaths(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "reject-selected")
	if err != nil {
		t.Fatal(err)
	}
	tooMany := make([]string, maximumRuntimeIntendedUntracked+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("path-%d.txt", index)
	}
	for _, test := range []struct {
		name  string
		paths []string
	}{
		{"absolute", []string{filepath.Join(repo, "outside.txt")}},
		{"parent traversal", []string{"../outside.txt"}},
		{"parent directory", []string{".."}},
		{"current directory", []string{"."}},
		{"non-clean separator", []string{"a//b.txt"}},
		{"non-clean current directory", []string{"./a.txt"}},
		{"duplicate", []string{"a.txt", "a.txt"}},
		{"too many", tooMany},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Begin(context.Background(), BeginAttemptRequest{
				RequestID: "reject-begin", WorkUnit: "reject", EvidenceGoal: "reject invalid selected paths",
				MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: test.paths,
			}); err == nil {
				t.Fatalf("accepted invalid selection %v", test.paths)
			}
			status, err := store.Status()
			if err != nil || status.Revision != "" || status.ActiveAttempt != nil || len(status.Attempts) != 0 {
				t.Fatalf("invalid selection mutated authority: status=%#v err=%v", status, err)
			}
		})
	}
}

func TestRuntimeSelectedUntrackedPopulationAccountsMixedCandidate(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRuntimeStore(context.Background(), repo, "selected-mixed")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "selected-mixed-begin", WorkUnit: "mixed candidate", EvidenceGoal: "account tracked and selected files",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "tracked change\n")
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("initial\nselected change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finished, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "selected-mixed-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "mixed candidate needs complete accounting",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.Attempts) != 1 || finished.Attempts[0].ChangedLines != 2 {
		t.Fatalf("mixed candidate changed lines = %#v, want tracked plus selected untracked lines", finished.Attempts)
	}
}

func TestRuntimeSelectedUntrackedCorrectionIsNotUnchanged(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRuntimeStore(context.Background(), repo, "selected-remediation")
	if err != nil {
		t.Fatal(err)
	}
	store.ReviewDisabled = true
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "selected-remediation-begin", WorkUnit: "selected correction", EvidenceGoal: "accept selected correction bytes",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("initial\nfailed change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "selected-remediation-failed", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "first selected candidate failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	correcting, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "selected-remediation-correct", WorkUnit: "selected correction", EvidenceGoal: "accept selected correction bytes",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "selected.txt"), []byte("initial\ncorrected change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: correcting.Revision, RequestID: "selected-remediation-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "selected correction changed the candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
		RemediatesEvidenceRevision: runtimeTestHash('b'),
	}); err != nil {
		t.Fatalf("selected correction was classified unchanged: %v", err)
	}
}

func TestRuntimeSelectedUntrackedPopulationPersistsAcrossBeginFinishReplayAndHandoff(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	worktree := filepath.Join(t.TempDir(), "selected-worktree")
	runRuntimeLedgerGit(t, repo, "worktree", "add", "-q", "-b", "selected-worktree", worktree)
	for _, root := range []string{repo, worktree} {
		if err := os.WriteFile(filepath.Join(root, "selected.txt"), []byte("initial\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	storeA, err := OpenRuntimeStore(context.Background(), repo, "selected-handoff")
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := OpenRuntimeStore(context.Background(), worktree, "selected-handoff")
	if err != nil {
		t.Fatal(err)
	}
	started, err := storeA.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "selected-handoff-begin", WorkUnit: "selected handoff", EvidenceGoal: "preserve selected path provenance",
		MaxAttempts: 2, MaxChangedLines: 20, IntendedUntracked: []string{"selected.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	handedOff, err := storeA.Handoff(context.Background(), HandoffAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "selected-handoff", DestinationWorktree: worktree,
	})
	if err != nil {
		t.Fatal(err)
	}
	if handedOff.ActiveAttempt == nil || len(handedOff.ActiveAttempt.IntendedUntracked) != 1 || handedOff.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
		t.Fatalf("handoff lost selected path provenance: %#v", handedOff.ActiveAttempt)
	}
	replayed, err := storeA.Status()
	if err != nil || replayed.ActiveAttempt == nil || len(replayed.ActiveAttempt.IntendedUntracked) != 1 || replayed.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
		t.Fatalf("replay lost selected path provenance: status=%#v err=%v", replayed, err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "selected.txt"), []byte("initial\nchanged in handoff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finished, err := storeB.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: handedOff.Revision, RequestID: "selected-handoff-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "handoff must account selected untracked bytes",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.Attempts) != 1 || finished.Attempts[0].ChangedLines != 1 || len(finished.Attempts[0].IntendedUntracked) != 1 || finished.Attempts[0].IntendedUntracked[0] != "selected.txt" {
		t.Fatalf("terminal selected handoff provenance = %#v", finished.Attempts)
	}
}

func TestRuntimeLegacyEmptyPopulationStillReplays(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "legacy-empty-selected")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := captureRuntimeCandidate(context.Background(), repo, []string{})
	if err != nil {
		t.Fatal(err)
	}
	request := legacyBeginAttemptRequest{
		RequestID: "legacy-empty-begin", WorkUnit: "legacy candidate", EvidenceGoal: "replay empty selected paths",
		MaxAttempts: 2, MaxChangedLines: 20,
	}
	// This fixture is pre-provenance JSON, not a current runtimeBeginEvent
	// serialization: its absent field must stay absent when replay validates it.
	payload := []byte(fmt.Sprintf(`{"schema":%q,"change":%q,"previous_revision":"","operation":%q,"request_id":%q,"request_digest":%q,"begin":{"objective_id":%q,"work_unit":%q,"evidence_goal":%q,"max_attempts":%d,"max_changed_lines":%d,"ordinal":1,"begin_candidate_identity":%q,"begin_candidate_tree":%q}}`+"\n",
		runtimeRecordSchema, store.Change, runtimeOperationBegin, request.RequestID,
		runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request),
		legacyRuntimeObjectiveID(store.Change, request.EvidenceGoal), request.WorkUnit, request.EvidenceGoal,
		request.MaxAttempts, request.MaxChangedLines, snapshot.Identity, snapshot.CandidateTree))
	if strings.Contains(string(payload), `"intended_untracked"`) {
		t.Fatalf("legacy fixture unexpectedly contains intended_untracked: %s", payload)
	}
	sum := sha256.Sum256(payload)
	revision := fmt.Sprintf("sha256:%x", sum)
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.loadRecord(revision)
	if err != nil || legacy.Begin == nil || legacy.Begin.IntendedUntracked != nil {
		t.Fatalf("legacy field presence after decode = %#v, err=%v", legacy.Begin, err)
	}
	status, err := store.Status()
	if err != nil || status.ActiveAttempt == nil || len(status.ActiveAttempt.IntendedUntracked) != 0 {
		t.Fatalf("legacy empty selected population did not replay: status=%#v err=%v", status, err)
	}
	modernStore, err := OpenRuntimeStore(context.Background(), repo, "modern-empty-selected")
	if err != nil {
		t.Fatal(err)
	}
	modern, err := modernStore.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "modern-empty-begin", WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
		MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	modernRecord, err := modernStore.loadRecord(modern.Revision)
	if err != nil || modernRecord.Begin == nil || modernRecord.Begin.IntendedUntracked == nil || len(*modernRecord.Begin.IntendedUntracked) != 0 {
		t.Fatalf("modern explicit-empty field presence after decode = %#v, err=%v", modernRecord.Begin, err)
	}
	beforeRecords := countRuntimeRecords(t, store.Dir)
	synced := false
	originalSync := runtimeSyncDirectory
	runtimeSyncDirectory = func(path string) error {
		if path == store.Dir {
			synced = true
		}
		return originalSync(path)
	}
	t.Cleanup(func() { runtimeSyncDirectory = originalSync })
	retry := CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: request.RequestID, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
		MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
	}, Token: revision}
	result, err := store.Acquire(context.Background(), retry)
	status, statusErr := store.Status()
	if err != nil || statusErr != nil || !synced || result.State != CompactStateProceed || result.Token != revision || status.Revision != revision || status.ActiveAttempt == nil || countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("legacy tokenized replay = %#v, status=%#v err=%v/%v records=%d", result, status, err, statusErr, countRuntimeRecords(t, store.Dir))
	}
	foreign := retry
	foreign.Token = runtimeTestHash('f')
	divergent := retry
	divergent.IntendedUntracked = []string{"other.txt"}
	empty := retry
	empty.IntendedUntracked = []string{}
	for _, rejected := range []CompactAcquireRequest{foreign, divergent, empty} {
		result, err := store.Acquire(context.Background(), rejected)
		if err != nil || result.State != CompactStateBlocked || result.Reason != CompactBlockInvalidContinuation || countRuntimeRecords(t, store.Dir) != beforeRecords {
			t.Fatalf("legacy rejected continuation = %#v, err=%v records=%d", result, err, countRuntimeRecords(t, store.Dir))
		}
	}
}
