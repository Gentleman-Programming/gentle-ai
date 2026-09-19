package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

func TestOpenCodeV1StatusEmitsProviderOwnedLensTasks(t *testing.T) {
	reviewEnabledHome(t)
	stubOpenCodeV1ReviewRuntime(t)
	repo, _, _, record := newArtifactReview(t, true)

	status, raw := openCodeLensTaskStatus(t, repo, record.State.LineageID)
	if status.Schema != "gentle-ai.review-integration.status/v8" {
		t.Fatalf("STATUS schema = %q, want v8", status.Schema)
	}
	if status.NextTransition == nil || status.NextTransition.Collect == nil ||
		status.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("OpenCode STATUS did not collect reviewer results: %#v", status.NextTransition)
	}
	if len(status.NextTransition.Collect.Inputs) != len(record.State.SelectedLenses) {
		t.Fatalf("OpenCode STATUS inputs = %d, want %d", len(status.NextTransition.Collect.Inputs), len(record.State.SelectedLenses))
	}
	for order, input := range status.NextTransition.Collect.Inputs {
		arguments, err := reviewTransitionArgumentMap(input.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		selectedOrder, err := strconv.Atoi(arguments["order"])
		if err != nil {
			t.Fatalf("input %d order = %q: %v", order, arguments["order"], err)
		}
		if input.ArtifactSubject == nil {
			t.Fatalf("input %d omitted artifact subject", order)
		}
		binding, err := json.Marshal(reviewLensContextBinding{
			Lineage: arguments["lineage"], Target: arguments["target"], Lens: arguments["lens"], Order: selectedOrder,
			Revision: arguments["expected-revision"], RepositoryContext: arguments["repository-context"],
			SubjectHash: input.ArtifactSubject.SubjectHash,
		})
		if err != nil {
			t.Fatal(err)
		}
		want := &ReviewProviderTask{
			Agent: arguments["lens"], Role: reviewProviderRoleLens,
			Prompt: reviewLensContextBindingHeader + " " + string(binding),
		}
		if !reflect.DeepEqual(input.ProviderTask, want) {
			t.Fatalf("input %d provider task = %#v, want %#v", order, input.ProviderTask, want)
		}
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("provider-owned lens STATUS validation: %v", err)
	}
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v8.schema.json"), raw)
}

func TestOpenCodeV1StatusRejectsMutatedLensProviderTasks(t *testing.T) {
	reviewEnabledHome(t)
	stubOpenCodeV1ReviewRuntime(t)
	repo, _, _, record := newArtifactReview(t, false)
	status, _ := openCodeLensTaskStatus(t, repo, record.State.LineageID)
	input := status.NextTransition.Collect.Inputs[0]
	if input.ProviderTask == nil {
		t.Fatal("OpenCode STATUS omitted provider task")
	}

	mutatedPrompt := func(mutate func(map[string]any)) string {
		t.Helper()
		encoded, found := strings.CutPrefix(input.ProviderTask.Prompt, reviewLensContextBindingHeader+" ")
		if !found {
			t.Fatalf("provider task prompt = %q", input.ProviderTask.Prompt)
		}
		var binding map[string]any
		if err := json.Unmarshal([]byte(encoded), &binding); err != nil {
			t.Fatal(err)
		}
		mutate(binding)
		payload, err := json.Marshal(binding)
		if err != nil {
			t.Fatal(err)
		}
		return reviewLensContextBindingHeader + " " + string(payload)
	}

	for _, test := range []struct {
		name   string
		mutate func(*ReviewTransitionInput)
	}{
		{name: "agent", mutate: func(input *ReviewTransitionInput) { input.ProviderTask.Agent = "review-unselected" }},
		{name: "role", mutate: func(input *ReviewTransitionInput) { input.ProviderTask.Role = "refuter" }},
		{name: "lineage", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) { binding["lineage"] = "other-lineage" })
		}},
		{name: "target", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) {
				binding["target"] = differentOpenCodeTransportTestSHA(binding["target"].(string))
			})
		}},
		{name: "lens", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) { binding["lens"] = "review-unselected" })
		}},
		{name: "order", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) { binding["order"] = float64(3) })
		}},
		{name: "revision", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) {
				binding["revision"] = differentOpenCodeTransportTestSHA(binding["revision"].(string))
			})
		}},
		{name: "repository context", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) { binding["repository_context"] = "rctx2:forged" })
		}},
		{name: "subject hash", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) {
				binding["subject_hash"] = differentOpenCodeTransportTestSHA(binding["subject_hash"].(string))
			})
		}},
		{name: "unknown field", mutate: func(candidate *ReviewTransitionInput) {
			candidate.ProviderTask.Prompt = mutatedPrompt(func(binding map[string]any) { binding["extra"] = true })
		}},
		{name: "trailing JSON", mutate: func(candidate *ReviewTransitionInput) { candidate.ProviderTask.Prompt += " {}" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneReviewStatusForLensTaskTest(t, status)
			test.mutate(&candidate.NextTransition.Collect.Inputs[0])
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), "lens provider task") {
				t.Fatalf("mutated STATUS validation = %v, want lens provider task refusal", err)
			}
		})
	}
}

func TestLensProviderTasksAreOpenCodeV1Only(t *testing.T) {
	reviewEnabledHome(t)
	repo, _, _, record := newArtifactReview(t, false)
	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex} {
		t.Run(string(runtime), func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview([]string{"status", "--cwd", repo, "--lineage", record.State.LineageID,
				"--contract", ReviewIntegrationContractV2, "--agent", string(runtime), "--next-transition"}, &output); err != nil {
				t.Fatal(err)
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if err := status.Validate(); err != nil {
				t.Fatalf("%s STATUS validation: %v", runtime, err)
			}
			validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v8.schema.json"), output.Bytes())
			for _, input := range status.NextTransition.Collect.Inputs {
				if input.ProviderTask != nil {
					t.Fatalf("%s input gained OpenCode provider task: %#v", runtime, input)
				}
			}
		})
	}
}

func stubOpenCodeV1ReviewRuntime(t *testing.T) {
	t.Helper()
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	t.Setenv("GENTLE_AI_OPENCODE_RELAY_CONTRACT", "")
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
}

func openCodeLensTaskStatus(t *testing.T, repo, lineage string) (ReviewTargetStatusResult, []byte) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--lineage", lineage,
		"--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentOpenCode), "--next-transition"}, &output); err != nil {
		t.Fatalf("OpenCode STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status, output.Bytes()
}

func cloneReviewStatusForLensTaskTest(t *testing.T, status ReviewTargetStatusResult) ReviewTargetStatusResult {
	t.Helper()
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var clone ReviewTargetStatusResult
	decodeStrictReviewJSON(t, payload, &clone)
	return clone
}
