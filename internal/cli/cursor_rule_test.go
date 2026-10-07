package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestCursorRulePostApplyVerification(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		ready         bool
	}{
		{"valid", "---\ndescription: Rules\nalwaysApply: true\n---\n\n## Rules\n", true},
		{"terminated document with comment", "---\nalwaysApply: true\n...\n# trailing comment\n---\n## Rules\n", true},
		{"malformed trailing content", "---\nalwaysApply: true\n...\ninvalid: [\n---\n## Rules\n", false},
		{"additional yaml document", "---\nalwaysApply: true\n...\n--- # second document\nalwaysApply: false\n---\n## Rules\n", false},
		{"missing frontmatter", "## Rules\n", false},
		{"missing activation", "---\ndescription: Rules\n---\n## Rules\n", false},
		{"disabled", "---\nalwaysApply: false\n---\n## Rules\n", false},
		{"string instead of boolean", "---\nalwaysApply: 'true'\n---\n## Rules\n", false},
		{"duplicate activation", "---\nalwaysApply: true\nalwaysApply: true\n---\n## Rules\n", false},
		{"malformed yaml", "---\nalwaysApply: [\n---\n## Rules\n", false},
		{"unclosed frontmatter", "---\nalwaysApply: true\n## Rules\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := cursor.NewAdapter().SystemPromptFile(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			report := runPostApplyVerification(postApplyVerificationInput{
				HomeDir: home, Scope: ScopeGlobal,
				Selection: model.Selection{Persona: model.PersonaGentleman},
				Resolved:  planner.ResolvedPlan{Agents: []model.AgentID{model.AgentCursor}, OrderedComponents: []model.ComponentID{model.ComponentPersona}},
			})
			if report.Ready != tc.ready {
				t.Fatalf("verification Ready = %v, want %v: %+v", report.Ready, tc.ready, report)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.content {
				t.Fatalf("verification must leave the rule unchanged: %v", err)
			}
		})
	}
}

func TestCursorInstallAndSyncActivation(t *testing.T) {
	home := convergenceTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := RunInstall([]string{"--agent", "cursor", "--preset", "minimal", "--scope", "global"}, system.DetectionResult{})
	if err != nil || !result.Verify.Ready {
		t.Fatalf("install = %+v, %v", result.Verify, err)
	}
	path := cursor.NewAdapter().SystemPromptFile(home)
	installed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const header = "---\ndescription: Gentle AI rules\nalwaysApply: true\n---\n\n"
	if !strings.HasPrefix(string(installed), header) {
		t.Fatal("install must generate Cursor activation frontmatter")
	}
	// Reproduce a legacy install without metadata, keeping every body byte.
	if err := os.WriteFile(path, installed[len(header):], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RunSync(nil); err != nil {
		t.Fatal(err)
	}
	healed, err := os.ReadFile(path)
	if err != nil || string(healed) != string(installed) {
		t.Fatalf("sync must restore metadata without changing the installed body: %v", err)
	}
	synced, err := RunSync(nil)
	if err != nil || synced.FilesChanged != 0 {
		t.Fatalf("repeated sync = %+v, %v", synced, err)
	}
}
