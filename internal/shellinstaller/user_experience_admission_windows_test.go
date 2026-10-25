//go:build windows

package shellinstaller

import (
	"strings"
	"testing"
)

func TestUserWindowsExperienceRequiresFreshTarget(t *testing.T) {
	if err := UserKernelCheck(); err != nil {
		t.Skip("native Windows11 unelevated qualification prerequisite: " + err.Error())
	}
	profile := DefaultUserExperience()
	token, err := InspectUserInstall(UserInstallRequest{Destination: t.TempDir(), Mode: "separate", Channel: "stable", Experience: &profile})
	if token != "" || err == nil || !strings.Contains(err.Error(), "new empty target") {
		t.Fatalf("preferences can alter an existing target: %q, %v", token, err)
	}
}
