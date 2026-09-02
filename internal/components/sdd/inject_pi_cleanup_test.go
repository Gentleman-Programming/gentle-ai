package sdd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
)

func TestRetirePiSystemPromptBlocksStripsManagedSectionsAndPreservesUserContent(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)

	fixture := "user text before\n" +
		"\n" +
		"<!-- gentle-ai:sdd-orchestrator -->\n" +
		"SDD body\n" +
		"<!-- /gentle-ai:sdd-orchestrator -->\n" +
		"\n" +
		"<!-- gentle-ai:strict-tdd-mode -->\n" +
		"Strict TDD Mode: enabled\n" +
		"<!-- /gentle-ai:strict-tdd-mode -->\n" +
		"\n" +
		"<!-- gentle-ai:persona -->\n" +
		"persona body\n" +
		"<!-- /gentle-ai:persona -->\n" +
		"\n" +
		"<!-- gentle-ai:codegraph-guidance -->\n" +
		"codegraph body\n" +
		"<!-- /gentle-ai:codegraph-guidance -->\n" +
		"\n" +
		"user text after\n"

	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(promptPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil {
		t.Fatalf("RetirePiSystemPromptBlocks() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("RetirePiSystemPromptBlocks() Changed = false, want true")
	}
	if got, want := result.Files, []string{promptPath}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("RetirePiSystemPromptBlocks() Files = %v, want %v", got, want)
	}

	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "user text before\n\nuser text after\n"
	if string(got) != want {
		t.Fatalf("APPEND_SYSTEM.md content = %q, want %q", string(got), want)
	}
}

// TestRetirePiSystemPromptBlocksStripsAgentRoutingBlock covers issue #4063:
// an older build could have written a gentle-ai:agent-routing block into
// Pi's APPEND_SYSTEM.md before the routing step learned to skip agents that
// do not support a managed system prompt. This block is retired the same way
// as the other legacy sections.
func TestRetirePiSystemPromptBlocksStripsAgentRoutingBlock(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)

	fixture := "user text before\n" +
		"\n" +
		"<!-- gentle-ai:agent-routing -->\n" +
		"routing body\n" +
		"<!-- /gentle-ai:agent-routing -->\n" +
		"\n" +
		"user text after\n"

	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(promptPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil {
		t.Fatalf("RetirePiSystemPromptBlocks() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("RetirePiSystemPromptBlocks() Changed = false, want true")
	}

	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "user text before\n\nuser text after\n"
	if string(got) != want {
		t.Fatalf("APPEND_SYSTEM.md content = %q, want %q", string(got), want)
	}
}

func TestRetirePiSystemPromptBlocksKeepsWhitespaceOnlyFile(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)

	fixture := "   \n" +
		"<!-- gentle-ai:persona -->\n" +
		"persona body\n" +
		"<!-- /gentle-ai:persona -->\n"

	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(promptPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil {
		t.Fatalf("RetirePiSystemPromptBlocks() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("RetirePiSystemPromptBlocks() Changed = false, want true")
	}

	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("APPEND_SYSTEM.md was removed after whitespace-only cleanup, want it kept: %v", err)
	}
	want := "   \n"
	if string(got) != want {
		t.Fatalf("APPEND_SYSTEM.md content = %q, want %q", string(got), want)
	}
}

func TestRetirePiSystemPromptBlocksMissingFileIsNoop(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)

	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil {
		t.Fatalf("RetirePiSystemPromptBlocks() error = %v", err)
	}
	if result.Changed {
		t.Fatal("RetirePiSystemPromptBlocks() Changed = true, want false for missing file")
	}
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("RetirePiSystemPromptBlocks() created a file that did not exist before, stat err = %v", err)
	}
}

func TestRetirePiSystemPromptBlocksSecondRunIsNoop(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)

	fixture := "user text\n" +
		"\n" +
		"<!-- gentle-ai:sdd-orchestrator -->\n" +
		"SDD body\n" +
		"<!-- /gentle-ai:sdd-orchestrator -->\n"

	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(promptPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := RetirePiSystemPromptBlocks(home, adapter); err != nil {
		t.Fatalf("first RetirePiSystemPromptBlocks() error = %v", err)
	}
	cleaned, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("ReadFile after first run: %v", err)
	}

	second, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil {
		t.Fatalf("second RetirePiSystemPromptBlocks() error = %v", err)
	}
	if second.Changed {
		t.Fatal("second RetirePiSystemPromptBlocks() Changed = true, want false (idempotent)")
	}

	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("ReadFile after second run: %v", err)
	}
	if string(got) != string(cleaned) {
		t.Fatalf("second run mutated file: got %q, want %q", string(got), string(cleaned))
	}
}
