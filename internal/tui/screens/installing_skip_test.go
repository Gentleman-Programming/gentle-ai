package screens

import (
	"strings"
	"testing"
)

func TestInstallingReportsSkippedNotInstalled(t *testing.T) {
	out := RenderInstalling(InstallProgress{Percent: 100, Done: true, Items: []ProgressItem{{Label: "logo", Status: "skipped"}}}, "*")
	if !strings.Contains(out, "logo (skipped)") || !strings.Contains(out, "1 skipped") || strings.Contains(out, "completed successfully") {
		t.Fatalf("misleading skip: %s", out)
	}
}
