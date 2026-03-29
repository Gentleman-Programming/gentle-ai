package rtk

import (
	"fmt"
	"os/exec"
	"strings"
)

// Package-level vars for testability.
var (
	lookPath = exec.LookPath
)

// VerifyInstalled checks if the rtk binary is available in PATH.
func VerifyInstalled() error {
	if _, err := lookPath("rtk"); err != nil {
		return fmt.Errorf("rtk binary not found in PATH: %w", err)
	}
	return nil
}

// VerifyVersion runs "rtk --version" and returns the trimmed output.
func VerifyVersion() (string, error) {
	cmd := execCommand("rtk", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("rtk --version failed: %w", err)
	}

	version := strings.TrimSpace(string(out))
	if version == "" {
		return "", fmt.Errorf("rtk --version returned empty output")
	}

	return version, nil
}

// VerifyHookStatus runs "rtk init --show" to report hook installation status.
func VerifyHookStatus() (string, error) {
	cmd := execCommand("rtk", "init", "--show")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("rtk init --show failed: %w\nOutput: %s", err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}
