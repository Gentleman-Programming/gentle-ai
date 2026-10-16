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
	"gopkg.in/yaml.v3"
)

// Captured by actual sdd.Inject in the disposable af4ce122 worktree, TestParityCapture.
// The four Claude review-* lens hashes were re-pinned deliberately when the
// shared reviewerprovider.SeverityRules joined the rendered severity section,
// and again when those rules gained the observable-harm qualification.
// Review agents ship only to receipt-driven development runtimes: Cursor
// and Kiro receive only Judgment Day, and Kimi keeps its main agent.
var installedHashes = map[model.AgentID]map[string]string{
	model.AgentClaudeCode: {
		"jd-fix-agent.md": "a62bf9736226b81512cdedecbf5b6888714e6ae9dd6881b220504b859d35a218", "jd-judge-a.md": "76452ecee8bcf44b07a9ddc2d95b1adf0ada0c569f3d984d4858375528b6abf5", "jd-judge-b.md": "314dce8eda219f1336a824d5b8d2671fd982610107f52baaefa4ec6861b7fdf5", "review-readability.md": "412fd47787837836131d8f1509264f70cebcfd6b769be9dd9fd8d12c8f42ab80", "review-refuter.md": "fa58bacaa0af136963db25d25abe7fccad91a87f3454024f3d283339308976db", "review-reliability.md": "69b837ddbac649009efd24d5db44b8c39587d11ab6e281fe80cd10e42b466958", "review-resilience.md": "702dc6e5d190e0ac77d796fdbb2b1174dea3bdde8c74ee91dca5ca0071c20824", "review-risk.md": "73ab3273f3a4c589d4ffc8e95a50381dc2e87f08fc047d05179490c383ac4b35",
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

// This pins the installed YAML configuration, not Claude's runtime enforcement.
func TestInstalledClaudeReviewAgentsHaveExplicitEmptyTools(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		guidance string
	}{
		{name: "without CodeGraph"},
		{name: "with CodeGraph", guidance: "Use CodeGraph before broad filesystem search."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{
				CodeGraphGuidanceMarkdown: tc.guidance,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Changed || len(result.Skipped) != 0 {
				t.Fatalf("unexpected fresh install result: %+v", result)
			}
			for _, name := range []string{
				"review-risk.md", "review-readability.md", "review-reliability.md",
				"review-resilience.md", "review-refuter.md",
			} {
				t.Run(name, func(t *testing.T) {
					data, err := os.ReadFile(filepath.Join(adapter.SubAgentsDir(home), name))
					if err != nil {
						t.Fatal(err)
					}
					body, found := strings.CutPrefix(string(data), "---\n")
					if !found {
						t.Fatal("installed agent has no YAML frontmatter")
					}
					frontmatter, _, found := strings.Cut(body, "\n---\n")
					if !found {
						t.Fatal("installed agent has unterminated YAML frontmatter")
					}
					var fields map[string]any
					if err := yaml.Unmarshal([]byte(frontmatter), &fields); err != nil {
						t.Fatalf("parse installed frontmatter: %v", err)
					}
					value, present := fields["tools"]
					if !present {
						t.Fatal("tools must be explicit; omitting it inherits available tools")
					}
					tools, ok := value.([]any)
					if !ok || len(tools) != 0 {
						t.Fatalf("tools = %#v, want an explicit empty YAML sequence", value)
					}
				})
			}
		})
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

// A v3.x Kimi gentleman.yaml predates the ownership ledger and declares every
// SDD subagent by path. Its released bytes are Gentle AI's, so the installer
// rewrites it; any other bytes stay preserved as before (#5253).
func TestKimiInstallRewritesReleasedPreLedgerParent(t *testing.T) {
	released, err := os.ReadFile(filepath.Join("..", "legacyassets", "testdata", "v3.7.0", "kimi-gentleman.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := agents.NewAdapter(model.AgentKimi)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		content   []byte
		rewritten bool
	}{
		{"released v3 bytes", released, true},
		{"edited v3 bytes", append(append([]byte(nil), released...), "    mine:\n      path: ./mine.yaml\n"...), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(adapter.SubAgentsDir(home), "gentleman.yaml")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, tc.content, 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.rewritten {
				if string(got) != string(tc.content) || !containsPath(result.Skipped, path) {
					t.Fatalf("edited parent was not preserved: %+v", result)
				}
				return
			}
			if string(got) == string(tc.content) || containsPath(result.Skipped, path) {
				t.Fatalf("released v3 parent was not rewritten: %+v", result)
			}
			ledger := readOwnershipLedger(t, filepath.Join(adapter.SubAgentsDir(home), reviewassets.OwnershipLedgerFilename))
			if ledger.Files["gentleman.yaml"] == "" {
				t.Fatal("rewritten parent not recorded in the ownership ledger")
			}
		})
	}
}
