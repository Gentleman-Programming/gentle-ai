package reviewassets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// rddAgentNames are the native agents receipt-driven development owns.
var rddAgentNames = []string{"review-risk", "review-readability", "review-reliability", "review-resilience", "review-refuter", "review-validator"}

func isRDDAgentFile(name string) bool {
	base := strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".yaml")
	for _, candidate := range rddAgentNames {
		if base == candidate {
			return true
		}
	}
	return false
}

func TestNativeAgentManifestShipsReviewAgentsOnlyToRDDRuntimes(t *testing.T) {
	t.Parallel()

	for agent, names := range NativeAgentManifest {
		for _, name := range names {
			if isRDDAgentFile(name) && !model.SupportsReceiptDrivenDevelopment(agent) {
				t.Errorf("%s installs RDD agent %s but does not support receipt-driven development", agent, name)
			}
		}
	}
	for agent, names := range RetiredNativeAgentManifest {
		if model.SupportsReceiptDrivenDevelopment(agent) {
			t.Errorf("%s supports receipt-driven development but lists retired review agents", agent)
		}
		for _, name := range names {
			if !isRDDAgentFile(name) {
				t.Errorf("%s retires %s, which is not an RDD agent", agent, name)
			}
		}
	}
	for _, want := range []string{"jd-fix-agent.md", "jd-judge-a.md", "jd-judge-b.md"} {
		for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentKiroIDE} {
			if !containsName(NativeAgentManifest[agent], want) {
				t.Errorf("%s lost Judgment Day agent %s", agent, want)
			}
		}
	}
	if !containsName(NativeAgentManifest[model.AgentKimi], "gentleman.yaml") {
		t.Error("Kimi lost its main agent")
	}
	for _, want := range []string{"review-risk.md", "review-refuter.md"} {
		if !containsName(NativeAgentManifest[model.AgentClaudeCode], want) {
			t.Errorf("Claude Code lost RDD agent %s", want)
		}
	}
}

func TestFreshInstallOnNonRDDRuntimesInstallsNoReviewAgents(t *testing.T) {
	t.Parallel()

	for _, agent := range []model.AgentID{model.AgentCursor, model.AgentKiroIDE, model.AgentKimi} {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			if _, err := InstallNativeAgents(home, adapter, InstallOptions{}); err != nil {
				t.Fatalf("InstallNativeAgents(%s) error = %v", agent, err)
			}
			entries, err := os.ReadDir(adapter.SubAgentsDir(home))
			if os.IsNotExist(err) {
				entries = nil
			} else if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if isRDDAgentFile(entry.Name()) {
					t.Errorf("fresh %s install wrote RDD agent %s", agent, entry.Name())
				}
			}
			for _, name := range NativeAgentManifest[agent] {
				if _, err := os.Stat(filepath.Join(adapter.SubAgentsDir(home), name)); err != nil {
					t.Errorf("fresh %s install is missing %s: %v", agent, name, err)
				}
			}
		})
	}
}

// TestUpgradeRemovesOwnedRetiredReviewAgents seeds what an earlier release
// installed on runtimes without receipt-driven development and proves the
// upgrade removes only what Gentle AI owns.
func TestUpgradeRemovesOwnedRetiredReviewAgents(t *testing.T) {
	t.Parallel()

	for _, agent := range []model.AgentID{model.AgentCursor, model.AgentKiroIDE, model.AgentKimi} {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			dir := adapter.SubAgentsDir(home)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}

			write := func(name, content string) string {
				t.Helper()
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			}
			managed := func(name string) string {
				t.Helper()
				content, err := renderNativeAgent(adapter, name, InstallOptions{})
				if err != nil {
					t.Fatalf("render %s: %v", name, err)
				}
				return content
			}

			// Recorded in the ledger with the bytes Gentle AI installed.
			ledgerOwned := write("review-risk.md", "review-risk as an earlier release rendered it\n")
			// Unrecorded, but byte-identical to the managed asset.
			assetOwned := write("review-refuter.md", managed("review-refuter.md"))
			// Recorded, then edited by the user.
			userEdited := write("review-reliability.md", "user edited review-reliability\n")
			// Never installed by Gentle AI.
			userOwned := write("review-readability.md", "my own review-readability agent\n")

			ledger := ownershipLedger{Version: ownershipVersion, Files: map[string]string{
				"review-risk.md":        installedHash([]byte("review-risk as an earlier release rendered it\n")),
				"review-reliability.md": installedHash([]byte("review-reliability as an earlier release rendered it\n")),
			}}
			for _, name := range NativeAgentManifest[agent] {
				content := managed(name)
				write(name, content)
				ledger.Files[name] = installedHash([]byte(content))
			}
			var kimiYAML string
			if agent == model.AgentKimi {
				kimiYAML = write("review-risk.yaml", managed("review-risk.yaml"))
				ledger.Files["review-risk.yaml"] = installedHash([]byte(managed("review-risk.yaml")))
			}
			encoded, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			ledgerFile := write(OwnershipLedgerFilename, string(encoded))
			if err := os.Chmod(ledgerFile, 0o600); err != nil {
				t.Fatal(err)
			}

			result, err := InstallNativeAgents(home, adapter, InstallOptions{})
			if err != nil {
				t.Fatalf("InstallNativeAgents(%s) error = %v", agent, err)
			}

			removed := []string{ledgerOwned, assetOwned}
			if kimiYAML != "" {
				removed = append(removed, kimiYAML)
			}
			for _, path := range removed {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Errorf("owned retired agent %s survived the upgrade: %v", path, err)
				}
				if !containsName(result.Files, path) {
					t.Errorf("removed agent %s is not reported as changed: %+v", path, result)
				}
			}
			for path, want := range map[string]string{
				userEdited: "user edited review-reliability\n",
				userOwned:  "my own review-readability agent\n",
			} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Errorf("user-owned %s = %q, %v; want preserved", path, got, err)
				}
				if containsName(result.Skipped, path) {
					t.Errorf("retired user agent %s is reported as a skipped update: %+v", path, result)
				}
			}
			for _, name := range NativeAgentManifest[agent] {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Errorf("retained agent %s was removed: %v", name, err)
				}
			}

			after, _, err := readOwnership(ledgerFile, NativeAgentManifest[agent])
			if len(NativeAgentManifest[agent]) == 0 {
				if _, statErr := os.Lstat(ledgerFile); !os.IsNotExist(statErr) {
					t.Fatalf("empty ownership ledger survived: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ledger after upgrade still names retired agents: %v", err)
			}
			if len(after.Files) != len(NativeAgentManifest[agent]) {
				t.Errorf("ledger after upgrade = %v, want only retained agents", after.Files)
			}
			info, err := os.Stat(ledgerFile)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("ledger mode = %v, want 0600 preserved", info.Mode().Perm())
			}

			second, err := InstallNativeAgents(home, adapter, InstallOptions{})
			if err != nil || second.Changed {
				t.Fatalf("second install = %+v, %v; want no change", second, err)
			}
		})
	}
}

func TestRetiredReviewAgentCleanupSkipsNonRegularFiles(t *testing.T) {
	t.Parallel()

	adapter, err := agents.NewAdapter(model.AgentCursor)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "user-agent.md")
	if err := os.WriteFile(target, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "review-risk.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallNativeAgents(home, adapter, InstallOptions{}); err != nil {
		t.Fatalf("InstallNativeAgents error = %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("symlinked user agent was removed: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "safe" {
		t.Fatalf("symlink target modified: %q", got)
	}
}

func TestCursorInstallCreatesSecurityAgent(t *testing.T) {
	t.Parallel()

	adapter, err := agents.NewAdapter(model.AgentCursor)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	result, err := InstallNativeAgents(home, adapter, InstallOptions{})
	if err != nil {
		t.Fatalf("InstallNativeAgents(cursor) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("cursor install reported no change on a fresh home")
	}
	path := filepath.Join(adapter.SubAgentsDir(home), "gentle-ai-security.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cursor install missing gentle-ai-security.md: %v", err)
	}
}

func TestNativeAgentPathsCoverRetiredAgents(t *testing.T) {
	t.Parallel()

	for _, agent := range []model.AgentID{model.AgentCursor, model.AgentKiroIDE, model.AgentKimi} {
		names := NativeAgentFileNames(agent)
		for _, name := range RetiredNativeAgentManifest[agent] {
			if !containsName(names, name) {
				t.Errorf("%s backup names miss retired agent %s", agent, name)
			}
		}
		for _, name := range NativeAgentManifest[agent] {
			if !containsName(names, name) {
				t.Errorf("%s backup names miss installed agent %s", agent, name)
			}
		}
	}
	if !NativeAgentsSupported(model.AgentCursor) || NativeAgentsSupported(model.AgentCodex) {
		t.Fatal("native agent support must cover cleanup-only runtimes and nothing else")
	}
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
