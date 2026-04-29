package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
)

func TestPIPhaseAssetsUseReportArtifactKeys(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{file: "pi/agents/sdd-verify.md", want: "sdd/{change-name}/verify-report"},
		{file: "pi/agents/sdd-archive.md", want: "sdd/{change-name}/archive-report"},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			content := string(assets.MustRead(tt.file))
			if !strings.Contains(content, tt.want) {
				t.Fatalf("%s missing required artifact key %q", tt.file, tt.want)
			}
		})
	}
}
