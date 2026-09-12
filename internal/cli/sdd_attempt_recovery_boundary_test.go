package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func historicalRecoverySuccessor(t *testing.T, landing string) (string, sddstatus.RuntimeStore, sddstatus.RuntimeStatus, []string) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	const change = "historical-successor"
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	acquired, _ := runCompactSDDAttempt(t, append(compactAcquireArgs(repo, change, "historical-acquire", 2),
		"--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt"))
	if acquired.State != "proceed" {
		t.Fatalf("initial acquire = %#v", acquired)
	}
	failed, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, acquired.Token, "historical-failure", "failed"))
	if failed.State != "proceed" {
		t.Fatalf("failed settle = %#v", failed)
	}
	settled := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	runReviewCLIGit(t, repo, "add", "selected.txt")
	if landing == "commit" {
		runReviewCLIGit(t, repo, "commit", "-qm", "land selected bytes")
	} else if landing == "commit-a" {
		runReviewCLIGit(t, repo, "commit", "-qam", "land selected bytes")
	}
	scope := []string{"--cwd", repo, "--change", change, "--work-unit", "narrower verification",
		"--evidence-goal", "verify selected bytes", "--max-attempts", "2", "--max-changed-lines", "10"}
	rescoped := runSDDAttemptStatus(t, append([]string{
		"rescope", "--expected-revision", settled.Revision, "--request-id", "historical-rescope",
		"--reason", "narrow zero-drift verification", "--actor", "fixture maintainer",
	}, scope...))
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	return repo, store, rescoped, scope
}

func TestRuntimeSuccessorAcquireReconcilesHistoricalUntracked(t *testing.T) {
	for _, landing := range []string{"staged", "commit", "commit-a"} {
		for _, operation := range []string{"acquire", "begin"} {
			t.Run(landing+"/"+operation, func(t *testing.T) {
				_, store, rescoped, scope := historicalRecoverySuccessor(t, landing)
				before := snapshotRuntimeAuthorityFiles(t, store.Dir)
				admission := runSDDAttemptStatus(t, append([]string{"status"}, scope...))
				if admission.BlockedReason != "" || !reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
					t.Fatalf("read-only successor admission = %#v", admission)
				}
				args := append([]string{operation, "--request-id", "historical-successor"}, scope...)
				if operation == "acquire" {
					successor, _ := runCompactSDDAttempt(t, args)
					if successor.State != "proceed" || successor.Token == "" {
						t.Fatalf("historical successor acquire = %#v", successor)
					}
				} else {
					runSDDAttemptStatus(t, append(args, "--expected-revision", rescoped.Revision))
				}
				current, err := store.Status()
				if err != nil || current.ActiveAttempt == nil || current.ActiveAttempt.BeginCandidateTree != rescoped.Objective.InitialCandidateTree ||
					current.LifetimeAttempts != 2 || current.CumulativeAttempts != 2 || current.LifetimeChangedLines != 0 {
					t.Fatalf("successor candidate/accounting = %#v, err=%v", current, err)
				}
				after := snapshotRuntimeAuthorityFiles(t, store.Dir)
				for name, payload := range before {
					if name != "HEAD" && name != "LOCK" && payload != after[name] {
						t.Fatalf("historical authority %s changed", name)
					}
				}
				entries, err := os.ReadDir(filepath.Join(store.Dir, "records"))
				if err != nil || len(entries) != 4 {
					t.Fatalf("records = %d, err=%v; want begin/finish/rescope/begin", len(entries), err)
				}
				payload, err := json.Marshal(after)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("authority bytes after successor: %s", payload)
			})
		}
	}
}

func TestRuntimeReplayRetainsLaterDrift(t *testing.T) {
	repo, store, _, scope := historicalRecoverySuccessor(t, "commit")
	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fresh := append([]string{"acquire", "--request-id", "fresh-invalid"}, scope...)
	fresh = append(fresh, "--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt")
	if err := RunSDDAttempt(fresh, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "selected.txt") {
		t.Fatalf("fresh tracked selection must refuse: %v", err)
	}
	if !reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
		t.Fatal("fresh-invalid selection changed authority")
	}
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\nlater drift\n", 0o644)
	admission := runSDDAttemptStatus(t, append([]string{"status"}, scope...))
	blocked, _ := runCompactSDDAttempt(t, append([]string{"acquire", "--request-id", "later-drift"}, scope...))
	if admission.BlockedReason != sddstatus.CompactBlockMaintainerDecision || blocked.Reason != "maintainer_decision" ||
		blocked.State != "blocked" || !reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
		t.Fatalf("later drift admission=%#v acquire=%#v", admission, blocked)
	}
}

func TestRuntimeLostCommittedReplyReplaysExactEvidence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "lost-settle-reply"
	acquired, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "lost-acquire", 2))
	// Real read-only invocation evidence, outside the source candidate: no-diff
	// execution is charged, and this digest does not claim source acceptance.
	evidence := []byte(fmt.Sprintf("command=git -C %q rev-parse HEAD\nexit=0\nstdout=%s\n", repo, runReviewCLIGit(t, repo, "rev-parse", "HEAD")))
	artifact := filepath.Join(t.TempDir(), "invocation.txt")
	if err := os.WriteFile(artifact, evidence, 0o600); err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(evidence))
	args := compactSettleArgsWithEvidence(repo, change, acquired.Token, "lost-settle", "failed", revision)
	lost := errors.New("fixture dropped committed reply")
	if err := RunSDDAttempt(args, reviewEmitFailureWriter{err: lost}); !errors.Is(err, lost) {
		t.Fatalf("lost reply = %v", err)
	}
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	// Later workspace edits must not turn receipt replay into another capture.
	writeUndeclaredWorkspaceFile(t, repo, "tracked.txt", "later workspace\n", 0o644)
	replayed, _ := runCompactSDDAttempt(t, args)
	status, err := store.Status()
	if err != nil || replayed.State != "proceed" || len(status.Attempts) != 1 || status.ActiveAttempt != nil || status.Complete ||
		status.LifetimeAttempts != 1 || status.CumulativeAttempts != 1 || status.LifetimeChangedLines != 0 || status.Attempts[0].EvidenceRevision != revision {
		t.Fatalf("committed failure replay=%#v status=%#v err=%v", replayed, status, err)
	}
	conflict, _ := runCompactSDDAttempt(t, compactSettleArgsWithEvidence(repo, change, acquired.Token, "lost-settle", "failed", cliAttemptHash('d')))
	if conflict.Reason != "invalid_continuation" || !reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
		t.Fatalf("conflicting replay changed authority: %#v", conflict)
	}
	readback, err := os.ReadFile(artifact)
	if err != nil || !bytes.Equal(readback, evidence) {
		t.Fatalf("invocation evidence changed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(store.Dir, "records"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("record count=%d err=%v", len(entries), err)
	}
	payload, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("invocation artifact=%s revision=%s bytes=%q authority=%s", artifact, revision, evidence, payload)
}

func TestRuntimeUncertainEffectRefusesUnsafeReplay(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "uncertain-effect"
	acquired, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "uncertain-acquire", 2))
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	// Retained observation, not a simulated successful external operation.
	observation := "external acknowledgement unavailable; outcome unknown"
	args := []string{"settle", "--cwd", repo, "--change", change, "--token", acquired.Token,
		"--request-id", "uncertain-settle", "--outcome", "interrupted", "--diagnosis", observation,
		"--harness-disposition", "reused", "--process-evidence", "no local child was launched"}
	if err := RunSDDAttempt(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "cleanup-evidence") {
		t.Fatalf("missing cleanup facts must refuse settlement: %v", err)
	}
	blocked, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "unsafe-reexecution", 2))
	if blocked.State != "blocked" || blocked.Reason != "active_attempt" || blocked.Token != acquired.Token ||
		!reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
		t.Fatalf("uncertain execution was replayed or settled: %#v", blocked)
	}
	settled, _ := runCompactSDDAttempt(t, append(args, "--cleanup-evidence", "no local process to clean; external outcome still unknown"))
	status, err := store.Status()
	if err != nil || settled.State != "proceed" || status.ActiveAttempt != nil || status.Complete || len(status.Attempts) != 1 ||
		status.Attempts[0].Outcome != sddstatus.AttemptInterrupted || status.Attempts[0].EvidenceRevision != "" ||
		status.Attempts[0].Diagnosis != observation || status.LifetimeAttempts != 1 || status.CumulativeAttempts != 1 || status.LifetimeChangedLines != 0 {
		t.Fatalf("truthful unknown settlement=%#v status=%#v err=%v", settled, status, err)
	}
}
