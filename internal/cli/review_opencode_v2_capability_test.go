package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

// OpenCode immutable review is admitted only for a matching pair: a V1 runtime
// with no relay declaration (the V1 plugin declares nothing), or a V2 runtime
// whose managed plugin declares exactly the V2 relay contract. Every other
// combination refuses before the relay reads any input.
func TestOpenCodeTransportCapabilityRequiresMatchingRuntimeAndDeclaration(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	for _, test := range []struct {
		name, declaration, version string
		admitted, probes           bool
	}{
		{"V1 runtime without declaration", "", "1.18.30", true, true},
		{"V2 runtime with the exact V2 declaration", openCodeRelayContractV2, "opencode v2.0.19", true, true},
		{"V2 runtime without declaration", "", "2.0.4", false, true},
		{"unknown runtime without declaration", "", "unknown", false, true},
		{"V1 runtime with the V2 declaration", openCodeRelayContractV2, "1.18.30", false, true},
		{"unknown runtime with the V2 declaration", openCodeRelayContractV2, "unknown", false, true},
		{"V2 runtime with another declaration", "gentle-ai.opencode-relay/v3", "2.0.19", false, false},
		{"V2 runtime with a padded declaration", openCodeRelayContractV2 + " ", "2.0.19", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(openCodeRelayContractEnvironment, test.declaration)
			probed := false
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				probed = true
				if !test.probes {
					t.Error("an unrecognized declaration must refuse before the PATH probe")
				}
				return opencode.CommandOutput{Stdout: []byte(test.version)}, nil
			}
			if got := reviewImmutableRuntimeCapability(model.AgentOpenCode).supportsImmutableReceiptReview(); got != test.admitted {
				t.Fatalf("capability admitted = %v, want %v", got, test.admitted)
			}
			if probed != test.probes {
				t.Fatalf("runtime probed = %v, want %v", probed, test.probes)
			}
			if !test.admitted {
				if err := runReviewOpenCodeTransport(nil, v2UnreadableInput{}, io.Discard); err == nil || !strings.Contains(err.Error(), reviewImmutableTransportUnsupportedCode) {
					t.Fatalf("direct relay did not refuse before input: %v", err)
				}
			}
		})
	}
}

type v2UnreadableInput struct{}

func (v2UnreadableInput) Read([]byte) (int, error) {
	panic("refused runtime read transport input before capability refusal")
}

// A V2 host whose PATH resolves a coexisting V1 binary must not inherit V1
// capability through the V2 plugin's relay: the declaration and the detected
// runtime disagree, so the relay refuses.
func TestOpenCodeV2TransportDeclarationCannotInheritV1(t *testing.T) {
	t.Setenv(openCodeRelayContractEnvironment, openCodeRelayContractV2)
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
	if reviewImmutableRuntimeCapability(model.AgentOpenCode).supportsImmutableReceiptReview() {
		t.Fatal("active V2 declaration inherited V1 capability")
	}
}
