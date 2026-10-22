package shellinstaller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUserExperienceSelectionPreservesNilBytesAndBindsValues(t *testing.T) {
	req, err := UserInstallFromEntry([]string{"target", "separate", "prefix", "agent", "token"})
	if err != nil || req.Experience != nil {
		t.Fatalf("legacy entry changed: %+v, %v", req, err)
	}
	fields := []string{"target", "separate", "prefix", "agent", "identity"}
	wantJSON := `{"Destination":"target","Mode":"separate","SharedPrefix":"prefix","SharedAgent":"agent","Confirmation":"token"}`
	if runtime.GOOS == "windows" {
		fields = []string{"target", "separate", "prefix", "agent", "stable", "identity"}
		wantJSON = strings.TrimSuffix(wantJSON, "}") + `,"Channel":"stable"}`
	}
	legacy, _ := json.Marshal(fields)
	wantHash := fmt.Sprintf("%x", sha256.Sum256(legacy))
	gotJSON, err := json.Marshal(req)
	if err != nil || string(gotJSON) != wantJSON || userConfirmation(req, "identity") != wantHash {
		t.Fatalf("nil profile changed retained bytes/hash: %s, %v", gotJSON, err)
	}
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	req.Experience = &profile
	encoded, _ := profile.Encode()
	bound, _ := json.Marshal(append(fields, encoded))
	if got := userConfirmation(req, "identity"); got == wantHash || got != fmt.Sprintf("%x", sha256.Sum256(bound)) {
		t.Fatal("experience was not appended to the physical selection hash")
	}
	profile.Persona = "neutral"
	if userConfirmation(req, "identity") == fmt.Sprintf("%x", sha256.Sum256(bound)) {
		t.Fatal("persona change keeps the old selection token")
	}
	profile.Persona = "custom"
	if userConfirmation(req, "identity") != "" {
		t.Fatal("invalid profile mints a confirmation")
	}
}

func TestUserExperienceSelectionRefusesUnsupportedProfileBeforeEffects(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	profile.GGA, profile.GGAProvider = "on", "codex"
	req := UserInstallRequest{Destination: target, Mode: "separate", Experience: &profile}
	if token, err := InspectUserInstall(req); err != nil || token == "" {
		t.Fatalf("qualified GGA profile was refused: %q, %v", token, err)
	}
	profile.GGAProvider = ""
	if token, err := InspectUserInstall(req); err != nil || token == "" {
		t.Fatalf("unprovided GGA profile was refused: %q, %v", token, err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("profile refusal changed destination: %v", err)
	}
}
