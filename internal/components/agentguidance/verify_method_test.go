package agentguidance

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Resolve the actual installed reference, not a repository-only source path.
func installedVerifierMethod(t *testing.T, home string, agent model.AgentID) string {
	t.Helper()
	adapter, err := agents.NewAdapter(agent)
	if err != nil {
		t.Fatal(err)
	}
	result, err := skills.Inject(home, adapter, []model.SkillID{model.SkillGoTesting})
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.SupportsSkills() || adapter.SkillsDir(home) == "" {
		if len(result.Files) != 0 {
			t.Fatal("unsupported skills capability wrote files")
		}
		return ""
	}
	path := filepath.Join(adapter.SkillsDir(home), "_shared", "verify-method.md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("installed verification method: %v", err)
	}
	want, err := fs.ReadFile(assets.FS, "skills/_shared/verify-method.md")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("installed method differs from embedded source: %v", err)
	}
	return string(got)
}

func TestInstalledVerifierMethodHandoff(t *testing.T) {
	for _, agent := range orchestratorRuntimes(t) {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			method := installedVerifierMethod(t, home, agent)
			if method == "" {
				return // Unsupported adapters do not gain a skill installation route.
			}
			for _, capability := range []string{"", "small"} {
				result, err := InjectRoutingWithOptions(home, agent, RoutingOptions{OrchestratorCapability: capability})
				if err != nil {
					t.Fatal(err)
				}
				prompt := deliveredGuidance(t, result.Files[0])
				for _, want := range []string{"_shared/verify-method.md", "change kind", "pastes or references", "returned probes to the writer"} {
					if !strings.Contains(prompt, want) {
						t.Errorf("capability %q lacks handoff %q", capability, want)
					}
				}
			}
			for _, want := range []string{"## Spec-derived probes", "## Evidence by change kind", "## Severity", "stored data stays byte-identical", "baseline copy the parent named", "EVERY spec item", "ready-to-commit regression tests", "every removed or rewritten sentence", "user's exact failing command", "mktemp -d", "No network, no installs", "never delete it yourself"} {
				if !strings.Contains(method, want) {
					t.Errorf("installed method lacks %q", want)
				}
			}
		})
	}
}
