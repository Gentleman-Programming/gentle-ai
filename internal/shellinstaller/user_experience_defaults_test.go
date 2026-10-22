package shellinstaller

import "testing"

func TestDefaultUserExperienceIsIndependentAndLegacyBound(t *testing.T) {
	want := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	first := DefaultUserExperience()
	if first != want {
		t.Fatalf("default experience changed or selected optional effects: %+v", first)
	}
	encoded, err := first.Encode()
	if err != nil || encoded != `{"persona":"gentleman","background_subagents":"on"}` {
		t.Fatalf("default experience changed legacy bytes: %q, %v", encoded, err)
	}
	decoded, err := DecodeUserExperience(encoded)
	if err != nil || decoded != want {
		t.Fatalf("default experience lost its complete round trip: %+v, %v", decoded, err)
	}
	first.Persona = "unmanaged"
	first.BackgroundSubagents = "off"
	first.GGA, first.GGAProvider = "on", "codex"
	if again := DefaultUserExperience(); again != want {
		t.Fatalf("editing one default selection changed another: %+v", again)
	}
}
