//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Preparation runs separately so the real launcher probe can hide original DATA.
func TestUserPNPMLaunchPrepare(t *testing.T) {
	if testing.Short() {
		t.Skip("requires qualified original DATA and ordinary supervisor build")
	}
	ctx := context.Background()
	data, err := os.ReadFile("/out/verified-location.json")
	if err != nil {
		t.Fatal(err)
	}
	var location NativeInstallationLocation
	if err := json.Unmarshal(data, &location); err != nil {
		t.Fatal(err)
	}
	stage, root := "/fixture/stage", "/fixture/final"
	staged, err := StageNativePNPMInstallation(ctx, "/out", stage, root, location)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := userCopySupervisor(ctx, "/qualified-supervisor", stage+"/supervisor")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stage+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	m := userManifest{Schema: userSchema, Destination: root, Prefix: root + "/pnpm", Agent: root + "/agent", Mode: "separate", SupervisorSHA: sha}
	m.PNPM = &userPNPMManifest{PackageRoot: strings.TrimSuffix(staged.Installation.ShellCLI, "/bin/gentle-shell.mjs"), ShellVersion: location.ShellVersion, PeerVersion: location.PiVersion, CodeSeal: staged.CodeSeal}
	m.PrefixIdentity, err = userIdentity(stage + "/pnpm")
	if err != nil {
		t.Fatal(err)
	}
	m.AgentIdentity, err = userIdentity(stage + "/agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := userPublishManifest(ctx, stage, stage+"/pnpm", stage+"/agent", m); err != nil {
		t.Fatal(err)
	}
}

func TestUserPNPMLaunch(t *testing.T) {
	if testing.Short() {
		t.Skip("requires separately published fixture with original DATA hidden")
	}
	if _, err := os.Stat("/out"); !os.IsNotExist(err) {
		t.Fatal("original DATA must be inaccessible", err)
	}
	ctx, root := context.Background(), "/fixture/final"
	before, err := PNPMCodeSeal(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	marker := filepath.Join(project, "preserve.txt")
	if err := os.WriteFile(marker, []byte("caller project"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"pi", "gentle-shell"} {
		for _, arg := range []string{"--version", "--help"} {
			cmd := exec.CommandContext(ctx, root+"/bin/"+role, arg)
			cmd.Dir = project
			cmd.Env = append(os.Environ(), "NODE_OPTIONS=--require=/must-not-inherit.js", "PI_CODING_AGENT_DIR=/must-not-use")
			output, err := cmd.CombinedOutput()
			if err != nil || len(output) == 0 {
				t.Fatalf("%s %s: %v %s", role, arg, err, output)
			}
			version := "1.3.0"
			if role == "gentle-shell" {
				version = "4.3.0"
			}
			if arg == "--version" && !strings.Contains(string(output), version) {
				t.Fatalf("wrong role version: %s", output)
			}
			t.Logf("%s %s: exit0 (%d bytes)", role, arg, len(output))
		}
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "caller project" {
		t.Fatal("caller project changed", err)
	}
	if after, err := PNPMCodeSeal(ctx, root); err != nil || after != before {
		t.Fatal("launch changed code", err)
	}
	if err := userLaunchPNPM(ctx, root, "not-a-role", nil, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("unknown launcher role accepted")
	}
	if err := os.WriteFile(root+"/pnpm/unsealed.js", []byte("unsealed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userLaunchPNPM(ctx, root, "pi", []string{"--version"}, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("unsealed code launched")
	}
}
