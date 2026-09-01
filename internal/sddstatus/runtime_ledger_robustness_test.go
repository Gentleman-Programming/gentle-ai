package sddstatus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// rewriteRuntimeHeadRecord republishes the ledger's HEAD record the way a
// different binary would have written it: the edited bytes are
// content-addressed into records/<sha256>.json and HEAD moves to them.
func rewriteRuntimeHeadRecord(t *testing.T, store RuntimeStore, edit func(payload []byte) []byte) string {
	t.Helper()
	headPath := filepath.Join(store.Dir, "HEAD")
	head, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.TrimSuffix(string(head), "\n")
	payload, err := os.ReadFile(filepath.Join(store.Dir, "records", strings.TrimPrefix(revision, "sha256:")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	edited := edit(payload)
	sum := sha256.Sum256(edited)
	next := "sha256:" + hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(store.Dir, "records", hex.EncodeToString(sum[:])+".json"), edited, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(headPath, []byte(next+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return next
}

func beginAndFailRuntimeObjective(t *testing.T, store RuntimeStore) RuntimeStatus {
	t.Helper()
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "begin-1", WorkUnit: "unit", EvidenceGoal: "prove the ledger tolerates its own future",
		MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('1'), Diagnosis: "bounded failure",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	return failed
}

// #2702: a record written by a newer binary with one additive field must stay
// readable; the sha256 revision already pins its bytes.
func TestRuntimeLedgerToleratesAdditiveUnknownRecordField(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "additive-field")
	beginAndFailRuntimeObjective(t, store)
	revision := rewriteRuntimeHeadRecord(t, store, func(payload []byte) []byte {
		return append(bytes.TrimSuffix(payload, []byte("}\n")), []byte(`,"future_field":"x"}`+"\n")...)
	})

	status, err := store.Status()
	if err != nil {
		t.Fatalf("status with additive field = %v", err)
	}
	if status.Revision != revision || !status.DecisionRequired || status.NextAction != RuntimeActionReset {
		t.Fatalf("status with additive field = %#v", status)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: revision, RequestID: "reset-1", Reason: "maintainer decision", Actor: "maintainer",
	})
	if err != nil || reset.Objective != nil {
		t.Fatalf("reset with additive field = %#v err=%v", reset, err)
	}
	acquired, err := store.Acquire(context.Background(), CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "acquire-1", WorkUnit: "unit", EvidenceGoal: "prove the ledger tolerates its own future",
		MaxAttempts: 1, MaxChangedLines: 20,
	}})
	if err != nil || acquired.State != CompactStateProceed {
		t.Fatalf("acquire with additive field = %#v err=%v", acquired, err)
	}
}

func TestRuntimeLedgerRefusesNewerRecordSchemaByName(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "newer-schema")
	beginAndFailRuntimeObjective(t, store)
	rewriteRuntimeHeadRecord(t, store, func(payload []byte) []byte {
		return bytes.Replace(payload, []byte(runtimeRecordSchema), []byte("gentle-ai.sdd-runtime-record/v2"), 1)
	})
	_, err := store.Status()
	if err == nil || !strings.Contains(err.Error(), `"schema"`) || !strings.Contains(err.Error(), "gentle-ai update") {
		t.Fatalf("newer schema status error = %v, want it to name the schema field and `gentle-ai update`", err)
	}
}

// #2612 / #3202: outside a Git repository the ledger has nowhere to live; the
// refusal must name the exit instead of leaking `git rev-parse`.
func TestOpenRuntimeStoreOutsideGitRepositoryNamesGitInit(t *testing.T) {
	_, err := OpenRuntimeStore(context.Background(), t.TempDir(), "no-repo")
	if err == nil || !strings.Contains(err.Error(), "git init") || !strings.Contains(err.Error(), "gentle-ai sdd-attempt") || strings.Contains(err.Error(), "rev-parse") {
		t.Fatalf("open outside Git = %v, want a refusal naming `git init` and the sdd-attempt rerun", err)
	}
}

// #3498: a tracked inventory past 8 MiB must not make acquire unavailable.
func TestCompactAcquireProceedsWithTrackedInventoryPastEightMiB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the long tracked paths this inventory needs exceed MAX_PATH on ordinary Windows test volumes")
	}
	repo := initRuntimeLedgerRepo(t)
	populateLargeTrackedInventory(t, repo)
	store := mustRuntimeStore(t, repo, "large-inventory")
	acquired, err := store.Acquire(context.Background(), CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "acquire-1", WorkUnit: "unit", EvidenceGoal: "prove a large inventory acquires",
		MaxAttempts: 1, MaxChangedLines: 20,
	}})
	if err != nil || acquired.State != CompactStateProceed {
		t.Fatalf("large inventory acquire = %#v err=%v", acquired, err)
	}
}

// populateLargeTrackedInventory commits enough long tracked paths that
// `git ls-files --cached -z` exceeds 8 MiB (12,800 paths of about 700 bytes).
func populateLargeTrackedInventory(t *testing.T, repo string) {
	t.Helper()
	blob := runRuntimeLedgerGit(t, repo, "hash-object", "-w", "--stdin")
	dir := strings.Repeat("d", 200) + "/" + strings.Repeat("e", 200) + "/" + strings.Repeat("f", 200)
	var index strings.Builder
	for i := 0; i < 12800; i++ {
		fmt.Fprintf(&index, "100644 %s 0\t%s/file-%05d-%s\n", blob, dir, i, strings.Repeat("g", 80))
	}
	command := exec.Command("git", "update-index", "--add", "--index-info")
	command.Dir = repo
	command.Stdin = strings.NewReader(index.String())
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git update-index --index-info: %v: %s", err, output)
	}
	runRuntimeLedgerGit(t, repo, "checkout-index", "-a")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "large inventory")
}

// #2504: a released LOCK must not keep advertising the exited owner's PID.
func TestRuntimeLedgerLockPayloadIsClearedAfterSettle(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "lock-release")
	acquired, err := store.Acquire(context.Background(), CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "acquire-1", WorkUnit: "unit", EvidenceGoal: "prove the lock is released clean",
		MaxAttempts: 1, MaxChangedLines: 20,
	}})
	if err != nil || acquired.State != CompactStateProceed {
		t.Fatalf("acquire = %#v err=%v", acquired, err)
	}
	appendRuntimeLedgerFile(t, repo, "settled\n")
	settled, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: acquired.Token, RequestID: "settle-1", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('2'), Diagnosis: "passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil || settled.State != CompactStateComplete {
		t.Fatalf("settle = %#v err=%v", settled, err)
	}
	payload, err := os.ReadFile(filepath.Join(store.Dir, "LOCK"))
	if err != nil || len(payload) != 0 {
		t.Fatalf("LOCK after settle = %q err=%v, want an empty released payload", payload, err)
	}
}
