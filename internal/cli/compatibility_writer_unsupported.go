//go:build !(aix || android || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows)

package cli

import (
	"fmt"
	"runtime"
)

// os.Root follows symlinks contained within its root, which does not meet the
// compatibility route's physical-directory contract. Fail closed where the
// platform lacks the Unix descriptor-relative no-follow primitives.
func newCompatibilityDirectoryWriter(homeDir, skillDir string) (compatibilityDirectoryWriter, error) {
	// refusal:by-design world-action: this platform cannot enforce the compatibility route's physical-directory contract.
	return nil, fmt.Errorf("compatibility skills refresh is disabled on %s: physical-directory containment is unavailable", runtime.GOOS)
}
