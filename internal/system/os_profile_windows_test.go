//go:build windows

package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOSProfileDirMatchesUnmodifiedUserProfile checks the real known-folder
// lookup against the inherited USERPROFILE before any test redirects it.
func TestOSProfileDirMatchesUnmodifiedUserProfile(t *testing.T) {
	userProfile := os.Getenv("USERPROFILE")
	if userProfile == "" {
		t.Skip("USERPROFILE is not set")
	}
	got, err := osProfileDir()
	if err != nil {
		t.Fatalf("osProfileDir() error = %v", err)
	}
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(userProfile)) {
		t.Fatalf("osProfileDir() = %q, want USERPROFILE %q", got, userProfile)
	}
}
