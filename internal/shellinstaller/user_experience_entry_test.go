package shellinstaller

import (
	"os"
	"path/filepath"
	"runtime"
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
	// GGA is retired from the shell installer: the retired selection decodes
	// and travels verbatim through the entry. The refusal moved out of the
	// transport: on the qualified Linux runtime inspection admits the retired
	// selection, and the install flow forces it inert before inspection
	// rebinds the confirmation preimage. Off the qualified runtime the
	// unsupported-public-effect refusal remains.
	switch runtime.GOOS {
	case "linux":
		// Deterministic fixture: an owned 0700 canonical root lets inspection
		// reach its real contract instead of failing on fixture filesystem
		// metadata (Go's t.TempDir can be 0755, which privateDirectory refuses).
		root := t.TempDir()
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatal(err)
		}
		args[0] = filepath.Join(canonical, "target")
		retargeted, err := UserInstallFromEntry(args)
		if err != nil {
			t.Fatal(err)
		}
		if token, err := InspectUserInstall(retargeted); err != nil || token == "" {
			t.Fatalf("retired GGA selection was not admitted inertly: %q, %v", token, err)
		}
	default:
		if runtime.GOOS == "windows" {
			break // Windows inspection depends on kernel/channel machinery; decode refusals below still apply.
		}
		if token, err := InspectUserInstall(req); err == nil || token != "" {
			t.Fatal("transport admitted unsupported public effects off the qualified runtime")
		}
	}
	for _, raw := range []string{"null", strings.Repeat(" ", 257) + encoded} {
		args[len(args)-1] = raw
		if _, err := UserInstallFromEntry(args); err == nil {
			t.Fatal("malformed/oversized raw profile admitted")
		}
	}
}
