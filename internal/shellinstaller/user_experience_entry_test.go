package shellinstaller

import (
	"os"
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
	// The transport inspection refuses any request whose selected parent is
	// not an owned 0700 directory. Normalize a permissive parent explicitly:
	// under a restrictive umask (the guest qualification worker runs
	// UMask=0077) t.TempDir is already 0700 and the refusal would silently
	// disappear, admitting the unsupported profile instead.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	args := []string{parent + "/target", "separate", "", "", "model-only-token", "--experience", encoded}
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
