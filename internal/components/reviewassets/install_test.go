package reviewassets_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// Captured by actual sdd.Inject in the disposable af4ce122 worktree, TestParityCapture.
var installedHashes = map[model.AgentID]map[string]string{
	model.AgentClaudeCode: {
		"jd-fix-agent.md": "a62bf9736226b81512cdedecbf5b6888714e6ae9dd6881b220504b859d35a218", "jd-judge-a.md": "76452ecee8bcf44b07a9ddc2d95b1adf0ada0c569f3d984d4858375528b6abf5", "jd-judge-b.md": "314dce8eda219f1336a824d5b8d2671fd982610107f52baaefa4ec6861b7fdf5", "review-readability.md": "3a15838d28ff2f02fca684e7036116917311f8bbe72c9d09b929015363d36737", "review-refuter.md": "fa58bacaa0af136963db25d25abe7fccad91a87f3454024f3d283339308976db", "review-reliability.md": "cd667908097d9d02d9c9ee0211b3040c4b4d507fbf2b21a78dd4bdf3d09e97ef", "review-resilience.md": "a4a186feb1b5e22b9edf09db9967416ddf3b41b613a268b7a9ee86f6cdc35986", "review-risk.md": "5c10ef801d1bddad5ee4f3e310b750b3dd5b98087c0f4ef1893f94c1d40c16b1",
	},
	model.AgentCursor: {
		"review-readability.md": "909498c1b0ed7d2b51a9febed5495e2c5234686309b9a42569baed8040ee60ee", "review-refuter.md": "73e5db3a40e80ccb0b15cf6465ffdc370da263afe68a79e821b3f13c82547301", "review-reliability.md": "ff628b20b1260bf66885b091fd97a5412f27c182e5db30c679a4bfcfa498b68a", "review-resilience.md": "f9f3066332936a69bfb155ac0d2f7a5d65033662a4ba78f0f9a3d4f4f4b24522", "review-risk.md": "ea5dffb44d50acd763d7584e35fc266ba3fdca3a4efd22f946c07383d2233905",
	},
	model.AgentKimi: {
		"gentleman.yaml": "4fd319f06d3381954556e7828c96bfc0901c428c1f00bacc407d1f63342349b1", "review-readability.md": "909498c1b0ed7d2b51a9febed5495e2c5234686309b9a42569baed8040ee60ee", "review-readability.yaml": "0d897d3fb2b461845c8975fcd1421026504f7e627f5d7f6199961365c6f0fe4e", "review-refuter.md": "73e5db3a40e80ccb0b15cf6465ffdc370da263afe68a79e821b3f13c82547301", "review-refuter.yaml": "3ca91e06cbe79e7133f3a471680a20eed43f0b17df215dddb3fa84869ac7f347", "review-reliability.md": "ff628b20b1260bf66885b091fd97a5412f27c182e5db30c679a4bfcfa498b68a", "review-reliability.yaml": "05d920268d3097e707ac88df3e722e2c631f4b4f28de03930ba65caef8b566b4", "review-resilience.md": "f9f3066332936a69bfb155ac0d2f7a5d65033662a4ba78f0f9a3d4f4f4b24522", "review-resilience.yaml": "f87c44f532d85fcca7f0cc28938c4ddd10271d2fa38e87c6e456788ac9835850", "review-risk.md": "ea5dffb44d50acd763d7584e35fc266ba3fdca3a4efd22f946c07383d2233905", "review-risk.yaml": "974ef76d2484bb8f6f87ab3edaeef15e82f9808a588251ac0b301e0cbf3cb620",
	},
	model.AgentKiroIDE: {
		"jd-fix-agent.md": "f60d26fa25e810c7c6d4526005b886aaca65d82337a51430206b22b605a3a265", "jd-judge-a.md": "ff22d142450a24db2beaf4f0e75f18ea5ec676721489523b99fe4c714aa9c4d0", "jd-judge-b.md": "7dc4cba47c0bad685485c4fdb2b67816edda30b7c8b3f6db426614b7b4357dc1", "review-readability.md": "90e66edd0abf2ca2a62a0dd3005d9a11df0f78b10abe8a68bca4929f699239fd", "review-refuter.md": "9572020e09b8b3a016e4a472d77c272dc5a71f5c19dbb7df7f1e8a69f447a783", "review-reliability.md": "e82460d8b8a3fbc4fd218613278806c2ca699f3d68cbf5612c44ea159c1ae540", "review-resilience.md": "a81923634935c4e5644cf7ec7753007e4d1f4b6dae3516e82e072b3c45b9af4c", "review-risk.md": "e5208aafcec8c7cfe9ab73b6bf9af1e39d4fd329d8bcd8bb717a728a84990d67",
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
	adapter, err := agents.NewAdapter(model.AgentCursor)
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
		if result.Changed || len(result.Files) != 0 || len(result.Skipped) != len(reviewassets.NativeAgentManifest[model.AgentCursor]) {
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
	if count != 32 {
		t.Fatalf("fixture has %d paths, want 32", count)
	}
}
