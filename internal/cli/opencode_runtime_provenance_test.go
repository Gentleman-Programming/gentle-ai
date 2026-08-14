package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestRunSyncRecordsOpenCodeRuntimeProvenanceWhenReviewPluginIsCurrent(t *testing.T) {
	home := t.TempDir()
	plugin := filepath.Join(home, ".config", "opencode", "plugins", "review-result-artifacts.ts")
	if err := os.MkdirAll(filepath.Dir(plugin), 0o755); err != nil {
		t.Fatal(err)
	}
	wantPlugin := []byte(assets.MustRead("opencode/plugins/review-result-artifacts.ts"))
	if err := os.WriteFile(plugin, wantPlugin, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := RunSyncWithSelection(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}); err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}
	gotPlugin, err := os.ReadFile(plugin)
	if err != nil || string(gotPlugin) != string(wantPlugin) {
		t.Fatalf("managed plugin changed while already current: %q, %v", gotPlugin, err)
	}
	encoded, err := os.ReadFile(state.Path(home))
	if err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		OpenCodeRuntimeProvenance struct {
			Executable string `json:"executable"`
			SHA256     string `json:"sha256"`
			Version    string `json:"version"`
		} `json:"opencode_runtime_provenance"`
	}
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(persisted.OpenCodeRuntimeProvenance.Executable) ||
		!strings.HasPrefix(persisted.OpenCodeRuntimeProvenance.SHA256, "sha256:") ||
		persisted.OpenCodeRuntimeProvenance.Version == "" {
		t.Fatalf("sync did not record a complete exact runtime pin: %#v", persisted.OpenCodeRuntimeProvenance)
	}
}
