//go:build linux

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUserHostPlatformRefusesUnsupportedArchitectureByName(t *testing.T) {
	if err := userHostPlatform("amd64"); err != nil {
		t.Fatalf("Linux amd64 refused: %v", err)
	}
	err := userHostPlatform("arm64")
	var private *PrivateRuntimeError
	if err == nil || !errors.As(err, &private) || private.Kind != "unavailable" {
		t.Fatalf("Linux arm64 admitted or untyped: %v", err)
	}
	if want := "unsupported host: Linux arm64; supported: Linux amd64 with a systemd user manager >=254"; !strings.Contains(err.Error(), want) {
		t.Fatalf("refusal does not name the condition: %v", err)
	}
	if strings.Contains(err.Error(), "glibc") || strings.Contains(err.Error(), "musl") {
		t.Fatalf("refusal names a libc heuristic that no longer exists: %v", err)
	}
}

func TestUserExecutableMountRefusesNoexec(t *testing.T) {
	parent := t.TempDir()
	noexec := func(path string, st *unix.Statfs_t) error {
		if path != parent {
			t.Fatalf("statfs probed %q, want the target parent %q", path, parent)
		}
		st.Flags = unix.ST_NOEXEC | unix.ST_NOSUID
		return nil
	}
	err := userExecutableMount(parent, noexec)
	var private *PrivateRuntimeError
	if err == nil || !errors.As(err, &private) || private.Kind != "refused" {
		t.Fatalf("noexec parent admitted or untyped: %v", err)
	}
	for _, want := range []string{parent, "noexec mount", "remount"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("noexec refusal lacks %q: %v", want, err)
		}
	}
	if err := userExecutableMount(parent, func(string, *unix.Statfs_t) error { return unix.EACCES }); err == nil || !strings.Contains(err.Error(), parent) {
		t.Fatalf("statfs failure admitted or unnamed: %v", err)
	}
	if err := userExecutableMount(parent, func(_ string, st *unix.Statfs_t) error { st.Flags = unix.ST_NOSUID; return nil }); err != nil {
		t.Fatalf("exec-capable mount refused: %v", err)
	}
	if err := userExecutableMount(parent, unix.Statfs); err != nil {
		t.Fatalf("real test temp mount refused: %v", err)
	}
}

// The preflight runs inside ValidateUserInstall, which the TUI and the
// install flow call through InspectUserInstall before any download; an
// unchanged valid selection still validates (existing behavior).
func TestValidateUserInstallKeepsAcceptingSupportedHost(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateUserInstall(UserInstallRequest{Mode: "separate", Destination: filepath.Join(canonical, "shell")}); err != nil {
		t.Fatalf("supported host and exec-capable parent refused: %v", err)
	}
}
