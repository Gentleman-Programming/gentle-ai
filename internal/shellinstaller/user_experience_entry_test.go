package shellinstaller

import (
	"strings"
	"testing"
)

func TestUserExperienceEntryPreservesSelectionAndRefusal(t *testing.T) {
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	profile.GlobalRDD = "off"
	profile.Context7 = "on"
	profile.StrictTDD = "on"
	profile.GGA = "on"
	profile.GGAProvider = "codex"
	encoded, _ := profile.Encode()
	args := []string{t.TempDir() + "/target", "separate", "", "", "model-only-token", "--experience", encoded}
	req, err := UserInstallFromEntry(args)
	if err != nil || req.Experience == nil || *req.Experience != profile {
		t.Fatalf("complete profile lost: %+v, %v", req, err)
	}
	if token, err := InspectUserInstall(req); err == nil || token != "" {
		t.Fatal("transport admitted unsupported public effects")
	}
	for _, raw := range []string{"null", strings.Repeat(" ", 257) + encoded} {
		args[len(args)-1] = raw
		if _, err := UserInstallFromEntry(args); err == nil {
			t.Fatal("malformed/oversized raw profile admitted")
		}
	}
}
