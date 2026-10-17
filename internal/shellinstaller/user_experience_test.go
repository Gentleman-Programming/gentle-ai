package shellinstaller

import "testing"

func TestUserExperienceLegacyEncoding(t *testing.T) {
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	encoded, err := profile.Encode()
	const legacy = `{"persona":"gentleman","background_subagents":"on"}`
	if err != nil || encoded != legacy {
		t.Fatalf("legacy profile encoding changed: %q, %v", encoded, err)
	}
}

func TestUserExperienceEncodeRejectsUnsupportedChoices(t *testing.T) {
	for _, profile := range []UserExperience{
		{Persona: "custom", BackgroundSubagents: "on"},
		{Persona: "neutral", BackgroundSubagents: "inherit"},
		{Persona: "neutral", BackgroundSubagents: "off", GlobalRDD: "inherit"},
	} {
		if _, err := profile.Encode(); err == nil {
			t.Fatalf("encoded unsupported experience: %+v", profile)
		}
	}
}
