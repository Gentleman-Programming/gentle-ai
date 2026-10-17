package shellinstaller

import (
	"strings"
	"testing"
)

func TestUserExperienceCoreRoundTripAndOptionalRDD(t *testing.T) {
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	encoded, err := profile.Encode()
	const legacy = `{"persona":"gentleman","background_subagents":"on"}`
	if err != nil || encoded != legacy {
		t.Fatalf("legacy profile encoding changed: %q, %v", encoded, err)
	}
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			for _, rdd := range []string{"", "on", "off"} {
				want := UserExperience{Persona: persona, BackgroundSubagents: background, GlobalRDD: rdd}
				encoded, err := want.Encode()
				if err != nil {
					t.Fatal(err)
				}
				got, err := DecodeUserExperience(encoded)
				if err != nil || got != want {
					t.Fatalf("profile round trip: %q -> %+v, %v", encoded, got, err)
				}
			}
		}
	}
}

func TestUserExperienceValidJSONAtSizeBoundary(t *testing.T) {
	valid, _ := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}.Encode()
	atLimit := strings.Repeat(" ", 256-len(valid)) + valid
	if _, err := DecodeUserExperience(atLimit); err != nil {
		t.Fatalf("valid JSON at 256 bytes refused: %v", err)
	}
	if _, err := DecodeUserExperience(" " + atLimit); err == nil {
		t.Fatal("valid JSON at 257 bytes bypassed the size guard")
	}
}

func TestUserExperienceRejectsAmbiguousOrUnsupportedChoices(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"persona":"custom","background_subagents":"on"}`,
		`{"persona":"neutral","background_subagents":"inherit"}`,
		`{"persona":"neutral","background_subagents":"off","global_rdd":""}`,
		`{"persona":"neutral","background_subagents":"off","global_rdd":"inherit"}`,
		`{"persona":"neutral","background_subagents":"off","global_rdd":null}`,
		`{"persona":"neutral","background_subagents":"off","global_rdd":true}`,
		`{"persona":"neutral","persona":"gentleman","background_subagents":"on"}`,
		`{"persona":"neutral","background_subagents":"off","sdd":true}`,
		`{"persona":"neutral","background_subagents":"off","strict_tdd":true}`,
		`{"persona":"neutral","background_subagents":"off"} {}`,
		`{"persona":"neutral","background_subagents":"off"`,
	} {
		if _, err := DecodeUserExperience(raw); err == nil {
			t.Fatalf("accepted unsupported experience: %q", raw)
		}
	}
	oversized := make([]byte, 257)
	if _, err := DecodeUserExperience(string(oversized)); err == nil {
		t.Fatal("accepted unbounded experience input")
	}
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
