//go:build linux || darwin

package shellinstaller

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserPNPMBindingPathText(t *testing.T) {
	root := filepath.Join(t.TempDir(), "it's a shell launch path")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/supervisor", []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"pi", "gentle-shell"} {
		script := filepath.Join(root, role)
		binding := userManifestBinding(root, role, userManifest{PNPM: &userPNPMManifest{}})
		if err := os.WriteFile(script, []byte(binding), 0700); err != nil {
			t.Fatal(err)
		}
		output, err := exec.Command(script, "--arg", "two words").CombinedOutput()
		want := strings.Join([]string{"shell", "launch-pnpm", root, role, "--arg", "two words", ""}, "\n")
		if err != nil || string(output) != want {
			t.Fatalf("%s path/argv: %v %q", role, err, output)
		}
		if role == "gentle-shell" {
			output, err = exec.Command(script, "install", "--inspect").CombinedOutput()
			if err != nil || string(output) != "shell\ninstall\n--inspect\n" {
				t.Fatal("install diversion", err, string(output))
			}
		}
	}
}
