//go:build linux && amd64

package shellinstaller

import "testing"

func TestUserPNPMAcquisitionCacheSelection(t *testing.T) {
	for _, tt := range []struct {
		path    string
		omitted bool
	}{
		{"pnpm/store/v11/files", true}, {"pnpm/store/v11/files/ab/content", true},
		{"pnpm/store/v11/files-backup/file", false}, {"pnpm/store/v11/links/package/index.js", false},
		{"pnpm/store/v11/index.db", false}, {"pnpm/store/v11/projects/package", false},
		{"home/.npm", true}, {"home/.npm/_npx/package.json", true},
		{"home/.npmrc", false}, {"home/.npm-backup/file", false},
		{"home/.gentle-shell/agent/npm/package.json", false},
		{"pnpm/.npm/file", false}, {"home/nested/.npm/file", false},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if got := userPNPMAcquisitionCache(tt.path); got != tt.omitted {
				t.Fatalf("cache selection %q = %v, want %v", tt.path, got, tt.omitted)
			}
		})
	}
}
