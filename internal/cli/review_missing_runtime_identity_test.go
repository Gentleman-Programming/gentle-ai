package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// The documented negotiated lifecycle route in
// internal/assets/claude/commands/gentle-sdd-apply.md and
// internal/assets/opencode/commands/sdd-apply.md declares no runtime
// identity. A read-only STATUS creates no authority, tier, budget, or
// collection state, so it has nothing to fail closed over: an undeclared
// identity is the manual/non-agent compatibility path that
// runReviewFacadeStart already documents, not an unsupported transport.
func TestNegotiatedStatusWithoutRuntimeIdentityIsAnswered(t *testing.T) {
	repo := initReviewCLIRepo(t)

	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "documented route", args: []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}},
		{name: "without transition", args: []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(test.args, &output)
			if err == nil {
				return
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Code == reviewImmutableTransportUnsupportedCode {
				t.Fatalf("undeclared runtime identity refused a read-only STATUS: %#v", failure)
			}
			t.Fatalf("negotiated STATUS failed: %v %#v", err, failure)
		})
	}
}

// A candidate-bearing repository must complete the documented route: STATUS
// answers, and the exact provider-returned START it names must execute
// rather than refuse the same undeclared identity one step later.
func TestNegotiatedRouteWithoutRuntimeIdentityReachesStart(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n", 0o644)

	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("negotiated STATUS failed: %v %s", err, statusOutput.String())
	}

	var status struct {
		NextTransition *struct {
			Execute *struct {
				Operation string `json:"operation"`
				Arguments []struct {
					Name  string `json:"name"`
					Token string `json:"token"`
				} `json:"arguments"`
			} `json:"execute"`
		} `json:"next_transition"`
	}
	if err := json.Unmarshal(statusOutput.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.NextTransition == nil || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("STATUS named no executable START: %s", statusOutput.String())
	}

	startArgs := []string{"start", "--cwd", repo}
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Name == "agent" {
			t.Fatalf("STATUS invented a runtime identity the caller never declared: %s", argument.Token)
		}
		startArgs = append(startArgs, argument.Token)
	}

	var startOutput bytes.Buffer
	if err := RunReview(startArgs, &startOutput); err != nil {
		failure := decodeReviewIntegrationFailure(t, startOutput.Bytes())
		if failure.Code == reviewImmutableTransportUnsupportedCode {
			t.Fatalf("undeclared runtime identity refused the provider-returned START: %#v", failure)
		}
		t.Fatalf("provider-returned START failed: %v %#v", err, failure)
	}
	if !strings.Contains(startOutput.String(), "consent") {
		t.Fatalf("negotiated START answered without the consent envelope: %s", startOutput.String())
	}
}

// The refusal for a genuinely unsupported declared runtime is untouched.
//
// OpenCode used to stand here as this test's eligible-but-transport-disabled
// example. Issue #2417 gave it a genuine immutable transport, so it is no
// longer an instance of the class this test is about, and asserting it still
// refuses would assert the defect rather than the property. Kilocode replaces
// it and keeps this arm's coverage exact: it is eligible, has no transport,
// and fails with the same reviewImmutableTransportUnsupportedCode, unlike Pi,
// which fails earlier with reviewTransportCapabilityUnsupportedCode.
//
// Codex also reaches the shared provider contract through its native adapter,
// so it is not an instance of the unsupported class either. The unknown
// identity is unchanged, so the property this test protects -- a declared
// identity is validated exactly as before, and every unsupported one still
// stops -- is still proven by two runtimes.
func TestDeclaredUnsupportedRuntimeStillRefusesNegotiatedStatus(t *testing.T) {
	repo := initReviewCLIRepo(t)
	for _, runtime := range []string{string(model.AgentKilocode), "unknown-runtime"} {
		t.Run(runtime, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview([]string{
				"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", runtime, "--next-transition",
			}, &output); err == nil {
				t.Fatalf("declared unsupported runtime %q was accepted", runtime)
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Code != reviewImmutableTransportUnsupportedCode || failure.NextAction != "stop" {
				t.Fatalf("declared unsupported runtime %q failure = %#v", runtime, failure)
			}
		})
	}
}

// TestDeclaredBuiltInRuntimeUsesProvenExecutorBoundary prevents the capability
// declaration from drifting from the supported fresh reviewer paths. Claude
// and OpenCode use their native provider contract in an ordinary session,
// without `OPENCODE_DISABLE_*` environment controls; Codex invokes its native
// provider adapter through the same Go-owned contract.
func TestDeclaredBuiltInRuntimeUsesProvenExecutorBoundary(t *testing.T) {
	repo := initReviewCLIRepo(t)
	for _, test := range []struct {
		runtime string
	}{
		{runtime: string(model.AgentClaudeCode)},
		{runtime: string(model.AgentOpenCode)},
		{runtime: string(model.AgentCodex)},
	} {
		t.Run(test.runtime, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview([]string{
				"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", test.runtime, "--next-transition",
			}, &output)
			if err != nil {
				t.Fatalf("declared built-in runtime %q did not reach negotiated STATUS: %v\n%s", test.runtime, err, output.String())
			}
			if strings.Contains(output.String(), reviewImmutableTransportUnsupportedCode) {
				t.Fatalf("declared built-in runtime %q was rejected by the executor boundary: %s", test.runtime, output.String())
			}
		})
	}
}
