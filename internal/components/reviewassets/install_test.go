package reviewassets_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Captured by actual sdd.Inject in the disposable af4ce122 worktree, TestParityCapture.
// Review agents ship only to receipt-driven development runtimes: Cursor
// installs none, Kiro keeps Judgment Day, and Kimi keeps its main agent.
var installedHashes = map[model.AgentID]map[string]string{
	model.AgentClaudeCode: {
		"jd-fix-agent.md": "a62bf9736226b81512cdedecbf5b6888714e6ae9dd6881b220504b859d35a218", "jd-judge-a.md": "76452ecee8bcf44b07a9ddc2d95b1adf0ada0c569f3d984d4858375528b6abf5", "jd-judge-b.md": "314dce8eda219f1336a824d5b8d2671fd982610107f52baaefa4ec6861b7fdf5", "review-readability.md": "3a15838d28ff2f02fca684e7036116917311f8bbe72c9d09b929015363d36737", "review-refuter.md": "fa58bacaa0af136963db25d25abe7fccad91a87f3454024f3d283339308976db", "review-reliability.md": "cd667908097d9d02d9c9ee0211b3040c4b4d507fbf2b21a78dd4bdf3d09e97ef", "review-resilience.md": "a4a186feb1b5e22b9edf09db9967416ddf3b41b613a268b7a9ee86f6cdc35986", "review-risk.md": "5c10ef801d1bddad5ee4f3e310b750b3dd5b98087c0f4ef1893f94c1d40c16b1",
	},
	model.AgentKimi: {
		"gentleman.yaml": "4fd319f06d3381954556e7828c96bfc0901c428c1f00bacc407d1f63342349b1",
	},
	model.AgentKiroIDE: {
		"jd-fix-agent.md": "f60d26fa25e810c7c6d4526005b886aaca65d82337a51430206b22b605a3a265", "jd-judge-a.md": "ff22d142450a24db2beaf4f0e75f18ea5ec676721489523b99fe4c714aa9c4d0", "jd-judge-b.md": "7dc4cba47c0bad685485c4fdb2b67816edda30b7c8b3f6db426614b7b4357dc1",
	},
}

func containsFile(files []string, path string) bool {
	for _, file := range files {
		if file == path {
			return true
		}
	}
	return false
}

func TestAllUnknownNativeAgentsLeaveLedgerAbsent(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentKiroIDE)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	initial, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil || !initial.Changed {
		t.Fatalf("initial install: %+v, %v", initial, err)
	}
	ledger := filepath.Join(adapter.SubAgentsDir(home), reviewassets.OwnershipLedgerFilename)
	if err := os.Remove(ledger); err != nil {
		t.Fatal(err)
	}
	for _, guidance := range []string{"", "new guidance"} {
		result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{CodeGraphGuidanceMarkdown: guidance})
		if err != nil {
			t.Fatal(err)
		}
		if result.Changed || len(result.Files) != 0 || len(result.Skipped) != len(reviewassets.NativeAgentManifest[model.AgentKiroIDE]) {
			t.Fatalf("all-skipped install result = %+v", result)
		}
		if _, err := os.Lstat(ledger); !os.IsNotExist(err) {
			t.Fatalf("all-skipped ledger exists: %v", err)
		}
	}
}

func TestInstalledNativeAgentParity(t *testing.T) {
	count := 0
	for agent, expected := range installedHashes {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatal(err)
			}
			opts := reviewassets.InstallOptions{CodeGraphGuidanceMarkdown: "Use CodeGraph before broad filesystem search."}
			result, err := reviewassets.InstallNativeAgents(home, adapter, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Changed || len(result.Files) != len(expected)+1 || !containsFile(result.Files, filepath.Join(adapter.SubAgentsDir(home), reviewassets.OwnershipLedgerFilename)) {
				t.Fatalf("unexpected install result: %+v", result)
			}
			entries, err := os.ReadDir(adapter.SubAgentsDir(home))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != len(expected)+1 {
				t.Fatalf("installed %d entries, want %d agents plus ledger", len(entries), len(expected)+1)
			}
			for name, want := range expected {
				if strings.HasPrefix(name, "sdd-") {
					t.Fatal("SDD asset in retained fixture")
				}
				path := filepath.Join(adapter.SubAgentsDir(home), name)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
					t.Errorf("%s hash: got %s want %s", name, got, want)
				}
			}
			second, err := reviewassets.InstallNativeAgents(home, adapter, opts)
			if err != nil {
				t.Fatal(err)
			}
			if second.Changed || len(second.Files) != 0 {
				t.Fatalf("second install changed files: %+v", second)
			}
		})
		count += len(expected)
	}
	if count != 12 {
		t.Fatalf("fixture has %d paths, want 12", count)
	}
}

// TestInstallNativeAgentsRemovesLegacySDDAgents covers issue #5253: a v3.x
// install left eleven sdd-*.md agents in the Claude agents directory, and v4
// removed neither of them. The names are retired by name because the ownership
// ledger only records names v4 wrote, so it cannot vouch for these files.
func TestInstallNativeAgentsRemovesLegacySDDAgents(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []string{
		"sdd-init", "sdd-explore", "sdd-research", "sdd-propose", "sdd-spec",
		"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	}
	for _, name := range legacy {
		body := "---\nname: " + name + "\ndescription: Legacy v3 SDD phase agent.\n---\n\nLegacy v3 SDD agent.\n"
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A user-authored agent the installer never manages must survive.
	userAgent := filepath.Join(dir, "my-own-agent.md")
	if err := os.WriteFile(userAgent, []byte("user-owned bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range legacy {
		path := filepath.Join(dir, name+".md")
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("legacy SDD agent %s survived install: %v", name, err)
		}
		if !containsFile(result.Files, path) {
			t.Errorf("removed legacy agent %s not reported in result.Files", name)
		}
	}
	if _, err := os.Stat(userAgent); err != nil {
		t.Errorf("user-owned agent removed: %v", err)
	}

	// The removal is idempotent: a second install has nothing left to retire.
	second, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range legacy {
		if containsFile(second.Files, filepath.Join(dir, name+".md")) {
			t.Errorf("second install reported legacy agent %s", name)
		}
	}
}

// TestInstallNativeAgentsLeavesLegacySDDSymlinkAlone pins the non-regular-file
// guard: a symlink is never Gentle AI's to delete, even under a retired name.
func TestInstallNativeAgentsLeavesLegacySDDSymlinkAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs POSIX privileges")
	}
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "elsewhere.md")
	if err := os.WriteFile(target, []byte("not ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "sdd-apply.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlinked legacy agent was removed: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlinked legacy agent replaced by %v", info.Mode())
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("symlink target removed: %v", err)
	}
}
