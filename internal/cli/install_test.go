package cli

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestParseInstallFlagsSupportsCSVAndRepeated(t *testing.T) {
	flags, err := ParseInstallFlags([]string{
		"--agent", "claude-code,opencode",
		"--agent", "cursor",
		"--component", "engram,sdd",
		"--component", "skills",
		"--skill", "sdd-apply",
		"--persona", "neutral",
		"--preset", "minimal",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("ParseInstallFlags() error = %v", err)
	}

	if !reflect.DeepEqual(flags.Agents, []string{"claude-code", "opencode", "cursor"}) {
		t.Fatalf("agents = %v", flags.Agents)
	}

	if !reflect.DeepEqual(flags.Components, []string{"engram", "sdd", "skills"}) {
		t.Fatalf("components = %v", flags.Components)
	}

	if !flags.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
}

func TestNormalizeInstallFlagsDefaults(t *testing.T) {
	input, err := NormalizeInstallFlags(InstallFlags{}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v", err)
	}

	want := model.Selection{
		Agents:  []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentKilocode, model.AgentGeminiCLI, model.AgentCodex, model.AgentCursor, model.AgentVSCodeCopilot, model.AgentAntigravity, model.AgentWindsurf},
		Persona: model.PersonaGentleman,
		Preset:  model.PresetFullGentleman,
		Components: []model.ComponentID{
			model.ComponentEngram,
			model.ComponentSDD,
			model.ComponentSkills,
			model.ComponentContext7,
			model.ComponentPersona,
			model.ComponentPermission,
			model.ComponentGGA,
		},
	}

	if !reflect.DeepEqual(input.Selection, want) {
		t.Fatalf("selection = %#v, want %#v", input.Selection, want)
	}
}

func TestNormalizeInstallFlagsRejectsUnknownPersona(t *testing.T) {
	_, err := NormalizeInstallFlags(InstallFlags{Persona: "wizard"}, system.DetectionResult{})
	if err == nil {
		t.Fatalf("NormalizeInstallFlags() expected error")
	}
}

func TestNormalizeSDDMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    model.SDDModeID
		wantErr bool
	}{
		{name: "empty returns zero value", input: "", want: ""},
		{name: "whitespace returns zero value", input: "   ", want: ""},
		{name: "single is valid", input: "single", want: model.SDDModeSingle},
		{name: "multi is valid", input: "multi", want: model.SDDModeMulti},
		{name: "invalid rejected", input: "turbo", wantErr: true},
		{name: "partial invalid", input: "mult", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSDDMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeSDDMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeSDDMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseInstallFlagsSDDMode(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{
			name: "flag absent defaults to empty",
			args: []string{"--agent", "opencode"},
			want: "",
		},
		{
			name: "flag set to multi",
			args: []string{"--agent", "opencode", "--sdd-mode", "multi"},
			want: "multi",
		},
		{
			name: "flag set to single",
			args: []string{"--agent", "opencode", "--sdd-mode", "single"},
			want: "single",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := ParseInstallFlags(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseInstallFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
			if flags.SDDMode != tt.want {
				t.Fatalf("flags.SDDMode = %q, want %q", flags.SDDMode, tt.want)
			}
		})
	}
}

func TestNormalizeInstallFlagsSDDModeMulti(t *testing.T) {
	input, err := NormalizeInstallFlags(
		InstallFlags{SDDMode: "multi"},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v", err)
	}
	if input.Selection.SDDMode != model.SDDModeMulti {
		t.Fatalf("SDDMode = %q, want %q", input.Selection.SDDMode, model.SDDModeMulti)
	}
}

func TestNormalizeInstallFlagsSDDModeInvalid(t *testing.T) {
	_, err := NormalizeInstallFlags(
		InstallFlags{SDDMode: "turbo"},
		system.DetectionResult{},
	)
	if err == nil {
		t.Fatal("expected error for invalid sdd-mode")
	}
}

func TestRunInstallDryRunSkipsExecution(t *testing.T) {
	result, err := RunInstall([]string{"--dry-run"}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}

	if len(result.Plan.Apply) == 0 {
		t.Fatalf("apply steps = 0, want > 0")
	}

	if len(result.Execution.Apply.Steps) != 0 || len(result.Execution.Prepare.Steps) != 0 {
		t.Fatalf("execution should be empty in dry-run")
	}
}

// ─── Detection-default consumer regression tests ───────────────────────────

// makeDetectionWithAgents builds a DetectionResult with the specified agents
// marked as Exists=true. All other agents are absent.
func makeDetectionWithAgents(present ...string) system.DetectionResult {
	var configs []system.ConfigState
	// Full canonical agent set — mirrors knownAgentConfigDirs in config_scan.go.
	known := []string{"claude-code", "opencode", "kilocode", "gemini-cli", "cursor", "vscode-copilot", "codex", "antigravity", "windsurf"}
	presentSet := make(map[string]bool, len(present))
	for _, p := range present {
		presentSet[p] = true
	}
	for _, agent := range known {
		configs = append(configs, system.ConfigState{
			Agent:       agent,
			Path:        "/tmp/fake/" + agent,
			Exists:      presentSet[agent],
			IsDirectory: presentSet[agent],
		})
	}
	return system.DetectionResult{Configs: configs}
}

// TestDefaultAgentsFromDetection_CodexIsIncludedWhenPresent is a regression
// guard: when the codex config dir exists, defaultAgentsFromDetection must
// include model.AgentCodex in its result. Previously the switch statement
// omitted codex, so detection-driven selection silently dropped it.
func TestDefaultAgentsFromDetection_CodexIsIncludedWhenPresent(t *testing.T) {
	detection := makeDetectionWithAgents("codex")
	agents := defaultAgentsFromDetection(detection)

	found := false
	for _, id := range agents {
		if id == model.AgentCodex {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("defaultAgentsFromDetection() did not include codex even though config dir is present; got %v", agents)
	}
}

// TestDefaultAgentsFromDetection_AllAgentsMappedCorrectly verifies every
// canonical agent string maps to its model.AgentID constant. This prevents
// silent drops when new agents are added to ScanConfigs without updating the
// consumer switch.
func TestDefaultAgentsFromDetection_AllAgentsMappedCorrectly(t *testing.T) {
	tests := []struct {
		configAgent string
		wantID      model.AgentID
	}{
		{"claude-code", model.AgentClaudeCode},
		{"opencode", model.AgentOpenCode},
		{"kilocode", model.AgentKilocode},
		{"gemini-cli", model.AgentGeminiCLI},
		{"cursor", model.AgentCursor},
		{"vscode-copilot", model.AgentVSCodeCopilot},
		{"codex", model.AgentCodex},
		{"antigravity", model.AgentAntigravity},
		{"windsurf", model.AgentWindsurf},
	}

	for _, tt := range tests {
		t.Run(tt.configAgent, func(t *testing.T) {
			detection := makeDetectionWithAgents(tt.configAgent)
			agents := defaultAgentsFromDetection(detection)

			found := false
			for _, id := range agents {
				if id == tt.wantID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("defaultAgentsFromDetection() missing %q → %q mapping; got %v",
					tt.configAgent, tt.wantID, agents)
			}
			// Exactly one agent should be in the result (only one dir exists).
			if len(agents) != 1 {
				t.Errorf("defaultAgentsFromDetection() returned %d agents, want 1; got %v", len(agents), agents)
			}
		})
	}
}
