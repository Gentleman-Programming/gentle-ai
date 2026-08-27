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
