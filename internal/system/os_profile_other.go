//go:build !windows

package system

import "errors"

// osProfileDir has no token-backed equivalent outside Windows.
func osProfileDir() (string, error) {
	return "", errors.New("operating-system profile lookup is only available on Windows")
}
