//go:build !windows

package reviewtransaction

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepairRefusalReportsIneffectivePrivateModeFilesystem pins #5112: when
// the no-follow chmod repair runs and the mode STILL reports group or other
// access, and the filesystem probe confirms the mount cannot represent private
// modes at all, the refusal becomes the typed capability diagnostic instead of
// a chmod suggestion the mount can never satisfy.
func TestRepairRefusalReportsIneffectivePrivateModeFilesystem(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, rarAuthorityDirectory)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	breakRARPrivateDirectoryChmod(t)
	previousProbe := rarPOSIXPrivateModeIneffective
	rarPOSIXPrivateModeIneffective = func(string) bool { return true }
	t.Cleanup(func() { rarPOSIXPrivateModeIneffective = previousProbe })

	err := repairAndValidatePrivateRARDirectory(path)
	var ineffective *PrivateModeIneffectiveError
	if !errors.As(err, &ineffective) {
		t.Fatalf("repair refusal on an ineffective private-mode filesystem = %v, want *PrivateModeIneffectiveError", err)
	}
	if ineffective.Path != path || !ineffective.Directory {
		t.Fatalf("refusal names %q (directory=%t), want %q (directory=true)", ineffective.Path, ineffective.Directory, path)
	}
	// The capability refusal is a refinement of the generic refusal, not a
	// new trust decision: fail-closed callers matching the sentinel keep
	// refusing exactly as before.
	if !errors.Is(err, errUnsafeRARAuthorityPath) {
		t.Fatalf("capability refusal must still match the generic unsafe-path sentinel, got %v", err)
	}
	message := ineffective.Error()
	if strings.Contains(message, "chmod 700") || strings.Contains(message, "chmod 600") {
		t.Fatalf("capability refusal suggests a chmod that cannot take effect: %s", message)
	}
	for _, want := range []string{"metadata", "remount"} {
		if !strings.Contains(message, want) {
			t.Fatalf("capability refusal %q does not name the real continuation (%q)", message, want)
		}
	}
}

// TestRepairRefusalStaysGenericOnACapableFilesystem pins the other half of the
// detection: a filesystem that CAN represent private modes keeps the existing
// refusal, whose printed chmod repair is exactly what makes it runnable.
func TestRepairRefusalStaysGenericOnACapableFilesystem(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, rarAuthorityDirectory)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	breakRARPrivateDirectoryChmod(t)
	previousProbe := rarPOSIXPrivateModeIneffective
	rarPOSIXPrivateModeIneffective = func(string) bool { return false }
	t.Cleanup(func() { rarPOSIXPrivateModeIneffective = previousProbe })

	err := repairAndValidatePrivateRARDirectory(path)
	var ineffective *PrivateModeIneffectiveError
	if errors.As(err, &ineffective) {
		t.Fatalf("capable filesystem produced a capability refusal: %v", err)
	}
	var unsafe *UnsafeRARPathError
	if !errors.As(err, &unsafe) {
		t.Fatalf("refusal on a capable filesystem = %v, want the generic *UnsafeRARPathError", err)
	}
}

// TestRepairStillFixesWeakenedDirectoryOnNativeFilesystem guards the native
// lane end to end: on a filesystem with effective POSIX metadata the existing
// no-follow repair still turns a weakened empty directory owner-only, and no
// capability diagnostic interferes.
func TestRepairStillFixesWeakenedDirectoryOnNativeFilesystem(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, rarAuthorityDirectory)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repairAndValidatePrivateRARDirectory(path); err != nil {
		t.Fatalf("native repair of a weakened empty directory = %v, want nil", err)
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("repaired directory = %v (%v), want mode 0700", info, statErr)
	}
}

// TestRefineProbesTheRefusedDirectoryNotItsParent pins the QA fix: the
// capability question is about the refused directory's own filesystem;
// probing the parent would misanswer a bind-mounted or overlayed RAR
// directory.
func TestRefineProbesTheRefusedDirectoryNotItsParent(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, rarAuthorityDirectory)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	breakRARPrivateDirectoryChmod(t)
	previousProbe := rarPOSIXPrivateModeIneffective
	probed := ""
	rarPOSIXPrivateModeIneffective = func(dir string) bool { probed = dir; return true }
	t.Cleanup(func() { rarPOSIXPrivateModeIneffective = previousProbe })

	if err := repairAndValidatePrivateRARDirectory(path); err == nil {
		t.Fatal("expected a refusal")
	}
	if probed != path {
		t.Fatalf("probe measured %q, want the refused directory %q itself", probed, path)
	}
}

// TestNativeProbeAnswersCapableAndLeavesNoTrace exercises the real probe on a
// filesystem with effective POSIX metadata: it must answer "capable" and, by
// unlinking its scratch file before measuring, leave the probed directory
// exactly as it found it -- even a crash cannot make the authority directory
// non-empty. An unprobeable directory must also answer "capable": an
// inconclusive probe never invents a refusal.
func TestNativeProbeAnswersCapableAndLeavesNoTrace(t *testing.T) {
	root := resolvedTempDir(t)
	if posixPrivateModeIneffective(root) {
		t.Fatal("native filesystem reported as private-mode-ineffective")
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("probe left traces in the probed directory: %v (%v)", entries, readErr)
	}
	if posixPrivateModeIneffective(filepath.Join(root, "does-not-exist")) {
		t.Fatal("unprobeable directory reported as private-mode-ineffective")
	}
}

// TestRefineFiresForNonEmptyDirectoriesOnIneffectiveMounts pins the refine's
// scope: a directory that already holds state is never repaired, but its
// refusal on an incapable mount is still the capability diagnostic -- the
// generic chmod suggestion would be exactly as unsatisfiable here.
func TestRefineFiresForNonEmptyDirectoriesOnIneffectiveMounts(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, rarAuthorityDirectory)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	breakRARPrivateDirectoryChmod(t)
	previousProbe := rarPOSIXPrivateModeIneffective
	rarPOSIXPrivateModeIneffective = func(string) bool { return true }
	t.Cleanup(func() { rarPOSIXPrivateModeIneffective = previousProbe })

	err := repairAndValidatePrivateRARDirectory(path)
	var ineffective *PrivateModeIneffectiveError
	if !errors.As(err, &ineffective) {
		t.Fatalf("weakened non-empty directory on an ineffective mount = %v, want the capability refusal", err)
	}
}

// TestSymlinkRefusalStaysUntyped pins that the capability refinement never
// hijacks a different refusal class: a substituted symlink is the operator's
// to remove, and the probe has nothing to say about it.
func TestSymlinkRefusalStaysUntyped(t *testing.T) {
	root := resolvedTempDir(t)
	victim := filepath.Join(root, "victim")
	if err := os.Mkdir(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, rarAuthorityDirectory)
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	previousProbe := rarPOSIXPrivateModeIneffective
	rarPOSIXPrivateModeIneffective = func(string) bool { return true }
	t.Cleanup(func() { rarPOSIXPrivateModeIneffective = previousProbe })

	err := repairAndValidatePrivateRARDirectory(path)
	var ineffective *PrivateModeIneffectiveError
	if errors.As(err, &ineffective) {
		t.Fatalf("symlink refusal hijacked into a capability refusal: %v", err)
	}
	var unsafe *UnsafeRARPathError
	if !errors.As(err, &unsafe) {
		t.Fatalf("symlink refusal = %v, want the generic *UnsafeRARPathError", err)
	}
}
