//go:build linux && shell_experience_canonical

package shellinstaller

import (
	"context"
	"os/exec"
	"testing"
)

// Run explicitly in the asserted offline Guest with its authenticated private
// Node and read-only SDK source. The normal Go suite has no Node dependency.
func TestUserStageExperienceActualSDKConsumers(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			t.Run(persona+"-"+background, func(t *testing.T) {
				root := installStagePath(t)
				profile := UserExperience{Persona: persona, BackgroundSubagents: background}
				if err := userPrepareInstallStage(context.Background(), root, &profile); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("/opt/node/bin/node", "--experimental-strip-types", "/source/e2e/shell-experience-canonical.mjs", root, persona, background)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("actual SDK consumer failed: %v\n%s", err, output)
				} else {
					t.Logf("%s", output)
				}
			})
		}
	}
}
