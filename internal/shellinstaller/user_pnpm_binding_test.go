package shellinstaller

import (
	"strings"
	"testing"
)

func TestUserPNPMBinding(t *testing.T) {
	for _, name := range []string{"pi", "gentle-shell"} {
		for _, root := range []string{"/private/root", "/private/it's a root", "/private/a shell launch b"} {
			if got := userManifestBinding(root, name, userManifest{}); got != userBinding(root, name) {
				t.Fatal("npm binding changed", got)
			}
			got := userManifestBinding(root, name, userManifest{PNPM: &userPNPMManifest{}})
			escaped := strings.ReplaceAll(root, "'", "'\\''")
			want := "exec '" + escaped + "/supervisor' shell launch-pnpm '" + escaped + "' '" + name + "' \"$@\"\n"
			if !strings.HasSuffix(got, want) {
				t.Fatal("role or argument forwarding lost", got)
			}
			if name == "gentle-shell" && !strings.Contains(got, "shell install \"$@\"; fi\n") {
				t.Fatal("install diversion changed", got)
			}
		}
	}
}
