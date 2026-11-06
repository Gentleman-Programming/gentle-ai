//go:build linux && amd64

package shellinstaller

import "testing"

func TestCodeGraphPreferencesAllowMainAndLegacyGGA(t *testing.T) {
	for _, profile := range []UserExperience{
		{Persona: "gentleman", BackgroundSubagents: "on", GlobalRDD: "on", CodeGraph: "on", SDKSource: "main-preview"},
		{Persona: "gentleman", BackgroundSubagents: "on", GlobalRDD: "on", CodeGraph: "on", GGA: "on", GGAProvider: "codex"},
	} {
		if !userCodeGraphPreferencesSupported(&profile) {
			t.Fatalf("qualified CodeGraph preferences refused: %+v", profile)
		}
	}
}
