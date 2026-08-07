package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func intendedUntrackedStatus(t *testing.T, repo string, args ...string) ReviewTargetStatusResult {
	var output bytes.Buffer
	if err := RunReviewStatus(append([]string{"--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo}, args...), &output); err != nil {
		t.Fatalf("review status %v: %v\n%s", args, err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}
func intendedUntrackedSelection(t *testing.T, status ReviewTargetStatusResult) (string, []string) {
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("selection transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "intended_untracked_selection" || input.Schema != reviewIntendedUntrackedSelectionSchema || input.CaptureOperation != "external.select_intended_untracked" {
		t.Fatalf("selection input = %#v", input)
	}
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err := json.Unmarshal([]byte(arguments["eligible_paths_json"]), &paths); err != nil {
		t.Fatal(err)
	}
	return arguments["expected_untracked_inventory"], paths
}
func assertNoUntrackedSelectionAuthority(t *testing.T, repo string) {
	if stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo); err != nil || len(stores) != 0 {
		t.Fatalf("incomplete untracked intent created authority: err=%v stores=%#v", err, stores)
	}
}
func TestIntendedUntrackedSelectionRequiresExplicitIntentBeforeAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate, with space.txt", "candidate\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, unrelatedCredentialPath, unrelatedCredentialContents, 0o600)
	digest, inventory := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	if !containsString(inventory, "candidate, with space.txt") || !containsString(inventory, unrelatedCredentialPath) || strings.Contains(strings.Join(inventory, "\n"), unrelatedCredentialMarker) {
		t.Fatalf("selection inventory = %q, want canonical paths but no candidate bytes", inventory)
	}
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "missing-intent"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--untracked-scope=exclude") || !strings.Contains(err.Error(), digest) {
		t.Fatalf("direct missing-intent refusal = %v, want exact continuation flags and digest", err)
	}
	assertNoUntrackedSelectionAuthority(t, repo)
}
func TestIntendedUntrackedSelectionScopesOnlyExplicitPaths(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "chosen, file.txt", "chosen\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, unrelatedCredentialPath, unrelatedCredentialContents, 0o600)
	writeUndeclaredWorkspaceFile(t, repo, "ignored.txt", "ignored\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, ".gitignore", "ignored.txt\n", 0o644)
	writeReviewStartCandidate(t, repo, "tracked.txt", "tracked change\n", 0o644)
	digest, inventory := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	if containsString(inventory, "ignored.txt") {
		t.Fatalf("ignored path entered selection inventory: %v", inventory)
	}
	selected := intendedUntrackedStatus(t, repo, "--untracked-scope=select", "--intended-untracked=chosen, file.txt", "--intended-untracked="+unrelatedCredentialPath, "--expected-untracked-inventory="+digest)
	if !reflect.DeepEqual(selected.Projection.Paths, []string{"chosen, file.txt", "tracked.txt", unrelatedCredentialPath}) {
		t.Fatalf("partial selected projection paths = %v", selected.Projection.Paths)
	}
	if selected.NextTransition == nil || selected.NextTransition.Kind != reviewNextTransitionExecute || selected.NextTransition.Execute == nil || selected.NextTransition.Execute.Operation != "review.start" || strings.Count(selected.NextTransition.Execute.Command, "--intended-untracked=") != 2 {
		t.Fatalf("selected transition = %#v, want executable START", selected.NextTransition)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "direct-parity", "--untracked-scope=select", "--intended-untracked=chosen, file.txt", "--intended-untracked=" + unrelatedCredentialPath, "--expected-untracked-inventory=" + digest}, &output); err != nil {
		t.Fatalf("direct selected start: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.TargetIdentity != selected.TargetIdentity {
		t.Fatalf("direct target = %s, negotiated target = %s", started.TargetIdentity, selected.TargetIdentity)
	}
}
func TestIntendedUntrackedSelectionFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		args func(string) []string
	}{
		{"missing mode", func(digest string) []string { return []string{"--expected-untracked-inventory=" + digest} }},
		{"missing digest", func(string) []string { return []string{"--untracked-scope=exclude"} }},
		{"stale digest", func(string) []string {
			return []string{"--untracked-scope=exclude", "--expected-untracked-inventory=sha256:" + strings.Repeat("a", 64)}
		}},
		{"duplicate path", func(digest string) []string {
			return []string{"--untracked-scope=select", "--intended-untracked=candidate.txt", "--intended-untracked=candidate.txt", "--expected-untracked-inventory=" + digest}
		}},
		{"duplicate mode", func(digest string) []string {
			return []string{"--untracked-scope=exclude", "--untracked-scope=exclude", "--expected-untracked-inventory=" + digest}
		}},
		{"duplicate digest", func(digest string) []string {
			return []string{"--untracked-scope=exclude", "--expected-untracked-inventory=" + digest, "--expected-untracked-inventory=" + digest}
		}},
		{"nonmember path", func(digest string) []string {
			return []string{"--untracked-scope=select", "--intended-untracked=missing.txt", "--expected-untracked-inventory=" + digest}
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
			digest, _ := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
			err := RunReviewFacadeStart(append([]string{"--cwd", repo, "--lineage", "fail-closed"}, tt.args(digest)...), &bytes.Buffer{})
			if err == nil {
				t.Fatal("incomplete untracked intent started review")
			}
			if err := RunReview(append([]string{"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo}, tt.args(digest)...), &bytes.Buffer{}); err == nil {
				t.Fatal("incomplete untracked intent reached negotiated status")
			}
			assertNoUntrackedSelectionAuthority(t, repo)
		})
	}
	if _, err := reviewTransitionArgumentMap([]ReviewTransitionArgument{{Name: "lineage", Value: "one"}, {Name: "lineage", Value: "two"}}, "review.finalize"); err == nil {
		t.Fatal("non-START transition accepted a repeated argument")
	}
}
