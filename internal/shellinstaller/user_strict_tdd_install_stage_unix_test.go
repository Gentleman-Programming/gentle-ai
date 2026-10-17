//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUserPrepareInstallStageStrictTDD(t *testing.T) {
	for _, choice := range []string{"", "on", "off"} {
		t.Run(choice, func(t *testing.T) {
			root := installStagePath(t)
			raw := `{"persona":"unmanaged","background_subagents":"off"`
			if choice != "" {
				raw += `,"strict_tdd":"` + choice + `"`
			}
			profile, err := DecodeUserExperience(raw + "}")
			if err != nil {
				t.Fatal(err)
			}
			before := profile
			if err := userPrepareInstallStage(context.Background(), root, "separate", &profile); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "config/strict-tdd.json")
			if choice == "" {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("absence acquired a strict policy", err)
				}
			} else {
				assertExperienceFile(t, path, "{\n  \"schema\": \"gentle-pi.strict-tdd/v1\",\n  \"policy\": \""+choice+"\"\n}\n")
			}
			if profile != before {
				t.Fatal("stage mutated the confirmation profile")
			}
			for _, name := range []string{"config/persona.json", "agent/strict-tdd.json", "project/strict-tdd.json"} {
				if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
					t.Fatal("stage wrote an unselected or unused policy", name, err)
				}
			}
		})
	}
}

func TestUserPrepareInstallStageStrictTDDRefusesBeforeWrites(t *testing.T) {
	for _, choice := range []string{"on", "off", "auto"} {
		t.Run(choice, func(t *testing.T) {
			root := installStagePath(t)
			parent := filepath.Dir(root)
			before, err := os.Stat(parent)
			if err != nil {
				t.Fatal(err)
			}
			profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
			profile.StrictTDD = choice
			if err := userPrepareInstallStage(context.Background(), root, "shared", &profile); err == nil {
				t.Fatal("Shared or invalid Strict TDD was accepted")
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("refusal created a stage", err)
			}
			after, err := os.Stat(parent)
			if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("refusal changed parent preimage", err)
			}
		})
	}
}

func TestUserPrepareInstallStageStrictTDDNilIsHistorical(t *testing.T) {
	root := installStagePath(t)
	if err := userPrepareInstallStage(context.Background(), root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "config/strict-tdd.json")); !os.IsNotExist(err) {
		t.Fatal("nil profile acquired policy", err)
	}
}
