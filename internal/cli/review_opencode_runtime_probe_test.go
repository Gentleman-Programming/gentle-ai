package cli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

// stubOpenCodeRuntimeProbeSequence answers each `opencode --version` probe
// from answers in order (the last one repeats) and counts the probes. An
// empty answer reports a probe failure.
func stubOpenCodeRuntimeProbeSequence(t *testing.T, answers ...string) *int {
	t.Helper()
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	freshOpenCodeRuntimeProbe(t)
	t.Setenv(openCodeRelayContractEnvironment, "")
	probes := 0
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		answer := answers[min(probes, len(answers)-1)]
		probes++
		if answer == "" {
			return opencode.CommandOutput{}, errors.New("opencode: executable file not found")
		}
		return opencode.CommandOutput{Stdout: []byte(answer)}, nil
	}
	return &probes
}

// TestOpenCodeRuntimeProbeRunsOncePerProcess is the guard ratchet for
// #4984/#5058/#5117: every review gate in one process must read the same
// OpenCode runtime answer, so a second `opencode --version` subprocess within
// one process is a regression, whether the first probe admitted or refused.
func TestOpenCodeRuntimeProbeRunsOncePerProcess(t *testing.T) {
	for _, test := range []struct {
		name     string
		answer   string
		admitted bool
	}{
		{name: "admitted", answer: "1.18.30", admitted: true},
		{name: "refused", answer: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			probes := stubOpenCodeRuntimeProbeSequence(t, test.answer)
			for range 3 {
				_, err := reviewRuntimeWithImmutableTransport(string(model.AgentOpenCode))
				if (err == nil) != test.admitted {
					t.Fatalf("START/STATUS gate admitted = %t, want %t: %v", err == nil, test.admitted, err)
				}
				_, err = reviewCaptureRuntimeWithBoundTransport(string(model.AgentOpenCode))
				if (err == nil) != test.admitted {
					t.Fatalf("capture gate admitted = %t, want %t: %v", err == nil, test.admitted, err)
				}
				if got := reviewImmutableRuntimeCapability(model.AgentOpenCode).supportsImmutableReceiptReview(); got != test.admitted {
					t.Fatalf("relay/consent gate admitted = %t, want %t", got, test.admitted)
				}
				reviewTransportSupportedRuntimeIDs()
			}
			if *probes != 1 {
				t.Fatalf("opencode --version probes in one process = %d, want exactly 1", *probes)
			}
		})
	}
}

// TestOpenCodeSupportedRuntimeListIsStatic pins that the advertised list never
// probes the host, so a refusal cannot list OpenCode on one call and omit it
// on the next (#4984).
func TestOpenCodeSupportedRuntimeListIsStatic(t *testing.T) {
	probes := stubOpenCodeRuntimeProbeSequence(t, "")
	if supported := reviewTransportSupportedRuntimeIDs(); !slices.Contains(supported, string(model.AgentOpenCode)) {
		t.Fatalf("supported runtimes = %q, want OpenCode listed regardless of this host's probe", supported)
	}
	if *probes != 0 {
		t.Fatalf("supported-runtime list ran %d opencode --version probes, want none", *probes)
	}
}

// TestOpenCodeConsentRelayDoesNotReprobeEligibility reproduces #5058: START
// admitted OpenCode, then the consent envelope's own validation probed again,
// read a different answer, and failed with "invalid consent question
// identity" instead of returning the consent question.
func TestOpenCodeConsentRelayDoesNotReprobeEligibility(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	stubOpenCodeRuntimeProbeSequence(t, "1.18.30", "")

	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentOpenCode), "--cwd", repo,
		"--lineage", "review-consent-probe-once", "--consent", "relay",
	}))
	question := decodeConsentQuestion(t, output.Bytes())
	if question.Schema != ReviewIntegrationConsentSchemaV3 || question.Agent != string(model.AgentOpenCode) {
		t.Fatalf("consent envelope = %#v, want the OpenCode consent question", question)
	}
}

// TestOpenCodeRuntimeProbeTimeoutIsReportedDistinctly pins that a probe that
// ran out of time names the timeout and the command that timed out instead of
// the generic "not eligible" refusal a genuinely unsupported runtime gets.
func TestOpenCodeRuntimeProbeTimeoutIsReportedDistinctly(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	freshOpenCodeRuntimeProbe(t)
	t.Setenv(openCodeRelayContractEnvironment, "")
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{}, context.DeadlineExceeded
	}
	_, err := reviewRuntimeWithImmutableTransport(string(model.AgentOpenCode))
	if err == nil {
		t.Fatal("a timed-out probe admitted OpenCode")
	}
	for _, want := range []string{"opencode --version", "timed out", "re-run"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("timeout refusal %q does not name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "the active runtime is not eligible") {
		t.Fatalf("timeout refusal reads like an unsupported runtime: %v", err)
	}
}

// TestOpenCodeLensSlotsInheritFrozenRuntimeWithoutAgent reproduces #4808: a
// lineage START froze to OpenCode, and a STATUS that omits --agent must still
// issue each lens slot's provider task instead of an unlaunchable slot.
func TestOpenCodeLensSlotsInheritFrozenRuntimeWithoutAgent(t *testing.T) {
	reviewEnabledHome(t)
	stubOpenCodeV1ReviewRuntime(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	started := startFacadeReviewForRuntime(t, repo, model.AgentOpenCode)

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition"}, &output); err != nil {
		t.Fatalf("STATUS without --agent: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("STATUS without --agent = %#v, want reviewer collection", status.NextTransition)
	}
	for order, input := range status.NextTransition.Collect.Inputs {
		if input.ProviderTask == nil || input.ProviderTask.Role != reviewProviderRoleLens {
			t.Fatalf("lens input %d = %#v, want the OpenCode provider task of the frozen runtime", order, input)
		}
	}
}
