package screens

import (
	"strings"
	"testing"
)

func TestRenderCompleteGGAProviderNotices(t *testing.T) {
	for _, notice := range []string{
		"GGA config unchanged: no provider is configured and selected agents have no known GGA mapping. Set a literal PROVIDER, then rerun gentle-ai sync.",
		"GGA config unchanged: ambiguous or dynamic assignments. Set one literal PROVIDER, then rerun gentle-ai sync; no shell was evaluated.",
	} {
		t.Run(notice, func(t *testing.T) {
			out := RenderComplete(CompletePayload{GGAInstalled: true, ManualActions: []string{notice}})
			if strings.Count(out, notice) != 1 || !strings.Contains(out, "Manual actions required") {
				t.Fatalf("GGA guidance missing or duplicated: %q", out)
			}
		})
	}
}
