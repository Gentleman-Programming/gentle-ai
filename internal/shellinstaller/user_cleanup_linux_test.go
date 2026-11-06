//go:build linux

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUserCleanupErrorClasses(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission denial requires an actual non-root process")
	}
	for _, kind := range []string{"permission", "replacement", "nil identity"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			workspace := filepath.Join(parent, "workspace")
			if err := os.Mkdir(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			identity, err := privateDirectory(workspace)
			if err != nil {
				t.Fatal(err)
			}
			blocked := filepath.Join(workspace, "blocked")
			if err := os.Mkdir(blocked, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(blocked, "sentinel"), []byte("retain"), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "permission":
				if err := os.Chmod(blocked, 0000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(blocked, 0700); err != nil {
						t.Error(err)
					}
				})
			case "replacement":
				if err := os.Rename(workspace, workspace+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(workspace, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(workspace, "foreign"), []byte("retain"), 0600); err != nil {
					t.Fatal(err)
				}
			case "nil identity":
				identity = nil
			}
			err = privateCleanup(workspace, identity)
			var failure *PrivateRuntimeError
			if kind == "permission" {
				if !os.IsPermission(err) || errors.As(err, &failure) {
					t.Fatalf("expected raw permission failure, got %v", err)
				}
				if err := os.Chmod(blocked, 0700); err != nil {
					t.Fatal(err)
				}
			} else if !errors.As(err, &failure) || failure.Kind != "uncertain" {
				t.Fatalf("expected held-identity refusal, got %v", err)
			}
			canary := filepath.Join(blocked, "sentinel")
			if kind == "replacement" {
				canary = filepath.Join(workspace, "foreign")
			}
			data, err := os.ReadFile(canary)
			if err != nil || string(data) != "retain" {
				t.Fatalf("cleanup erased refused evidence: %q %v", data, err)
			}
		})
	}
}
