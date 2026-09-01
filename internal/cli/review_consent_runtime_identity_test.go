package cli

// Issue #2676: a negotiated START explicitly bound to a non-Claude runtime
// (OpenCode, Codex) reported "claude-code" as the consent/v3 envelope's
// top-level `agent`, while the envelope's own follow-up invocations were
// already correctly bound to the real runtime. These tests pin the fix: the
// top-level identity now matches the declared binding, and Validate() stays
// fail-closed about which identities the v3 shape may ever name.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNegotiatedConsentEnvelopeBindsTheDeclaredRuntimeIdentity drives a
// negotiated v2 START explicitly bound to each supported immutable-transport
// runtime and asserts the consent envelope's top-level agent matches the
// declared binding, exactly like its follow-up invocations already did. The
// claude-code case is the regression check: it must stay exactly as today.
func TestNegotiatedConsentEnvelopeBindsTheDeclaredRuntimeIdentity(t *testing.T) {
	for _, agent := range []string{"opencode", "codex", "claude-code"} {
		t.Run(agent, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			stubReviewConsole(t, false, "")
			writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

			output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
				"start", "--contract", ReviewIntegrationContractV2, "--agent", agent, "--cwd", repo,
				"--lineage", "review-consent-runtime-" + agent, "--consent", "relay",
			}))
			question := decodeConsentQuestion(t, output.Bytes())
			if err := question.Validate(); err != nil {
				t.Fatalf("consent envelope bound to %q does not validate: %v\n%s", agent, err, output.String())
			}
			if question.Schema != ReviewIntegrationConsentSchemaV3 || question.Contract != ReviewIntegrationContractV2 || question.Agent != agent {
				t.Fatalf("consent envelope identity = %#v, want top-level agent %q", question, agent)
			}
			// The reported bug was exactly this divergence: the follow-up
			// invocations were already bound to the real runtime while the
			// top-level identity lied about it. Assert both stay bound together.
			for _, choice := range question.Choices {
				if !strings.Contains(choice.Invocation, "--agent "+agent) {
					t.Fatalf("choice invocation for %q diverges from its own consent envelope identity: %#v", agent, choice)
				}
			}
		})
	}
}

// TestConsentValidateRejectsUnknownOrEmptyRuntimeIdentity keeps Validate()
// fail-closed: the v3 shape must name a runtime that actually carries the
// immutable receipt-review transport (the same authority
// reviewRuntimeWithImmutableTransport gates START on), never an arbitrary
// string, and never empty.
func TestConsentValidateRejectsUnknownOrEmptyRuntimeIdentity(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "consent-v3.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var base ReviewIntegrationConsentResult
	if err := json.Unmarshal(payload, &base); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name  string
		agent string
	}{
		{name: "unknown runtime", agent: "not-a-real-runtime"},
		// Kilocode is eligible under the fixed RDD policy but dormant for the
		// immutable-review-executor contract (no proven fresh-reviewer
		// boundary yet), so it can never legitimately reach negotiated START
		// consent; the v3 envelope must not name it either.
		{name: "eligible but unsupported runtime", agent: "kilocode"},
		{name: "empty runtime", agent: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mutated := base
			mutated.Agent = tt.agent
			if err := mutated.Validate(); err == nil {
				t.Fatalf("consent envelope validated with runtime identity %q", tt.agent)
			}
		})
	}
}

// TestConsentValidateAcceptsHistoricalShapes pins that neither the v1 legacy
// contract nor the pre-v3 v2-schema/empty-agent shape were disturbed by
// making Validate() fail-closed about the runtime identity.
func TestConsentValidateAcceptsHistoricalShapes(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "consent.fixture.json"),
		filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "consent.fixture.json"),
	} {
		t.Run(path, func(t *testing.T) {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var result ReviewIntegrationConsentResult
			if err := json.Unmarshal(payload, &result); err != nil {
				t.Fatal(err)
			}
			if err := result.Validate(); err != nil {
				t.Fatalf("historical shape %s no longer validates: %v", path, err)
			}
		})
	}
}
