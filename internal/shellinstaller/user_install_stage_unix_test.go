//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func installStagePath(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "installed")
}

func TestUserPrepareInstallStagePreservesLegacyLayout(t *testing.T) {
	root := installStagePath(t)
	if err := userPrepareInstallStage(context.Background(), root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "home", "tmp", "config", "state", "agent", "project", "bin"} {
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil || info.Mode() != os.ModeDir|0700 {
			t.Fatalf("legacy directory %q changed: %v, %v", name, info, err)
		}
	}
	for _, name := range []string{"user.npmrc", "global.npmrc"} {
		assertExperienceFile(t, filepath.Join(root, "config", name), "")
	}
	for _, name := range []string{"persona.json", "background-subagents.json"} {
		if _, err := os.Lstat(filepath.Join(root, "config", name)); !os.IsNotExist(err) {
			t.Fatalf("legacy nil profile wrote %s: %v", name, err)
		}
	}
}

func TestUserPrepareInstallStageAppliesCanonicalProfile(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			t.Run(persona+"-"+background, func(t *testing.T) {
				root := installStagePath(t)
				profile := UserExperience{Persona: persona, BackgroundSubagents: background}
				if err := userPrepareInstallStage(context.Background(), root, "separate", &profile); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(filepath.Join(root, "agent", "mcp.json")); !os.IsNotExist(err) {
					t.Fatalf("ordinary profile wrote MCP policy: %v", err)
				}
				assertExperienceFile(t, filepath.Join(root, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \""+background+"\"\n}\n")
				path := filepath.Join(root, "config/persona.json")
				if persona == "unmanaged" {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("unmanaged became a persona write: %v", err)
					}
				} else {
					assertExperienceFile(t, path, "{\n  \"mode\": \""+persona+"\"\n}\n")
				}
			})
		}
	}
}

func TestUserPrepareInstallStageRefusesBeforeCreation(t *testing.T) {
	for _, scenario := range []string{"invalid", "nil-context", "canceled", "parent-mode", "relative", "existing-file", "existing-directory", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root := installStagePath(t)
			profile, ctx := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}, context.Background()
			switch scenario {
			case "invalid":
				profile.BackgroundSubagents = "auto"
			case "nil-context":
				ctx = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "parent-mode":
				if err := os.Chmod(filepath.Dir(root), 0755); err != nil {
					t.Fatal(err)
				}
			case "relative":
				root = "."
			case "existing-file":
				if err := os.WriteFile(root, []byte("personal preimage"), 0600); err != nil {
					t.Fatal(err)
				}
			case "existing-directory":
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("unresolved-personal-preimage", root); err != nil {
					t.Fatal(err)
				}
			}
			before, beforeErr := os.Lstat(root)
			if err := userPrepareInstallStage(ctx, root, "separate", &profile); err == nil {
				t.Fatal("unsafe or unsupported stage was created")
			}
			after, afterErr := os.Lstat(root)
			if beforeErr == nil {
				if afterErr != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
					t.Fatalf("refusal modified the preimage: %v", afterErr)
				}
			} else if !os.IsNotExist(afterErr) {
				t.Fatalf("refusal created a stage: %v", afterErr)
			}
		})
	}
}
