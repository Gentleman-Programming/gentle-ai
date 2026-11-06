//go:build linux

package shellinstaller

import "testing"

// The native coordinator requires an empty capability bounding set and no
// supplementary groups. An ordinary user under a systemd user manager cannot
// meet either (the coordinator unit exits 218, then the namespace guard
// refuses), so a fresh Separate install keeps the legacy worker, which
// installs end to end.
func TestUserNativeInstallDispatchKeepsLegacyWorker(t *testing.T) {
	if userNativeInstallDispatch(true) {
		t.Fatal("a fresh Separate install was routed to the native coordinator")
	}
	if userNativeInstallDispatch(false) {
		t.Fatal("a non-fresh request was routed to the native coordinator")
	}
}

// enableNativeInstall turns the native coordinator on for tests of its own
// machinery and restores the shipped default afterwards. A re-executed test
// binary runs the same test function, so calling it first covers the child.
func enableNativeInstall(t *testing.T) {
	t.Helper()
	previous := userNativeInstallEnabled
	userNativeInstallEnabled = true
	t.Cleanup(func() { userNativeInstallEnabled = previous })
}
