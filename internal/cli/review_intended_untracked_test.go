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
	reviewEnabledHome(t)
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

func TestNegotiatedStatusUnbornUntrackedRequiresSelectionBeforeSnapshot(t *testing.T) {
	reviewModeHome(t)
	repo := initUnbornReviewCLIRepo(t)
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "global"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	const path = "scripts/unborn-candidate.sh"
	writeUndeclaredWorkspaceFile(t, repo, path, "#!/bin/sh\necho candidate\n", 0o755)

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "opencode", "--next-transition",
	}, &output); err != nil {
		t.Fatalf("unborn negotiated STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	digest, inventory := intendedUntrackedSelection(t, status)
	if digest == "" || !reflect.DeepEqual(inventory, []string{path}) {
		t.Fatalf("selection inventory = digest %q paths %v, want only %q", digest, inventory, path)
	}
	if status.Authority != nil {
		t.Fatalf("selection STATUS created authority: %#v", status.Authority)
	}
	assertNoUntrackedSelectionAuthority(t, repo)
}

func TestIntendedUntrackedSelectionSubmissionExecutesSelectedStatusThenPrintedStart(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "docs/chosen, file.md", "# Chosen\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "docs/second file,with comma.md", "# Second\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, unrelatedCredentialPath, unrelatedCredentialContents, 0o600)
	writeUndeclaredWorkspaceFile(t, repo, "ignored.txt", "ignored\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, ".gitignore", "ignored.txt\n", 0o644)
	writeReviewStartCandidate(t, repo, "docs/tracked.md", "# Tracked\n", 0o644)

	status := intendedUntrackedStatus(t, repo, "--agent", "pi")
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("selection transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "intended_untracked_selection" || input.Schema != reviewIntendedUntrackedSelectionSchema || input.CaptureOperation != "external.select_intended_untracked" {
		t.Fatalf("selection input = %#v", input)
	}
	if input.Submission == nil {
		t.Fatal("initial intended-untracked selection lacks its provider-owned selected-STATUS submission")
	}
	submission := input.Submission
	if submission.OperationToken != "status" || submission.Value == nil || submission.Value.Schema != reviewIntendedUntrackedSelectionSchema || submission.Value.SubstitutionLocation != 4 || submission.ArgumentTokens[2] != "--agent=pi" {
		t.Fatalf("selection submission = %#v", submission)
	}
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	answer, _ := json.Marshal(reviewIntendedUntrackedSelection{Schema: reviewIntendedUntrackedSelectionSchema, UntrackedScope: "select", ExpectedInventory: arguments["expected_untracked_inventory"], Intended: []string{"docs/chosen, file.md"}})
	tokens := append([]string{submission.OperationToken, "--cwd", repo}, submission.ArgumentTokens...)
	for index := range tokens {
		tokens[index] = strings.ReplaceAll(tokens[index], reviewSubmissionValuePlaceholder, string(answer))
	}
	var output bytes.Buffer
	if err := RunReview(tokens, &output); err != nil {
		t.Fatal(err)
	}
	var selected ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &selected)
	if selected.Schema != ReviewIntegrationStatusSchemaV6 || selected.NextTransition == nil || selected.NextTransition.Execute == nil || selected.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("selected STATUS = %#v", selected)
	}
	started := decodeNegotiatedReviewStart(t, executePrintedReview(t, repo, selected.NextTransition.Execute.Command))
	if started.State != reviewtransaction.StateApproved || started.Action != "closed" {
		t.Fatalf("printed START = %#v", started)
	}
}

func TestIntendedUntrackedSelectionUsesCanonicalPathsAndPrintedStart(t *testing.T) {
	reviewEnabledHome(t)
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
	if started.State != reviewtransaction.StateApproved || started.Action != "closed" {
		t.Fatalf("printed zero-lens START = %#v", started)
	}
}

func TestIntendedUntrackedInventoryChangesFailClosedBeforeAuthority(t *testing.T) {
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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

func TestIntendedUntrackedSelectWithoutPathsNamesSelectedPathContinuation(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
	digest, _ := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--lineage", "select-without-path", "--untracked-scope=select", "--expected-untracked-inventory=" + digest,
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("selection without paths started review")
	}
	for _, want := range []string{
		"--untracked-scope=select",
		"--intended-untracked=<repo-relative-path>",
		"--expected-untracked-inventory=" + digest,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("select-without-path guidance = %q, missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "--untracked-scope=exclude") {
		t.Fatalf("select-without-path guidance changed the selected mode: %q", err)
	}
	assertNoUntrackedSelectionAuthority(t, repo)
}

func TestIntendedUntrackedConsentFollowUpPreservesSelectedScope(t *testing.T) {
	reviewEnabledHome(t)
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

func TestConsentFollowUpPrintedPathFlagsRoundTripWindowsNativePaths(t *testing.T) {
	for _, test := range []struct {
		name, cwd, policy, trace string
	}{
		{name: "cwd", cwd: `C:\Users\reviewer\gentle ai`},
		{name: "policy", cwd: "/repo", policy: `C:\Users\reviewer\policy files\review policy.json`},
		{name: "trace", cwd: "/repo", trace: `C:\Users\reviewer\traces\operation trace.json`},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := reviewConsentFollowUpBase(
				test.cwd, "sha256:target", reviewtransaction.ProjectionWorkspace,
				"windows-path-roundtrip", "", test.policy, "reliability", test.trace, false, false,
				ReviewIntegrationContractV2, "", "", reviewIntendedUntrackedScope{},
			)

			words, err := SplitPrintedCommandWords(command)
			if err != nil {
				t.Fatalf("split printed consent follow-up: %v", err)
			}
			want := []string{
				"gentle-ai", "review", "start",
				"--contract", ReviewIntegrationContractV2,
				"--cwd", test.cwd,
				"--target", "sha256:target",
				"--projection", string(reviewtransaction.ProjectionWorkspace),
				"--lineage", "windows-path-roundtrip",
			}
			if test.policy != "" {
				want = append(want, "--policy", test.policy)
			}
			if test.trace != "" {
				want = append(want, "--trace", test.trace)
			}
			if !reflect.DeepEqual(words, want) {
				t.Fatalf("printed consent follow-up words = %#v, want %#v\ncommand: %q", words, want, command)
			}
		})
	}
}

// Issue #2895: intended-untracked refusals named `gentle-ai review status
// --next-transition`, which the parser refuses without a negotiated contract
// and runtime identity. The named continuation is extracted and executed.
func TestIntendedUntrackedRefusalsNameARunnableStatusInvocation(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
	digest, _ := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	undeclared := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "runnable-inventory-undeclared"}, &bytes.Buffer{})
	writeUndeclaredWorkspaceFile(t, repo, "added-after-status.txt", "added\n", 0o644)
	stale := RunReviewFacadeStart(append([]string{"--cwd", repo, "--lineage", "runnable-inventory-stale"}, intendedUntrackedSelectArgs(digest, "candidate.txt")...), &bytes.Buffer{})
	for name, err := range map[string]error{"undeclared selection": undeclared, "stale inventory": stale} {
		if err == nil || !strings.Contains(err.Error(), "--contract "+ReviewIntegrationContractV2) {
			t.Fatalf("%s START = %v, want a refusal naming the negotiated STATUS form", name, err)
		}
		start := strings.Index(err.Error(), "`gentle-ai review status")
		rest := err.Error()[start+1:]
		tokens := strings.Fields(rest[:strings.IndexByte(rest, '`')])[2:]
		for index, token := range tokens {
			tokens[index] = strings.NewReplacer("<repo>", repo, "<runtime>", "claude-code").Replace(token)
		}
		var output bytes.Buffer
		if runErr := RunReview(tokens, &output); runErr != nil {
			t.Fatalf("%s refusal named `gentle-ai review %s`, which the parser refuses: %v\n%s", name, strings.Join(tokens, " "), runErr, output.String())
		}
	}
	assertNoUntrackedSelectionAuthority(t, repo)
}
