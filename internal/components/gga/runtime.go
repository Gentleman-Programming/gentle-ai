package gga

import (
	"fmt"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// RuntimeLibDir returns the runtime lib path used by gga.
func RuntimeLibDir(homeDir string) string {
	return filepath.Join(homeDir, ".local", "share", "gga", "lib")
}

// RuntimePRModePath returns the expected pr_mode.sh runtime path.
func RuntimePRModePath(homeDir string) string {
	return filepath.Join(RuntimeLibDir(homeDir), "pr_mode.sh")
}

// EnsureRuntimeAssets ensures critical gga runtime files are current.
//
// Behavior change from "only-if-missing" to "always-write":
// WriteFileAtomic performs a content-equality check — it is a no-op when the
// embedded asset matches the file on disk, and an atomic replace when it differs.
// This guarantees pr_mode.sh stays current after gentle-ai updates without
// touching the file on every sync when nothing has changed.
func EnsureRuntimeAssets(homeDir string) error {
	prModePath := RuntimePRModePath(homeDir)

	content, err := assets.Read("gga/pr_mode.sh")
	if err != nil {
		return fmt.Errorf("read embedded gga runtime asset pr_mode.sh: %w", err)
	}

	if _, err := filemerge.WriteFileAtomic(prModePath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write gga runtime file %q: %w", prModePath, err)
	}

	return nil
}
