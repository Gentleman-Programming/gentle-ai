package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

func TestOpenCodeInstallerBetaPresentation(t *testing.T) {
	for _, tt := range []struct {
		name, version string
		count         int
	}{
		{"fresh install", "", 1}, {"V1", "1.18.30", 1}, {"V2", "opencode v2.0.4", 2}, {"unknown existing", "unknown", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			old := opencode.VersionRunnerOverride
			t.Cleanup(func() { opencode.VersionRunnerOverride = old })
			calls := 0
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				calls++
				return opencode.CommandOutput{Stdout: []byte(tt.version)}, nil
			}
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen = ScreenAgents
			m.Selection.Agents = []model.AgentID{model.AgentOpenCode}
			next, _ := m.Update(openCodePresentationCommand()())
			got := next.(Model)
			view := got.View()
			if n := strings.Count(view, "OpenCode V2 (beta)"); n != tt.count {
				t.Fatalf("beta count=%d want%d: %s", n, tt.count, view)
			}
			if tt.count == 1 && !strings.Contains(view, "opencode") {
				t.Fatal("V1 or unknown canonical label changed")
			}
			if !strings.Contains(view, "integration is being tested") {
				t.Fatal("missing fresh-install integration note")
			}
			if calls != 1 {
				t.Fatalf("render probed runtime: calls=%d", calls)
			}
			if len(got.Selection.Agents) != 1 || got.Selection.Agents[0] != model.AgentOpenCode {
				t.Fatal("presentation changed canonical selection")
			}
		})
	}
}
