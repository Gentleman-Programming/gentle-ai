package agentguidance_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

// The v5.0.0 OpenCode routing digest guards the release fixture independently
// of the current renderer (only step 7 changed). Its installer recognized both
// blocks by marker, not by prompt digest as it does for separate role agents.
const releasedOpenCodeRoutingDigest = "6f527301c5a429f53ac1157269aae8d34da71c20d590ea7bbf5f32487b9830a0"

const releasedContinue = "Authorized work continues: runnable in-scope steps run, a pending child is a wait event whose completion resumes the turn without asking the human, and only a real human decision, the user's explicit stop, a denial, missing consent, or an exhausted limit stops the work it applies to, never independent authorized work."

const releasedOpenCodeClose = "7. **Close.** Report the verified outcome, every failed, skipped, or pending check, and the next step. Hedged wording is not a stop: a user saying they may stop you or resume later asks for notes and separate commits. " +
	releasedContinue + " A tracked task ends the turn pending only while it waits on already-launched work, for the affected work a denial, missing consent, the user's explicit stop, or an exhausted limit applies to, or for a genuine human decision still outstanding; independent authorized work continues, and it never abandons runnable work or presents an unverified outcome as final. " +
	"Missing, partial, unavailable, or declined proof forbids claiming completion of the affected work, never an independently authorized next step, while a required authorized check the existing budget still allows keeps running; stop checking only inside an exhausted or denied boundary, report that exact blocker and the decision that is genuinely the human's, and never invent a pass. Never turn a report into a blind self-reprompt loop. " +
	"The native review candidate is a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature branch; native review runs only under the user-owned receipt-driven development switch."

func TestReleasedOpenCodeCloseUpgradesAndUninstalls(t *testing.T) {
	const agent = model.AgentOpenCode
	const userText = "# My instructions\n"
	const closeMarker = "\n7. **Close.** "
	routing, err := agentguidance.RenderRouting(agent)
	if err != nil {
		t.Fatal(err)
	}
	before, closeTail, found := strings.Cut(routing, closeMarker)
	_, after, ended := strings.Cut(closeTail, "\n")
	if !found || !ended {
		t.Fatal("current OpenCode routing has no complete Close step")
	}
	releasedRouting := before + "\n" + releasedOpenCodeClose + "\n" + after
	if sum := fmt.Sprintf("%x", sha256.Sum256([]byte(releasedRouting))); sum != releasedOpenCodeRoutingDigest {
		t.Fatalf("v5.0.0 OpenCode routing fixture SHA-256 = %s, want %s", sum, releasedOpenCodeRoutingDigest)
	}
	orchestrator, err := agentguidance.RenderOrchestratorWithSource(agent, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	releasedOrchestrator := strings.Replace(orchestrator, continueAuthorizedWorkRule, releasedContinue, 1)
	releasedOrchestrator = strings.Replace(releasedOrchestrator, failedCheckRule, "An applicable quick check runs once", 1)
	if releasedOrchestrator == orchestrator {
		t.Fatal("could not reconstruct previous OpenCode orchestrator wording")
	}
	prompt := filemerge.InjectMarkdownSection(userText, agentguidance.OrchestratorSectionID, releasedOrchestrator)
	prompt = filemerge.InjectMarkdownSection(prompt, agentguidance.RoutingSectionID, agentguidance.InjectRemoteAuthorization(releasedRouting))

	for _, upgrade := range []bool{false, true} {
		name := "uninstall directly"
		if upgrade {
			name = "upgrade then uninstall"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			path := opencode.NewAdapter().SettingsPath(home)
			settings, err := json.Marshal(map[string]any{"agent": map[string]any{
				"gentle-orchestrator": map[string]string{"prompt": prompt},
				"user-owned":          map[string]string{"prompt": "Do not touch"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, settings, 0o600); err != nil {
				t.Fatal(err)
			}
			readPrompt := func() string {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				root, err := filemerge.UnmarshalJSONObject(data)
				if err != nil {
					t.Fatal(err)
				}
				agents, _ := root["agent"].(map[string]any)
				user, _ := agents["user-owned"].(map[string]any)
				if user["prompt"] != "Do not touch" {
					t.Fatal("user-owned agent changed")
				}
				managed, _ := agents["gentle-orchestrator"].(map[string]any)
				actual, _ := managed["prompt"].(string)
				return actual
			}
			if upgrade {
				if result, err := agentguidance.InjectRoutingWithOptions(home, agent, agentguidance.RoutingOptions{}); err != nil || !result.Changed {
					t.Fatalf("upgrade: changed=%t, err=%v", result.Changed, err)
				}
				upgraded := readPrompt()
				for _, want := range []string{userText, strings.TrimSpace(orchestrator), strings.TrimSpace(agentguidance.InjectRemoteAuthorization(routing))} {
					if strings.Count(upgraded, want) != 1 {
						t.Fatal("upgrade did not preserve user text or replace each managed block exactly once")
					}
				}
				if strings.Contains(upgraded, releasedOpenCodeClose) || strings.Contains(upgraded, releasedContinue) {
					t.Fatal("upgrade left previous managed wording installed")
				}
			}
			service, err := uninstall.NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.PartialUninstall([]model.AgentID{agent}, nil); err != nil {
				t.Fatal(err)
			}
			if got := readPrompt(); strings.TrimSpace(got) != strings.TrimSpace(userText) {
				t.Fatalf("uninstall prompt = %q, want only user instructions", got)
			}
		})
	}
}
