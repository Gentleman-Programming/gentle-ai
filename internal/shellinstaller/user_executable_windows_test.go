package shellinstaller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test binary runs from the runner's TEMP, which go test may address by
// its 8.3 short form. The installer image path must be the long form of the
// same file and pass the custody identity check that reads it.
func TestUserWindowsExecutableIsCanonicalLongPath(t *testing.T) {
	self, err := userWindowsExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := userWindowsIdentity(self, false); err != nil {
		t.Fatalf("installer image path %q fails the identity check: %v", self, err)
	}
	launched, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	before, beforeErr := os.Stat(launched)
	after, afterErr := os.Stat(self)
	if beforeErr != nil || afterErr != nil || !os.SameFile(before, after) {
		t.Fatalf("expanded path %q is not the launched image %q: %v %v", self, launched, beforeErr, afterErr)
	}
	if !strings.EqualFold(filepath.VolumeName(self), filepath.VolumeName(launched)) {
		t.Fatalf("expansion changed the drive: %q -> %q", launched, self)
	}
}
