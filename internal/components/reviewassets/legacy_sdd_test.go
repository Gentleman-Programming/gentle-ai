package reviewassets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

const legacySDDFixtureDir = "testdata/legacy_sdd/v3.7.0"

// renderV3ClaudeSDDAgent reproduces, step for step, how the v3.7.0
// sdd.Inject subagent copy loop (internal/components/sdd/inject.go, step 3c)
// wrote one Claude Code SDD agent. The template and both contracts are the
// v3.7.0 bytes from git history, stored under testdata.
func renderV3ClaudeSDDAgent(t *testing.T, name, claudeModel, effort, guidance string) string {
	t.Helper()
	read := func(file string) string {
		data, err := os.ReadFile(filepath.Join(legacySDDFixtureDir, file))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	content := read(name)
	content = strings.ReplaceAll(content, "{{CLAUDE_MODEL}}", claudeModel)
	if effort == "" {
		content = strings.ReplaceAll(content, "{{CLAUDE_EFFORT_FRONTMATTER}}\n", "")
	} else {
		content = strings.ReplaceAll(content, "{{CLAUDE_EFFORT_FRONTMATTER}}", "effort: "+effort)
	}
	content = engramToolPlaceholder.ReplaceAllString(content, "mcp__engram__$1, mcp__plugin_engram_engram__$1")
	if guidance != "" {
		end := strings.Index(content, "\n---\n")
		lines := strings.Split(content[:end], "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "tools:") {
				lines[i] = line + ", mcp__codegraph__codegraph_explore"
			}
		}
		content = strings.Join(lines, "\n") + content[end:]
		content = filemerge.InjectMarkdownSection(content, "codegraph-guidance", guidance)
	}
	content = filemerge.InjectMarkdownSection(content, "agent-language-contract", strings.TrimSpace(read("agent-language-contract.md")))
	return filemerge.InjectMarkdownSection(content, "remote-authorization", strings.TrimSpace(read("remote-authorization-contract.md")))
}

func claudeAgentsDir(t *testing.T) (agents.Adapter, string, string) {
	t.Helper()
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return adapter, home, dir
}

func TestInstallRemovesUnmodifiedV3LegacySDDAgents(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, model, effort, guidance string
	}{
		{name: "opus with effort and codegraph", model: "opus", effort: "high", guidance: "## CodeGraph\n\nUse CodeGraph first.\n"},
		{name: "sonnet with default effort", model: "sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			adapter, home, dir := claudeAgentsDir(t)
			legacy := []string{"sdd-apply.md", "sdd-verify.md"}
			for _, name := range legacy {
				content := renderV3ClaudeSDDAgent(t, name, tc.model, tc.effort, tc.guidance)
				if !strings.Contains(content, "model: "+tc.model+"\n") {
					t.Fatalf("fixture %s lacks model line", name)
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			first, err := InstallNativeAgents(home, adapter, InstallOptions{})
			if err != nil {
				t.Fatalf("InstallNativeAgents() error = %v", err)
			}
			for _, name := range legacy {
				path := filepath.Join(dir, name)
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Errorf("v3 legacy agent %s survived: %v", name, err)
				}
				if !containsName(first.Files, path) {
					t.Errorf("removed legacy agent %s not listed in Files %v", path, first.Files)
				}
				if containsName(first.Skipped, path) {
					t.Errorf("removed legacy agent %s listed as Skipped", path)
				}
			}

			second, err := InstallNativeAgents(home, adapter, InstallOptions{})
			if err != nil {
				t.Fatalf("second InstallNativeAgents() error = %v", err)
			}
			if second.Changed || len(second.Files) != 0 || len(second.Skipped) != 0 {
				t.Fatalf("second run = %+v; want no-op", second)
			}
		})
	}
}

func TestInstallPreservesLegacySDDAgentsItCannotProveOwned(t *testing.T) {
	t.Parallel()

	v3 := func(t *testing.T) string { return renderV3ClaudeSDDAgent(t, "sdd-apply.md", "opus", "", "") }
	for _, tc := range []struct {
		name    string
		content func(t *testing.T) string
		link    bool
	}{
		{name: "edited body", content: func(t *testing.T) string {
			return strings.Replace(v3(t), "Do NOT delegate further.", "Delegate when it helps.", 1)
		}},
		{name: "added frontmatter key", content: func(t *testing.T) string {
			return strings.Replace(v3(t), "model: opus\n", "model: opus\ncolor: blue\n", 1)
		}},
		{name: "appended user section", content: func(t *testing.T) string {
			return v3(t) + "\n## My rules\n\nAlways run make lint.\n"
		}},
		{name: "user agent under the same name", content: func(*testing.T) string {
			return "---\nname: sdd-apply\n---\n\nMy own apply agent.\n"
		}},
		{name: "symlink to a v3 render", content: v3, link: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			adapter, home, dir := claudeAgentsDir(t)
			path := filepath.Join(dir, "sdd-apply.md")
			content := []byte(tc.content(t))
			if tc.link {
				target := filepath.Join(t.TempDir(), "sdd-apply.md")
				if err := os.WriteFile(target, content, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, content, 0o644); err != nil {
				t.Fatal(err)
			}
			custom := filepath.Join(dir, "sdd-custom.md")
			if err := os.WriteFile(custom, []byte("my custom agent"), 0o644); err != nil {
				t.Fatal(err)
			}

			for run := 1; run <= 2; run++ {
				result, err := InstallNativeAgents(home, adapter, InstallOptions{})
				if err != nil {
					t.Fatalf("run %d: InstallNativeAgents() error = %v", run, err)
				}
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, content) {
					t.Fatalf("run %d: preserved agent changed: %q, %v", run, got, err)
				}
				if !containsName(result.Skipped, path) {
					t.Errorf("run %d: preserved legacy agent not reported in Skipped %v", run, result.Skipped)
				}
				if containsName(result.Files, path) {
					t.Errorf("run %d: preserved legacy agent listed in Files", run)
				}
				if containsName(result.Skipped, custom) || containsName(result.Files, custom) {
					t.Errorf("run %d: unmanaged sdd-custom.md was reported: %+v", run, result)
				}
				if got, err := os.ReadFile(custom); err != nil || string(got) != "my custom agent" {
					t.Fatalf("run %d: unmanaged sdd-custom.md changed: %q, %v", run, got, err)
				}
			}
		})
	}
}

func TestLegacySDDCleanupIsClaudeOnly(t *testing.T) {
	t.Parallel()

	adapter, err := agents.NewAdapter(model.AgentKiroIDE)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sdd-apply.md")
	content := renderV3ClaudeSDDAgent(t, "sdd-apply.md", "opus", "", "")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := InstallNativeAgents(home, adapter, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != content {
		t.Fatalf("Kiro sdd-apply.md changed: %v", err)
	}
	if containsName(result.Files, path) || containsName(result.Skipped, path) {
		t.Fatalf("Kiro run reported Claude legacy cleanup: %+v", result)
	}
}

func TestNativeAgentFileNamesSnapshotsLegacySDDAgents(t *testing.T) {
	t.Parallel()

	names := NativeAgentFileNames(model.AgentClaudeCode)
	for _, want := range []string{
		"sdd-apply.md", "sdd-archive.md", "sdd-design.md", "sdd-explore.md", "sdd-init.md", "sdd-onboard.md",
		"sdd-propose.md", "sdd-research.md", "sdd-spec.md", "sdd-tasks.md", "sdd-verify.md",
	} {
		if !containsName(names, want) {
			t.Errorf("NativeAgentFileNames(claude-code) misses legacy %s, so rollback cannot restore it", want)
		}
	}
}
