//go:build windows

package filemerge

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicFollowsDirectoryJunction(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir(target) error = %v", err)
	}

	junction := filepath.Join(root, "junction")
	createDirectoryJunction(t, junction, target)

	content := []byte("junction payload\n")
	path := filepath.Join(junction, "config.txt")
	if _, err := WriteFileAtomic(path, content, 0o644); err != nil {
		t.Fatalf("WriteFileAtomic() through directory junction error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(target, "config.txt"))
	if err != nil {
		t.Fatalf("ReadFile(target/config.txt) error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("target content = %q, want %q", got, content)
	}
}

func TestWriteFileAtomicFollowsRelativeDirectoryJunction(t *testing.T) {
	root := t.TempDir()
	targetName := "relative-target-" + filepath.Base(root)
	target := filepath.Join(root, targetName)
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir(target) error = %v", err)
	}

	junction := filepath.Join(root, "junction")
	createDirectoryJunction(t, junction, targetName)

	content := []byte("relative junction payload\n")
	path := filepath.Join(junction, "config.txt")
	if _, err := WriteFileAtomic(path, content, 0o644); err != nil {
		t.Fatalf("WriteFileAtomic() through relative directory junction error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(target, "config.txt"))
	if err != nil {
		t.Fatalf("ReadFile(target/config.txt) error = %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("target content = %q, want %q", got, content)
	}
}

func TestEnsureAtomicParentDirRejectsUnsafeDirectoryJunctionTargets(t *testing.T) {
	tests := []struct {
		name          string
		replaceTarget func(t *testing.T, target string)
	}{
		{
			name: "broken target",
			replaceTarget: func(t *testing.T, target string) {
				if err := os.RemoveAll(target); err != nil {
					t.Fatalf("RemoveAll(target) error = %v", err)
				}
			},
		},
		{
			name: "target is not a directory",
			replaceTarget: func(t *testing.T, target string) {
				if err := os.RemoveAll(target); err != nil {
					t.Fatalf("RemoveAll(target) error = %v", err)
				}
				if err := os.WriteFile(target, []byte("not a directory"), 0o644); err != nil {
					t.Fatalf("WriteFile(target) error = %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatalf("Mkdir(target) error = %v", err)
			}
			junction := filepath.Join(root, "junction")
			createDirectoryJunction(t, junction, target)
			tt.replaceTarget(t, target)

			err := ensureAtomicParentDir(junction, filepath.Join(junction, "config.txt"))
			if err == nil {
				t.Fatal("ensureAtomicParentDir() error = nil, want unsafe junction target rejection")
			}
		})
	}
}

func TestWriteFileAtomicRejectsJunctionLoop(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	if err := os.Mkdir(first, 0o755); err != nil {
		t.Fatalf("Mkdir(first) error = %v", err)
	}
	if err := os.Mkdir(second, 0o755); err != nil {
		t.Fatalf("Mkdir(second) error = %v", err)
	}
	if err := os.Remove(first); err != nil {
		t.Fatalf("Remove(first) error = %v", err)
	}
	createDirectoryJunction(t, first, second)
	if err := os.Remove(second); err != nil {
		t.Fatalf("Remove(second) error = %v", err)
	}
	createDirectoryJunction(t, second, first)

	_, err := WriteFileAtomic(filepath.Join(first, "config.txt"), []byte("new\n"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want junction-loop rejection")
	}
}

func createDirectoryJunction(t *testing.T, link, target string) {
	t.Helper()
	command := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target)
	command.Dir = filepath.Dir(link)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J %q %q error = %v: %s", link, target, err, output)
	}
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Remove(junction) error = %v", err)
		}
	})
}
