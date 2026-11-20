//go:build darwin

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

const darwinPreflightChildEnv = "GENTLE_AI_TEST_PREFLIGHT_CHILD"

// darwinPreflightUntrustedRun re-executes this test binary from a
// world-writable directory, where the supervisor policy refuses it, and runs
// the named child test there. userExecutable is a plain function, so placing
// the binary is the only way to reach the refusal without a new seam.
func darwinPreflightUntrustedRun(t *testing.T, mode string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dir, "shellinstaller.test")
	if err := os.WriteFile(copyPath, data, 0700); err != nil {
		t.Fatal(err)
	}
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(copyPath, "-test.run=^TestDarwinPreflightUntrustedChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), darwinPreflightChildEnv+"="+mode, darwinPreflightChildEnv+"_WORK="+work)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child %s failed: %v\n%s", mode, err, out)
	}
}

func TestDarwinPreflightRefusesFreshInstallFromUntrustedSupervisor(t *testing.T) {
	darwinPreflightUntrustedRun(t, "fresh")
}

func TestDarwinPreflightDoesNotCheckSupervisorWhenReconfiguringExisting(t *testing.T) {
	darwinPreflightUntrustedRun(t, "reconfigure")
}

func TestDarwinPreflightReconfigureStillRunsVolumeProbe(t *testing.T) {
	darwinPreflightUntrustedRun(t, "reconfigure-volume")
}

func TestDarwinPreflightUntrustedChild(t *testing.T) {
	mode := os.Getenv(darwinPreflightChildEnv)
	if mode == "" {
		t.Skip("runs only as a child of darwinPreflightUntrustedRun")
	}
	work := os.Getenv(darwinPreflightChildEnv + "_WORK")
	if self, err := userExecutable(); err != nil {
		t.Fatal(err)
	} else if _, err := userSupervisorSHA(context.Background(), self); err == nil {
		t.Fatal("child binary must be an untrusted supervisor")
	}
	existing := filepath.Join(work, "shell")
	if err := os.Mkdir(existing, 0700); err != nil {
		t.Fatal(err)
	}
	switch mode {
	case "fresh":
		err := userDarwinPreflight(UserInstallRequest{Destination: filepath.Join(work, "fresh")})
		if err == nil || !strings.Contains(err.Error(), "supervisor provenance") {
			t.Fatalf("fresh install from an untrusted supervisor = %v, want supervisor provenance refusal", err)
		}
	case "reconfigure":
		t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", existing)
		if err := userDarwinPreflight(UserInstallRequest{Destination: existing}); err != nil {
			t.Fatalf("reconfiguring an existing install must not check supervisor provenance: %v", err)
		}
		if names := darwinPreflightEntries(t, work); len(names) != 1 || names[0] != "shell" {
			t.Fatalf("preflight left entries %v", names)
		}
	case "reconfigure-volume":
		t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", existing)
		saved := userFullSync
		t.Cleanup(func() { userFullSync = saved })
		userFullSync = func(uintptr) error { return unix.ENOTSUP }
		if err := userDarwinPreflight(UserInstallRequest{Destination: existing}); err == nil || !strings.Contains(err.Error(), "F_FULLFSYNC") {
			t.Fatalf("reconfigure must still run the volume probe: %v", err)
		}
	default:
		t.Fatalf("unknown mode %q", mode)
	}
}

func TestDarwinVolumePreflightPreservesForeignProbeNamedFiles(t *testing.T) {
	parent := t.TempDir()
	foreign := []string{".gentle-volume-probe.foreign", ".gentle-volume-probe.foreign.excl"}
	for _, name := range foreign {
		if err := os.WriteFile(filepath.Join(parent, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 50; i++ {
		if err := userVolumePreflight(parent); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	names := darwinPreflightEntries(t, parent)
	if len(names) != len(foreign) {
		t.Fatalf("entries after 50 probes = %v, want only %v", names, foreign)
	}
	for _, name := range foreign {
		if got, err := os.ReadFile(filepath.Join(parent, name)); err != nil || string(got) != "keep" {
			t.Fatalf("foreign %s = %q, %v; want preserved", name, got, err)
		}
	}
}

func TestDarwinVolumePreflightPanicLeavesNoTempFile(t *testing.T) {
	parent := t.TempDir()
	saved := userFullSync
	t.Cleanup(func() { userFullSync = saved })
	userFullSync = func(uintptr) error { panic("probe panic") }
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected the probe panic to propagate")
			}
		}()
		_ = userVolumePreflight(parent)
	}()
	if names := darwinPreflightEntries(t, parent); len(names) != 0 {
		t.Fatalf("panicking probe left entries %v", names)
	}
}
