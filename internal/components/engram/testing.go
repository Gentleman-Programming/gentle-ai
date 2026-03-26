package engram

import (
	"errors"
	"testing"
)

// SetupMockEngramPath mocks execLookPath to return a fixed absolute path for engram.
// This is exported for use in external test packages (e.g., golden_test.go).
func SetupMockEngramPath(t *testing.T) func() {
	t.Helper()
	original := execLookPath
	execLookPath = func(file string) (string, error) {
		if file == "engram" {
			return "/usr/local/bin/engram", nil
		}
		return "", errors.New("not found")
	}
	return func() { execLookPath = original }
}
