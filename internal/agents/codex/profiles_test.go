package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSddProfilePathsIsReadOnlyLegacyInventory(t *testing.T) {
	root := t.TempDir()
	want := []string{
		filepath.Join(root, "sdd-strong.config.toml"),
		filepath.Join(root, "sdd-mid.config.toml"),
		filepath.Join(root, "sdd-cheap.config.toml"),
	}
	if got := SddProfilePaths(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy paths = %v, want %v", got, want)
	}
	for _, path := range want {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("path inventory created profile %s: %v", path, err)
		}
	}
}
