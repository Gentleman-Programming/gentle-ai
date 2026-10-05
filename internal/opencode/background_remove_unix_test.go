//go:build unix

package opencode

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestManagedLauncherOpenRefusalErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "too many symlink traversals", err: unix.ELOOP},
		{name: "BSD no-follow link count", err: unix.EMLINK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !managedLauncherOpenRefusal(tt.err) {
				t.Fatalf("managedLauncherOpenRefusal(%v) = false, want true", tt.err)
			}
		})
	}
	if managedLauncherOpenRefusal(errors.New("unrelated open failure")) {
		t.Fatal("managedLauncherOpenRefusal accepted an unrelated error")
	}
}
