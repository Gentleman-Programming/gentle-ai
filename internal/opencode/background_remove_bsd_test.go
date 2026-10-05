//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package opencode

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestManagedLauncherOpenRefusalErrorsIncludesEFTYPE(t *testing.T) {
	if !managedLauncherOpenRefusal(unix.EFTYPE) {
		t.Fatal("managedLauncherOpenRefusal(EFTYPE) = false, want true")
	}
}
