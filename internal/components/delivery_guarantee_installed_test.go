package components_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
)

// deliveryGuaranteePhrases are the semantic rules that every installed
// protocol output must state so memory bookkeeping can never replace the
// final user-facing reply — including when a memory call fails or times out
// (issue #1042 acceptance scope). Matching is done on whitespace-normalized,
// lowercased content so the assertions survive reformatting and line wraps,
// unlike golden byte-equality.
var deliveryGuaranteePhrases = []struct {
	name   string
	phrase string
}{
	{"memory-is-bookkeeping", "bookkeeping"},
	{"saving-never-counts-as-answering", "never counts as answering"},
	{"turn-ends-with-answer-as-final-message", "final message"},
	{"save-before-composing-the-reply", "before composing"},
	{"covers-memory-call-failure-and-timeout", "fails or times out"},
	{"failure-still-delivers-the-answer", "answer anyway"},
}

func assertDeliveryGuarantee(t *testing.T, surface string, content []byte) {
	t.Helper()
	normalized := strings.ToLower(strings.Join(strings.Fields(string(content)), " "))
	for _, inv := range deliveryGuaranteePhrases {
		if !strings.Contains(normalized, inv.phrase) {
			t.Errorf("installed output %q violates delivery-guarantee invariant %q: missing phrase %q", surface, inv.name, inv.phrase)
		}
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
		assertDeliveryGuarantee(t, "claude CLAUDE.md (slim)", claudeMD)
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
		assertDeliveryGuarantee(t, "antigravity GEMINI.md (full)", rulesFile)
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
		assertDeliveryGuarantee(t, "codex engram-instructions.md (full+passive-capture)", instructions)
	})
}
