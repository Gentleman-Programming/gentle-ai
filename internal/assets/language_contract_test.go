package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var sddArtifactLanguageContractRequired = []string{
	"Generated technical artifacts default to English",
	"If technical artifacts are explicitly requested in another language, use a neutral/professional register",
	"Public/contextual comments follow the target context language",
	"Explicit user language or tone overrides win; otherwise use a neutral/professional register",
}

var sddOrchestratorLanguageContractRequired = append([]string{
	"The active persona controls direct user/orchestrator conversation only.",
}, sddArtifactLanguageContractRequired...)

var sddLanguageSpecificFallbacks = []string{
	"If Spanish technical artifacts are explicitly requested",
	"Spanish comments default to neutral/professional Spanish",
}

var sddKnownLanguageLeaks = []string{
	"elegí",
	"Respondé",
	"¿Querés ajustar algo o continuamos?",
}

var directReplyEnglishNoCodeSwitchRequired = []string{
	"If the selected reply language is English, every part of the direct reply must be English: greetings, interjections, acknowledgements, transition phrases, and the first sentence. Do not use Hola, dale, listo, Spanish punctuation, or other Spanish fragments.",
	"Prompts starting with or dominated by hi, hello, hey, or similar English greetings are English prompts unless the user explicitly asks for another language.",
}

func TestManagedDirectReplyAssetsEnforceEnglishNoCodeSwitching(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		combineWith string // "" when the asset alone still carries the contract
	}{
		{name: "claude gentleman output style", path: "claude/output-style-gentleman.md"},
		{name: "claude neutral output style", path: "claude/output-style-neutral.md"},
		// Claude and Kimi personas are residuals (Decision 1) — evaluate the
		// combined persona-residual + output-style channel, not the persona
		// file alone.
		{name: "claude gentleman persona", path: "claude/persona-gentleman.md", combineWith: "claude/output-style-gentleman.md"},
		{name: "generic gentleman persona", path: "generic/persona-gentleman.md"},
		{name: "generic neutral persona", path: "generic/persona-neutral.md"},
		{name: "hermes gentleman persona", path: "hermes/persona-gentleman.md"},
		{name: "hermes neutral persona", path: "hermes/persona-neutral.md"},
		{name: "kiro gentleman persona", path: "kiro/persona-gentleman.md"},
		{name: "kimi gentleman output style", path: "kimi/output-style-gentleman.md"},
		{name: "kimi neutral output style", path: "kimi/output-style-neutral.md"},
		{name: "kimi gentleman persona", path: "kimi/persona-gentleman.md", combineWith: "kimi/output-style-gentleman.md"},
		{name: "opencode gentleman persona", path: "opencode/persona-gentleman.md"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := MustRead(tc.path)
			if tc.combineWith != "" {
				content += "\n" + MustRead(tc.combineWith)
			}
			for _, required := range directReplyEnglishNoCodeSwitchRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("%s (combined=%q) missing direct-reply English no-code-switch contract %q", tc.path, tc.combineWith, required)
				}
			}
		})
	}
}

func TestSDDOrchestratorAssetsEnforceLanguageContract(t *testing.T) {
	assetPaths := allSDDOrchestratorAssetPaths(t)
	if len(assetPaths) < 11 {
		t.Fatalf("SDD orchestrator asset count = %d, want at least 11", len(assetPaths))
	}

	for _, path := range assetPaths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			for _, required := range sddOrchestratorLanguageContractRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing language contract wording %q", path, required)
				}
			}
			for _, fallback := range sddLanguageSpecificFallbacks {
				if strings.Contains(content, fallback) {
					t.Fatalf("%s contains language-specific fallback wording %q", path, fallback)
				}
			}

			for _, leak := range sddKnownLanguageLeaks {
				if strings.Contains(content, leak) {
					t.Fatalf("%s contains persona-agnostic language leak %q", path, leak)
				}
			}
		})
	}
}

func TestSDDPhaseSkillsEnforceLanguageContract(t *testing.T) {
	for _, path := range allSDDPhaseSkillAssetPaths(t) {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			for _, required := range sddArtifactLanguageContractRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing language contract wording %q", path, required)
				}
			}
			for _, fallback := range sddLanguageSpecificFallbacks {
				if strings.Contains(content, fallback) {
					t.Fatalf("%s contains language-specific fallback wording %q", path, fallback)
				}
			}
		})
	}
}

func TestSupportedAgentSDDLanguageMatrix(t *testing.T) {
	tests := []struct {
		agent string
		path  string
	}{
		{agent: "claude-code", path: "claude/sdd-orchestrator.md"},
		{agent: "opencode", path: "opencode/sdd-orchestrator.md"},
		{agent: "kilocode", path: "opencode/sdd-orchestrator.md"},
		{agent: "gemini-cli", path: "gemini/sdd-orchestrator.md"},
		{agent: "cursor", path: "cursor/sdd-orchestrator.md"},
		{agent: "vscode-copilot", path: "generic/sdd-orchestrator.md"},
		{agent: "codex", path: "codex/sdd-orchestrator.md"},
		{agent: "antigravity", path: "antigravity/sdd-orchestrator.md"},
		{agent: "windsurf", path: "windsurf/sdd-orchestrator.md"},
		{agent: "kimi", path: "kimi/sdd-orchestrator.md"},
		{agent: "qwen-code", path: "qwen/sdd-orchestrator.md"},
		{agent: "kiro-ide", path: "kiro/sdd-orchestrator.md"},
		{agent: "openclaw", path: "generic/sdd-orchestrator.md"},
		{agent: "pi", path: "generic/sdd-orchestrator.md"},
		{agent: "trae-ide", path: "generic/sdd-orchestrator.md"},
		{agent: "hermes", path: "hermes/sdd-orchestrator.md"},
	}

	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			content := MustRead(tc.path)
			for _, required := range sddOrchestratorLanguageContractRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("agent %s asset %s missing language contract wording %q", tc.agent, tc.path, required)
				}
			}
		})
	}
}

func allSDDPhaseSkillAssetPaths(t *testing.T) []string {
	t.Helper()
	paths, err := fs.Glob(FS, "skills/sdd-*/SKILL.md")
	if err != nil {
		t.Fatalf("Glob embedded SDD phase skills: %v", err)
	}
	if len(paths) != 10 {
		t.Fatalf("SDD phase skill asset count = %d, want 10", len(paths))
	}
	sort.Strings(paths)
	return paths
}

func TestShippedReviewAssetsDoNotInstructFixTouchedLineDiscovery(t *testing.T) {
	for _, path := range allReviewLifecycleAssetPaths(t) {
		content := MustRead(path)
		if strings.Contains(content, "MUST review only fix-touched lines") {
			t.Fatalf("%s retains stale broad post-fix discovery instructions", path)
		}
	}
}

func TestSDDOrchestratorAssetsEnforceInteractiveProposalGates(t *testing.T) {
	assetPaths := allSDDOrchestratorAssetPaths(t)
	if len(assetPaths) < 11 {
		t.Fatalf("SDD orchestrator asset count = %d, want at least 11", len(assetPaths))
	}

	for _, path := range assetPaths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			if path == "claude/sdd-orchestrator.md" {
				content = MustRead("claude/sdd-orchestrator-workflow.md")
			}
			for _, required := range []string{
				"Interactive approval is phase-scoped",
				"approve only the immediate next phase",
				"Before the `sdd-propose` phase in interactive mode",
				"proposal question round",
				"business problem",
				"business rules",
				"implications and impact",
				"edge cases",
				"Do not ask about test commands, PR shape, changed-line budget",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing interactive proposal gate wording %q", path, required)
				}
			}
		})
	}
}

func TestSDDProposeAssetsRequireProposalQuestionRound(t *testing.T) {
	assetPaths := allSDDProposeAssetPaths(t)
	if len(assetPaths) < 4 {
		t.Fatalf("SDD propose asset count = %d, want at least 4", len(assetPaths))
	}

	for _, path := range assetPaths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			for _, required := range []string{
				"Offer the user a proposal question round",
				"second question round",
				"business problem",
				"target users and situations",
				"business rules",
				"implications and impact",
				"edge cases",
				"decision gaps",
				"Do not ask about test commands, PR shape, changed-line budget, or other harness decisions unless the user explicitly asks to discuss delivery",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing proposal question-round wording %q", path, required)
				}
			}
		})
	}
}

func TestSharedSDDProposeSkillRequiresProposalQuestionRound(t *testing.T) {
	content := MustRead("skills/sdd-propose/SKILL.md")
	for _, required := range []string{
		"Offer the user a proposal question round",
		"second question round",
		"business problem",
		"target users and situations",
		"business rules",
		"implications and impact",
		"edge cases",
		"decision gaps",
		"Do not ask about test commands, PR shape, changed-line budget, or other harness decisions unless the user explicitly asks to discuss delivery",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("skills/sdd-propose/SKILL.md missing proposal question-round wording %q", required)
		}
	}
}

func TestCommentWriterLanguageContractSources(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "embedded", content: MustRead("skills/comment-writer/SKILL.md")},
		{name: "root", content: readRepoRootFile(t, "skills/comment-writer/SKILL.md")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, required := range []string{
				"target context language",
				"explicitly requests a language",
				"neutral/professional Spanish by default",
			} {
				if !strings.Contains(tc.content, required) {
					t.Fatalf("%s comment-writer source missing %q", tc.name, required)
				}
			}

			for _, forcedDefault := range []string{
				"If writing in Spanish, use Rioplatense Spanish/voseo",
				"use Rioplatense Spanish/voseo: `podés`, `tenés`, `fijate`, `dale`",
				"agregá",
				"separaría este cambio",
			} {
				if strings.Contains(tc.content, forcedDefault) {
					t.Fatalf("%s comment-writer source demonstrates regional Spanish as the default via %q", tc.name, forcedDefault)
				}
			}
		})
	}
}

func TestGentlemanPersonaKeepsDirectConversationVoice(t *testing.T) {
	// Claude and Kimi personas are residuals (Decision 1) — the direct-
	// conversation voice now lives exclusively in the output style; evaluate
	// the combined persona-residual + output-style channel for those two.
	tests := []struct {
		path        string
		combineWith string
	}{
		{path: "claude/persona-gentleman.md", combineWith: "claude/output-style-gentleman.md"},
		{path: "generic/persona-gentleman.md"},
		{path: "kiro/persona-gentleman.md"},
		{path: "kimi/persona-gentleman.md", combineWith: "kimi/output-style-gentleman.md"},
		{path: "opencode/persona-gentleman.md"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			content := MustRead(tc.path)
			if tc.combineWith != "" {
				content += "\n" + MustRead(tc.combineWith)
			}
			for _, required := range []string{"Rioplatense", "voseo", "Passionate teacher"} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s (combined=%q) missing Gentleman direct-conversation voice marker %q", tc.path, tc.combineWith, required)
				}
			}
		})
	}
}

func TestNeutralPersonaAssetsProvideMentorParityWithoutRegionalVoice(t *testing.T) {
	for _, path := range []string{
		"generic/persona-neutral.md",
		"hermes/persona-neutral.md",
	} {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			for _, required := range []string{
				"Response-length contract",
				"minimum useful response",
				"Ask at most one question at a time",
				"STOP and wait",
				"Do not present option menus",
				"verification",
				"CONCEPTS > CODE",
				"Generated technical artifacts default to English",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing neutral parity contract %q", path, required)
				}
			}

			for _, banned := range []string{
				"Rioplatense",
				"voseo",
				"Gentleman regional voice",
				"When replying to the user in Spanish, use warm natural Rioplatense Spanish",
			} {
				if strings.Contains(content, banned) {
					t.Fatalf("%s contains banned regional neutral wording %q", path, banned)
				}
			}
		})
	}
}

func TestNeutralOutputStyleAssetsProvideMeaningfulContract(t *testing.T) {
	for _, path := range []string{
		"claude/output-style-neutral.md",
		"kimi/output-style-neutral.md",
	} {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			if strings.TrimSpace(content) == "" {
				t.Fatalf("%s is empty", path)
			}
			for _, required := range []string{
				"Neutral Output Style",
				"minimum useful response",
				"Ask at most one question at a time",
				"STOP",
				"Do not offer option menus",
				"verify",
				"Generated technical artifacts default to English",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing output-style contract %q", path, required)
				}
			}
			for _, banned := range []string{"Rioplatense", "voseo", "Gentleman Output Style"} {
				if strings.Contains(content, banned) {
					t.Fatalf("%s contains banned neutral output-style wording %q", path, banned)
				}
			}
		})
	}
}

func allSDDOrchestratorAssetPaths(t *testing.T) []string {
	t.Helper()
	var paths []string
	if err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "/sdd-orchestrator.md") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir embedded assets: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func allReviewLifecycleAssetPaths(t *testing.T) []string {
	t.Helper()
	var paths []string
	if err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		if strings.Contains(MustRead(path), "MUST review only fix-touched lines") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir embedded review assets: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func allSDDProposeAssetPaths(t *testing.T) []string {
	t.Helper()
	var paths []string
	if err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "/agents/sdd-propose.md") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir embedded assets: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func readRepoRootFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", rel)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}
