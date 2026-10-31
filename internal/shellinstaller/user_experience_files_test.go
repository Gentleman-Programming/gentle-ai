package shellinstaller

import (
	"runtime"
	"testing"
)

func TestUserStageExperienceFilesCanonical(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			t.Run(persona+"-"+background, func(t *testing.T) {
				profile := UserExperience{Persona: persona, BackgroundSubagents: background}
				before := profile
				files, err := userStageExperienceFiles(&profile)
				if err != nil {
					t.Fatal(err)
				}
				count := 2
				if persona == "unmanaged" {
					count = 1
				}
				if len(files) != count || profile != before {
					t.Fatalf("unexpected files or mutated profile: %v, %+v", files, profile)
				}
				backgroundBytes := "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \"" + background + "\"\n}\n"
				if files[0].name != "background-subagents.json" || string(files[0].data) != backgroundBytes {
					t.Fatalf("canonical background bytes differ: %+v", files[0])
				}
				if persona != "unmanaged" {
					personaBytes := "{\n  \"mode\": \"" + persona + "\"\n}\n"
					if files[1].name != "persona.json" || string(files[1].data) != personaBytes {
						t.Fatalf("canonical persona bytes differ: %+v", files[1])
					}
				}
				// Returned buffers must not alias later selections.
				files[0].data[0] = '!'
				fresh, err := userStageExperienceFiles(&profile)
				if err != nil || string(fresh[0].data) != backgroundBytes {
					t.Fatalf("selections share mutable bytes: %v", err)
				}
			})
		}
	}
}

func TestUserStageExperienceFilesRefusals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile UserExperience
	}{
		{"empty", UserExperience{}},
		{"unknown persona", UserExperience{Persona: "custom", BackgroundSubagents: "on"}},
		{"unknown background", UserExperience{Persona: "neutral", BackgroundSubagents: "inherit"}},
		{"invalid RDD", UserExperience{Persona: "neutral", BackgroundSubagents: "off", GlobalRDD: "inherit"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.profile
			if files, err := userStageExperienceFiles(&tc.profile); err == nil || files != nil || tc.profile != before {
				t.Fatalf("refusal leaked files or changed selection: %v, %v", files, err)
			}
		})
	}
	if files, err := userStageExperienceFiles(nil); err != nil || files != nil {
		t.Fatalf("nil legacy selection gained files: %v, %v", files, err)
	}
}

// Main preview + explicit RDD: Windows stages it in parity with Stable (the
// RDD state itself is written by the Windows stage writer, not as a config
// file); every other platform keeps the authenticated-producer refusal.
func TestUserStageExperienceFilesMainPreviewRDD(t *testing.T) {
	for _, tc := range []struct {
		name    string
		names   []string
		profile UserExperience
	}{
		{"explicit RDD on", []string{"background-subagents.json", "persona.json"}, UserExperience{Persona: "gentleman", BackgroundSubagents: "on", GlobalRDD: "on", SDKSource: "main-preview"}},
		{"explicit RDD off", []string{"background-subagents.json"}, UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GlobalRDD: "off", SDKSource: "main-preview"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.profile
			files, err := userStageExperienceFiles(&tc.profile)
			if tc.profile != before {
				t.Fatalf("staging changed the selection: %+v", tc.profile)
			}
			if runtime.GOOS != "windows" {
				if err == nil || files != nil {
					t.Fatalf("Main preview RDD admitted off Windows: %v, %v", files, err)
				}
				return
			}
			if err != nil || len(files) != len(tc.names) {
				t.Fatalf("Windows Main preview RDD refused or files differ: %v, %v", files, err)
			}
			for index, name := range tc.names {
				if files[index].name != name {
					t.Fatalf("staged file %d = %q, want %q", index, files[index].name, name)
				}
			}
		})
	}
}
