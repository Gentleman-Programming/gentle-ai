package rtk

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestVerifyInstalled(t *testing.T) {
	// Save and restore original lookPath
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	t.Run("rtk found in PATH succeeds", func(t *testing.T) {
		lookPath = func(name string) (string, error) {
			if name == "rtk" {
				return "/usr/local/bin/rtk", nil
			}
			return "", fmt.Errorf("not found")
		}

		if err := VerifyInstalled(); err != nil {
			t.Errorf("VerifyInstalled() error = %v, want nil", err)
		}
	})

	t.Run("rtk not found returns error", func(t *testing.T) {
		lookPath = func(name string) (string, error) {
			return "", fmt.Errorf("not found")
		}

		err := VerifyInstalled()
		if err == nil {
			t.Error("VerifyInstalled() error = nil, want error")
		}
	})
}

func TestVerifyVersionSuccess(t *testing.T) {
	// This test only runs if rtk is actually installed in the environment.
	// It's a smoke test — skipped in CI if rtk is not available.
	if _, err := exec.LookPath("rtk"); err != nil {
		t.Skip("rtk not installed, skipping VerifyVersion integration test")
	}

	version, err := VerifyVersion()
	if err != nil {
		t.Errorf("VerifyVersion() error = %v", err)
	}
	if version == "" {
		t.Error("VerifyVersion() returned empty version")
	}
}
