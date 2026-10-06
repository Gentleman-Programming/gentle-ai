package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// #4808 made every slot of a lineage START froze to a runtime inherit that
// runtime when STATUS omits --agent, not only the targeted validator (#3805).
// These characterize the change for the compiled runtimes: a STATUS without
// --agent must answer exactly what `STATUS --agent <frozen>` answers, and a
// lineage that froze no runtime keeps the manual route.

// frozenRuntimeReview starts a lineage frozen to runtime (empty keeps the
// manual route). With refuterRequired, its one lens is captured with a severe
// inferential finding, so the transaction-wide refuter batch is next.
func frozenRuntimeReview(t *testing.T, runtime model.AgentID, refuterRequired bool) (string, string) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	started := startFacadeReviewForRuntime(t, repo, runtime)
	if !refuterRequired {
		return repo, started.LineageID
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalBehaviorActivated,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	return repo, started.LineageID
}

func frozenRuntimeStatusTransition(t *testing.T, repo, lineage string, extra ...string) (*ReviewNextTransition, []byte) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview(append([]string{"status", "--cwd", repo, "--lineage", lineage,
		"--contract", ReviewIntegrationContractV2, "--next-transition"}, extra...), &output); err != nil {
		t.Fatalf("STATUS %v: %v\n%s", extra, err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.NextTransition == nil {
		t.Fatalf("STATUS %v returned no next transition", extra)
	}
	encoded, err := json.Marshal(status.NextTransition)
	if err != nil {
		t.Fatal(err)
	}
	return status.NextTransition, encoded
}

func TestStatusWithoutAgentInheritsFrozenCompiledRuntime(t *testing.T) {
	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex} {
		for _, phase := range []struct {
			name            string
			refuterRequired bool
			reason          string
		}{
			{name: "lens", reason: "reviewer_results_required"},
			{name: "refuter", refuterRequired: true},
		} {
			t.Run(string(runtime)+"/"+phase.name, func(t *testing.T) {
				repo, lineage := frozenRuntimeReview(t, runtime, phase.refuterRequired)
				declared, declaredJSON := frozenRuntimeStatusTransition(t, repo, lineage, "--agent", string(runtime))
				_, inheritedJSON := frozenRuntimeStatusTransition(t, repo, lineage)
				if !bytes.Equal(inheritedJSON, declaredJSON) {
					t.Fatalf("STATUS without --agent diverged from --agent %s\nwithout: %s\nwith:    %s", runtime, inheritedJSON, declaredJSON)
				}
				if declared.Kind != reviewNextTransitionCollect || declared.Collect == nil || len(declared.Collect.Inputs) == 0 {
					t.Fatalf("frozen %s %s STATUS = %s, want a provider collection", runtime, phase.name, declaredJSON)
				}
				if phase.reason != "" && declared.ReasonCode != phase.reason {
					t.Fatalf("frozen %s %s reason = %q, want %q", runtime, phase.name, declared.ReasonCode, phase.reason)
				}
				for index, input := range declared.Collect.Inputs {
					arguments, err := reviewTransitionArgumentMap(input.Arguments)
					if err != nil {
						t.Fatal(err)
					}
					if arguments["agent"] != string(runtime) {
						t.Fatalf("input %d = %#v, want in-process capture bound to --agent %s", index, input, runtime)
					}
				}
			})
		}
	}
}

func TestStatusWithoutAgentAndWithoutFrozenRuntimeStaysManual(t *testing.T) {
	for _, phase := range []struct {
		name            string
		refuterRequired bool
	}{
		{name: "lens"},
		{name: "refuter", refuterRequired: true},
	} {
		t.Run(phase.name, func(t *testing.T) {
			repo, lineage := frozenRuntimeReview(t, "", phase.refuterRequired)
			_, manualJSON := frozenRuntimeStatusTransition(t, repo, lineage)
			_, declaredJSON := frozenRuntimeStatusTransition(t, repo, lineage, "--agent", string(model.AgentClaudeCode))
			if bytes.Equal(manualJSON, declaredJSON) {
				t.Fatalf("an unfrozen lineage inherited a runtime it never declared: %s", manualJSON)
			}
			for _, token := range []string{`"name":"agent"`, `"provider_task"`} {
				if strings.Contains(string(manualJSON), token) {
					t.Fatalf("unfrozen %s STATUS invented %s: %s", phase.name, token, manualJSON)
				}
			}
		})
	}
}
