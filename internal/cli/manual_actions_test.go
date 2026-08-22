package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
)

// TestRenderInstallManualActions pins the CLI renderer. It previously read only
// the Pi CodeGraph result, so an action any other apply step raised through the
// pipeline was collected, propagated, and then silently dropped before reaching
// the user. Pi CodeGraph actions are copied into ExecutionResult.ManualActions
// during propagation, so the two sources overlap and must not print twice.
func TestRenderInstallManualActions(t *testing.T) {
	piAction := "Pi CodeGraph runtime verification is pending."
	stepAction := "Something optional was skipped; here is how to finish it."

	tests := []struct {
		name   string
		result InstallResult
		action string
		want   int
	}{
		{
			name:   "renders pipeline actions the renderer used to drop",
			result: InstallResult{Execution: pipeline.ExecutionResult{ManualActions: []string{stepAction}}},
			action: stepAction,
			want:   1,
		},
		{
			name: "deduplicates an action present in both sources",
			result: InstallResult{
				PiCodeGraph: &communitytool.PiCodeGraphResult{ManualActions: []string{piAction}},
				Execution:   pipeline.ExecutionResult{ManualActions: []string{piAction}},
			},
			action: piAction,
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RenderInstallManualActions(tt.result)
			if got := strings.Count(out, tt.action); got != tt.want {
				t.Fatalf("action rendered %d times, want %d: %q", got, tt.want, out)
			}
		})
	}
}

// TestRenderInstallManualActionsEmptyRendersNothing keeps the renderer silent
// when there is nothing to say, so an ordinary run gains no trailing section.
func TestRenderInstallManualActionsEmptyRendersNothing(t *testing.T) {
	if out := RenderInstallManualActions(InstallResult{}); out != "" {
		t.Fatalf("RenderInstallManualActions() = %q, want empty", out)
	}
}
