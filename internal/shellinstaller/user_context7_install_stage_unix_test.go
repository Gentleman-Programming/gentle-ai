//go:build linux || darwin

package shellinstaller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestUserPrepareInstallStageContext7(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			for _, choice := range []string{"", "on", "off"} {
				t.Run(persona+"-"+background+"-"+choice, func(t *testing.T) {
					root := installStagePath(t)
					profile := UserExperience{Persona: persona, BackgroundSubagents: background, Context7: choice}
					before := profile
					encoded, err := profile.Encode()
					if err != nil {
						t.Fatal(err)
					}
					if err := userPrepareInstallStage(context.Background(), root, "separate", &profile); err != nil {
						t.Fatal(err)
					}
					assertExperienceFile(t, filepath.Join(root, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \""+background+"\"\n}\n")
					personaPath := filepath.Join(root, "config/persona.json")
					if persona == "unmanaged" {
						if _, err := os.Lstat(personaPath); !os.IsNotExist(err) {
							t.Fatal("unmanaged acquired a persona file", err)
						}
					} else {
						assertExperienceFile(t, personaPath, "{\n  \"mode\": \""+persona+"\"\n}\n")
					}
					mcp := filepath.Join(root, "agent/mcp.json")
					if choice == "" {
						if _, err := os.Lstat(mcp); !os.IsNotExist(err) {
							t.Fatal("absent preference became MCP policy", err)
						}
					} else {
						assertExperienceFile(t, mcp, fmt.Sprintf("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"enabled\": %t,\n      \"exposure\": \"codemode\",\n      \"url\": \"https://mcp.context7.com/mcp\"\n    }\n  }\n}\n", choice == "on"))
					}
					if _, err := os.Lstat(filepath.Join(root, "config/mcp.json")); !os.IsNotExist(err) {
						t.Fatal("stage created an unused config-home MCP policy", err)
					}
					afterEncoded, err := profile.Encode()
					if err != nil || profile != before || afterEncoded != encoded {
						t.Fatal("staging modified profile or its encoded confirmation input", err)
					}
				})
			}
		}
	}
}

func TestUserPrepareInstallStageContext7ModeRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, mode, choice string
		nilProfile         bool
	}{
		{name: "unknown-mode", mode: "auto"},
		{name: "empty-mode"},
		{name: "nil-unknown-mode", mode: "auto", nilProfile: true},
		{name: "nil-empty-mode", nilProfile: true},
		{name: "invalid-choice", mode: "separate", choice: "auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := installStagePath(t)
			parent := filepath.Dir(root)
			canary := filepath.Join(parent, "personal-mcp.json")
			assertData := "historical personal malformed MCP bytes\n"
			if err := os.WriteFile(canary, []byte(assertData), 0600); err != nil {
				t.Fatal(err)
			}
			beforeParent, err := os.Stat(parent)
			if err != nil {
				t.Fatal(err)
			}
			beforeCanary, err := os.Stat(canary)
			if err != nil {
				t.Fatal(err)
			}
			profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
			profile.Context7 = tc.choice
			selected := &profile
			if tc.nilProfile {
				selected = nil
			}
			err = userPrepareInstallStage(context.Background(), root, tc.mode, selected)
			if err == nil {
				t.Fatal("invalid mode or Context7 preference was admitted", err)
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("refusal created the stage", err)
			}
			afterParent, err := os.Stat(parent)
			if err != nil || !os.SameFile(beforeParent, afterParent) || beforeParent.Mode() != afterParent.Mode() || !beforeParent.ModTime().Equal(afterParent.ModTime()) {
				t.Fatal("refusal modified the parent preimage", err)
			}
			assertExperienceFile(t, canary, assertData)
			afterCanary, err := os.Stat(canary)
			if err != nil || !os.SameFile(beforeCanary, afterCanary) || !beforeCanary.ModTime().Equal(afterCanary.ModTime()) {
				t.Fatal("refusal modified the personal canary preimage", err)
			}
		})
	}
}

func TestUserPrepareInstallStageSharedWithoutContext7(t *testing.T) {
	for _, profile := range []*UserExperience{nil, {Persona: "unmanaged", BackgroundSubagents: "off"}} {
		root := installStagePath(t)
		if err := userPrepareInstallStage(context.Background(), root, "shared", profile); err != nil {
			t.Fatal(err)
		}
		if profile != nil {
			assertExperienceFile(t, filepath.Join(root, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \"off\"\n}\n")
		}
		for _, name := range []string{"agent/mcp.json", "config/mcp.json", "config/persona.json"} {
			if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
				t.Fatal("Shared staging created an unselected preference", name, err)
			}
		}
	}
}

func TestUserPrepareInstallStageContext7PreservesOccupiedRoot(t *testing.T) {
	for _, choice := range []string{"on", "off"} {
		t.Run(choice, func(t *testing.T) {
			root := installStagePath(t)
			if err := os.MkdirAll(filepath.Join(root, "config"), 0700); err != nil {
				t.Fatal(err)
			}
			persona := filepath.Join(root, "config/persona.json")
			canary := "malformed historical persona bytes\n"
			if err := os.WriteFile(persona, []byte(canary), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(persona)
			if err != nil {
				t.Fatal(err)
			}
			profile := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", Context7: choice}
			if err := userPrepareInstallStage(context.Background(), root, "separate", &profile); err == nil {
				t.Fatal("canonical MCP staging reused an occupied runtime")
			}
			assertExperienceFile(t, persona, canary)
			after, err := os.Stat(persona)
			if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("staging touched historical unmanaged persona", err)
			}
			for _, name := range []string{"agent", "config/background-subagents.json"} {
				if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
					t.Fatal("occupied-root refusal created an installer object", err)
				}
			}
		})
	}
}
