//go:build windows

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #5959: an installation that recorded a reused system Go has an empty
// runtime/go and only runtime/system-go.json. Reusing that tree drops the
// marker and leaves neither go.exe nor go.zip, so the recorded pinned Go is
// reusable only when the installation actually unpacked it.
func TestUserWindowsGoReusable(t *testing.T) {
	pin, err := userWindowsGoPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	nodeReused, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSupervisor})
	if err != nil {
		t.Fatal(err)
	}
	nodeUpdated, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentNode})
	if err != nil {
		t.Fatal(err)
	}
	pinned := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pinned, "runtime", "go", "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	system := t.TempDir()
	if err := os.MkdirAll(filepath.Join(system, "runtime", "go"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, "runtime", "system-go.json"), []byte(`{"Go":"C:\\Program Files\\Go\\bin\\go.exe","Version":"go1.26.5"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		destination string
		manifest    userWindowsManifest
		scope       userInstallScope
		want        bool
	}{
		{"pinned Go with reused Node", pinned, userWindowsManifest{GoSHA: pin}, nodeReused, true},
		{"system Go marker with reused Node", system, userWindowsManifest{GoSHA: pin}, nodeReused, false},
		{"updated Node", pinned, userWindowsManifest{GoSHA: pin}, nodeUpdated, false},
		{"different Go pin", pinned, userWindowsManifest{GoSHA: "other"}, nodeReused, false},
		{"system Go marker with updated Node", system, userWindowsManifest{GoSHA: pin}, nodeUpdated, false},
	} {
		if got := userWindowsGoReusable(tc.destination, &tc.manifest, tc.scope); got != tc.want {
			t.Errorf("%s: userWindowsGoReusable = %v, want %v", tc.name, got, tc.want)
		}
	}
	if userWindowsGoReusable(pinned, nil, nodeReused) {
		t.Error("an unrecorded installation must not reuse Go")
	}
}
