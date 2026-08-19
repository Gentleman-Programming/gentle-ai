package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestIntendedUntrackedCollectionCarriesExecutableStatusSubmission(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)

	status := intendedUntrackedStatus(t, repo)
	input := status.NextTransition.Collect.Inputs[0]
	if input.Submission == nil || input.Submission.OperationToken != "status" {
		t.Fatalf("selection submission = %#v, want provider-owned STATUS", input.Submission)
	}
	if err := input.Submission.Validate(); err != nil {
		t.Fatalf("selection submission validation: %v", err)
	}
	assertSubmissionTransitionSchema(t, status)
}

func TestIntendedUntrackedSubmissionPreservesPlainWorkspaceSelectorsAndRuntime(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "tracked\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)

	initial := intendedUntrackedStatus(t, repo, "--agent", "opencode")
	if initial.Projection.Kind != reviewtransaction.TargetCurrentChanges || initial.Projection.Projection != reviewtransaction.ProjectionWorkspace {
		t.Fatalf("plain initial projection = %#v", initial.Projection)
	}
	input := initial.NextTransition.Collect.Inputs[0]
	if input.Submission == nil {
		t.Fatal("plain initial STATUS omitted its submission descriptor")
	}
	for _, token := range []string{
		"--contract=" + ReviewIntegrationContractV2,
		"--next-transition=true",
		"--agent=opencode",
		"--projection=workspace",
	} {
		if !containsString(input.Submission.ArgumentTokens, token) {
			t.Fatalf("plain submission tokens = %v, want %q", input.Submission.ArgumentTokens, token)
		}
	}

	selected, err := executeIntendedUntrackedSubmission(t, *input.Submission, repo, "select", "candidate.txt")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Projection.Kind != reviewtransaction.TargetCurrentChanges || selected.Projection.Projection != reviewtransaction.ProjectionWorkspace ||
		selected.TargetIdentity == initial.TargetIdentity || !containsString(selected.Projection.Paths, "candidate.txt") {
		t.Fatalf("selected plain projection = %#v, target=%s initial=%s", selected.Projection, selected.TargetIdentity, initial.TargetIdentity)
	}
	if selected.NextTransition == nil || selected.NextTransition.Execute == nil {
		t.Fatalf("selected plain transition = %#v", selected.NextTransition)
	}
	arguments, err := reviewTransitionArgumentMap(selected.NextTransition.Execute.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["agent"] != "opencode" || arguments["projection"] != string(reviewtransaction.ProjectionWorkspace) {
		t.Fatalf("selected plain START arguments = %#v", arguments)
	}
}

func TestIntendedUntrackedSubmissionPreservesWorkspaceOverlaySelectorsAndIdentity(t *testing.T) {
	repo := initReviewCLIRepo(t)
	baseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
	const runtimeAgent = "opencode"

	initial := intendedUntrackedStatus(t, repo, "--agent", runtimeAgent, "--base-ref", baseRef, "--workspace-overlay")
	if initial.Projection.Kind != reviewtransaction.TargetBaseWorkspaceOverlay || initial.Projection.Projection != reviewtransaction.ProjectionWorkspace {
		t.Fatalf("overlay initial projection = %#v", initial.Projection)
	}
	input := initial.NextTransition.Collect.Inputs[0]
	if input.Submission == nil {
		t.Fatal("overlay initial STATUS omitted its submission descriptor")
	}
	for _, token := range []string{
		"--agent=" + runtimeAgent,
		"--projection=workspace",
		"--base-ref=" + baseRef,
		"--workspace-overlay=true",
	} {
		if !containsString(input.Submission.ArgumentTokens, token) {
			t.Fatalf("overlay submission tokens = %v, want %q", input.Submission.ArgumentTokens, token)
		}
	}

	selected, err := executeIntendedUntrackedSubmission(t, *input.Submission, repo, "select", "candidate.txt")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Projection.Kind != reviewtransaction.TargetBaseWorkspaceOverlay || selected.Projection.Projection != reviewtransaction.ProjectionWorkspace ||
		selected.Projection.BaseTree != initial.Projection.BaseTree || selected.TargetIdentity == initial.TargetIdentity ||
		!containsString(selected.Projection.Paths, "candidate.txt") {
		t.Fatalf("selected overlay projection = %#v, target=%s initial=%s", selected.Projection, selected.TargetIdentity, initial.TargetIdentity)
	}
	if selected.NextTransition == nil || selected.NextTransition.Execute == nil || selected.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("selected overlay transition = %#v", selected.NextTransition)
	}
	arguments, err := reviewTransitionArgumentMap(selected.NextTransition.Execute.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["agent"] != runtimeAgent {
		t.Fatalf("selected overlay START arguments = %#v, want agent=%q", arguments, runtimeAgent)
	}
	for _, token := range []string{"--agent=" + runtimeAgent, "--projection=workspace", "--workspace-overlay=true"} {
		if !strings.Contains(selected.NextTransition.Execute.Command, token) {
			t.Fatalf("selected overlay command = %q, want %q", selected.NextTransition.Execute.Command, token)
		}
	}
	var question ReviewIntegrationConsentResult
	decodeStrictReviewJSON(t, executePrintedReview(t, repo, selected.NextTransition.Execute.Command), &question)
	invocation := ""
	for _, choice := range question.Choices {
		if strings.Contains(choice.Invocation, "--consent granted") {
			invocation = choice.Invocation
			break
		}
	}
	if invocation == "" {
		t.Fatalf("overlay consent question omitted granted invocation: %#v", question.Choices)
	}
	if !strings.Contains(invocation, "--agent "+runtimeAgent) {
		t.Fatalf("overlay granted START = %q, want agent=%q", invocation, runtimeAgent)
	}
	started := decodeNegotiatedReviewStart(t, executePrintedReview(t, repo, invocation))
	if started.TargetMode != reviewtransaction.TargetBaseWorkspaceOverlay || negotiatedStartTarget(started) != selected.TargetIdentity {
		t.Fatalf("printed overlay START = %#v, selected target=%s", started, selected.TargetIdentity)
	}
	staleCommand := strings.Replace(selected.NextTransition.Execute.Command, selected.TargetIdentity, initial.TargetIdentity, 1)
	words, err := SplitPrintedCommandWords(staleCommand)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReview(words[2:], &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "stale_target_identity") {
		t.Fatalf("stale overlay START = %v, want stale_target_identity", err)
	}
}

func TestIntendedUntrackedSelectionKeepsV1AndUnnegotiatedFlows(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "tracked\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)

	var statusOutput bytes.Buffer
	if err := RunReviewStatus([]string{"--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatalf("v1 intended-untracked STATUS: %v\n%s", err, statusOutput.String())
	}
	var legacy ReviewTargetStatusResult
	if err := json.Unmarshal(statusOutput.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Contract != ReviewIntegrationContractV1 || legacy.NextTransition == nil || legacy.NextTransition.Collect == nil || len(legacy.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("v1 intended-untracked STATUS = %#v", legacy)
	}
	if legacy.NextTransition.Collect.Inputs[0].Submission != nil {
		t.Fatalf("v1 intended-untracked STATUS carried a v2 submission: %#v", legacy.NextTransition.Collect.Inputs[0].Submission)
	}
	arguments, err := reviewTransitionArgumentMap(legacy.NextTransition.Collect.Inputs[0].Arguments)
	if err != nil {
		t.Fatal(err)
	}

	var startOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--lineage", "unnegotiated-intended", "--untracked-scope=select",
		"--expected-untracked-inventory=" + arguments["expected_untracked_inventory"], "--intended-untracked=candidate.txt",
	}, &startOutput); err != nil {
		t.Fatalf("unnegotiated intended-untracked START: %v\n%s", err, startOutput.String())
	}
}

func executeIntendedUntrackedSubmission(t *testing.T, descriptor ReviewTransitionSubmission, repo, scope string, paths ...string) (ReviewTargetStatusResult, error) {
	t.Helper()
	arguments := intendedUntrackedSubmissionArguments(t, descriptor, repo, scope, paths...)
	var output bytes.Buffer
	err := RunReview(arguments, &output)
	if err != nil {
		return ReviewTargetStatusResult{}, fmt.Errorf("%w: %s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status, nil
}

func intendedUntrackedSubmissionArguments(t *testing.T, descriptor ReviewTransitionSubmission, repo, scope string, paths ...string) []string {
	t.Helper()
	arguments := append([]string{descriptor.OperationToken}, descriptor.ArgumentTokens...)
	values := map[string][]string{"cwd": {repo}, "untracked_scope": {scope}, "intended_untracked": paths}
	for _, slot := range descriptor.Values {
		index := slot.SubstitutionLocation + 1
		placeholder := "{{" + slot.Slot + "}}"
		if slot.Repeated {
			replacements := make([]string, len(values[slot.Slot]))
			for valueIndex, value := range values[slot.Slot] {
				replacements[valueIndex] = strings.Replace(arguments[index], placeholder, value, 1)
			}
			arguments = append(arguments[:index], append(replacements, arguments[index+1:]...)...)
			continue
		}
		arguments[index] = strings.Replace(arguments[index], placeholder, values[slot.Slot][0], 1)
	}
	return arguments
}

func TestIntendedUntrackedPublishedSchemaMatchesAllEmittedLayouts(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
	base := intendedUntrackedStatus(t, repo)
	schema := compileWholeNativeStatusSchema(t, "status-v5.schema.json")
	digest := "sha256:" + strings.Repeat("a", 64)
	selectors := []struct {
		name     string
		selector reviewTransitionSelector
	}{
		{name: "current", selector: reviewTransitionSelector{Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace}},
		{name: "base diff", selector: reviewTransitionSelector{Kind: reviewtransaction.TargetBaseDiff, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "origin/main"}},
		{name: "base overlay by ref", selector: reviewTransitionSelector{Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionWorkspace, BaseRef: "origin/main", WorkspaceOverlay: true}},
		{name: "base overlay by tree", selector: reviewTransitionSelector{Kind: reviewtransaction.TargetBaseWorkspaceOverlay, Projection: reviewtransaction.ProjectionWorkspace, BaseTree: strings.Repeat("b", 40), WorkspaceOverlay: true}},
	}
	for _, selector := range selectors {
		for _, runtime := range []string{"", "claude-code", "codex", "opencode"} {
			for _, lineage := range []string{"", "layout-lineage"} {
				t.Run(selector.name+"/runtime="+runtime+"/lineage="+lineage, func(t *testing.T) {
					descriptor := reviewIntendedUntrackedSubmission(ReviewIntegrationContractV2, digest, reviewIntendedUntrackedSubmissionContext{
						Selector: selector.selector,
						Runtime:  runtime,
						Lineage:  lineage,
					})
					if descriptor == nil {
						t.Fatal("runtime refused a legitimately emitted intended-untracked layout")
					}
					if err := schema.Validate(intendedUntrackedStatusDocument(t, base, *descriptor)); err != nil {
						t.Fatalf("published schema rejected emitted layout: %v", err)
					}
				})
			}
		}
	}
}

func TestIntendedUntrackedPublishedSchemaRejectsMalformedDescriptorGrammar(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
	base := intendedUntrackedStatus(t, repo)
	schema := compileWholeNativeStatusSchema(t, "status-v5.schema.json")
	digest := "sha256:" + strings.Repeat("a", 64)
	valid := reviewIntendedUntrackedSubmission(ReviewIntegrationContractV2, digest, reviewIntendedUntrackedSubmissionContext{
		Selector: reviewTransitionSelector{Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace},
	})
	if valid == nil {
		t.Fatal("failed to construct valid intended-untracked descriptor")
	}
	for _, test := range []struct {
		name   string
		mutate func(*ReviewTransitionSubmission)
	}{
		{name: "unrelated selector token", mutate: func(descriptor *ReviewTransitionSubmission) {
			descriptor.ArgumentTokens[3] = "--unexpected-selector=true"
		}},
		{name: "unsupported runtime token", mutate: func(descriptor *ReviewTransitionSubmission) {
			descriptor.ArgumentTokens = append([]string{}, reviewIntendedUntrackedSubmission(ReviewIntegrationContractV2, digest, reviewIntendedUntrackedSubmissionContext{
				Selector: reviewTransitionSelector{Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace},
				Runtime:  "opencode",
			}).ArgumentTokens...)
			descriptor.ArgumentTokens[3] = "--agent=unknown-runtime"
		}},
		{name: "uppercase inventory digest", mutate: func(descriptor *ReviewTransitionSubmission) {
			descriptor.ArgumentTokens[5] = "--expected-untracked-inventory=sha256:" + strings.Repeat("A", 64)
		}},
		{name: "non-final placeholder tail", mutate: func(descriptor *ReviewTransitionSubmission) {
			descriptor.ArgumentTokens = append(descriptor.ArgumentTokens, "--unexpected=true")
		}},
		{name: "mismatched scope location", mutate: func(descriptor *ReviewTransitionSubmission) {
			descriptor.Values[1].SubstitutionLocation = 5
		}},
		{name: "mismatched repeated path location", mutate: func(descriptor *ReviewTransitionSubmission) {
			descriptor.Values[2].SubstitutionLocation = 5
		}},
		{name: "non-final repeated slot", mutate: func(descriptor *ReviewTransitionSubmission) {
			descriptor.Values = []ReviewTransitionSubmissionValue{descriptor.Values[2], descriptor.Values[0], descriptor.Values[1]}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := *valid
			candidate.ArgumentTokens = append([]string{}, valid.ArgumentTokens...)
			candidate.Values = append([]ReviewTransitionSubmissionValue{}, valid.Values...)
			test.mutate(&candidate)
			if err := schema.Validate(intendedUntrackedStatusDocument(t, base, candidate)); err == nil {
				t.Fatal("published schema accepted malformed intended-untracked descriptor")
			}
		})
	}
}

func intendedUntrackedStatusDocument(t *testing.T, base ReviewTargetStatusResult, descriptor ReviewTransitionSubmission) map[string]any {
	t.Helper()
	status := base
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("base intended-untracked status transition = %#v", status.NextTransition)
	}
	transition := *status.NextTransition
	collection := *transition.Collect
	inputs := append([]ReviewTransitionInput{}, collection.Inputs...)
	input := inputs[0]
	input.Submission = &descriptor
	inputs[0] = input
	collection.Inputs = inputs
	transition.Collect = &collection
	status.NextTransition = &transition
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	return document
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
	initial := intendedUntrackedStatus(t, repo)
	selected, err := executeIntendedUntrackedSubmission(t, *initial.NextTransition.Collect.Inputs[0].Submission, repo, "select", selectedPaths...)
	if err != nil {
		t.Fatal(err)
	}
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
	if initial.TargetIdentity == selected.TargetIdentity {
		t.Fatal("selected STATUS reused the pre-selection target identity")
	}
	staleCommand := strings.Replace(selected.NextTransition.Execute.Command, selected.TargetIdentity, initial.TargetIdentity, 1)
	words, err := SplitPrintedCommandWords(staleCommand)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReview(words[2:], &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "stale_target_identity") {
		t.Fatalf("pre-selection START target = %v, want stale_target_identity", err)
	}
}

func TestIntendedUntrackedSelectAndExcludeSubmissionsReturnExecutableStart(t *testing.T) {
	for _, scope := range []string{"select", "exclude"} {
		t.Run(scope, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "tracked.txt", "tracked\n", 0o644)
			writeUndeclaredWorkspaceFile(t, repo, "candidate.txt", "candidate\n", 0o644)
			initial := intendedUntrackedStatus(t, repo)
			paths := []string(nil)
			if scope == "select" {
				paths = []string{"candidate.txt"}
			}
			selected, err := executeIntendedUntrackedSubmission(t, *initial.NextTransition.Collect.Inputs[0].Submission, repo, scope, paths...)
			if err != nil {
				t.Fatal(err)
			}
			if selected.NextTransition == nil || selected.NextTransition.Kind != reviewNextTransitionExecute || selected.NextTransition.Execute.Operation != "review.start" {
				t.Fatalf("selected STATUS transition = %#v", selected.NextTransition)
			}
		})
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
			initial := intendedUntrackedStatus(t, repo)
			if test.remove {
				if err := os.Remove(filepath.Join(repo, "candidate.txt")); err != nil {
					t.Fatal(err)
				}
			} else {
				writeUndeclaredWorkspaceFile(t, repo, "added-after-status.txt", "added\n", 0o644)
			}
			_, err := executeIntendedUntrackedSubmission(t, *initial.NextTransition.Collect.Inputs[0].Submission, repo, "select", "candidate.txt")
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
