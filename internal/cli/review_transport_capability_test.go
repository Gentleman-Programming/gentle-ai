package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestImmutableReviewRuntimeMatrix(t *testing.T) {
	for _, test := range []struct {
		name      string
		runtime   string
		eligible  bool
		transport reviewImmutableTransport
		supported bool
	}{
		{name: "Claude prompt carried", runtime: string(model.AgentClaudeCode), eligible: true, transport: reviewImmutableTransportClaudePromptCarried, supported: true},
		{name: "OpenCode is pending #2076", runtime: string(model.AgentOpenCode), eligible: true, transport: reviewImmutableTransportUnsupported},
		{name: "Codex is pending #2208", runtime: string(model.AgentCodex), eligible: true, transport: reviewImmutableTransportUnsupported},
		{name: "Pi", runtime: string(model.AgentPi), transport: reviewImmutableTransportUnsupported},
		{name: "Kilo", runtime: string(model.AgentKilocode), transport: reviewImmutableTransportUnsupported},
		{name: "unknown", runtime: "unknown-runtime", transport: reviewImmutableTransportUnsupported},
		{name: "alias", runtime: "open-code", transport: reviewImmutableTransportUnsupported},
		{name: "casing", runtime: "OpenCode", transport: reviewImmutableTransportUnsupported},
	} {
		t.Run(test.name, func(t *testing.T) {
			capability := reviewImmutableRuntimeCapability(model.AgentID(test.runtime))
			if capability.Eligible != test.eligible || capability.Transport != test.transport || capability.supportsImmutableReceiptReview() != test.supported {
				t.Fatalf("runtime %q capability = %#v, supported %t", test.runtime, capability, capability.supportsImmutableReceiptReview())
			}
			identity, err := reviewRuntimeWithImmutableTransport(test.runtime)
			if test.supported {
				if err != nil || identity != model.AgentID(test.runtime) {
					t.Fatalf("supported runtime %q = %q, %v", test.runtime, identity, err)
				}
				return
			}
			if err == nil || identity != "" {
				t.Fatalf("unsupported runtime %q = %q, %v", test.runtime, identity, err)
			}
		})
	}
}

func TestUnsupportedImmutableReviewTransportStopsBeforeRepositoryOrAuthority(t *testing.T) {
	const target = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	missingRepository := t.TempDir() + "/missing"

	for _, test := range []struct {
		name    string
		runtime string
	}{
		{name: "OpenCode", runtime: string(model.AgentOpenCode)},
		{name: "Codex", runtime: string(model.AgentCodex)},
		{name: "Pi", runtime: string(model.AgentPi)},
		{name: "Kilo", runtime: string(model.AgentKilocode)},
		{name: "unknown", runtime: "unknown-runtime"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, invocation := range []struct {
				name      string
				operation string
				args      []string
			}{
				{name: "status without transition", operation: "review.status", args: []string{"status", "--contract", ReviewIntegrationContractV2, "--cwd", missingRepository}},
				{name: "status", operation: "review.status", args: []string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", missingRepository}},
				{name: "start before target", operation: "review.start", args: []string{"start", "--contract", ReviewIntegrationContractV2, "--projection", "workspace", "--cwd", missingRepository}},
				{name: "start", operation: "review.start", args: []string{"start", "--contract", ReviewIntegrationContractV2, "--target", target, "--projection", "workspace", "--cwd", missingRepository}},
			} {
				t.Run(invocation.name, func(t *testing.T) {
					args := append(append([]string{}, invocation.args...), "--agent", test.runtime)
					var output bytes.Buffer
					if err := RunReview(args, &output); err == nil {
						t.Fatalf("%s accepted runtime %q", invocation.name, test.runtime)
					}
					failure := decodeReviewIntegrationFailure(t, output.Bytes())
					if failure.Operation != invocation.operation ||
						failure.Code != reviewImmutableTransportUnsupportedCode ||
						failure.MutationOutcome != ReviewMutationNotStarted ||
						failure.AuthorityApplicability != "not_evaluated" ||
						failure.RetrySafe ||
						failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
						failure.NextAction != "stop" ||
						failure.Message != reviewImmutableTransportUnsupportedReason.Message {
						t.Fatalf("unsupported transport failure = %#v", failure)
					}
				})
			}
		})
	}

	repo := initReviewCLIRepo(t)
	for _, args := range [][]string{
		{"status", "--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentOpenCode), "--next-transition", "--cwd", repo},
		{"start", "--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentCodex), "--target", target, "--projection", "workspace", "--cwd", repo},
	} {
		if err := RunReview(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("unsupported invocation succeeded: %v", args)
		}
	}
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 0 {
		t.Fatalf("unsupported runtime created review authority: %#v", stores)
	}
}

// An absent or duplicated --agent is a malformed or stale route, not a claim
// about any runtime's transport. Issue #2242 reported the absent case being
// reported as immutable_review_transport_unsupported with next_action stop and
// no way out, which named a capability the facade never evaluated.
func TestUnresolvedRuntimeIdentityIsNotReportedAsUnsupportedTransport(t *testing.T) {
	missingRepository := t.TempDir() + "/missing"

	for _, test := range []struct {
		name      string
		operation string
		args      []string
	}{
		{
			name: "status without runtime identity", operation: "review.status",
			args: []string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", missingRepository},
		},
		{
			name: "status with duplicate runtime identity", operation: "review.status",
			args: []string{
				"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", missingRepository,
				"--agent", string(model.AgentClaudeCode), "--agent", string(model.AgentClaudeCode),
			},
		},
		{
			name: "start without runtime identity", operation: "review.start",
			args: []string{"start", "--contract", ReviewIntegrationContractV2, "--projection", "workspace", "--cwd", missingRepository},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview(test.args, &output); err == nil {
				t.Fatalf("%s accepted an unresolved runtime identity", test.name)
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != test.operation ||
				failure.Code != reviewRuntimeIdentityUnresolvedCode ||
				failure.MutationOutcome != ReviewMutationNotStarted ||
				failure.AuthorityApplicability != "not_evaluated" ||
				failure.NextAction != "correct_request" ||
				!failure.RetrySafe ||
				failure.Message != reviewRuntimeIdentityUnresolvedReason.Message {
				t.Fatalf("unresolved runtime identity failure = %#v", failure)
			}
			if !strings.Contains(failure.Cause, "gentle-ai sync") {
				t.Fatalf("failure cause names no recovery path: %q", failure.Cause)
			}
		})
	}
}

func TestSupportedImmutableReviewRuntimeIsCarriedIntoV2Start(t *testing.T) {
	for _, runtime := range []string{string(model.AgentClaudeCode)} {
		t.Run(runtime, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n", 0o644)
			var output bytes.Buffer
			if err := RunReview([]string{
				"status", "--contract", ReviewIntegrationContractV2, "--agent", runtime,
				"--next-transition", "--cwd", repo,
			}, &output); err != nil {
				t.Fatal(err)
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
			if status.NextTransition == nil || status.NextTransition.Execute == nil ||
				!strings.Contains(status.NextTransition.Execute.Command, "--agent="+runtime) {
				t.Fatalf("runtime-bound START transition = %#v", status.NextTransition)
			}
		})
	}
}
