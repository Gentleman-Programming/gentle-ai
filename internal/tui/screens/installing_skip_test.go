package screens

import (
	"strings"
	"testing"
)

func TestInstallingStatusIcons(t *testing.T) {
	for _, tc := range []struct {
		status string
		row    string
	}{
		{"pending", "· step"},
		{"running", "* step"},
		{"succeeded", "✓ step"},
		{"failed", "✗ step"},
		{"skipped", "− step (skipped)"},
		{"rolled-back", "↶ step (rolled back)"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			out := RenderInstalling(InstallProgress{Items: []ProgressItem{{Label: "step", Status: tc.status}}}, "*")
			if !strings.Contains(out, tc.row+"\n") {
				t.Fatalf("output = %q, want row %q", out, tc.row)
			}
		})
	}
}

func TestInstallingReportsSkippedNotInstalled(t *testing.T) {
	out := RenderInstalling(InstallProgress{Percent: 100, Done: true, Items: []ProgressItem{{Label: "logo", Status: "skipped"}}}, "*")
	if !strings.Contains(out, "logo (skipped)") || !strings.Contains(out, "1 skipped") || strings.Contains(out, "completed successfully") {
		t.Fatalf("misleading skip: %s", out)
	}
}
