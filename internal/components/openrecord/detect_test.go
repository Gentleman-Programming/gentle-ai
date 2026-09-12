package openrecord

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFallbackPaths(t *testing.T) {
	home := "/home/user"

	t.Run("unix returns both destinations", func(t *testing.T) {
		got := FallbackPaths(home, "")
		want := []string{
			filepath.Join(home, ".local", "bin", "openrecord"),
			filepath.Join(home, "go", "bin", "openrecord"),
		}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("FallbackPaths() = %v, want %v", got, want)
		}
	})

	t.Run("windows discriminator adds .exe to the go-bin path only", func(t *testing.T) {
		got := FallbackPaths(home, `C:\Users\user\AppData\Local`)
		want := []string{
			filepath.Join(home, ".local", "bin", "openrecord"),
			filepath.Join(home, "go", "bin", "openrecord.exe"),
		}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("FallbackPaths() = %v, want %v", got, want)
		}
	})

	t.Run("empty home returns no paths", func(t *testing.T) {
		if got := FallbackPaths("", ""); got != nil {
			t.Fatalf("FallbackPaths(\"\", \"\") = %v, want nil", got)
		}
	})
}

func TestGoInstallDestinationDir(t *testing.T) {
	t.Run("GOBIN wins when set", func(t *testing.T) {
		previous := goEnvValue
		defer func() { goEnvValue = previous }()
		goEnvValue = func(key string) (string, error) {
			if key == "GOBIN" {
				return "/custom/gobin", nil
			}
			return "", nil
		}

		got, err := goInstallDestinationDir()
		if err != nil || got != "/custom/gobin" {
			t.Fatalf("goInstallDestinationDir() = (%q, %v), want /custom/gobin", got, err)
		}
	})

	t.Run("falls back to the first GOPATH entry plus bin", func(t *testing.T) {
		previous := goEnvValue
		defer func() { goEnvValue = previous }()
		goEnvValue = func(key string) (string, error) {
			if key == "GOPATH" {
				return "/custom/gopath", nil
			}
			return "", nil
		}

		got, err := goInstallDestinationDir()
		want := filepath.Join("/custom/gopath", "bin")
		if err != nil || got != want {
			t.Fatalf("goInstallDestinationDir() = (%q, %v), want %q", got, err, want)
		}
	})
}

// TestResolveBinaryPathConsultsGoInstallDestination pins that a custom
// GOBIN/GOPATH — matching neither hardcoded FallbackPaths entry — is still
// found, so detection never misses an install that a custom Go environment
// produced.
func TestResolveBinaryPathConsultsGoInstallDestination(t *testing.T) {
	customDir := t.TempDir()
	binaryPath := filepath.Join(customDir, BinaryName)
	if err := os.WriteFile(binaryPath, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}

	previous := goEnvValue
	defer func() { goEnvValue = previous }()
	goEnvValue = func(key string) (string, error) {
		if key == "GOBIN" {
			return customDir, nil
		}
		return "", nil
	}

	path, found := resolveBinaryPath(t.TempDir(), fakeDetector{})
	if !found || path != binaryPath {
		t.Fatalf("resolveBinaryPath() = (%q, %v), want (%q, true)", path, found, binaryPath)
	}
}
