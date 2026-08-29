package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDAttemptAcquireRefusesUndeclaredEligibleUntrackedScopeBeforeToken(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, "undeclared-untracked")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = RunSDDAttempt([]string{
		"acquire", "--cwd", repo, "--change", "undeclared-untracked", "--request-id", "undeclared-acquire",
		"--work-unit", "untracked scope", "--evidence-goal", "require explicit intent", "--max-attempts", "2", "--max-changed-lines", "20",
	}, &output)
	if err == nil {
		t.Fatalf("undeclared eligible untracked scope issued authority: %s", output.String())
	}
	if !strings.Contains(err.Error(), "gentle-ai review status --next-transition") || !strings.Contains(err.Error(), "gentle-ai sdd-attempt acquire") {
		t.Fatalf("undeclared scope guidance = %q, want inventory then acquire commands", err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != "" || status.ActiveAttempt != nil || len(status.Attempts) != 0 {
		t.Fatalf("undeclared scope consumed authority: status=%#v err=%v", status, statusErr)
	}
}

func TestRunSDDAttemptAcquireSelectsInventoryValidatedPaths(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunSDDAttempt([]string{
		"acquire", "--cwd", repo, "--change", "selected-untracked", "--request-id", "selected-acquire",
		"--work-unit", "untracked scope", "--evidence-goal", "bind selected path", "--max-attempts", "2", "--max-changed-lines", "20",
		"--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt",
	}, &output); err != nil {
		t.Fatalf("inventory-validated selection was refused: %v", err)
	}
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, "selected-untracked")
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.ActiveAttempt == nil || len(status.ActiveAttempt.IntendedUntracked) != 1 || status.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
		t.Fatalf("selected acquire did not create one active attempt: status=%#v err=%v", status, err)
	}
}

func TestRunSDDAttemptAcquireTokenReusesSelectedUntrackedScope(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "other.txt", "other\n", 0o644)
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	base := []string{
		"acquire", "--cwd", repo, "--change", "tokenized-selected-untracked", "--request-id", "tokenized-selected-acquire",
		"--work-unit", "untracked scope", "--evidence-goal", "reuse selected path", "--max-attempts", "2", "--max-changed-lines", "20",
	}
	first, _ := runCompactSDDAttempt(t, append(append([]string{}, base...),
		"--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt"))
	if first.State != "proceed" || first.Token == "" {
		t.Fatalf("selected acquire = %#v", first)
	}

	retried, _ := runCompactSDDAttempt(t, append(append([]string{}, base...), "--token", first.Token))
	if retried != first {
		t.Fatalf("tokenized retry = %#v, want %#v", retried, first)
	}
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, "tokenized-selected-untracked")
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.ActiveAttempt == nil || len(status.ActiveAttempt.IntendedUntracked) != 1 || status.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
		t.Fatalf("tokenized retry changed selected provenance: status=%#v err=%v", status, err)
	}
	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	for _, args := range [][]string{
		append(append([]string{}, base...), "--token", cliAttemptHash('f')),
		append(append([]string{}, base...), "--token", first.Token, "--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "other.txt"),
	} {
		result, _ := runCompactSDDAttempt(t, args)
		if result.State != "blocked" || result.Reason != "invalid_continuation" {
			t.Fatalf("foreign token or changed selection = %#v", result)
		}
		if after := snapshotRuntimeAuthorityFiles(t, store.Dir); !reflect.DeepEqual(before, after) {
			t.Fatal("foreign token or changed selection mutated authority")
		}
	}
}

func TestRunSDDAttemptRescopeSuccessorInheritsSelectedUntrackedScope(t *testing.T) {
	for _, operation := range []string{"begin", "acquire"} {
		t.Run(operation, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			change := "rescoped-selected-" + operation
			writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
			writeUndeclaredWorkspaceFile(t, repo, "unrelated.txt", "unrelated\n", 0o644)
			_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			acquired, _ := runCompactSDDAttempt(t, []string{
				"acquire", "--cwd", repo, "--change", change, "--request-id", "selected-acquire-a",
				"--work-unit", "objective A", "--evidence-goal", "prove selected scope", "--max-attempts", "2", "--max-changed-lines", "20",
				"--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt",
			})
			failed, _ := runCompactSDDAttempt(t, append([]string{
				"settle", "--cwd", repo, "--change", change, "--token", acquired.Token, "--request-id", "selected-settle-a",
				"--outcome", "failed", "--evidence-revision", cliAttemptHash('a'),
			}, "--diagnosis", "zero-drift failure", "--harness-disposition", "reused", "--cleanup-evidence", "cleanup complete", "--process-evidence", "process scan clean"))
			if failed.State != "proceed" {
				t.Fatalf("zero-drift selected settlement = %#v", failed)
			}
			settled := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
			rescoped := runSDDAttemptStatus(t, []string{
				"rescope", "--cwd", repo, "--change", change, "--expected-revision", settled.Revision, "--request-id", "selected-rescope-b",
				"--work-unit", "objective B", "--evidence-goal", "prove inherited selected scope", "--max-attempts", "2", "--max-changed-lines", "20",
				"--reason", "maintainer narrowed the failed zero-drift objective", "--actor", "maintainer",
			})

			status := runSDDAttemptStatus(t, []string{
				"status", "--cwd", repo, "--change", change, "--work-unit", "objective B", "--evidence-goal", "prove inherited selected scope",
				"--max-attempts", "2", "--max-changed-lines", "20",
			})
			if status.BlockedReason != "" || status.BlockedExit != "" {
				t.Fatalf("declaration-free rescope-successor status = reason %q exit %q, want an executable continuation", status.BlockedReason, status.BlockedExit)
			}

			continuation := []string{
				operation, "--cwd", repo, "--change", change, "--request-id", "selected-" + operation + "-b",
				"--work-unit", "objective B", "--evidence-goal", "prove inherited selected scope", "--max-attempts", "2", "--max-changed-lines", "20",
			}
			if operation == "begin" {
				continuation = append(continuation, "--expected-revision", rescoped.Revision)
			}
			store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			divergent := append(append([]string{}, continuation...), "--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "unrelated.txt")
			if operation == "acquire" {
				result, _ := runCompactSDDAttempt(t, divergent)
				if result.State != "blocked" || result.Reason != string(sddstatus.CompactBlockMaintainerDecision) {
					t.Fatalf("divergent rescope-successor acquire = %#v", result)
				}
			} else if err := RunSDDAttempt(divergent, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "objective changed") {
				t.Fatalf("divergent rescope-successor begin = %v", err)
			}
			afterDivergence, err := store.Status()
			if err != nil || afterDivergence.Revision != rescoped.Revision || afterDivergence.ActiveAttempt != nil {
				t.Fatalf("divergent rescope-successor declaration mutated authority: status=%#v err=%v", afterDivergence, err)
			}
			if operation == "acquire" {
				result, _ := runCompactSDDAttempt(t, continuation)
				if result.State != "proceed" || result.Token == "" {
					t.Fatalf("declaration-free rescope-successor acquire = %#v", result)
				}
			}
			if operation == "begin" {
				started := runSDDAttemptStatus(t, continuation)
				if started.ActiveAttempt == nil || len(started.ActiveAttempt.IntendedUntracked) != 1 || started.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
					t.Fatalf("declaration-free rescope-successor begin = %#v", started)
				}
			}
			current, err := store.Status()
			if err != nil || current.ActiveAttempt == nil || len(current.ActiveAttempt.IntendedUntracked) != 1 || current.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
				t.Fatalf("rescope successor swept unrelated untracked paths: status=%#v err=%v", current, err)
			}
		})
	}
}

func TestRunSDDAttemptAcquireReplayPreservesInheritedRescopeSelection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "rescoped-replay"
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "unrelated.txt", "unrelated\n", 0o644)
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	acquired, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "selected-acquire-a",
		"--work-unit", "objective A", "--evidence-goal", "prove selected scope", "--max-attempts", "2", "--max-changed-lines", "20",
		"--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt",
	})
	runCompactSDDAttempt(t, append(compactSettleArgs(repo, change, acquired.Token, "selected-settle-a", "failed"), "--evidence-revision", cliAttemptHash('a')))
	settled := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	runSDDAttemptStatus(t, []string{
		"rescope", "--cwd", repo, "--change", change, "--expected-revision", settled.Revision, "--request-id", "selected-rescope-b",
		"--work-unit", "objective B", "--evidence-goal", "prove inherited selected scope", "--max-attempts", "2", "--max-changed-lines", "20",
		"--reason", "maintainer narrowed the failed zero-drift objective", "--actor", "maintainer",
	})
	args := []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", "selected-acquire-b",
		"--work-unit", "objective B", "--evidence-goal", "prove inherited selected scope", "--max-attempts", "2", "--max-changed-lines", "20",
	}
	first, _ := runCompactSDDAttempt(t, args)
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotRuntimeAuthorityFiles(t, store.Dir)
	replayed, _ := runCompactSDDAttempt(t, args)
	if first.State != "proceed" || first.Token == "" || replayed.State != first.State || replayed.Token != first.Token || !reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
		t.Fatalf("declaration-free replay = %#v, want %#v without authority mutation", replayed, first)
	}
	divergent, _ := runCompactSDDAttempt(t, append(append([]string{}, args...), "--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "unrelated.txt"))
	if divergent.State != "blocked" || divergent.Reason != string(sddstatus.CompactBlockInvalidContinuation) || !reflect.DeepEqual(before, snapshotRuntimeAuthorityFiles(t, store.Dir)) {
		t.Fatalf("divergent replay = %#v, want invalid_continuation without authority mutation", divergent)
	}
}

func TestRunSDDAttemptBeginSelectsInventoryValidatedPaths(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := RunSDDAttempt([]string{
		"begin", "--cwd", repo, "--change", "selected-begin", "--expected-revision", "", "--request-id", "selected-begin-request",
		"--work-unit", "untracked scope", "--evidence-goal", "bind selected path", "--max-attempts", "2", "--max-changed-lines", "20",
		"--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("inventory-validated begin was refused: %v", err)
	}
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, "selected-begin")
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.ActiveAttempt == nil || len(status.ActiveAttempt.IntendedUntracked) != 1 || status.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
		t.Fatalf("selected begin did not preserve provenance: status=%#v err=%v", status, err)
	}
}

func TestRunSDDAttemptSelectedUntrackedDoesNotSweepIgnoredOrUnrelatedPaths(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "selected.txt", "selected\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, unrelatedCredentialPath, unrelatedCredentialContents, 0o600)
	writeUndeclaredWorkspaceFile(t, repo, "ignored.txt", "ignored\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, ".gitignore", "ignored.txt\n", 0o644)
	inventory, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	credentialEligible := false
	for _, candidate := range inventory {
		if candidate == "ignored.txt" {
			t.Fatalf("inventory admitted ignored path: %v", inventory)
		}
		if candidate == unrelatedCredentialPath {
			credentialEligible = true
		}
	}
	if !credentialEligible {
		t.Fatalf("credential fixture is not eligible, so the test cannot prove it was not swept: %v", inventory)
	}
	var output bytes.Buffer
	if err := RunSDDAttempt([]string{
		"acquire", "--cwd", repo, "--change", "excluded-untracked", "--request-id", "excluded-acquire",
		"--work-unit", "untracked scope", "--evidence-goal", "exclude unrelated paths", "--max-attempts", "2", "--max-changed-lines", "20",
		"--untracked-scope", "select", "--expected-untracked-inventory", digest, "--intended-untracked", "selected.txt",
	}, &output); err != nil {
		t.Fatalf("selected-path acquire failed: %v", err)
	}
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, "excluded-untracked")
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.ActiveAttempt == nil || len(status.ActiveAttempt.IntendedUntracked) != 1 || status.ActiveAttempt.IntendedUntracked[0] != "selected.txt" {
		t.Fatalf("unselected paths entered attempt provenance: status=%#v err=%v", status, err)
	}

}

func TestRunSDDAttemptRejectsNestedRepositoryUntrackedScope(t *testing.T) {
	nestedRepo := initReviewCLIRepo(t)
	nested := filepath.Join(nestedRepo, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, nested, "init", "-q")
	err := RunSDDAttempt([]string{
		"acquire", "--cwd", nestedRepo, "--change", "nested-untracked", "--request-id", "nested-acquire",
		"--work-unit", "untracked scope", "--evidence-goal", "exclude nested repository", "--max-attempts", "2", "--max-changed-lines", "20",
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "another Git repository") {
		t.Fatalf("nested repository refusal = %v, want an untracked nested-repository refusal", err)
	}
}

// TestRunSDDAttemptSettleDeclaresUntrackedFilesBornDuringTheAttempt drives the
// whole of #3806 through the real CLI: an attempt that starts against a
// workspace with nothing eligible, creates a file while it runs, and then has
// to say what that file is before it can settle.
func TestRunSDDAttemptSettleDeclaresUntrackedFilesBornDuringTheAttempt(t *testing.T) {
	for _, test := range []struct {
		name         string
		scope        string
		selected     []string
		changedLines int
	}{
		{"select accounts the file", "select", []string{"born.txt"}, 3},
		{"exclude leaves it out on the record", "exclude", nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			change := "born-during-" + test.scope
			acquired, _ := runCompactSDDAttempt(t, []string{
				"acquire", "--cwd", repo, "--change", change, "--request-id", "born-acquire",
				"--work-unit", "born during", "--evidence-goal", "account what the attempt creates",
				"--max-attempts", "2", "--max-changed-lines", "20",
			})
			if acquired.State != "proceed" || acquired.Token == "" {
				t.Fatalf("clean acquire = %#v", acquired)
			}
			writeUndeclaredWorkspaceFile(t, repo, "born.txt", "one\ntwo\nthree\n", 0o644)

			settle := []string{
				"settle", "--cwd", repo, "--change", change, "--token", acquired.Token, "--request-id", "born-settle",
				"--outcome", "passed", "--evidence-revision", "sha256:" + strings.Repeat("a", 64),
				"--diagnosis", "work unit complete", "--harness-disposition", "invalidated",
				"--cleanup-evidence", "cleanup completed", "--process-evidence", "process scan clean",
			}
			blocked, _ := runCompactSDDAttempt(t, settle)
			if blocked.State != "blocked" || blocked.Reason != string(sddstatus.CompactBlockUndeclaredUntracked) {
				t.Fatalf("undeclared born-during settlement = %#v, want blocked/%s", blocked, sddstatus.CompactBlockUndeclaredUntracked)
			}
			if !strings.Contains(blocked.Exit, "born.txt") ||
				!strings.Contains(blocked.Exit, "--untracked-scope=select") ||
				!strings.Contains(blocked.Exit, "--untracked-scope=exclude") {
				t.Fatalf("refusal does not name the file and both exits: %#v", blocked)
			}

			_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			declared := append(append([]string{}, settle...), "--untracked-scope", test.scope, "--expected-untracked-inventory", digest)
			for _, path := range test.selected {
				declared = append(declared, "--intended-untracked", path)
			}
			settled, _ := runCompactSDDAttempt(t, declared)
			if settled.State != "complete" {
				t.Fatalf("declared settlement = %#v", settled)
			}
			store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			status, err := store.Status()
			if err != nil || len(status.Attempts) != 1 {
				t.Fatalf("declared settlement did not settle one attempt: status=%#v err=%v", status, err)
			}
			settledAttempt := status.Attempts[0]
			if settledAttempt.ChangedLines != test.changedLines {
				t.Fatalf("changed lines = %d, want %d: %#v", settledAttempt.ChangedLines, test.changedLines, settledAttempt)
			}
			if len(settledAttempt.IntendedUntracked) != len(test.selected) ||
				(len(test.selected) == 1 && settledAttempt.IntendedUntracked[0] != test.selected[0]) {
				t.Fatalf("settlement provenance = %#v, want %#v", settledAttempt.IntendedUntracked, test.selected)
			}
		})
	}
}
