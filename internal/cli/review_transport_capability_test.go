package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestImmutableReviewRuntimeMatrix runs with no OPENCODE_DISABLE_* variable
// set: OpenCode's shared advisory transport (rdd-advisory-transport
// SKILL.md) does not depend on host isolation controls, so an ordinary
// session must resolve identically to Claude's.
func TestImmutableReviewRuntimeMatrix(t *testing.T) {
	for _, test := range []struct {
		name      string
		runtime   string
		eligible  bool
		transport reviewImmutableTransport
		supported bool
	}{
		{name: "Claude prompt carried fresh executor", runtime: string(model.AgentClaudeCode), eligible: true, transport: reviewImmutableTransportClaudePromptCarried, supported: true},
		{name: "OpenCode provider injected fresh executor", runtime: string(model.AgentOpenCode), eligible: true, transport: reviewImmutableTransportOpenCodeProviderInjected, supported: true},
		{name: "Codex advisory scratch process", runtime: string(model.AgentCodex), eligible: true, transport: reviewImmutableTransportCodexAdvisoryScratchProcess, supported: true},
		{name: "Kilo has no native executor", runtime: string(model.AgentKilocode), eligible: true, transport: reviewImmutableTransportUnsupported},
		{name: "Pi", runtime: string(model.AgentPi), transport: reviewImmutableTransportUnsupported},
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
		// startCode is the failure code a "start"-operation invocation
		// expects. Wave 4 S4 (design.md decision 5) added the broader
		// review-transport-capability admission gate, which now runs
		// before this narrower immutable-transport check on `review
		// start` specifically: a runtime absent from or unrecognised by
		// the canonical capability manifest (Pi, unknown) is now caught
		// by the broader gate first. `review status` is unaffected — the
		// new gate only runs in `review start` — so its expected code
		// stays reviewImmutableTransportUnsupportedCode for every runtime.
		startCode string
	}{
		{name: "Kilo", runtime: string(model.AgentKilocode), startCode: reviewImmutableTransportUnsupportedCode},
		{name: "Pi", runtime: string(model.AgentPi), startCode: reviewTransportCapabilityUnsupportedCode},
		{name: "unknown", runtime: "unknown-runtime", startCode: reviewTransportCapabilityUnsupportedCode},
		{name: "logical orchestrator role", runtime: "gentle-orchestrator", startCode: reviewTransportCapabilityUnsupportedCode},
		// OpenCode used to stand here, refused for lacking its host isolation
		// controls. The shared advisory transport (rdd-advisory-transport
		// SKILL.md) retired that requirement: OpenCode's output is advisory
		// until native Go admits it, so an ordinary already-running session
		// is sufficient and OpenCode is a genuinely supported runtime now,
		// exercised instead by TestSupportedImmutableReviewTransportReachesRepositoryValidation.
		//
		// Codex used to stand here too, refused for lacking an enforceable
		// fresh-reviewer boundary (#2208). The shared advisory transport's
		// CodexAdapter (internal/advisoryreview) supplies that boundary --
		// organically proven by TestRealCodexReviewerOrdinarySessionAdmitsRawOutput
		// in e2e/organicruntime -- so Codex is a genuinely supported runtime
		// now too, exercised by the same
		// TestSupportedImmutableReviewTransportReachesRepositoryValidation.
		//
		// An undeclared runtime identity is deliberately absent from this
		// matrix: it makes no transport claim to refuse, so it stays on the
		// manual/non-agent compatibility path. See
		// review_missing_runtime_identity_test.go.
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, invocation := range []struct {
				name      string
				operation string
				args      []string
				code      string
				message   string
			}{
				{name: "status without transition", operation: "review.status", args: []string{"status", "--contract", ReviewIntegrationContractV2, "--cwd", missingRepository}, code: reviewImmutableTransportUnsupportedCode, message: reviewImmutableTransportUnsupportedReason.Message},
				{name: "status", operation: "review.status", args: []string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", missingRepository}, code: reviewImmutableTransportUnsupportedCode, message: reviewImmutableTransportUnsupportedReason.Message},
				{name: "start before target", operation: "review.start", args: []string{"start", "--contract", ReviewIntegrationContractV2, "--projection", "workspace", "--cwd", missingRepository}, code: test.startCode},
				{name: "start", operation: "review.start", args: []string{"start", "--contract", ReviewIntegrationContractV2, "--target", target, "--projection", "workspace", "--cwd", missingRepository}, code: test.startCode},
			} {
				t.Run(invocation.name, func(t *testing.T) {
					args := append([]string{}, invocation.args...)
					if test.runtime != "" {
						args = append(args, "--agent", test.runtime)
					}
					wantMessage := invocation.message
					if wantMessage == "" {
						wantMessage = reviewImmutableTransportUnsupportedReason.Message
						if invocation.code == reviewTransportCapabilityUnsupportedCode {
							wantMessage = reviewTransportCapabilityUnsupportedReason.Message
						}
					}
					var output bytes.Buffer
					if err := RunReview(args, &output); err == nil {
						t.Fatalf("%s accepted runtime %q", invocation.name, test.runtime)
					}
					failure := decodeReviewIntegrationFailure(t, output.Bytes())
					if failure.Operation != invocation.operation ||
						failure.Code != invocation.code ||
						failure.MutationOutcome != ReviewMutationNotStarted ||
						failure.AuthorityApplicability != "not_evaluated" ||
						failure.RetrySafe ||
						failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
						failure.NextAction != "stop" ||
						failure.Message != wantMessage {
						t.Fatalf("unsupported transport failure = %#v", failure)
					}
				})
			}
		})
	}

	repo := initReviewCLIRepo(t)
	for _, args := range [][]string{
		{"status", "--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentKilocode), "--next-transition", "--cwd", repo},
		{"start", "--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentPi), "--target", target, "--projection", "workspace", "--cwd", repo},
		{"start", "--contract", ReviewIntegrationContractV2, "--agent", "unknown-runtime", "--target", target, "--projection", "workspace", "--cwd", repo},
		{"start", "--contract", ReviewIntegrationContractV2, "--agent", "gentle-orchestrator", "--target", target, "--projection", "workspace", "--cwd", repo},
		{"start", "--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentClaudeCode), "--agent", string(model.AgentClaudeCode), "--target", target, "--projection", "workspace", "--cwd", repo},
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

// TestSupportedImmutableReviewTransportReachesRepositoryValidation proves
// both supported runtimes reach repository validation in an ordinary session:
// neither depends on OPENCODE_DISABLE_PROJECT_CONFIG or
// OPENCODE_DISABLE_EXTERNAL_SKILLS, which this test deliberately leaves unset.
func TestSupportedImmutableReviewTransportReachesRepositoryValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		runtime string
	}{
		{name: "Claude", runtime: string(model.AgentClaudeCode)},
		{name: "OpenCode", runtime: string(model.AgentOpenCode)},
		{name: "Codex", runtime: string(model.AgentCodex)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview([]string{
				"status", "--contract", ReviewIntegrationContractV2, "--agent", test.runtime,
				"--next-transition", "--cwd", t.TempDir() + "/missing",
			}, &output)
			if err == nil {
				t.Fatal("missing repository unexpectedly succeeded")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Code == reviewImmutableTransportUnsupportedCode || failure.Code == reviewTransportCapabilityUnsupportedCode {
				t.Fatalf("supported runtime was rejected before repository validation: %#v", failure)
			}
		})
	}
}

func TestImmutableReviewTransportRefusalNamesWorkingExits(t *testing.T) {
	for _, runtime := range []model.AgentID{model.AgentKilocode, model.AgentPi} {
		t.Run(string(runtime), func(t *testing.T) {
			_, err := reviewRuntimeWithImmutableTransport(string(runtime))
			if err == nil {
				t.Fatal("want a refusal")
			}
			const exit = "gentle-ai review mode disable --scope clone --cwd <repo>"
			if !strings.Contains(err.Error(), exit) {
				t.Fatalf("refusal does not name the clone-scoped kill switch: %v", err)
			}
			if !strings.Contains(err.Error(), string(model.AgentClaudeCode)) || !strings.Contains(err.Error(), string(model.AgentOpenCode)) {
				t.Fatalf("refusal does not name both supported runtimes: %v", err)
			}
			if strings.Contains(err.Error(), "supported immutable review runtimes: "+string(runtime)) {
				t.Fatalf("refusal lists itself as supported: %v", err)
			}
		})
	}
}

func TestV21RejectsDuplicateRuntimeAgentsBeforeRepositoryAccess(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--cwd", t.TempDir() + "/missing",
		"--agent", string(model.AgentClaudeCode), "--agent", string(model.AgentClaudeCode),
	}, &output)
	if err == nil {
		t.Fatal("v2.1 STATUS accepted multiple runtime identities")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != reviewImmutableTransportUnsupportedCode || failure.Operation != "review.status" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "not_evaluated" {
		t.Fatalf("duplicate runtime failure = %#v", failure)
	}
}
