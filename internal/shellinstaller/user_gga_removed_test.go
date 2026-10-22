package shellinstaller

import "testing"

func TestUserForceGGAInert(t *testing.T) {
	profile := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "on", GGAProvider: "codex"}
	userForceGGAInert(&profile)
	if profile.GGA != "off" || profile.GGAProvider != "" {
		t.Fatalf("GGA was not forced inert: %+v", profile)
	}
	if err := profile.Validate(); err != nil {
		t.Fatalf("retired GGA should not validate: %v", err)
	}
}
