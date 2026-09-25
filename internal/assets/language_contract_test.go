package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var oddArtifactLanguageContractRequired = []string{
	"Generated technical artifacts default to English",
	"If technical artifacts are explicitly requested in another language, use a neutral/professional register",
	"Public/contextual comments follow the target context language",
	"Explicit user language or tone overrides win; otherwise use a neutral/professional register",
}

var oddOrchestratorLanguageContractRequired = append([]string{
	"The active persona controls direct user/orchestrator conversation only.",
}, oddArtifactLanguageContractRequired...)

var oddLanguageSpecificFallbacks = []string{
	"If Spanish technical artifacts are explicitly requested",
	"Spanish comments default to neutral/professional Spanish",
}

var oddKnownLanguageLeaks = []string{
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

func TestODDOrchestratorAssetsEnforceLanguageContract(t *testing.T) {
	assetPaths := allSDDOrchestratorAssetPaths(t)
	if len(assetPaths) != 12 {
		t.Fatalf("ODD orchestrator asset count = %d, want 12", len(assetPaths))
	}

	for _, path := range assetPaths {
		t.Run(path, func(t *testing.T) {
			content := oddLanguageContractContent(t, path)
			for _, required := range oddOrchestratorLanguageContractRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing language contract wording %q", path, required)
				}
			}
			for _, fallback := range oddLanguageSpecificFallbacks {
				if strings.Contains(content, fallback) {
					t.Fatalf("%s contains language-specific fallback wording %q", path, fallback)
				}
			}
			for _, leak := range oddKnownLanguageLeaks {
				if strings.Contains(content, leak) {
					t.Fatalf("%s contains persona-agnostic language leak %q", path, leak)
				}
			}
		})
	}
}

// The templated prompts use the shared ODD language section; Codex carries
// the contract inline. Verify the binding before examining effective wording.
func oddLanguageContractContent(t *testing.T, path string) string {
	t.Helper()
	content := MustRead(path)
	if path == "codex/orchestrator.md" {
		return content
	}
	const placeholder = "{{GENTLE_AI_ODD_SECTION:Language Domain Contract}}"
	if strings.Count(content, placeholder) != 1 {
		t.Fatalf("%s must reference the shared ODD language contract exactly once", path)
	}
	shared := MustRead("skills/_shared/odd-orchestrator-sections.md")
	const start = "<!-- sdd-orchestrator-section:Language Domain Contract:start -->"
	const end = "<!-- sdd-orchestrator-section:Language Domain Contract:end -->"
	from := strings.Index(shared, start)
	to := strings.Index(shared, end)
	if from < 0 || to <= from {
		t.Fatal("shared ODD language section is missing")
	}
	section := shared[from+len(start) : to]
	if !strings.Contains(section, "When delegating, forward this contract to the executor") {
		t.Fatal("shared ODD language section must forward the contract to delegated reviewers and writers")
	}
	return strings.Replace(content, placeholder, section, 1)
}

func TestSupportedAgentODDLanguageMatrix(t *testing.T) {
	tests := []struct {
		agent string
		path  string
	}{
		{agent: "claude-code", path: "claude/orchestrator.md"},
		{agent: "opencode", path: "opencode/orchestrator.md"},
		{agent: "kilocode", path: "opencode/orchestrator.md"},
		{agent: "gemini-cli", path: "gemini/orchestrator.md"},
		{agent: "cursor", path: "cursor/orchestrator.md"},
		{agent: "vscode-copilot", path: "generic/orchestrator.md"},
		{agent: "codex", path: "codex/orchestrator.md"},
		{agent: "antigravity", path: "antigravity/orchestrator.md"},
		{agent: "windsurf", path: "windsurf/orchestrator.md"},
		{agent: "kimi", path: "kimi/orchestrator.md"},
		{agent: "qwen-code", path: "qwen/orchestrator.md"},
		{agent: "kiro-ide", path: "kiro/orchestrator.md"},
		{agent: "openclaw", path: "generic/orchestrator.md"},
		{agent: "pi", path: "generic/orchestrator.md"},
		{agent: "trae-ide", path: "generic/orchestrator.md"},
		{agent: "hermes", path: "hermes/orchestrator.md"},
	}

	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			content := oddLanguageContractContent(t, tc.path)
			for _, required := range oddOrchestratorLanguageContractRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("agent %s asset %s missing language contract wording %q", tc.agent, tc.path, required)
				}
			}
		})
	}
}

func TestShippedReviewAssetsDoNotInstructFixTouchedLineDiscovery(t *testing.T) {
	// Review and Judgment Day are retained delegated roles. Their prompts do
	// not repeat the language contract: ODD orchestrators forward it to them.
	for _, path := range allReviewLifecycleAssetPaths(t) {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			if strings.Contains(content, "MUST review only fix-touched lines") {
				t.Fatalf("%s retains stale broad post-fix discovery instructions", path)
			}
		})
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
		if strings.HasSuffix(path, "/orchestrator.md") {
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
	for _, runtime := range []string{"claude", "cursor", "kimi", "kiro"} {
		for _, role := range []string{"readability", "refuter", "reliability", "resilience", "risk"} {
			paths = append(paths, runtime+"/agents/review-"+role+".md")
		}
	}
	for _, runtime := range []string{"claude", "kiro"} {
		for _, role := range []string{"fix-agent", "judge-a", "judge-b"} {
			paths = append(paths, runtime+"/agents/jd-"+role+".md")
		}
	}
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

const preWriteArtifactSelfCheckRequired = "Before any Write/Edit whose content is an artifact, re-verify the artifact language rules."

const neutralToneDialectAntiDriftRequired = "The same rule applies to tone and dialect: do not adopt regional forms from memory context, prior turns, or quoted material."

func TestPersonaChannelsCarryPreWriteArtifactSelfCheck(t *testing.T) {
	paths := []string{
		"claude/output-style-gentleman.md",
		"claude/output-style-neutral.md",
		"kimi/output-style-gentleman.md",
		"kimi/output-style-neutral.md",
		"generic/persona-gentleman.md",
		"generic/persona-neutral.md",
		"hermes/persona-gentleman.md",
		"hermes/persona-neutral.md",
		"kiro/persona-gentleman.md",
		"opencode/persona-gentleman.md",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			if !strings.Contains(content, preWriteArtifactSelfCheckRequired) {
				t.Fatalf("%s: missing pre-write artifact self-check sentence", path)
			}
		})
	}
}

func TestNeutralChannelsExtendAntiDriftToToneAndDialect(t *testing.T) {
	paths := []string{
		"claude/output-style-neutral.md",
		"kimi/output-style-neutral.md",
		"generic/persona-neutral.md",
		"hermes/persona-neutral.md",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			if !strings.Contains(content, neutralToneDialectAntiDriftRequired) {
				t.Fatalf("%s: missing tone/dialect anti-drift sentence", path)
			}
		})
	}
}
