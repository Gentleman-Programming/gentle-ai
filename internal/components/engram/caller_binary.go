package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// CallerBinary returns the caller-supplied Engram executable named by
// system.EngramBinEnv. When it is set, install uses it as is: no brew, no
// download, no `go install` and no PATH change. Only an absolute value
// counts; ValidateCallerBinary rejects anything else at startup.
func CallerBinary() (string, bool) {
	bin := os.Getenv(system.EngramBinEnv)
	if bin == "" || !filepath.IsAbs(bin) {
		return "", false
	}
	return filepath.Clean(bin), true
}

// ValidateCallerBinary checks that system.EngramBinEnv, when set, names an
// absolute path to an existing executable file.
func ValidateCallerBinary() error {
	bin := os.Getenv(system.EngramBinEnv)
	if bin == "" {
		return nil
	}
	if !filepath.IsAbs(bin) {
		return fmt.Errorf("%s must be an absolute path, got %q", system.EngramBinEnv, bin)
	}
	info, err := os.Stat(bin)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s=%s: file is missing", system.EngramBinEnv, bin)
		}
		return fmt.Errorf("%s=%s: %w", system.EngramBinEnv, bin, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s=%s: not a regular file", system.EngramBinEnv, bin)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s=%s: file is not executable", system.EngramBinEnv, bin)
	}
	return nil
}
