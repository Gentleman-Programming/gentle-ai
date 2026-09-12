package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestRenderUninstallProfilesShowsExplicitEngramChoices(t *testing.T) {
	output := RenderUninstallProfiles([]string{"cheap"}, []string{"cheap"}, true, true, true, model.EngramUninstallScopeNone, 1)
	for _, want := range []string{"No cleanup", "Keep all Engram data and configuration", "Project-only cleanup", "Global cleanup"} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderUninstallProfiles() missing %q:\n%s", want, output)
		}
	}
}
