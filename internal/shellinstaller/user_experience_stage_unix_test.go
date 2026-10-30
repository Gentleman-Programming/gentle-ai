//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func experienceStage(t *testing.T) string {
	t.Helper()
	// userStageExperience guards the root with privateDirectory, which requires
	// the canonical spelling; on darwin t.TempDir is the /var alias.
	root := userUnixTempRoot(t)
	for _, path := range []string{root, filepath.Join(root, "config")} {
		if path == root {
			if err := os.Chmod(path, 0700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestUserStageExperienceCanonicalFiles(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			t.Run(persona+"-"+background, func(t *testing.T) {
				root := experienceStage(t)
				profile := UserExperience{Persona: persona, BackgroundSubagents: background}
				if err := userStageExperience(context.Background(), root, &profile); err != nil {
					t.Fatal(err)
				}
				want := "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \"" + background + "\"\n}\n"
				assertExperienceFile(t, filepath.Join(root, "config/background-subagents.json"), want)
				path := filepath.Join(root, "config/persona.json")
				if persona == "unmanaged" {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("unmanaged wrote a persona: %v", err)
					}
				} else {
					assertExperienceFile(t, path, "{\n  \"mode\": \""+persona+"\"\n}\n")
				}
			})
		}
	}
}

func assertExperienceFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("canonical bytes differ: %s: %q, %v", path, data, err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode() != 0600 {
		t.Fatalf("private file mode differs: %v, %v", info, err)
	}
}

func TestUserStageExperiencePreservesUnmanagedAndNil(t *testing.T) {
	root := experienceStage(t)
	path := filepath.Join(root, "config/persona.json")
	canary := "historical malformed persona bytes\n"
	if err := os.WriteFile(path, []byte(canary), 0600); err != nil {
		t.Fatal(err)
	}
	preimage, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := userStageExperience(nil, "missing", nil); err != nil {
		t.Fatalf("nil profile is not no-op: %v", err)
	}
	profile := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off"}
	if err := userStageExperience(context.Background(), root, &profile); err != nil {
		t.Fatal(err)
	}
	assertExperienceFile(t, path, canary)
	fresh, err := os.Lstat(path)
	if err != nil || !os.SameFile(preimage, fresh) || !preimage.ModTime().Equal(fresh.ModTime()) {
		t.Fatalf("unmanaged replaced or rewrote the persona: %v", err)
	}
}

func TestUserStageExperienceRefusesBeforeWrites(t *testing.T) {
	for _, scenario := range []string{"invalid", "rdd", "canceled", "existing", "symlink", "hardlink", "config-mode", "config-symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root := experienceStage(t)
			profile, ctx := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}, context.Background()
			config := filepath.Join(root, "config")
			background := filepath.Join(config, "background-subagents.json")
			outside := filepath.Join(t.TempDir(), "canary")
			if err := os.WriteFile(outside, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "invalid":
				profile.Persona = "custom"
			case "rdd":
				profile.GlobalRDD = "on"
				profile.SDKSource = "main-preview" // This consumer combination is still refused.
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "existing":
				if err := os.WriteFile(background, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(outside, background); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(outside, background); err != nil {
					t.Fatal(err)
				}
			case "config-symlink":
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(outside), config); err != nil {
					t.Fatal(err)
				}
			case "config-mode":
				if err := os.Chmod(config, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := userStageExperience(ctx, root, &profile); err == nil {
				t.Fatal("unsafe staged preference was accepted")
			}
			if _, err := os.Lstat(filepath.Join(config, "persona.json")); !os.IsNotExist(err) {
				t.Fatalf("refusal created persona: %v", err)
			}
			if data, err := os.ReadFile(outside); err != nil || string(data) != "preserve" {
				t.Fatalf("refusal changed outside file: %q, %v", data, err)
			}
			if scenario == "existing" || scenario == "symlink" || scenario == "hardlink" {
				if data, err := os.ReadFile(background); err != nil || string(data) != "preserve" {
					t.Fatalf("preimage changed: %q, %v", data, err)
				}
			} else if _, err := os.Lstat(background); !os.IsNotExist(err) {
				t.Fatalf("refusal wrote background: %v", err)
			}
		})
	}
}

func TestUserStageExperienceRejectsExplicitRDD(t *testing.T) {
	skipDarwinUnqualifiedMain(t)
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	profile.GlobalRDD = "off"
	profile.SDKSource = "main-preview"
	// The stock PNPM publication route has no Main producer, so the profile is
	// refused before any path is touched; Main itself is served by its worker.
	if err := userStageExperience(context.Background(), "missing", &profile); err == nil || !strings.Contains(err.Error(), "Main SDK requires its authenticated runtime producer") {
		t.Fatalf("explicit RDD was dropped or touched paths before refusal: %v", err)
	}
}
