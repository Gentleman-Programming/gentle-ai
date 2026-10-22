//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUserExperienceFreshSeparateAdmission(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			t.Run(persona+"-"+background, func(t *testing.T) {
				parent := t.TempDir()
				if err := os.Chmod(parent, 0700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(parent, "target")
				profile := UserExperience{Persona: persona, BackgroundSubagents: background}
				req := UserInstallRequest{Destination: target, Mode: "separate", Experience: &profile}
				token, err := InspectUserInstall(req)
				if err != nil || token == "" {
					t.Fatalf("working base profile cannot reach physical review: %q, %v", token, err)
				}
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatal("inspection installed before consent")
				}
				if err := userPrepareInstallStage(context.Background(), target, req.Mode, req.Experience); err != nil {
					t.Fatal(err)
				}
				assertExperienceFile(t, filepath.Join(target, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \""+background+"\"\n}\n")
				personaPath := filepath.Join(target, "config/persona.json")
				if persona == "unmanaged" {
					if _, err := os.Lstat(personaPath); !os.IsNotExist(err) {
						t.Fatal("unmanaged writes persona")
					}
				} else {
					assertExperienceFile(t, personaPath, "{\n  \"mode\": \""+persona+"\"\n}\n")
				}
			})
		}
	}
}

func TestUserExperienceAdmissionKeepsScopeAndFeatureRefusals(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	profile := DefaultUserExperience()
	existing := filepath.Join(parent, "existing")
	if err := os.Mkdir(existing, 0700); err != nil {
		t.Fatal(err)
	}
	for _, req := range []UserInstallRequest{
		{Destination: existing, Mode: "separate", Experience: &profile},
		{Destination: filepath.Join(parent, "shared"), Mode: "shared", Experience: &profile},
	} {
		if token, err := InspectUserInstall(req); err == nil || token != "" {
			t.Fatal("preferences admitted outside fresh Separate")
		}
	}
	profile.CodeGraph = "on"
	profile.GGA, profile.GGAProvider = "on", "codex"
	if token, err := InspectUserInstall(UserInstallRequest{Destination: filepath.Join(parent, "advanced"), Mode: "separate", Experience: &profile}); err != nil || token == "" {
		t.Fatalf("qualified advanced effects refused: %v", err)
	}
	profile.GGAProvider = ""
	if token, err := InspectUserInstall(UserInstallRequest{Destination: filepath.Join(parent, "unprovided"), Mode: "separate", Experience: &profile}); err != nil || token == "" {
		t.Fatalf("GGA ON without provider refused: %v", err)
	}
}
