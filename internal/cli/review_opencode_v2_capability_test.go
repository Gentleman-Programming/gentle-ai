package cli

import (
	"context"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
	"io"
	"strings"
	"testing"
)

func TestOpenCodeV2TransportCapabilityUnavailable(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	for _, version := range []string{"2.0.4", "unknown"} {
		t.Run(version, func(t *testing.T) {
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte(version)}, nil
			}
			if err := runReviewOpenCodeTransport(nil, v2UnreadableInput{}, io.Discard); err == nil || !strings.Contains(err.Error(), reviewImmutableTransportUnsupportedCode) {
				t.Fatalf("direct relay did not refuse before input: %v", err)
			}
			if reviewImmutableRuntimeCapability(model.AgentOpenCode).supportsImmutableReceiptReview() {
				t.Fatal("unproven runtime advertised")
			}
		})
	}
}

type v2UnreadableInput struct{}

func (v2UnreadableInput) Read([]byte) (int, error) {
	panic("unproven V2 read transport input before capability refusal")
}

func TestOpenCodeV2TransportDeclarationCannotInheritV1(t *testing.T) {
	t.Setenv("GENTLE_AI_OPENCODE_RELAY_CONTRACT", "gentle-ai.opencode-relay/v2-staged")
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		t.Fatal("declared unproven host must refuse before PATH probe")
		return opencode.CommandOutput{Stdout: []byte("1.18.30")}, nil
	}
	if reviewImmutableRuntimeCapability(model.AgentOpenCode).supportsImmutableReceiptReview() {
		t.Fatal("active V2 inherited V1 capability")
	}
}
