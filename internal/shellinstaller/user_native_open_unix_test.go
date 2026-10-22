//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPrivateNativeOpenPinsSelectedFile(t *testing.T) {
	for _, name := range []string{"unchanged", "same-byte replacement", "symlink", "FIFO"} {
		t.Run(name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "source")
			data := []byte("owned source DATA\n")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := privatePhysical(path)
			if err != nil {
				t.Fatal(err)
			}
			if name != "unchanged" {
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				switch name {
				case "same-byte replacement":
					err = os.WriteFile(path, data, 0600)
				case "symlink":
					err = os.Symlink("source.old", path)
				case "FIFO":
					err = syscall.Mkfifo(path, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			selected, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			// Replace only an owned fixture after the same selection snapshot
			// the real privateNativeFile caller passes to this opener.
			file, err := privateNativeOpenFile(path, before)
			if (err == nil) != (name == "unchanged") {
				t.Fatalf("selected descriptor result: %v", err)
			}
			if file != nil {
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			after, err := os.Lstat(path)
			if err != nil || privateStamp(selected) != privateStamp(after) {
				t.Fatal("opener changed the controlled selection", err)
			}
			if name == "unchanged" {
				pin := fmt.Sprintf("%x", sha256.Sum256(data))
				if _, err := privateNativeFile(context.Background(), path, 0600, int64(len(data)), pin); err != nil {
					t.Fatal("real native-file consumer rejected a stable source", err)
				}
			}
		})
	}
}

func TestPrivateNativeEntriesRejectsAliasesAndNonDirectories(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	link, fifo, file := filepath.Join(root, "link"), filepath.Join(root, "fifo"), filepath.Join(root, "file")
	if err := os.Symlink("directory", link); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("owned DATA"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, link, fifo, file} {
		selected, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := privateNativeEntries(path)
		if (err == nil) != (path == directory) || len(entries) != 0 {
			t.Fatalf("physical directory result for %q: %v", filepath.Base(path), err)
		}
		after, err := os.Lstat(path)
		if err != nil || privateStamp(selected) != privateStamp(after) {
			t.Fatal("inventory changed the controlled selection", err)
		}
	}
}
