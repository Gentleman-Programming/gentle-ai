//go:build linux && shell_experience_canonical

package shellinstaller

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Explicit offline package-consumer boundary: checksum-matched published bytes
// are extracted privately, without install scripts, network or a rebuilt SDK.
// This is not a full package lifecycle or installed-platform acceptance test.
func TestUserStageExperienceFrozenPackageConsumers(t *testing.T) {
	lockData, err := os.ReadFile("/source/scripts/user-locks/modern/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Packages map[string]struct{ Version, Resolved, Integrity string }
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	pin := lock.Packages["node_modules/gentle-pi"]
	if pin.Version == "" || pin.Resolved != "https://registry.npmjs.org/gentle-pi/-/gentle-pi-"+pin.Version+".tgz" {
		t.Fatal("fixture must track the exact selected frozen package")
	}
	data, err := os.ReadFile("/frozen-gentle-pi.tgz")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum512(data)
	if "sha512-"+base64.StdEncoding.EncodeToString(digest[:]) != pin.Integrity {
		t.Fatal("package bytes differ from the frozen lock; do not extract or execute")
	}
	pkg := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	cmd := context7ProofCommand(ctx, "/usr/bin/tar", "-xzf", "/frozen-gentle-pi.tgz", "--strip-components=1", "-C", pkg)
	if output, err := context7ProofOutput(cmd); err != nil {
		t.Fatalf("authenticated package extraction failed: %v\n%s", err, output)
	}
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			t.Run(persona+"-"+background, func(t *testing.T) {
				root := installStagePath(t)
				profile := UserExperience{Persona: persona, BackgroundSubagents: background}
				if err := userPrepareInstallStage(ctx, root, "separate", &profile); err != nil {
					t.Fatal(err)
				}
				cmd := context7ProofCommand(ctx, "/opt/node/bin/node", "--experimental-strip-types", "/source/e2e/shell-experience-canonical.mjs", root, persona, background, pkg)
				if output, err := context7ProofOutput(cmd); err != nil {
					t.Fatalf("frozen package consumer failed: %v\n%s", err, output)
				} else {
					t.Logf("%s", output)
				}
			})
		}
	}
}
