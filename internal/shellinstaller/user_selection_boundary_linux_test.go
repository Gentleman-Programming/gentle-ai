//go:build linux

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserInstallSelectionRejectsUnsafePathBeforeEffects(t *testing.T) {
	for _, name := range []string{"with space", "o'brien", "semi;colon", "dollar$", "unicode-é"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			parent := filepath.Join(root, name)
			if err := os.Mkdir(parent, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(parent, "target")
			if err := ValidateUserInstall(UserInstallRequest{Destination: target, Mode: "separate"}); err == nil {
				t.Fatal("unsafe helper path admitted before runtime acquisition")
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("rejected selection changed destination: %v", err)
			}
		})
	}
}

func TestUserInstallSelectionRejectsInverseContainment(t *testing.T) {
	for _, selected := range []string{"prefix", "agent", "disjoint"} {
		t.Run(selected, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			prefix, agent := filepath.Join(root, "prefix"), filepath.Join(root, "agent")
			cli := filepath.Join(prefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
			if err := os.MkdirAll(filepath.Dir(cli), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(agent, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cli, []byte("fixture, never executed"), 0400); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "prefix-independent")
			if selected != "disjoint" {
				target = filepath.Join(root, selected, "nested-target")
			}
			err := ValidateUserInstall(UserInstallRequest{Destination: target, Mode: "shared", SharedPrefix: prefix, SharedAgent: agent})
			if (err != nil) != (selected != "disjoint") {
				t.Fatalf("selection %s: err=%v", selected, err)
			}
		})
	}
}
