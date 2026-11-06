package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

// TestAntigravityEngramBackupTargetsIncludeLegacyPlugin pins that Engram
// injection, which retires the legacy plugin under antigravity-cli/plugins
// (#5930), never deletes a file the install or sync snapshot does not cover.
func TestAntigravityEngramBackupTargetsIncludeLegacyPlugin(t *testing.T) {
	home := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentAntigravity}, Components: []model.ComponentID{model.ComponentEngram}}

	install, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	if err != nil {
		t.Fatalf("installBackupTargets() error = %v", err)
	}
	sync, err := syncBackupTargetsScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents))
	if err != nil {
		t.Fatalf("syncBackupTargetsScoped() error = %v", err)
	}
	for _, path := range engram.AntigravityLegacyPluginAssetPaths(home) {
		if !containsPath(install, path) {
			t.Fatalf("install backup targets missing legacy plugin asset %q: %v", path, install)
		}
		if !containsPath(sync, path) {
			t.Fatalf("sync backup targets missing legacy plugin asset %q: %v", path, sync)
		}
	}
}
