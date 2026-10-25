package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func rddAgentFileNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "review-") {
			names = append(names, entry.Name())
		}
	}
	return names
}

// TestFreshInstallShipsReviewAgentsOnlyToRDDRuntimes installs every runtime
// with a native agents directory and checks that review agents land only where
// receipt-driven development ships, while Judgment Day stays where it was.
func TestFreshInstallShipsReviewAgentsOnlyToRDDRuntimes(t *testing.T) {
	for _, tc := range []struct {
		agent  model.AgentID
		review bool
		jd     bool
	}{
		{model.AgentClaudeCode, true, true},
		{model.AgentCursor, false, true},
		{model.AgentKiroIDE, false, true},
		{model.AgentKimi, false, false},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			home := t.TempDir()
			selection := model.Selection{Agents: []model.AgentID{tc.agent}}
			runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))

			dir := resolveAdapters([]model.AgentID{tc.agent})[0].SubAgentsDir(home)
			review := rddAgentFileNames(t, dir)
			if got := len(review) > 0; got != tc.review {
				t.Fatalf("%s review agents = %v, want installed=%v", tc.agent, review, tc.review)
			}
			_, err := os.Stat(filepath.Join(dir, "jd-judge-a.md"))
			if got := err == nil; got != tc.jd {
				t.Fatalf("%s jd-judge-a installed = %v, want %v", tc.agent, got, tc.jd)
			}
			if tc.agent == model.AgentKimi {
				if _, err := os.Stat(filepath.Join(dir, "gentleman.yaml")); err != nil {
					t.Fatalf("Kimi lost its main agent: %v", err)
				}
			}
		})
	}
}

// TestFreshInstallShipsOpenCodeReviewAgents keeps the OpenCode RDD agents.
func TestFreshInstallShipsOpenCodeReviewAgents(t *testing.T) {
	home := installTestHome(t)
	if _, err := RunInstall([]string{"--agent", "opencode", "--preset", "full-gentleman"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Agent map[string]json.RawMessage `json:"agent"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"review-risk", "review-readability", "review-reliability", "review-resilience", "review-refuter", "review-validator", "jd-judge-a", "jd-judge-b", "jd-fix-agent"} {
		if _, ok := settings.Agent[name]; !ok {
			t.Errorf("OpenCode install is missing %q", name)
		}
	}
}

// TestUpgradeRemovesOwnedReviewAgentsFromNonRDDRuntimes seeds review agents an
// earlier release installed and recorded in the ownership ledger, plus a user's
// own review agent, and proves install and sync remove only Gentle AI's files.
func TestUpgradeRemovesOwnedReviewAgentsFromNonRDDRuntimes(t *testing.T) {
	for _, flow := range []string{"install", "sync"} {
		for _, agent := range []model.AgentID{model.AgentCursor, model.AgentKiroIDE, model.AgentKimi} {
			t.Run(flow+"/"+string(agent), func(t *testing.T) {
				home := t.TempDir()
				selection := model.Selection{Agents: []model.AgentID{agent}}
				dir := resolveAdapters([]model.AgentID{agent})[0].SubAgentsDir(home)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				owned := filepath.Join(dir, "review-risk.md")
				ownedBytes := []byte("review-risk as an earlier release installed it\n")
				if err := os.WriteFile(owned, ownedBytes, 0o644); err != nil {
					t.Fatal(err)
				}
				user := filepath.Join(dir, "review-readability.md")
				if err := os.WriteFile(user, []byte("my own reviewer\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				ledger, err := json.Marshal(map[string]any{"version": 1, "files": map[string]string{
					"review-risk.md": fmt.Sprintf("%x", sha256.Sum256(ownedBytes)),
				}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, reviewassets.OwnershipLedgerFilename), ledger, 0o644); err != nil {
					t.Fatal(err)
				}

				if flow == "install" {
					runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
				} else {
					runSyncInjectionSteps(t, home, selection)
				}

				if _, err := os.Lstat(owned); !os.IsNotExist(err) {
					t.Fatalf("owned review agent survived the %s: %v", flow, err)
				}
				if got, err := os.ReadFile(user); err != nil || string(got) != "my own reviewer\n" {
					t.Fatalf("user review agent = %q, %v; want preserved", got, err)
				}
				names := reviewassets.NativeAgentManifest[agent]
				if agent == model.AgentCursor {
					names = []string{"jd-judge-a.md", "jd-judge-b.md", "jd-fix-agent.md"}
				}
				for _, name := range names {
					if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
						t.Fatalf("retained agent %s missing after %s: %v", name, flow, err)
					}
				}
			})
		}
	}
}
