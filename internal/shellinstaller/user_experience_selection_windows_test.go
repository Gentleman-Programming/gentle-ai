package shellinstaller

import (
	"encoding/json"
	"strings"
	"testing"
)

// The retained selection.json is the six-field provision ABI: install writes
// it without the profile and preferences live in the staged files, so any
// retained Experience (even a fully supported one) is refused, never ignored.
func TestUserWindowsRetainedSelectionRejectsUnsupportedExperience(t *testing.T) {
	for _, profile := range []UserExperience{
		{Persona: "gentleman", BackgroundSubagents: "on"},
		{Persona: "neutral", BackgroundSubagents: "off", StrictTDD: "on", GlobalRDD: "on", CodeGraph: "on", Context7: "off", SDKSource: "main-preview"},
	} {
		profile := profile
		req := UserInstallRequest{Destination: `C:\owned\shell`, Mode: "separate", Channel: "stable", Experience: &profile}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := userWindowsSelection(data, req.Destination)
		if err == nil || !strings.Contains(err.Error(), "retained selection carries an experience profile") {
			t.Fatalf("retained selection silently accepts an unconsumed profile: %+v, %v", got, err)
		}
		if got != (UserInstallRequest{}) {
			t.Fatalf("refused selection returned a request: %+v", got)
		}
	}
	req := UserInstallRequest{Destination: `C:\owned\shell`, Mode: "separate", Channel: "stable"}
	data, _ := json.Marshal(req)
	if got, err := userWindowsSelection(data, req.Destination); err != nil || got != req {
		t.Fatalf("legacy nil selection changed: %+v, %v", got, err)
	}
}
