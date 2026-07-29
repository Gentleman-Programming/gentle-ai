//go:build windows

package reviewtransaction

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishNoReplaceSupportsExtendedWindowsPaths(t *testing.T) {
	dir := t.TempDir()
	for index := 0; index < 6; index++ {
		dir = filepath.Join(dir, strings.Repeat(string(rune('a'+index)), 48))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(source, []byte("receipt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishNoReplace(source, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "second.tmp")
	if err := os.WriteFile(second, []byte("conflict\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishNoReplace(second, destination); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing destination error = %v", err)
	}
}

func TestExtendedWindowsPathPreservesExistingNamespacePrefix(t *testing.T) {
	for _, path := range []string{`\\?\C:\repo\receipt.json`, `\\?\UNC\server\share\receipt.json`} {
		got, err := extendedWindowsPath(path)
		if err != nil || got != path {
			t.Fatalf("extendedWindowsPath(%q) = %q, %v", path, got, err)
		}
	}
}

func TestReplaceFileAtomicSupportsExtendedWindowsPaths(t *testing.T) {
	dir := t.TempDir()
	for index := 0; index < 6; index++ {
		dir = filepath.Join(dir, strings.Repeat(string(rune('a'+index)), 48))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "journal.json")
	if err := os.WriteFile(source, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceFileAtomic(source, destination); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "new\n" {
		t.Fatalf("destination = %q, want new payload", payload)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after replace: %v", err)
	}
}
