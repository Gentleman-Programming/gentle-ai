package agents

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestSettingsFileFormatFor(t *testing.T) {
	tests := []struct {
		name  string
		agent model.AgentID
		want  SettingsFileFormat
	}{
		{name: "Claude JSON", agent: model.AgentClaudeCode, want: SettingsFileFormatJSONObject},
		{name: "OpenCode JSON", agent: model.AgentOpenCode, want: SettingsFileFormatJSONObject},
		{name: "Kimi TOML", agent: model.AgentKimi, want: SettingsFileFormatTOML},
		{name: "Hermes YAML", agent: model.AgentHermes, want: SettingsFileFormatYAML},
		{name: "unknown agent", agent: model.AgentID("unknown"), want: SettingsFileFormatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SettingsFileFormatFor(tt.agent); got != tt.want {
				t.Fatalf("SettingsFileFormatFor(%q) = %q, want %q", tt.agent, got, tt.want)
			}
		})
	}
}
