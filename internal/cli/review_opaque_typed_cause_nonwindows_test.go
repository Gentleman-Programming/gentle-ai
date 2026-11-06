//go:build !windows

package cli

import (
	"os"
	"testing"
)

func makeOpaqueSharedDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
}
