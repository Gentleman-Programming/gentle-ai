package update

import "testing"

// TestIsSelfToolName verifies the closed-set self-tool identity predicate
// (D-01, REQ-22.3). Matching is case- and edge-whitespace-insensitive so the
// cmd/axiom init() rename can never disable an upgrade safeguard.
func TestIsSelfToolName(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{name: "accepts lowercase axiom", toolName: "axiom", want: true},
		{name: "accepts lowercase gentle-ai", toolName: "gentle-ai", want: true},
		{name: "accepts uppercase AXIOM", toolName: "AXIOM", want: true},
		{name: "accepts mixed case with edge spaces", toolName: " Gentle-AI ", want: true},
		{name: "rejects engram", toolName: "engram", want: false},
		{name: "rejects gga", toolName: "gga", want: false},
		{name: "rejects empty", toolName: "", want: false},
		{name: "rejects lookalike Ax1om", toolName: "Ax1om", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSelfToolName(tt.toolName); got != tt.want {
				t.Errorf("IsSelfToolName(%q) = %v, want %v", tt.toolName, got, tt.want)
			}
		})
	}
}

// TestIsSelfTool verifies the ToolInfo-level identity predicate (D-01)
// delegates to IsSelfToolName on the tool name.
func TestIsSelfTool(t *testing.T) {
	tests := []struct {
		name string
		tool ToolInfo
		want bool
	}{
		{name: "accepts ToolInfo named axiom", tool: ToolInfo{Name: "axiom"}, want: true},
		{name: "accepts ToolInfo named gentle-ai", tool: ToolInfo{Name: "gentle-ai"}, want: true},
		{name: "rejects ToolInfo named engram", tool: ToolInfo{Name: "engram"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSelfTool(tt.tool); got != tt.want {
				t.Errorf("IsSelfTool(%+v) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}
