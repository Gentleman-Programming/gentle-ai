package tui

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update/upgrade"
)

// TestSelfToolIdentityDetection verifies that the three H-4 sites detect the
// self-tool under both historical names: axiom and gentle-ai (F7.2).
func TestSelfToolIdentityDetection(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{name: "detects axiom", toolName: "axiom", want: true},
		{name: "detects gentle-ai", toolName: "gentle-ai", want: true},
		{name: "rejects engram", toolName: "engram", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := update.IsSelfToolName(tc.toolName); got != tc.want {
				t.Errorf("IsSelfToolName(%q) = %v, want %v", tc.toolName, got, tc.want)
			}

			// Site 1: reportUpgradedGentleAI in model.go
			report := upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{
				{ToolName: tc.toolName, Status: upgrade.UpgradeSucceeded},
			}}
			if got := reportUpgradedGentleAI(report); got != tc.want {
				t.Errorf("reportUpgradedGentleAI(%q) = %v, want %v", tc.toolName, got, tc.want)
			}

			// Site 2: GentleAIUpgradeVersion in model.go
			m := Model{UpgradeReport: &upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{
				{ToolName: tc.toolName, Status: upgrade.UpgradeSucceeded, NewVersion: "v1.0.0"},
			}}}
			_, ok := m.GentleAIUpgradeVersion()
			if ok != tc.want {
				t.Errorf("GentleAIUpgradeVersion(%q) ok = %v, want %v", tc.toolName, ok, tc.want)
			}
		})
	}
}
