package components_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
)

type installedDeliveryGuaranteeInvariant struct {
	name   string
	phrase string
}

// deliveryGuaranteePhrases are the common semantic rules that every installed
// protocol output must state so memory bookkeeping can never replace the final
// user-facing reply (issue #1042 acceptance scope). Matching is done on
// whitespace-normalized, lowercased content so assertions survive reformatting
// and line wraps, unlike golden byte-equality.
var deliveryGuaranteePhrases = []installedDeliveryGuaranteeInvariant{
	{"memory-is-bookkeeping", "bookkeeping"},
	{"saving-never-counts-as-answering", "never counts as answering"},
	{"turn-ends-with-answer-as-final-message", "final message"},
	{"save-before-composing-the-reply", "before composing"},
}

var installedFullFailureContract = []installedDeliveryGuaranteeInvariant{
	{"failure-delivers-complete-answer", "fails or times out, deliver the complete answer anyway and note the failure briefly"},
	{"failure-never-blocks-truncates-or-replaces", "failed or slow memory operation never blocks, truncates, or replaces the reply"},
}

var installedSlimFailureContract = []installedDeliveryGuaranteeInvariant{
	{"failure-delivers-answer", "fails or times out, deliver the answer anyway"},
	{"failure-never-blocks-or-replaces", "memory failures never block or replace the reply"},
}

const fullPositiveFailureContract = "If a memory call (`mem_save`, `mem_judge`, `mem_session_summary`) fails or times out, deliver the complete answer anyway and note the failure briefly — a failed or slow memory operation never blocks, truncates, or replaces the reply."
const fullOppositeFailureContract = "If a memory call (`mem_save`, `mem_judge`, `mem_session_summary`) fails or times out, do not deliver the complete answer anyway; a failed or slow memory operation may block, truncate, or replace the reply."
const slimPositiveFailureContract = "If a memory call fails or times out, deliver the answer anyway — memory failures never block or replace the reply."
const slimOppositeFailureContract = "If a memory call fails or times out, do not deliver the answer anyway — memory failures may block or replace the reply."

func normalizeInstalledDeliveryGuarantee(content string) string {
	return strings.ToLower(strings.Join(strings.Fields(content), " "))
}

func missingInstalledDeliveryGuarantees(content []byte, failureContract []installedDeliveryGuaranteeInvariant) []string {
	normalized := normalizeInstalledDeliveryGuarantee(string(content))
	invariants := append(append([]installedDeliveryGuaranteeInvariant{}, deliveryGuaranteePhrases...), failureContract...)
	missing := make([]string, 0)
	for _, inv := range invariants {
		if !strings.Contains(normalized, inv.phrase) {
			missing = append(missing, inv.name)
		}
	}
	return missing
}

func assertDeliveryGuarantee(t *testing.T, surface string, content []byte, failureContract []installedDeliveryGuaranteeInvariant) {
	t.Helper()
	for _, missing := range missingInstalledDeliveryGuarantees(content, failureContract) {
		t.Errorf("installed output %q violates delivery-guarantee invariant %q", surface, missing)
	}
}

func assertRejectsOppositeFailureContract(t *testing.T, surface string, content []byte, failureContract []installedDeliveryGuaranteeInvariant, positive, opposite string) {
	t.Helper()
	normalizedContent := normalizeInstalledDeliveryGuarantee(string(content))
	mutated := []byte(strings.Replace(
		normalizedContent,
		normalizeInstalledDeliveryGuarantee(positive),
		normalizeInstalledDeliveryGuarantee(opposite),
		1,
	))
	if string(mutated) == normalizedContent {
		t.Fatalf("positive failure contract was not found in installed output %q", surface)
	}
	if missing := missingInstalledDeliveryGuarantees(mutated, failureContract); len(missing) == 0 {
		t.Fatalf("installed output %q accepted opposite failure wording that retained weak fragments", surface)
	}
}

// TestDeliveryGuarantee_InstalledOutputs runs the real injection into a temp
// home and asserts the delivery-guarantee semantics on representative
// installed outputs, one per protocol variant:
//   - Claude Code CLAUDE.md — slim section (engram version above the
//     Decision 1 floor, the surface where the bug was reproduced)
//   - Antigravity GEMINI.md — full section
//   - Codex engram-instructions.md — full + passive-capture concatenation
func TestDeliveryGuarantee_InstalledOutputs(t *testing.T) {
	t.Run("claude-slim", func(t *testing.T) {
		home := t.TempDir()
		engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")

		result, err := engram.InjectWithOptions(home, claudeAdapter(), engram.InjectOptions{Version: "1.18.0"})
		if err != nil {
			t.Fatalf("engram.InjectWithOptions(claude) error = %v", err)
		}
		if !result.Changed {
			t.Fatal("engram.InjectWithOptions(claude) changed = false")
		}

		claudeMD := readTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
		assertDeliveryGuarantee(t, "claude CLAUDE.md (slim)", claudeMD, installedSlimFailureContract)
		assertRejectsOppositeFailureContract(t, "claude CLAUDE.md (slim)", claudeMD, installedSlimFailureContract, slimPositiveFailureContract, slimOppositeFailureContract)
	})

	t.Run("antigravity-full", func(t *testing.T) {
		home := t.TempDir()
		engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")

		result, err := engram.Inject(home, antigravityAdapter())
		if err != nil {
			t.Fatalf("engram.Inject(antigravity) error = %v", err)
		}
		if !result.Changed {
			t.Fatal("engram.Inject(antigravity) changed = false")
		}

		rulesFile := readTestFile(t, filepath.Join(home, ".gemini", "GEMINI.md"))
		assertDeliveryGuarantee(t, "antigravity GEMINI.md (full)", rulesFile, installedFullFailureContract)
		assertRejectsOppositeFailureContract(t, "antigravity GEMINI.md (full)", rulesFile, installedFullFailureContract, fullPositiveFailureContract, fullOppositeFailureContract)
	})

	t.Run("codex-instructions", func(t *testing.T) {
		home := t.TempDir()
		engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")

		result, err := engram.Inject(home, codexAdapter())
		if err != nil {
			t.Fatalf("engram.Inject(codex) error = %v", err)
		}
		if !result.Changed {
			t.Fatal("engram.Inject(codex) changed = false")
		}

		instructions := readTestFile(t, filepath.Join(home, ".codex", "engram-instructions.md"))
		assertDeliveryGuarantee(t, "codex engram-instructions.md (full+passive-capture)", instructions, installedFullFailureContract)
		assertRejectsOppositeFailureContract(t, "codex engram-instructions.md (full+passive-capture)", instructions, installedFullFailureContract, fullPositiveFailureContract, fullOppositeFailureContract)
	})
}
