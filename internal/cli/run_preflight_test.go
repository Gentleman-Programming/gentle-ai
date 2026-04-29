package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestCheckDependenciesStepFailsWhenKimiUVMissing(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		if file == "uv" {
			return "", errNotFound{}
		}
		return "/usr/bin/" + file, nil
	})
	t.Cleanup(restore)

	step := checkDependenciesStep{
		id:      "prepare:check-dependencies",
		profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true},
		selection: model.Selection{
			Agents: []model.AgentID{model.AgentKimi},
		},
	}

	err := step.Run()
	if err == nil {
		t.Fatal("checkDependenciesStep.Run() expected error for missing uv when Kimi is selected")
	}

	if !strings.Contains(err.Error(), "Kimi") || !strings.Contains(err.Error(), "uv") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, expected Kimi uv remediation", err.Error())
	}
}

func TestCheckDependenciesStepDoesNotRequireUVForOtherAgents(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		return "", errNotFound{}
	})
	t.Cleanup(restore)

	step := checkDependenciesStep{
		id:      "prepare:check-dependencies",
		profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true},
		selection: model.Selection{
			Agents: []model.AgentID{model.AgentClaudeCode},
		},
	}

	if err := step.Run(); err != nil {
		t.Fatalf("checkDependenciesStep.Run() unexpected error = %v", err)
	}
}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }

func TestValidatePiMultiModelPreflightFailsWithoutPiSubagents(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPiCodingAgent},
		Components: []model.ComponentID{model.ComponentSDD},
		SDDMode:    model.SDDModeMulti,
	}

	err := validatePiMultiModelPreflight(home, workspace, selection)
	if err == nil {
		t.Fatal("validatePiMultiModelPreflight() expected error when pi-subagents is absent")
	}

	if !strings.Contains(err.Error(), "PI multi-model requires installing the `pi-subagents` extension.") {
		t.Fatalf("validatePiMultiModelPreflight() error = %q, expected canonical PI requirement", err.Error())
	}

	if !strings.Contains(err.Error(), "pi install npm:pi-subagents") {
		t.Fatalf("validatePiMultiModelPreflight() error = %q, expected install command guidance", err.Error())
	}
}

func TestValidatePiMultiModelPreflightAllowsWhenPiSubagentsPresent(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workspace, ".pi", "extensions", "pi-subagents"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPiCodingAgent},
		Components: []model.ComponentID{model.ComponentSDD},
		SDDMode:    model.SDDModeMulti,
	}

	if err := validatePiMultiModelPreflight(home, workspace, selection); err != nil {
		t.Fatalf("validatePiMultiModelPreflight() unexpected error = %v", err)
	}
}

func TestValidatePiMultiModelPreflightNoopForOpenCodeRegressionSafety(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode},
		Components: []model.ComponentID{model.ComponentSDD},
		SDDMode:    model.SDDModeMulti,
	}

	if err := validatePiMultiModelPreflight(home, workspace, selection); err != nil {
		t.Fatalf("validatePiMultiModelPreflight() should ignore non-PI selection, got err = %v", err)
	}
}
