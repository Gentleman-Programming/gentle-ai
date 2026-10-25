package shellinstaller

import "testing"

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
		{"explicit RDD on", UserExperience{Persona: "gentleman", BackgroundSubagents: "on", GlobalRDD: "on", SDKSource: "main-preview"}},
		{"explicit RDD off", UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GlobalRDD: "off", SDKSource: "main-preview"}},
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
