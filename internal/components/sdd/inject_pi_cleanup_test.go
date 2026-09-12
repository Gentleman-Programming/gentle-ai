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
	want := "user text before\n\n\n\n\n\n\n\n\n\nuser text after\n"
	if string(got) != want {
		t.Fatalf("APPEND_SYSTEM.md content = %q, want %q", string(got), want)
	}
}

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
	want := "user text before\n\n\n\nuser text after\n"
	if string(got) != want {
		t.Fatalf("APPEND_SYSTEM.md content = %q, want %q", string(got), want)
	}
}

func TestRetirePiSystemPromptBlocksDeletesOwnedEmptyFile(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)
	fixture := "   \n<!-- gentle-ai:persona -->\npersona body\n<!-- /gentle-ai:persona -->\n"
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(promptPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || !result.Changed {
		t.Fatalf("RetirePiSystemPromptBlocks() = %#v, %v", result, err)
	}
	if _, err := os.Lstat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("APPEND_SYSTEM.md remains after owned-only cleanup: %v", err)
	}
}

func TestRetirePiSystemPromptBlocksMissingFileIsNoop(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)
	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || result.Changed {
		t.Fatalf("RetirePiSystemPromptBlocks() = %#v, %v", result, err)
	}
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("RetirePiSystemPromptBlocks() created a file: %v", err)
	}
}

func TestRetirePiSystemPromptBlocksSecondRunIsNoop(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	promptPath := adapter.SystemPromptFile(home)
	fixture := "user text\n<!-- gentle-ai:sdd-orchestrator -->\nSDD body\n<!-- /gentle-ai:sdd-orchestrator -->\n"
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RetirePiSystemPromptBlocks(home, adapter); err != nil {
		t.Fatal(err)
	}
	cleaned, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second cleanup = %#v, %v", second, err)
	}
	got, err := os.ReadFile(promptPath)
	if err != nil || string(got) != string(cleaned) {
		t.Fatalf("second run mutated file: %q, %v", got, err)
	}
}

func TestRetirePiSystemPromptBlocksSafeguards(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	path := adapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user\r\n<!-- gentle-ai:persona -->\r\nmanaged\r\n<!-- /gentle-ai:persona -->\r\ntail"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RetirePiSystemPromptBlocks(home, adapter); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "user\r\n\r\ntail" {
		t.Fatalf("unowned bytes changed: %q", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode())
	}
	if err := os.WriteFile(path, []byte("<!-- gentle-ai:persona -->\nunpaired"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result, err := RetirePiSystemPromptBlocks(home, adapter); err != nil || result.Changed {
		t.Fatalf("unpaired cleanup = %#v, %v", result, err)
	}

	target := filepath.Join(home, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := RetirePiSystemPromptBlocks(home, adapter); err == nil {
		t.Fatal("direct symlink accepted")
	}
	if link, err := os.Readlink(path); err != nil || link != target {
		t.Fatalf("direct symlink changed: %q, %v", link, err)
	}
	if got, _ := os.ReadFile(target); string(got) != "keep" {
		t.Fatalf("direct symlink target changed: %q", got)
	}

	linkedHome, parent := t.TempDir(), filepath.Join(t.TempDir(), "agent")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(linkedHome, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent, filepath.Join(linkedHome, ".pi", "agent")); err != nil {
		t.Fatal(err)
	}
	linkedPath := filepath.Join(linkedHome, ".pi", "agent", "APPEND_SYSTEM.md")
	if err := os.WriteFile(linkedPath, []byte("<!-- gentle-ai:persona -->\nmanaged\n<!-- /gentle-ai:persona -->\nuser"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RetirePiSystemPromptBlocks(linkedHome, adapter); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(linkedPath); string(got) != "\nuser" {
		t.Fatalf("linked parent cleanup = %q", got)
	}
}
