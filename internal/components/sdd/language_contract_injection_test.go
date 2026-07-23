package sdd

import (
	"strings"
	"testing"
)

// TestInjectLanguageContractIntoPromptAppendsCanonicalBlock pins defect 4 of
// issue #1702: every rendered sub-agent prompt must carry the canonical
// executor language contract, injected from one source at render time.
func TestInjectLanguageContractIntoPromptAppendsCanonicalBlock(t *testing.T) {
	prompt := "---\nname: sdd-apply\n---\n\nDo the work.\n"
	got := injectLanguageContractIntoPrompt(prompt)

	if !strings.Contains(got, "Generated artifacts (code, comments, UI copy, docs, specs, tests, commit messages, memory entries) default to English.") {
		t.Fatalf("contract text missing from injected prompt:\n%s", got)
	}
	if !strings.Contains(got, "Never use regional slang or dialect-specific grammar in any artifact") {
		t.Fatalf("dialect prohibition missing from injected prompt:\n%s", got)
	}
	if !strings.Contains(got, "agent-language-contract") {
		t.Fatalf("managed section marker missing — injection must be marker-bound for idempotence:\n%s", got)
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
