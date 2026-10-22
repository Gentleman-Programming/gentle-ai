//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishAndRemoveUserShellLauncher(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, "install")
	bin := filepath.Join(target, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gentle-shell"), []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := PublishUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".local", "bin", "gentle-shell")
	if got, err := os.Readlink(link); err != nil || got != filepath.Join(bin, "gentle-shell") {
		t.Fatalf("launcher differs: %q %v", got, err)
	}
	if err := RemoveUserShellLauncher(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("launcher was not removed: %v", err)
	}
}
