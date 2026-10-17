package shellinstaller

import (
	"encoding/json"
	"testing"
)

func TestUserWindowsRetainedSelectionRejectsUnsupportedExperience(t *testing.T) {
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	req := UserInstallRequest{Destination: `C:\owned\shell`, Mode: "separate", Channel: "stable", Experience: &profile}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := userWindowsSelection(data, req.Destination); err == nil {
		t.Fatal("retained selection silently accepts an unconsumed profile")
	}
	req.Experience = nil
	data, _ = json.Marshal(req)
	if got, err := userWindowsSelection(data, req.Destination); err != nil || got != req {
		t.Fatalf("legacy nil selection changed: %+v, %v", got, err)
	}
}
