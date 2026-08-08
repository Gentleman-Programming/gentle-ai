package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func intendedUntrackedStatus(t *testing.T, repo string, args ...string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewStatus(append([]string{"--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo}, args...), &output); err != nil {
		t.Fatalf("review status %v: %v\n%s", args, err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}

func intendedUntrackedSelection(t *testing.T, status ReviewTargetStatusResult) (string, []string) {
	t.Helper()
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
	t.Helper()
	if stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo); err != nil || len(stores) != 0 {
		t.Fatalf("incomplete untracked intent created authority: err=%v stores=%#v", err, stores)
	}
}

func intendedUntrackedSelectArgs(digest string, paths ...string) []string {
	args := []string{"--untracked-scope=select", "--expected-untracked-inventory=" + digest}
	for _, path := range paths {
		args = append(args, "--intended-untracked="+path)
	}
	return args
}

func executePrintedReview(t *testing.T, repo, command string) []byte {
	t.Helper()
	words, err := SplitPrintedCommandWords(command)
	if err != nil || len(words) < 3 || words[0] != "gentle-ai" || words[1] != "review" || words[2] != "start" {
		t.Fatalf("printed START = %q: %v", command, err)
	}
	t.Chdir(repo)
	var output bytes.Buffer
	if err := RunReview(words[2:], &output); err != nil {
		t.Fatalf("execute printed START: %v\n%s", err, output.String())
	}
	return output.Bytes()
}

func TestIntendedUntrackedPreflightRequiresExplicitIntentBeforeAuthority(t *testing.T) {
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

	err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--lineage", "exclude-untracked-only", "--untracked-scope=exclude", "--expected-untracked-inventory=" + digest,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), reviewStartEmptyCandidateHint) {
		t.Fatalf("untracked-only exclusion = %v, want empty-candidate refusal", err)
	}
	assertNoUntrackedSelectionAuthority(t, repo)
}

func TestIntendedUntrackedSelectionUsesCanonicalPathsAndPrintedStart(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "docs/chosen, file.md", "# Chosen\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "docs/second file,with comma.md", "# Second\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, unrelatedCredentialPath, unrelatedCredentialContents, 0o600)
	writeUndeclaredWorkspaceFile(t, repo, "ignored.txt", "ignored\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, ".gitignore", "ignored.txt\n", 0o644)
	writeReviewStartCandidate(t, repo, "docs/tracked.md", "# Tracked\n", 0o644)

	digest, inventory := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	if containsString(inventory, "ignored.txt") || !containsString(inventory, unrelatedCredentialPath) {
		t.Fatalf("selection inventory = %v, want ignored excluded and sensitive path eligible", inventory)
	}
	sensitive := intendedUntrackedStatus(t, repo, intendedUntrackedSelectArgs(digest, unrelatedCredentialPath)...)
	if !containsString(sensitive.Projection.Paths, unrelatedCredentialPath) || !containsString(sensitive.Projection.IntendedUntracked, unrelatedCredentialPath) {
		t.Fatalf("explicit sensitive selection = %#v", sensitive.Projection)
	}

	selectedPaths := []string{"docs/chosen, file.md", "docs/second file,with comma.md"}
	selected := intendedUntrackedStatus(t, repo, intendedUntrackedSelectArgs(digest, selectedPaths...)...)
	if !reflect.DeepEqual(selected.Projection.Paths, []string{"docs/chosen, file.md", "docs/second file,with comma.md", "docs/tracked.md"}) {
		t.Fatalf("selected projection paths = %v", selected.Projection.Paths)
	}
	if selected.NextTransition == nil || selected.NextTransition.Kind != reviewNextTransitionExecute || selected.NextTransition.Execute == nil || selected.NextTransition.Execute.Operation != "review.start" || strings.Count(selected.NextTransition.Execute.Command, "--intended-untracked=") != len(selectedPaths) {
		t.Fatalf("selected transition = %#v, want executable START", selected.NextTransition)
	}
	started := decodeNegotiatedReviewStart(t, executePrintedReview(t, repo, selected.NextTransition.Execute.Command))
	if negotiatedStartTarget(started) != selected.TargetIdentity {
		t.Fatalf("printed START target = %s, negotiated target = %s", negotiatedStartTarget(started), selected.TargetIdentity)
	}
}

func TestIntendedUntrackedInventoryChangesFailClosedBeforeAuthority(t *testing.T) {
	for _, test := range []struct {
		name   string
		remove bool
	}{{"addition", false}, {"removal", true}} {
		t.Run(test.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
			digest, _ := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
			if test.remove {
				if err := os.Remove(filepath.Join(repo, "candidate.txt")); err != nil {
					t.Fatal(err)
				}
			} else {
				writeUndeclaredWorkspaceFile(t, repo, "added-after-status.txt", "added\n", 0o644)
			}
			err := RunReviewFacadeStart(append([]string{"--cwd", repo, "--lineage", "stale-inventory-" + test.name}, intendedUntrackedSelectArgs(digest, "candidate.txt")...), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "untracked inventory changed") {
				t.Fatalf("stale inventory START = %v", err)
			}
			assertNoUntrackedSelectionAuthority(t, repo)
		})
	}
}

func TestIntendedUntrackedInvalidIntentNeverCreatesAuthority(t *testing.T) {
	cases := [][]string{
		{"missing mode", "--expected-untracked-inventory=<digest>"},
		{"missing digest", "--untracked-scope=exclude"},
		{"exclude with selection", "--untracked-scope=exclude", "--expected-untracked-inventory=<digest>", "--intended-untracked=candidate.txt"},
		{"select without selection", "--untracked-scope=select", "--expected-untracked-inventory=<digest>"},
		{"duplicate selected path", "--untracked-scope=select", "--expected-untracked-inventory=<digest>", "--intended-untracked=candidate.txt", "--intended-untracked=candidate.txt"},
		{"nonmember path", "--untracked-scope=select", "--expected-untracked-inventory=<digest>", "--intended-untracked=missing.txt"},
		{"duplicate mode", "--untracked-scope=exclude", "--untracked-scope=exclude", "--expected-untracked-inventory=<digest>"},
	}
	for _, test := range cases {
		t.Run(test[0], func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
			digest, _ := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
			args := append([]string(nil), test[1:]...)
			for index := range args {
				args[index] = strings.ReplaceAll(args[index], "<digest>", digest)
			}
			if err := RunReviewFacadeStart(append([]string{"--cwd", repo, "--lineage", "invalid-intent"}, args...), &bytes.Buffer{}); err == nil {
				t.Fatal("invalid intent started review")
			}
			assertNoUntrackedSelectionAuthority(t, repo)
		})
	}
}

func TestIntendedUntrackedConsentFollowUpPreservesSelectedScope(t *testing.T) {
	repo := initReviewCLIRepo(t)
	path := "scripts/deploy candidate.sh"
	writeUndeclaredWorkspaceFile(t, repo, path, "#!/bin/sh\necho deploy\n", 0o755)
	digest, _ := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	status := intendedUntrackedStatus(t, repo, intendedUntrackedSelectArgs(digest, path)...)

	var question ReviewIntegrationConsentResult
	decodeStrictReviewJSON(t, executePrintedReview(t, repo, status.NextTransition.Execute.Command), &question)
	invocation := ""
	for _, choice := range question.Choices {
		if strings.Contains(choice.Invocation, "--consent granted") {
			invocation = choice.Invocation
			break
		}
	}
	if invocation == "" || !strings.Contains(invocation, "--intended-untracked") || !strings.Contains(invocation, "deploy candidate.sh") {
		t.Fatalf("consent follow-up omitted selected scope: %#v", question.Choices)
	}
	started := decodeNegotiatedReviewStart(t, executePrintedReview(t, repo, invocation))
	if negotiatedStartTarget(started) != status.TargetIdentity {
		t.Fatalf("consent START target = %s, selected status target = %s", negotiatedStartTarget(started), status.TargetIdentity)
	}
}
