package reviewassets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func claudeAgentsDir(t *testing.T) (string, agents.Adapter, string) {
	t.Helper()
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	return home, adapter, adapter.SubAgentsDir(home)
}

func claudeFrontmatter(t *testing.T, path string) (map[string]string, string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("%s has no frontmatter", path)
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("%s has unterminated frontmatter", path)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(content[4:4+end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		fields[key] = strings.TrimSpace(value)
	}
	return fields, content[4+end+5:]
}

// #5611: an Explore or general-purpose override is written only when the user
// picked a model for that built-in row, and it keeps Explore read-only.
func TestClaudeBuiltInOverridesFollowExplicitPicks(t *testing.T) {
	home, adapter, dir := claudeAgentsDir(t)
	explore := filepath.Join(dir, "Explore.md")
	general := filepath.Join(dir, "general-purpose.md")

	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{
		ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{"default": {Model: model.ClaudeModelOpus}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{explore, general} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s installed without an explicit pick (err %v)", path, err)
		}
	}

	picked := reviewassets.InstallOptions{ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
		"builtin-explore":         {Model: model.ClaudeModelHaiku},
		"builtin-general-purpose": {Model: model.ClaudeModelSonnet, Effort: model.ClaudeEffortLow},
	}}
	result, err := reviewassets.InstallNativeAgents(home, adapter, picked)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPath(result.Files, explore) || !containsPath(result.Files, general) {
		t.Fatalf("overrides not reported as installed: %+v", result)
	}
	fields, _ := claudeFrontmatter(t, explore)
	if fields["name"] != "Explore" || fields["model"] != "haiku" {
		t.Fatalf("Explore frontmatter = %v", fields)
	}
	if _, ok := fields["effort"]; ok {
		t.Fatalf("Explore wrote an effort for haiku: %v", fields)
	}
	for _, tool := range []string{"Agent", "Edit", "Write", "NotebookEdit"} {
		if !strings.Contains(fields["disallowedTools"], tool) {
			t.Fatalf("Explore override allows %s: %v", tool, fields)
		}
	}
	fields, _ = claudeFrontmatter(t, general)
	if fields["name"] != "general-purpose" || fields["model"] != "sonnet" || fields["effort"] != "low" {
		t.Fatalf("general-purpose frontmatter = %v", fields)
	}
	if _, ok := fields["tools"]; ok {
		t.Fatalf("general-purpose override restricts tools: %v", fields)
	}
	ledger := readOwnershipLedger(t, filepath.Join(dir, reviewassets.OwnershipLedgerFilename))
	if ledger.Files["Explore.md"] == "" || ledger.Files["general-purpose.md"] == "" {
		t.Fatalf("overrides not recorded as owned: %v", ledger.Files)
	}

	again, err := reviewassets.InstallNativeAgents(home, adapter, picked)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed {
		t.Fatalf("second install with the same picks changed files: %+v", again)
	}

	cleared, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{explore, general} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s kept after its pick was cleared (err %v)", path, err)
		}
		if !containsPath(cleared.Files, path) {
			t.Fatalf("removal of %s not reported: %+v", path, cleared)
		}
	}
	ledger = readOwnershipLedger(t, filepath.Join(dir, reviewassets.OwnershipLedgerFilename))
	if _, ok := ledger.Files["Explore.md"]; ok {
		t.Fatalf("cleared override still owned: %v", ledger.Files)
	}
}

// #5611: a user's own Explore agent is never overwritten or removed.
func TestClaudeBuiltInOverridePreservesUserAgent(t *testing.T) {
	home, adapter, dir := claudeAgentsDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	explore := filepath.Join(dir, "Explore.md")
	const mine = "---\nname: Explore\nmodel: opus\n---\nmine\n"
	if err := os.WriteFile(explore, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	picked := reviewassets.InstallOptions{ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
		"builtin-explore": {Model: model.ClaudeModelHaiku},
	}}
	result, err := reviewassets.InstallNativeAgents(home, adapter, picked)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPath(result.Skipped, explore) {
		t.Fatalf("user Explore not reported as preserved: %+v", result)
	}
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(explore); err != nil || string(got) != mine {
		t.Fatalf("user Explore changed: %q, %v", got, err)
	}
}
