package screens

import (
	"regexp"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestScreensShowNoRetiredWorkflowText guards S2 of the SDD retirement: no
// screen offers SDD or OpenSpec as a current feature.
func TestScreensShowNoRetiredWorkflowText(t *testing.T) {
	retired := regexp.MustCompile(`(?i)sdd|openspec`)
	for name, view := range map[string]string{
		"complete":            RenderComplete(CompletePayload{ConfiguredAgents: 1, InstalledComponents: 1}),
		"preset":              RenderPreset(model.PresetEcosystemOnly, 0),
		"opencode background": RenderOpenCodeBackground(0),
		"pi background":       RenderPiBackground(0),
		"uninstall profiles":  RenderUninstallProfiles([]string{"cheap"}, nil, false, model.EngramUninstallScopeGlobal, 0),
		"uninstall confirm":   RenderUninstallConfirm(model.UninstallModeFull, []model.AgentID{model.AgentWindsurf}, []model.ComponentID{model.ComponentSkills}, nil, model.EngramUninstallScopeGlobal, false, 0, false, 0),
	} {
		if match := retired.FindString(view); match != "" {
			t.Errorf("%s screen shows retired workflow text %q:\n%s", name, match, view)
		}
	}
}

func TestEcosystemPresetDescribesItsComponents(t *testing.T) {
	view := RenderPreset(model.PresetEcosystemOnly, 0)
	if !regexp.MustCompile(`Memory \+ skills \+ docs \+ GGA`).MatchString(view) {
		t.Fatalf("Dev Stack description does not list its components:\n%s", view)
	}
}
