//go:build windows

package shellinstaller

import (
	"os"
	"strings"
	"testing"
)

// stubUserWindowsKernelGate bypasses only the Windows 11 / unelevated host
// qualification (the CI runner is an elevated Windows Server); the manifest,
// identity and confirmation checks below stay real. The previous gate is
// restored after the test.
func stubUserWindowsKernelGate(t *testing.T) {
	t.Helper()
	previous := UserKernelGate
	t.Cleanup(func() { UserKernelGate = previous })
	UserKernelGate = func() error { return nil }
}

// A confirmed managed reconfigure retires the existing installation to its
// .replaced- backup before staging, then publishes into the now-absent
// destination. The confirmation must therefore describe the publication-time
// state, not the identity of the tree being replaced; otherwise the mandatory
// pre-publication re-inspection can never match the wizard-approved token and
// every full reconfigure refuses to publish. The completed-owned-installation
// manifest validation still gates the retire itself.
func TestUserWindowsReconfigureConfirmationSurvivesRetire(t *testing.T) {
	stubUserWindowsKernelGate(t)
	root := userWindowsLaunchFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	req := UserInstallRequest{Destination: root, Mode: "separate", Channel: "stable", Experience: &UserExperience{Persona: "gentleman", BackgroundSubagents: "on", SDKSource: "stable"}}
	token, err := InspectUserInstall(req)
	if err != nil || token == "" {
		t.Fatalf("reconfigure inspection refused a completed owned installation: token=%q err=%v", token, err)
	}
	// The retire step the reconfigure performs before staging preserves the
	// tree unchanged on the same volume.
	if err := os.Rename(root, root+".replaced-fixture"); err != nil {
		t.Fatal(err)
	}
	observed, err := InspectUserInstall(req)
	if err != nil {
		t.Fatalf("post-retire inspection failed: %v", err)
	}
	if observed != token {
		t.Fatalf("reconfigure confirmation changed across the retire step: before=%s after=%s", token, observed)
	}
}

// The publication-bound reconfigure token does not weaken the guards: an
// explicit experience over an existing destination still requires the managed
// reconfigure route, and a destination that is not a completed owned
// installation is still refused.
func TestUserWindowsReconfigureConfirmationKeepsGuards(t *testing.T) {
	stubUserWindowsKernelGate(t)
	root := userWindowsLaunchFixture(t)
	req := UserInstallRequest{Destination: root, Mode: "separate", Channel: "stable", Experience: &UserExperience{Persona: "gentleman", BackgroundSubagents: "on", SDKSource: "stable"}}
	if _, err := InspectUserInstall(req); err == nil || !strings.Contains(err.Error(), "explicit experience requires a new empty target") {
		t.Fatalf("explicit experience over an existing destination was accepted without the reconfigure route: err=%v", err)
	}
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	if err := os.WriteFile(root+`\manifest.json`, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if observed, err := InspectUserInstall(req); err == nil || observed != "" || !strings.Contains(err.Error(), "not a completed owned Windows installation") {
		t.Fatalf("corrupted manifest destination was accepted: token=%q err=%v", observed, err)
	}
}
