package reviewassets_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Captured by actual sdd.Inject in the disposable af4ce122 worktree, TestParityCapture.
// The four Claude review-* lens hashes were re-pinned deliberately when the
// shared reviewerprovider.SeverityRules joined the rendered severity section.
// Review agents ship only to receipt-driven development runtimes: Cursor
// installs none, Kiro keeps Judgment Day, and Kimi keeps its main agent.
var installedHashes = map[model.AgentID]map[string]string{
	model.AgentClaudeCode: {
		"jd-fix-agent.md": "a62bf9736226b81512cdedecbf5b6888714e6ae9dd6881b220504b859d35a218", "jd-judge-a.md": "76452ecee8bcf44b07a9ddc2d95b1adf0ada0c569f3d984d4858375528b6abf5", "jd-judge-b.md": "314dce8eda219f1336a824d5b8d2671fd982610107f52baaefa4ec6861b7fdf5", "review-readability.md": "72633bfd1b4d25133fb553f7b286b2636602f13656e057cea7b19f80f97316b4", "review-refuter.md": "fa58bacaa0af136963db25d25abe7fccad91a87f3454024f3d283339308976db", "review-reliability.md": "13d281903a54872fc2aad88c50e7c694c2b8f2f9f6360fcdf5d1bed2a1719aaf", "review-resilience.md": "08bdeeb9f63229da1e03cd8a34c5aa473b7cd5789457c5b84e17ee5bfd2fb5bb", "review-risk.md": "623c693f1becfc80f056eb99f73bb78b13ac512b0cddbbdb29c4ba7dfdf3cfc2",
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
