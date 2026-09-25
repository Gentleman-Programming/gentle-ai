package agentguidance

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/pi"
)

func TestRetirePiSystemPromptBlocksPreservesUnownedAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	path := adapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	input := "user\n<!-- gentle-ai:persona -->\npersona\n<!-- /gentle-ai:persona -->\n<!-- gentle-ai:agent-routing -->\nrouting\n<!-- /gentle-ai:agent-routing -->\n<!-- gentle-ai:unknown -->\nkeep\n<!-- /gentle-ai:unknown -->\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || !first.Changed {
		t.Fatalf("first cleanup = %+v, %v", first, err)
	}
	want := "user\n\n\n<!-- gentle-ai:unknown -->\nkeep\n<!-- /gentle-ai:unknown -->\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("content = %q, %v", got, err)
	}
	second, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second cleanup = %+v, %v", second, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("second cleanup changed content = %q, %v", got, err)
	}
}

func TestRetirePiSystemPromptBlocksPreservesUnpairedMarkerAndMode(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	path := adapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	input := "user\r\n<!-- gentle-ai:persona -->\r\nmanaged\r\n<!-- /gentle-ai:persona -->\r\ntail"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || !result.Changed {
		t.Fatalf("cleanup = %+v, %v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "user\r\n\r\ntail" {
		t.Fatalf("unowned bytes = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, %v; want 0600", info, err)
		}
	}
	unpaired := "<!-- gentle-ai:persona -->\nunpaired"
	if err := os.WriteFile(path, []byte(unpaired), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || result.Changed {
		t.Fatalf("unpaired cleanup = %+v, %v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != unpaired {
		t.Fatalf("unpaired marker changed = %q, %v", got, err)
	}
}

func TestRetirePiSystemPromptBlocksRemovesOwnedEmptyFile(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	path := adapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("  \n<!-- gentle-ai:persona -->\nmanaged\n<!-- /gentle-ai:persona -->\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || !result.Changed {
		t.Fatalf("cleanup = %+v, %v", result, err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("owned-only file remains: %v", err)
	}
}

func TestRetirePiSystemPromptBlocksRefusesDirectSymlink(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	path := adapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := RetirePiSystemPromptBlocks(home, adapter); err == nil {
		t.Fatal("direct symlink accepted")
	}
	if link, err := os.Readlink(path); err != nil || link != target {
		t.Fatalf("symlink changed = %q, %v", link, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "keep" {
		t.Fatalf("target changed = %q, %v", got, err)
	}
}

func TestRetirePiSystemPromptBlocksAllowsSymlinkedParent(t *testing.T) {
	home := t.TempDir()
	adapter := pi.NewAdapter()
	path := adapter.SystemPromptFile(home)
	parent := filepath.Dir(path)
	realParent := filepath.Join(t.TempDir(), "agent")
	if err := os.MkdirAll(realParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(parent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realParent, parent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(path, []byte("<!-- gentle-ai:persona -->\nmanaged\n<!-- /gentle-ai:persona -->\nuser"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RetirePiSystemPromptBlocks(home, adapter)
	if err != nil || !result.Changed {
		t.Fatalf("linked-parent cleanup = %+v, %v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "\nuser" {
		t.Fatalf("linked-parent content = %q, %v", got, err)
	}
}
