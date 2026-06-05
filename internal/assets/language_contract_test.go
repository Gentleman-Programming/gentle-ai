package assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var sddLanguageContractRequired = []string{
	"The active persona controls direct user/orchestrator conversation only.",
	"Generated technical artifacts default to English",
	"Public/contextual comments follow the target context language",
	"If Spanish technical artifacts are explicitly requested, use neutral/professional Spanish",
}

var sddKnownLanguageLeaks = []string{
	"elegí",
	"Respondé",
	"¿Querés ajustar algo o continuamos?",
}

func TestSDDOrchestratorAssetsEnforceLanguageContract(t *testing.T) {
	assetPaths := allSDDOrchestratorAssetPaths(t)
	if len(assetPaths) < 11 {
		t.Fatalf("SDD orchestrator asset count = %d, want at least 11", len(assetPaths))
	}

	for _, path := range assetPaths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			for _, required := range sddLanguageContractRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing language contract wording %q", path, required)
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
	}

	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			content := MustRead(tc.path)
			for _, required := range sddLanguageContractRequired {
				if !strings.Contains(content, required) {
					t.Fatalf("agent %s asset %s missing language contract wording %q", tc.agent, tc.path, required)
				}
			}
		})
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
	for _, path := range []string{
		"claude/persona-gentleman.md",
		"generic/persona-gentleman.md",
		"kiro/persona-gentleman.md",
		"kimi/persona-gentleman.md",
		"opencode/persona-gentleman.md",
	} {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			for _, required := range []string{"Rioplatense", "voseo", "Passionate teacher"} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing Gentleman direct-conversation voice marker %q", path, required)
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
