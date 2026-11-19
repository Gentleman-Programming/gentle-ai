//go:build darwin

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func darwinPreflightEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// darwinPreflightTrustedSelf skips a test whose test binary lives where the
// unchanged supervisor policy refuses it; the volume probe runs only after it.
func darwinPreflightTrustedSelf(t *testing.T) {
	t.Helper()
	self, err := userExecutable()
	if err == nil {
		_, err = userSupervisorSHA(context.Background(), self)
	}
	if err != nil {
		t.Skipf("test binary is not a trusted supervisor here: %v", err)
	}
}

func TestDarwinVolumePreflightAcceptsCapableVolumeAndLeavesNothing(t *testing.T) {
	parent := t.TempDir()
	if err := userVolumePreflight(parent); err != nil {
		t.Fatalf("APFS temp volume refused: %v", err)
	}
	if names := darwinPreflightEntries(t, parent); len(names) != 0 {
		t.Fatalf("probe left entries %v", names)
	}
}

func TestDarwinVolumePreflightRefusesMissingFullSyncBeforeDownloads(t *testing.T) {
	parent := t.TempDir()
	saved := userFullSync
	t.Cleanup(func() { userFullSync = saved })
	userFullSync = func(uintptr) error { return unix.ENOTSUP }
	err := userVolumePreflight(parent)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "refused" || !errors.Is(err, unix.ENOTSUP) || !strings.Contains(err.Error(), "F_FULLFSYNC") || !strings.Contains(err.Error(), parent) {
		t.Fatalf("unsupported F_FULLFSYNC = %v, want a refused volume naming F_FULLFSYNC and %q", err, parent)
	}
	if names := darwinPreflightEntries(t, parent); len(names) != 0 {
		t.Fatalf("refused probe left entries %v", names)
	}
}

func TestDarwinPreflightSkipsVerificationOfExistingInstallation(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "shell")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	darwinPreflightTrustedSelf(t)
	saved := userFullSync
	t.Cleanup(func() { userFullSync = saved })
	userFullSync = func(uintptr) error { return unix.ENOTSUP }
	if err := userDarwinPreflight(UserInstallRequest{Destination: destination}); err != nil {
		t.Fatalf("verifying an existing installation stages nothing, so preflight must accept: %v", err)
	}
	fresh := filepath.Join(filepath.Dir(destination), "fresh")
	if err := userDarwinPreflight(UserInstallRequest{Destination: fresh}); err == nil || !strings.Contains(err.Error(), "F_FULLFSYNC") {
		t.Fatalf("a fresh install must run the volume preflight: %v", err)
	}
}

func TestDarwinInspectionInstallsPlatformPreflight(t *testing.T) {
	darwinPreflightTrustedSelf(t)
	saved := userFullSync
	t.Cleanup(func() { userFullSync = saved })
	userFullSync = func(uintptr) error { return unix.ENOTSUP }
	err := userPlatformPreflight(UserInstallRequest{Destination: filepath.Join(t.TempDir(), "shell")})
	if err == nil || !strings.Contains(err.Error(), "F_FULLFSYNC") {
		t.Fatalf("inspection preflight is not the darwin preflight: %v", err)
	}
}
