//go:build linux || darwin

package shellinstaller

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCheckStableUserShellUpdate(t *testing.T) {
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"), Mode: "separate"})
	update, err := CheckUserShellUpdate(context.Background(), UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Channel != "stable" || !update.Available || len(update.Components) == 0 {
		t.Fatalf("stable update check differs: %+v", update)
	}
}
