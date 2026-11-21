//go:build linux && amd64

package shellinstaller

import (
	"strings"
	"testing"
)

// The Shell role must carry the Gentle Shell program identity; the pi role reaches the same
// launcher and must not claim it, or pi would publish a desktop feed as if it were Shell.
func TestUserPNPMLaunchEnvIdentity(t *testing.T) {
	for _, role := range []string{"pi", "gentle-shell"} {
		env := userPNPMLaunchEnv(role, []string{"HOME=/h"}, "/prefix", "/root")
		joined := strings.Join(env, "\n")
		if !strings.Contains(joined, "HOME=/h") || !strings.Contains(joined, "PNPM_HOME=/prefix") {
			t.Fatalf("%s: base environment lost: %q", role, env)
		}
		hasIdentity := strings.Contains(joined, "PI_PROGRAM_STATUS_APP=gentle-shell")
		if want := role == "gentle-shell"; hasIdentity != want {
			t.Fatalf("%s: identity present = %v, want %v", role, hasIdentity, want)
		}
	}
}
