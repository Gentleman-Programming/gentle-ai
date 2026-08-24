package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// piRelayHandshakeRefusalCause is the exact cause a handshake-less `pi`
// invocation must read (#3440). It names the one missing condition and
// nothing else.
//
// The old cause offered `gentle-ai review mode disable --scope clone` and
// then listed `claude-code, opencode, codex` as the supported runtimes. That
// list is built by reviewTransportSupportedRuntimeIDs, which calls
// reviewImmutableRuntimeCapability for every agent IN THIS PROCESS -- under
// the exact condition that is failing -- so `pi` can never appear in the
// message that would explain how to enable `pi`. A community report read that
// message and concluded v2.4.0 had dropped Pi support, when the fix was one
// exported variable.
const piRelayHandshakeRefusalCause = "the active runtime is not eligible for immutable receipt review; " +
	"pi is eligible only while GENTLE_PI_REVIEW_RELAY_CONTRACT declares the exact relay contract this binary admits, " +
	"which the gentle-pi host exports on every invocation it relays; export it in this shell and re-run"

// TestPiTransportRefusalNamesTheHandshakeNotTheKillSwitch runs the exact
// documented shell entry point both ways round: the identical command must
// refuse actionably without the handshake and succeed with it. Asserting only
// the refusal would leave the message free to name a condition that is not
// actually the deciding one.
func TestPiTransportRefusalNamesTheHandshakeNotTheKillSwitch(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statusArgs := []string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentPi), "--next-transition",
	}

	t.Run("handshake absent names the handshake", func(t *testing.T) {
		t.Setenv(reviewPiHostRelayContractEnvironment, "")
		var output bytes.Buffer
		if err := RunReview(statusArgs, &output); err == nil {
			t.Fatalf("handshake-less pi STATUS succeeded: %s", output.String())
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code != reviewImmutableTransportUnsupportedCode {
			t.Fatalf("handshake-less pi failure code = %q, want %q", failure.Code, reviewImmutableTransportUnsupportedCode)
		}
		if failure.Cause != piRelayHandshakeRefusalCause {
			t.Fatalf("handshake-less pi cause =\n\t%q\nwant\n\t%q", failure.Cause, piRelayHandshakeRefusalCause)
		}
		// The two halves of the old cause, named explicitly: disabling
		// receipt-driven review is a legitimate exit from the practice, but it
		// is not the remedy here, and a supported list that structurally
		// cannot contain `pi` reads as "pi was dropped".
		if strings.Contains(failure.Cause, "review mode disable") {
			t.Fatalf("handshake-less pi cause still offers the kill switch as the remedy: %q", failure.Cause)
		}
		if strings.Contains(failure.Cause, "supported immutable review runtimes") {
			t.Fatalf("handshake-less pi cause still lists a supported set that cannot contain pi: %q", failure.Cause)
		}
	})

	t.Run("handshake present admits the identical command", func(t *testing.T) {
		t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
		var output bytes.Buffer
		if err := RunReview(statusArgs, &output); err != nil {
			t.Fatalf("pi STATUS with the declared handshake refused: %v\n%s", err, output.String())
		}
		if strings.Contains(output.String(), reviewImmutableTransportUnsupportedCode) {
			t.Fatalf("pi STATUS with the declared handshake still reported the transport refusal: %s", output.String())
		}
	})
}

// TestPiRelayHandshakeGuidanceSurvivesTheFailureCausePrivacyGate is the guard
// the first draft of this fix needed, and the reason the cause names the
// variable without its value.
//
// This prose reaches the operator only as the negotiated envelope's `cause`,
// and every refusal crosses reviewScrubDefectReportField to get there. That
// gate rewrites any `KEY=VALUE` token to `<redacted>` in full, and any
// `/`-rooted run to `<redacted>` from the slash onward. A cause that spells
// the handshake out therefore arrives as advice to export `<redacted>`:
// strictly worse than the misleading message it replaced.
func TestPiRelayHandshakeGuidanceSurvivesTheFailureCausePrivacyGate(t *testing.T) {
	guidance := reviewPiRelayHandshakeGuidance()
	if scrubbed := reviewScrubDefectReportField(guidance); scrubbed != guidance {
		t.Fatalf("the privacy gate rewrites the pi guidance before the operator reads it:\n\tguidance %q\n\tscrubbed %q", guidance, scrubbed)
	}
	if !strings.Contains(guidance, reviewPiHostRelayContractEnvironment) {
		t.Fatalf("the pi guidance no longer names the missing condition: %q", guidance)
	}

	// The tripwire for the half of #3440 this change cannot deliver. While
	// the gate still destroys the spelled-out handshake, the omission is a
	// constraint. The day it stops, this fails and the cause owes the
	// operator the exact value.
	spelled := reviewPiHostRelayContractEnvironment + "=" + reviewPiHostRelayContract
	if scrubbed := reviewScrubDefectReportField("export " + spelled + " and re-run"); scrubbed != "export "+reviewDefectReportRedactionMarker+" and re-run" {
		t.Fatalf("the privacy gate no longer destroys %q (it now renders %q); the pi cause can carry the exact handshake and should", spelled, scrubbed)
	}
	if scrubbed := reviewScrubDefectReportField("the value " + reviewPiHostRelayContract); scrubbed != "the value gentle-pi.review-relay"+reviewDefectReportRedactionMarker {
		t.Fatalf("the privacy gate no longer truncates the bare contract value (it now renders %q); the pi cause can carry it and should", scrubbed)
	}
}

// TestPiHandshakeGuidanceStaysScopedToPi proves the new cause does not leak.
// Two directions: a runtime the handshake cannot help keeps the generic exit
// guidance, and `pi` never advertises itself as supported while it is refused.
func TestPiHandshakeGuidanceStaysScopedToPi(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, "")

	for _, runtime := range []string{string(model.AgentKilocode), "unknown-runtime"} {
		t.Run(runtime, func(t *testing.T) {
			_, err := reviewRuntimeWithImmutableTransport(runtime)
			if err == nil {
				t.Fatal("want a refusal")
			}
			if strings.Contains(err.Error(), reviewPiHostRelayContractEnvironment) {
				t.Fatalf("refusal for %q leaks the pi relay handshake: %v", runtime, err)
			}
			if !strings.Contains(err.Error(), "gentle-ai review mode disable --scope clone --cwd <repo>") {
				t.Fatalf("refusal for %q lost the generic exit guidance: %v", runtime, err)
			}
		})
	}

	t.Run("pi is never offered as its own substitute", func(t *testing.T) {
		_, err := reviewRuntimeWithImmutableTransport(string(model.AgentPi))
		if err == nil {
			t.Fatal("want a refusal")
		}
		if strings.Contains(err.Error(), "supported immutable review runtimes") {
			t.Fatalf("pi refusal reintroduced the substitute list: %v", err)
		}
	})
}

// TestPiRelayHandshakeIsSoleMissingConditionStaysDiagnostic pins the predicate
// that selects the cause. It must never be read as an admission decision:
// reviewImmutableRuntimeCapability stays the only authority, and the predicate
// is false the moment the handshake is declared, whatever the capability says.
func TestPiRelayHandshakeIsSoleMissingConditionStaysDiagnostic(t *testing.T) {
	for _, test := range []struct {
		name     string
		declared string
		agent    model.AgentID
		want     bool
	}{
		{name: "pi without the handshake", declared: "", agent: model.AgentPi, want: true},
		{name: "pi with a stale contract", declared: "gentle-pi.review-relay/v0", agent: model.AgentPi, want: true},
		{name: "pi with the exact handshake", declared: reviewPiHostRelayContract, agent: model.AgentPi, want: false},
		{name: "kilocode without the handshake", declared: "", agent: model.AgentKilocode, want: false},
		{name: "opencode without the handshake", declared: "", agent: model.AgentOpenCode, want: false},
		{name: "unknown runtime", declared: "", agent: model.AgentID("unknown-runtime"), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, test.declared)
			if got := reviewPiRelayHandshakeIsSoleMissingCondition(test.agent); got != test.want {
				t.Fatalf("reviewPiRelayHandshakeIsSoleMissingCondition(%q) = %t, want %t", test.agent, got, test.want)
			}
			// Eligibility is unchanged by the diagnostic in every case.
			capability := reviewImmutableRuntimeCapability(test.agent)
			if test.agent == model.AgentPi && test.declared != reviewPiHostRelayContract && capability.supportsImmutableReceiptReview() {
				t.Fatalf("the diagnostic widened pi eligibility: %#v", capability)
			}
		})
	}
}
