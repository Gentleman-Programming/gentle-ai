package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// TestGGAAvailableDetectsViaLookPath verifies that ggaAvailable returns true
// when gga is found on PATH via cmdLookPath.
func TestGGAAvailableDetectsViaLookPath(t *testing.T) {
	origLookPath := cmdLookPath
	cmdLookPath = func(file string) (string, error) {
		if file == "gga" {
			return "/usr/local/bin/gga", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { cmdLookPath = origLookPath })

	if !ggaAvailable(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}) {
		t.Fatal("ggaAvailable() = false, want true when gga is on PATH")
	}
}

// TestGGAAvailableDetectsViaLocalBin verifies that ggaAvailable returns true
// when gga exists at ~/.local/bin/gga (default for install.sh on Linux/macOS).
func TestGGAAvailableDetectsViaLocalBin(t *testing.T) {
	tmpHome := t.TempDir()
	localBin := filepath.Join(tmpHome, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localBin, "gga"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	origLookPath := cmdLookPath
	origHomeDir := osUserHomeDir
	origStat := osStat
	cmdLookPath = func(file string) (string, error) { return "", os.ErrNotExist }
	osUserHomeDir = func() (string, error) { return tmpHome, nil }
	osStat = os.Stat
	t.Cleanup(func() {
		cmdLookPath = origLookPath
		osUserHomeDir = origHomeDir
		osStat = origStat
	})

	if !ggaAvailable(system.PlatformProfile{OS: "linux", PackageManager: "apt"}) {
		t.Fatal("ggaAvailable() = false, want true when gga is at ~/.local/bin/gga")
	}
}

// TestGGAAvailableDetectsViaHomebrewOptPrefix verifies that ggaAvailable returns
// true when gga exists at /opt/homebrew/bin/gga (Apple Silicon Homebrew default).
func TestGGAAvailableDetectsViaHomebrewOptPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	fakeOptHomebrew := filepath.Join(tmpDir, "opt", "homebrew", "bin", "gga")
	if err := os.MkdirAll(filepath.Dir(fakeOptHomebrew), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fakeOptHomebrew, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	origLookPath := cmdLookPath
	origHomeDir := osUserHomeDir
	origStat := osStat
	cmdLookPath = func(file string) (string, error) { return "", os.ErrNotExist }
	osUserHomeDir = func() (string, error) { return tmpDir, nil }
	// Override osStat to redirect well-known brew paths to our temp dir.
	osStat = func(name string) (os.FileInfo, error) {
		switch name {
		case "/opt/homebrew/bin/gga":
			return os.Stat(fakeOptHomebrew)
		case "/usr/local/bin/gga":
			return nil, os.ErrNotExist
		default:
			return os.Stat(name)
		}
	}
	t.Cleanup(func() {
		cmdLookPath = origLookPath
		osUserHomeDir = origHomeDir
		osStat = origStat
	})

	if !ggaAvailable(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}) {
		t.Fatal("ggaAvailable() = false, want true when gga is at /opt/homebrew/bin/gga")
	}
}

// TestGGAAvailableDetectsViaHomebrewUsrLocalPrefix verifies that ggaAvailable
// returns true when gga exists at /usr/local/bin/gga (Intel Mac Homebrew default).
func TestGGAAvailableDetectsViaHomebrewUsrLocalPrefix(t *testing.T) {
	origLookPath := cmdLookPath
	origHomeDir := osUserHomeDir
	origStat := osStat
	cmdLookPath = func(file string) (string, error) { return "", os.ErrNotExist }
	osUserHomeDir = func() (string, error) { return t.TempDir(), nil }
	osStat = func(name string) (os.FileInfo, error) {
		switch name {
		case "/opt/homebrew/bin/gga":
			return nil, os.ErrNotExist
		case "/usr/local/bin/gga":
			// Simulate gga present here.
			return os.Stat(os.DevNull)
		default:
			return nil, os.ErrNotExist
		}
	}
	t.Cleanup(func() {
		cmdLookPath = origLookPath
		osUserHomeDir = origHomeDir
		osStat = origStat
	})

	if !ggaAvailable(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}) {
		t.Fatal("ggaAvailable() = false, want true when gga is at /usr/local/bin/gga")
	}
}

// TestGGAAvailableReturnsFalseWhenNotFound verifies that ggaAvailable returns
// false when gga is not found via any detection path.
func TestGGAAvailableReturnsFalseWhenNotFound(t *testing.T) {
	origLookPath := cmdLookPath
	origHomeDir := osUserHomeDir
	origStat := osStat
	cmdLookPath = func(file string) (string, error) { return "", os.ErrNotExist }
	osUserHomeDir = func() (string, error) { return t.TempDir(), nil }
	osStat = func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	t.Cleanup(func() {
		cmdLookPath = origLookPath
		osUserHomeDir = origHomeDir
		osStat = origStat
	})

	if ggaAvailable(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}) {
		t.Fatal("ggaAvailable() = true, want false when gga is not installed anywhere")
	}
}

// TestGGAAvailableBrewPathsSkippedOnLinux verifies that the Homebrew-specific
// paths (/opt/homebrew/bin/gga, /usr/local/bin/gga) are NOT checked on Linux
// even if those paths happen to exist (they never exist there in practice, but
// the guard ensures no cross-platform false positives).
func TestGGAAvailableBrewPathsSkippedOnLinux(t *testing.T) {
	origLookPath := cmdLookPath
	origHomeDir := osUserHomeDir
	origStat := osStat
	cmdLookPath = func(file string) (string, error) { return "", os.ErrNotExist }
	osUserHomeDir = func() (string, error) { return t.TempDir(), nil }

	statCallCount := 0
	osStat = func(name string) (os.FileInfo, error) {
		if name == "/opt/homebrew/bin/gga" || name == "/usr/local/bin/gga" {
			statCallCount++
		}
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() {
		cmdLookPath = origLookPath
		osUserHomeDir = origHomeDir
		osStat = origStat
	})

	ggaAvailable(system.PlatformProfile{OS: "linux", PackageManager: "apt"})
	if statCallCount > 0 {
		t.Fatalf("ggaAvailable() checked Homebrew paths on Linux (%d calls), expected 0", statCallCount)
	}
}
