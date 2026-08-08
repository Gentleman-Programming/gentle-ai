package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const preWriteArtifactLanguageCheck = "Before any Write/Edit whose content is an artifact, re-verify these artifact language rules."

// TestInjectLanguageContractIntoPromptAppendsCanonicalBlock pins defect 4 of
// issue #1702: every rendered sub-agent prompt must carry the canonical
// executor language contract, injected from one source at render time.
func TestInjectLanguageContractIntoPromptAppendsCanonicalBlock(t *testing.T) {
	prompt := "---\nname: sdd-apply\n---\n\nDo the work.\n"
	got := injectLanguageContractIntoPrompt(prompt)

	// Assert against the canonical asset itself, not duplicated fragments:
	// the whole contract (English default, explicit-Spanish register, dialect
	// prohibition) must survive injection verbatim, and wording tweaks to the
	// asset stay single-sourced.
	if canonical := strings.TrimSpace(agentLanguageContract()); !strings.Contains(got, canonical) {
		t.Fatalf("canonical contract missing from injected prompt:\ncontract:\n%s\nprompt:\n%s", canonical, got)
	}
	if !strings.Contains(got, "agent-language-contract") {
		t.Fatalf("managed section marker missing — injection must be marker-bound for idempotence:\n%s", got)
	}
	if !strings.Contains(got, preWriteArtifactLanguageCheck) {
		t.Fatalf("pre-write artifact language check missing from injected prompt:\n%s", got)
	}
}

// TestInjectLanguageContractIntoPromptIsIdempotent pins re-render stability:
// sync re-renders installed agents, so double injection must not duplicate.
func TestInjectLanguageContractIntoPromptIsIdempotent(t *testing.T) {
	prompt := "---\nname: sdd-apply\n---\n\nDo the work.\n"
	once := injectLanguageContractIntoPrompt(prompt)
	twice := injectLanguageContractIntoPrompt(once)
	if once != twice {
		t.Fatalf("double injection changed the prompt:\nfirst:\n%s\nsecond:\n%s", once, twice)
	}
}

// TestInjectLanguageContractIntoOpenCodeSubagentPrompts pins OpenCode parity:
// JSON-embedded sub-agent prompts get the same contract as markdown agents.
// Primary-mode agents and {file:...} indirections are skipped, mirroring the
// CodeGraph guidance injection.
func TestInjectLanguageContractIntoOpenCodeSubagentPrompts(t *testing.T) {
	agentMap := map[string]any{
		"sdd-apply": map[string]any{
			"mode":   "subagent",
			"prompt": "Implement the tasks.",
		},
		"gentleman": map[string]any{
			"mode":   "primary",
			"prompt": "Primary persona prompt.",
		},
		"sdd-verify": map[string]any{
			"mode":   "subagent",
			"prompt": "{file:./AGENTS.md}",
		},
	}

	injectLanguageContractIntoOpenCodeSubagentPrompts(agentMap)

	apply := agentMap["sdd-apply"].(map[string]any)["prompt"].(string)
	if !strings.Contains(apply, "default to English") {
		t.Fatalf("subagent prompt missing contract:\n%s", apply)
	}
	primary := agentMap["gentleman"].(map[string]any)["prompt"].(string)
	if strings.Contains(primary, "default to English") {
		t.Fatal("primary-mode agent must not receive the executor contract")
	}
	verify := agentMap["sdd-verify"].(map[string]any)["prompt"].(string)
	if verify != "{file:./AGENTS.md}" {
		t.Fatalf("file-indirection prompt must be untouched, got %q", verify)
	}
}

// TestWriteSharedPromptFilesCarryLanguageContract pins the {file:...} gap:
// OpenCode phases that load shared prompt files through file indirection skip
// the in-settings injection, so the shared files themselves must carry the
// canonical contract.
func TestWriteSharedPromptFilesCarryLanguageContract(t *testing.T) {
	homeDir := t.TempDir()
	if _, err := WriteSharedPromptFiles(homeDir, nil); err != nil {
		t.Fatalf("WriteSharedPromptFiles() error = %v", err)
	}

	entries, err := os.ReadDir(SharedPromptDir(homeDir))
	if err != nil {
		t.Fatalf("read shared prompt dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no shared prompt files written")
	}
	canonical := strings.TrimSpace(agentLanguageContract())
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(SharedPromptDir(homeDir), entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if !strings.Contains(string(content), canonical) {
			t.Errorf("%s: missing canonical language contract", entry.Name())
		}
	}
}
