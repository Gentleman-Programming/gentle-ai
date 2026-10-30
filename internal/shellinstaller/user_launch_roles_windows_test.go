//go:build windows

package shellinstaller

import (
	"path/filepath"
	"testing"
)

func TestUserWindowsLaunchRoles(t *testing.T) {
	root := `C:\owned home`
	for _, tt := range []struct {
		product string
		member  string
	}{
		{"pi", "prefix/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js"},
		{"gentle-shell", "prefix/node_modules/gentle-pi/bin/gentle-shell.mjs"},
	} {
		t.Run(tt.product, func(t *testing.T) {
			want := filepath.Join(root, filepath.FromSlash(tt.member))
			if got := userWindowsLaunchCLI(root, tt.product); got != want {
				t.Fatalf("%s CLI = %q; want %q", tt.product, got, want)
			}
		})
	}
}
