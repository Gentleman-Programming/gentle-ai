package cli

import (
	"bytes"
	"testing"
)

func TestRunUninstallWithoutFlagsPrintsUsage(t *testing.T) {
	var buf bytes.Buffer
	_, err := RunUninstall(nil, &buf)
	if err == nil {
		t.Fatal("expected error when no flags provided")
	}
}
