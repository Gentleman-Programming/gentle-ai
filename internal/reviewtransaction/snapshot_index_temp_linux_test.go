//go:build linux

package reviewtransaction

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSnapshotIndexTemporaryPlacementAndCleanup(t *testing.T) {
	for _, scenario := range []string{"usable", "missing", "regular-file", "unwritable", "unsafe-shared", "symlink", "unsafe-ancestor"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "unwritable" && os.Geteuid() == 0 {
				t.Skip("root bypasses the process-temp permission denial")
			}
			gitDir, parent := t.TempDir(), t.TempDir()
			processTemp := filepath.Join(parent, "process-temp")
			switch scenario {
			case "usable", "unwritable":
				if err := os.Mkdir(processTemp, 0700); err != nil {
					t.Fatal(err)
				}
				if scenario == "unwritable" {
					if err := os.Chmod(processTemp, 0500); err != nil {
						t.Fatal(err)
					}
				}
			case "regular-file":
				if err := os.WriteFile(processTemp, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unsafe-shared":
				if err := os.Mkdir(processTemp, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(processTemp, 0777); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(t.TempDir(), processTemp); err != nil {
					t.Fatal(err)
				}
			case "unsafe-ancestor":
				if err := os.Mkdir(processTemp, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(parent, 0777); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"TMPDIR", "TEMP", "TMP"} {
				t.Setenv(name, processTemp)
			}
			file, cleanup, err := createSnapshotIndex(gitDir)
			if err != nil {
				t.Fatal(err)
			}
			path := file.Name()
			info, err := file.Stat()
			if err != nil || info.Mode().Perm() != 0600 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
				t.Fatal("private index permissions/ownership differ")
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if scenario == "usable" {
				directory := filepath.Dir(path)
				if filepath.Dir(directory) != processTemp {
					t.Fatal("usable process temporaries fell back into Git")
				}
				dirInfo, err := os.Stat(directory)
				if err != nil || dirInfo.Mode().Perm() != 0700 || dirInfo.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
					t.Fatal("index and sibling lock are not in an owned private directory")
				}
				if err := os.WriteFile(path+".lock", []byte("owned interrupted lock"), 0600); err != nil {
					t.Fatal(err)
				}
				cleanup()
				if _, err := os.Lstat(directory); !os.IsNotExist(err) {
					t.Fatal("owned scratch directory/lock survived cleanup")
				}
			} else {
				if filepath.Dir(path) != gitDir || !strings.HasPrefix(filepath.Base(path), ".gentle-ai-review-index-") {
					t.Fatal("restricted process temporaries lost the Git-local fallback")
				}
				cleanup()
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("fallback index survived cleanup")
				}
			}
		})
	}
}

func TestSnapshotIndexReportsUnavailableScratchWithoutCreatingFallback(t *testing.T) {
	parent := t.TempDir()
	for _, name := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(name, filepath.Join(parent, "missing-process-temp"))
	}
	gitDir := filepath.Join(parent, "missing-git-dir")
	file, cleanup, err := createSnapshotIndex(gitDir)
	if err == nil || file != nil || cleanup != nil {
		t.Fatal("unavailable scratch accepted or returned usable resources")
	}
	if _, err := os.Lstat(gitDir); !os.IsNotExist(err) {
		t.Fatal("scratch allocation fabricated a Git directory")
	}
}
