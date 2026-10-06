package legacyassets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// The fixtures under testdata/v3.7.0 are real v3.7.0 renders: the Claude and
// Kiro files are that release's golden outputs, and the Cursor and Kimi files
// are that release's assets plus the managed blocks the renderer appended.
func releasedRender(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "v3.7.0", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRetiredSDDAgentFilesCoverEveryReleasedFamily(t *testing.T) {
	for _, tc := range []struct {
		agent model.AgentID
		want  []string
	}{
		{model.AgentClaudeCode, []string{"sdd-apply.md", "sdd-verify.md", "sdd-research.md"}},
		{model.AgentKiroIDE, []string{"sdd-apply.md", "sdd-onboard.md"}},
		{model.AgentCursor, []string{"sdd-apply.md", "sdd-init.md"}},
		{model.AgentKimi, []string{"sdd-apply.md", "sdd-apply.yaml"}},
	} {
		got := RetiredSDDAgentFiles(tc.agent)
		for _, name := range tc.want {
			if !slices.Contains(got, name) {
				t.Errorf("%s retired SDD agents = %v, missing %s", tc.agent, got, name)
			}
		}
		for _, name := range got {
			if !strings.HasPrefix(name, "sdd-") {
				t.Errorf("%s inventory names non-SDD file %s", tc.agent, name)
			}
		}
	}
	if got := RetiredSDDAgentFiles(model.AgentCodex); len(got) != 0 {
		t.Errorf("Codex never installed native SDD agents, got %v", got)
	}
}

func TestOwnsRetiredSDDAgentAcceptsReleasedRenders(t *testing.T) {
	for _, tc := range []struct {
		agent   model.AgentID
		name    string
		fixture string
	}{
		{model.AgentClaudeCode, "sdd-apply.md", "claude-sdd-apply.md"},
		{model.AgentKiroIDE, "sdd-apply.md", "kiro-sdd-apply.md"},
		{model.AgentCursor, "sdd-apply.md", "cursor-sdd-apply.md"},
		{model.AgentKimi, "sdd-apply.md", "kimi-sdd-apply.md"},
		{model.AgentKimi, "sdd-apply.yaml", "kimi-sdd-apply.yaml"},
	} {
		if !OwnsRetiredSDDAgent(tc.agent, tc.name, []byte(releasedRender(t, tc.fixture))) {
			t.Errorf("%s %s: released render not recognized as Gentle AI owned", tc.agent, tc.name)
		}
	}
}

func TestOwnsRetiredSDDAgentAcceptsEverySubstitutedField(t *testing.T) {
	render := releasedRender(t, "claude-sdd-apply.md")
	variant := strings.Replace(render, "model: sonnet\n", "model: opus\neffort: high\n", 1)
	variant = strings.Replace(variant, "mcp__plugin_engram_engram__mem_update\n", "mcp__plugin_engram_engram__mem_update, mcp__codegraph__codegraph_explore\n", 1)
	variant += "\n<!-- gentle-ai:codegraph-guidance -->\n## CodeGraph\n\nmachine-specific guidance\n<!-- /gentle-ai:codegraph-guidance -->\n"
	if variant == render || !strings.Contains(variant, "effort: high") || !strings.Contains(variant, "codegraph_explore") {
		t.Fatal("fixture substitution did not apply")
	}
	if !OwnsRetiredSDDAgent(model.AgentClaudeCode, "sdd-apply.md", []byte(variant)) {
		t.Error("render with another model, effort, CodeGraph grant, and guidance block not owned")
	}
	if !OwnsRetiredSDDAgent(model.AgentClaudeCode, "sdd-apply.md", []byte(strings.ReplaceAll(render, "\n", "\r\n"))) {
		t.Error("CRLF line endings are not authored content")
	}
	kiro := strings.Replace(releasedRender(t, "kiro-sdd-apply.md"), `tools: ["@builtin", "@engram"]`, `tools: ["@builtin", "@engram", "@codegraph"]`, 1)
	if !OwnsRetiredSDDAgent(model.AgentKiroIDE, "sdd-apply.md", []byte(strings.Replace(kiro, "model: auto", "model: claude-opus-4.6", 1))) {
		t.Error("Kiro render with another model and CodeGraph grant not owned")
	}
}

func TestOwnsRetiredSDDAgentRejectsUserAuthoredBytes(t *testing.T) {
	render := releasedRender(t, "claude-sdd-apply.md")
	for name, content := range map[string]string{
		"body edit":           strings.Replace(render, "Do NOT delegate further.", "Delegate when useful.", 1),
		"added body line":     strings.Replace(render, "\n---\n", "\n---\n\nMy team rule.\n", 1),
		"description edit":    strings.Replace(render, "Implement code changes", "Implement my changes", 1),
		"tools edit":          strings.Replace(render, "tools: Read,", "tools: Read, WebFetch,", 1),
		"unmanaged appendix":  render + "\n## My notes\n",
		"unclosed managed id": render + "\n<!-- gentle-ai:custom -->\nmine\n",
		"empty":               "",
	} {
		if OwnsRetiredSDDAgent(model.AgentClaudeCode, "sdd-apply.md", []byte(content)) {
			t.Errorf("%s: user-authored bytes treated as owned", name)
		}
	}
	if OwnsRetiredSDDAgent(model.AgentClaudeCode, "sdd-verify.md", []byte(render)) {
		t.Error("ownership must be proven for the same file name")
	}
	if OwnsRetiredSDDAgent(model.AgentCursor, "sdd-apply.md", []byte(render)) {
		t.Error("ownership must be proven for the same agent family")
	}
}

func TestRetireSDDAgentsRemovesOnlyOwnedFilesAndIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	owned := write("sdd-apply.md", releasedRender(t, "claude-sdd-apply.md"))
	edited := write("sdd-verify.md", "---\nname: sdd-verify\n---\nmy own verifier\n")
	kept := write("review-risk.md", "retained owner\n")
	if err := os.Symlink(owned, filepath.Join(dir, "sdd-explore.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	result, err := RetireSDDAgents(model.AgentClaudeCode, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Removed, []string{owned}) {
		t.Errorf("removed = %v, want only %s", result.Removed, owned)
	}
	if want := []string{filepath.Join(dir, "sdd-explore.md"), edited}; !slices.Equal(result.Preserved, want) {
		t.Errorf("preserved = %v, want %v", result.Preserved, want)
	}
	for _, path := range []string{edited, kept, filepath.Join(dir, "sdd-explore.md")} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s removed: %v", path, err)
		}
	}
	if _, err := os.Lstat(owned); !os.IsNotExist(err) {
		t.Errorf("owned retired agent survived: %v", err)
	}

	again, err := RetireSDDAgents(model.AgentClaudeCode, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Removed) != 0 {
		t.Errorf("second retirement removed %v", again.Removed)
	}
	if action := PreservedSDDAgentAction(edited); !strings.Contains(action, edited) || !strings.Contains(action, "move or delete it") {
		t.Errorf("preserved action is not actionable: %s", action)
	}
}

func TestRetireSDDAgentsKeepsKimiPairsTogether(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".kimi", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// sdd-apply: both halves are released bytes, so the pair is removed.
	write("sdd-apply.yaml", releasedRender(t, "kimi-sdd-apply.yaml"))
	write("sdd-apply.md", releasedRender(t, "kimi-sdd-apply.md"))
	// sdd-verify: an owned-looking YAML still points at the user's prompt, so
	// removing it alone would break the user's agent.
	write("sdd-verify.yaml", releasedRender(t, "kimi-sdd-verify.yaml"))
	write("sdd-verify.md", "my verifier prompt\n")
	if !OwnsRetiredSDDAgent(model.AgentKimi, "sdd-verify.yaml", []byte(releasedRender(t, "kimi-sdd-verify.yaml"))) {
		t.Fatal("released Kimi YAML not owned; the pair case would prove nothing")
	}

	result, err := RetireSDDAgents(model.AgentKimi, dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(dir, "sdd-apply.md"), filepath.Join(dir, "sdd-apply.yaml")}; !slices.Equal(result.Removed, want) {
		t.Errorf("removed = %v, want %v", result.Removed, want)
	}
	for _, name := range []string{"sdd-verify.yaml", "sdd-verify.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s of a partially user-owned pair removed: %v", name, err)
		}
	}
}

func TestRetireSDDAgentsFollowsSymlinkedAgentsDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "dotfiles", "agents")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "sdd-apply.md"), []byte(releasedRender(t, "cursor-sdd-apply.md")), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".cursor", "agents")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result, err := RetireSDDAgents(model.AgentCursor, link)
	if err != nil || len(result.Removed) != 1 {
		t.Fatalf("symlinked agents directory: %+v, %v", result, err)
	}
}

func TestRetireSDDAgentsMissingDirectoryIsNoOp(t *testing.T) {
	result, err := RetireSDDAgents(model.AgentKiroIDE, filepath.Join(t.TempDir(), "absent"))
	if err != nil || len(result.Removed) != 0 || len(result.Preserved) != 0 {
		t.Fatalf("missing directory: %+v, %v", result, err)
	}
}
