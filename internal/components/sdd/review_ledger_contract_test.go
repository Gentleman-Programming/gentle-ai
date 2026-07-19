package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

var requiredLedgerClauses = boundedReviewRequiredClauses

const requiredOrchestratorMergeModeClause = "Parent orchestrator and native CLI only"

func TestBoundedReviewContractLeavesCanonicalizationToNativeGo(t *testing.T) {
	content := boundedReviewContract()
	for _, want := range []string{
		"Native Go validates, canonicalizes, persists, hashes, reopens, and binds results",
		"models never construct canonical bytes or hashes",
		"Freeze merged findings",
		"append the exact immutable candidate diff and changed-path manifest",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("orchestrator contract missing %q", want)
		}
	}
	for _, forbidden := range []string{"canonical empty ledger bytes are exactly", "MUST NOT serialize", "Unknown native finding fields remain rejected"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("orchestrator contract retains model-facing canonicalization rule %q", forbidden)
		}
	}
}

func TestDedicatedReviewAndJudgmentAssetsRenderRoleContracts(t *testing.T) {
	assetsByFamily := map[string][]string{
		"claude": {
			"claude/agents/review-risk.md", "claude/agents/review-readability.md",
			"claude/agents/review-reliability.md", "claude/agents/review-resilience.md",
			"claude/agents/jd-judge-a.md", "claude/agents/jd-judge-b.md",
		},
		"cursor": {
			"cursor/agents/review-risk.md", "cursor/agents/review-readability.md",
			"cursor/agents/review-reliability.md", "cursor/agents/review-resilience.md",
		},
		"kimi": {
			"kimi/agents/review-risk.md", "kimi/agents/review-readability.md",
			"kimi/agents/review-reliability.md", "kimi/agents/review-resilience.md",
		},
		"kiro": {
			"kiro/agents/review-risk.md", "kiro/agents/review-readability.md",
			"kiro/agents/review-reliability.md", "kiro/agents/review-resilience.md",
			"kiro/agents/jd-judge-a.md", "kiro/agents/jd-judge-b.md",
		},
	}
	for family, paths := range assetsByFamily {
		for _, path := range paths {
			t.Run(family+"/"+path, func(t *testing.T) {
				content := renderBoundedReviewAsset(path)
				assertTextContainsClauses(t, path, content, []string{"read-only", "candidate", "BLOCKER", "CRITICAL", "causal", "proof"})
				assertNoReviewerLifecycleInstructions(t, path, content)
			})
		}
	}
}

func TestDedicatedReviewersAndRefutersAreStructurallyReadOnly(t *testing.T) {
	for _, path := range []string{
		"claude/agents/review-risk.md", "claude/agents/review-readability.md",
		"claude/agents/review-reliability.md", "claude/agents/review-resilience.md",
		"claude/agents/review-refuter.md",
	} {
		frontmatter := markdownFrontmatter(t, path)
		for _, forbidden := range []string{"Bash", "Write", "Edit"} {
			if strings.Contains(frontmatter, forbidden) {
				t.Errorf("%s frontmatter grants %s", path, forbidden)
			}
		}
	}
	for _, path := range []string{
		"kiro/agents/review-risk.md", "kiro/agents/review-readability.md",
		"kiro/agents/review-reliability.md", "kiro/agents/review-resilience.md",
		"kiro/agents/review-refuter.md", "kiro/agents/jd-judge-a.md", "kiro/agents/jd-judge-b.md",
	} {
		if frontmatter := markdownFrontmatter(t, path); !strings.Contains(frontmatter, `tools: ["read"]`) {
			t.Errorf("%s is not read-only:\n%s", path, frontmatter)
		}
	}
	for _, path := range []string{
		"cursor/agents/review-risk.md", "cursor/agents/review-readability.md",
		"cursor/agents/review-reliability.md", "cursor/agents/review-resilience.md",
		"cursor/agents/review-refuter.md",
	} {
		if frontmatter := markdownFrontmatter(t, path); !strings.Contains(frontmatter, "readonly: true") {
			t.Errorf("%s is not read-only", path)
		}
	}
	for _, path := range []string{
		"claude/agents/review-refuter.md", "cursor/agents/review-refuter.md",
		"kimi/agents/review-refuter.md", "kiro/agents/review-refuter.md",
	} {
		assertNoReviewerLifecycleInstructions(t, path, renderBoundedReviewAsset(path))
	}
	for _, path := range []string{
		"kimi/agents/review-risk.yaml", "kimi/agents/review-readability.yaml",
		"kimi/agents/review-reliability.yaml", "kimi/agents/review-resilience.yaml",
		"kimi/agents/review-refuter.yaml",
	} {
		content := assets.MustRead(path)
		for _, excluded := range []string{"multiagent:Task", "shell:Shell", "file:WriteFile", "file:StrReplaceFile"} {
			if !strings.Contains(content, excluded) {
				t.Errorf("%s does not exclude %s", path, excluded)
			}
		}
	}
}

func TestOpenCodeOverlaysRenderBoundedReadOnlyReviewRoles(t *testing.T) {
	for _, path := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
		t.Run(path, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal([]byte(assets.MustRead(path)), &root); err != nil {
				t.Fatal(err)
			}
			agentsMap := root["agent"].(map[string]any)
			expandOpenCodeBoundedReviewAgents(agentsMap)
			for _, name := range []string{"review-risk", "review-readability", "review-reliability", "review-resilience"} {
				agent := agentsMap[name].(map[string]any)
				prompt := agent["prompt"].(string)
				assertTextContainsClauses(t, path+" "+name, prompt, []string{"## Scope", "## Candidate-Causal Admission", "## Severity", "## Evidence", "## Output"})
				assertNoReviewerLifecycleInstructions(t, path+" "+name, prompt)
				assertOpenCodeReadOnlyTools(t, path+" "+name, agent["tools"].(map[string]any))
			}
			for _, name := range []string{"jd-judge-a", "jd-judge-b"} {
				agent := agentsMap[name].(map[string]any)
				prompt := agent["prompt"].(string)
				if prompt != judgmentDayReviewerContract() {
					t.Errorf("%s %s does not use the native role-only judgment contract", path, name)
				}
				assertNoReviewerLifecycleInstructions(t, path+" "+name, prompt)
				assertOpenCodeReadOnlyTools(t, path+" "+name, agent["tools"].(map[string]any))
			}
			refuter := agentsMap[opencode.ReviewRefuterAgent].(map[string]any)
			refuterPrompt := refuter["prompt"].(string)
			if !strings.Contains(refuterPrompt, "exactly ONE transaction-wide inferential batch") || !strings.Contains(refuterPrompt, "terminate") {
				t.Errorf("%s refuter prompt is not bounded: %s", path, refuterPrompt)
			}
			assertNoReviewerLifecycleInstructions(t, path+" refuter", refuterPrompt)
			assertOpenCodeReadOnlyTools(t, path+" refuter", refuter["tools"].(map[string]any))
		})
	}
}

func TestOpenCodeRenderedReviewProtocolCost(t *testing.T) {
	home := t.TempDir()
	if _, err := Inject(home, opencodeAdapter(), ""); err != nil {
		t.Fatalf("Inject(opencode) error = %v", err)
	}
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(payload, &settings); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		agents        []string
		beforeChars   int
		wantChars     int
		maxCharacters int
	}{
		{name: "standard", agents: []string{"review-reliability"}, beforeChars: 42_301, wantChars: 6_994, maxCharacters: 7_000},
		{name: "full-4R", agents: []string{"review-risk", "review-resilience", "review-readability", "review-reliability"}, beforeChars: 106_998, wantChars: 14_185, maxCharacters: 16_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chars, _ := measurePromptCost(boundedReviewContract())
			for _, agent := range tt.agents {
				promptChars, _ := measurePromptCost(settings.Agent[agent].Prompt)
				chars += promptChars
			}
			tokens := chars / 4
			t.Logf("before=%d characters/%d estimated tokens after=%d/%d", tt.beforeChars, tt.beforeChars/4, chars, tokens)
			if chars != tt.wantChars {
				t.Fatalf("rendered protocol cost = %d characters / %d estimated tokens, want deterministic total %d / %d", chars, tokens, tt.wantChars, tt.wantChars/4)
			}
			if chars > tt.maxCharacters {
				t.Fatalf("rendered protocol cost = %d characters / %d estimated tokens, target <= %d / %d", chars, tokens, tt.maxCharacters, tt.maxCharacters/4)
			}
		})
	}
}

func measurePromptCost(prompt string) (characters, estimatedTokens int) {
	characters = utf8.RuneCountInString(prompt)
	return characters, characters / 4
}

func markdownFrontmatter(t *testing.T, path string) string {
	t.Helper()
	parts := strings.SplitN(assets.MustRead(path), "---", 3)
	if len(parts) != 3 {
		t.Fatalf("%s missing frontmatter", path)
	}
	return parts[1]
}

func assertOpenCodeReadOnlyTools(t *testing.T, label string, tools map[string]any) {
	t.Helper()
	want := map[string]bool{"*": false, "read": true, "write": false, "edit": false, "bash": false, "task": false}
	if len(tools) != len(want) {
		t.Fatalf("%s tools = %#v", label, tools)
	}
	for name, expected := range want {
		if got, ok := tools[name].(bool); !ok || got != expected {
			t.Errorf("%s tool %s = %v, want %v", label, name, tools[name], expected)
		}
	}
}

func assertTextContainsClauses(t *testing.T, label, content string, clauses []string) {
	t.Helper()
	for _, clause := range clauses {
		if !strings.Contains(content, clause) {
			t.Errorf("%s missing required clause %q", label, clause)
		}
	}
}

func assertNoReviewerLifecycleInstructions(t *testing.T, label, content string) {
	t.Helper()
	forbidden := regexp.MustCompile(`(?i)\b(bundle|receipt|mirror|release|gate)s?\b`)
	if match := forbidden.FindString(content); match != "" {
		t.Errorf("%s reviewer prompt contains lifecycle instruction term %q", label, match)
	}
	lower := strings.ToLower(content)
	for _, phrase := range []string{"ordinary 4r", "fix/re-judgment", "launches review-refuter", "review/start", "review-resume", "correction transaction", "scoped validator"} {
		if strings.Contains(lower, phrase) {
			t.Errorf("%s reviewer prompt contains lifecycle routing phrase %q", label, phrase)
		}
	}
}

func readGentleOrchestratorPrompt(t *testing.T, settingsPath string) string {
	t.Helper()
	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatal(err)
	}
	agentsMap := root["agent"].(map[string]any)
	orchestrator := agentsMap["gentle-orchestrator"].(map[string]any)
	return orchestrator["prompt"].(string)
}

func assertOpenCodeRefuterToolsReadOnly(t *testing.T, label string, tools map[string]any) {
	t.Helper()
	assertOpenCodeReadOnlyTools(t, label, tools)
}
