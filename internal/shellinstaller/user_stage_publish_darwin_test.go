//go:build darwin

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// publishStageFixture returns a verified-tree stand-in and a fresh destination
// beside it in one private parent.
func publishStageFixture(t *testing.T) (string, string) {
	t.Helper()
	parent := t.TempDir()
	stage := filepath.Join(parent, ".gentle-node-stage.fixture", "node")
	if err := os.MkdirAll(filepath.Join(stage, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "BOOTSTRAP-SHA256SUMS"), []byte("inventory\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return stage, filepath.Join(parent, "node")
}

// publishSnapshot records every path below root with its kind and contents.
func publishSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		if info.IsDir() {
			snapshot[relative] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		snapshot[relative] = "file:" + string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestDarwinUserPublishStageRenamesIntoFreshDestination(t *testing.T) {
	stage, destination := publishStageFixture(t)
	before := publishSnapshot(t, stage)
	if err := userPublishStage(stage, destination); err != nil {
		t.Fatalf("userPublishStage = %v", err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("stage still present after publication: %v", err)
	}
	if after := publishSnapshot(t, destination); len(after) != len(before) || after["BOOTSTRAP-SHA256SUMS"] != before["BOOTSTRAP-SHA256SUMS"] || after["bin"] != "dir" {
		t.Fatalf("published tree = %v; want %v", after, before)
	}
}

func TestDarwinUserPublishStageRefusesExistingDestination(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, destination string)
	}{
		{"empty directory", func(t *testing.T, destination string) {
			if err := os.Mkdir(destination, 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"non-empty directory", func(t *testing.T, destination string) {
			if err := os.Mkdir(destination, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(destination, "racer"), []byte("same uid\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"file", func(t *testing.T, destination string) {
			if err := os.WriteFile(destination, []byte("same uid\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage, destination := publishStageFixture(t)
			tc.prepare(t, destination)
			stageBefore, destinationBefore := publishSnapshot(t, stage), publishSnapshot(t, destination)
			err := userPublishStage(stage, destination)
			if !errors.Is(err, syscall.EEXIST) {
				t.Fatalf("userPublishStage = %v; want EEXIST", err)
			}
			if after := publishSnapshot(t, destination); !sameSnapshot(after, destinationBefore) {
				t.Fatalf("destination mutated: %v; want %v", after, destinationBefore)
			}
			if after := publishSnapshot(t, stage); !sameSnapshot(after, stageBefore) {
				t.Fatalf("stage mutated: %v; want %v", after, stageBefore)
			}
		})
	}
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for path, kind := range a {
		if b[path] != kind {
			return false
		}
	}
	return true
}
