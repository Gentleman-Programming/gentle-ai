package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// windowsProfile is a convenience profile for Windows tests that exercise the
// npm preflight (scoop is the package manager reported by Windows scoop users).
var windowsProfile = system.PlatformProfile{OS: "windows", PackageManager: "winget", Supported: true}

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
	// Claude Code requires npm (not uv). Verify that when npm is present but
	// uv is absent, the step passes — proving uv is not required for npm agents.
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		if file == "npm" {
			return "/usr/local/bin/npm", nil
		}
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

// --- npm preflight tests (Bug A: node/npm missing gate) ---

// TestCheckDependenciesStepFailsWhenNpmMissingForClaudeCode verifies that a missing
// npm blocks the pipeline with a clear, actionable error before any npm command runs.
// This is the regression test for the Windows issue where the pipeline proceeded to
// `npm install -g …` and surfaced a cryptic "exec: npm: executable file not found".
func TestCheckDependenciesStepFailsWhenNpmMissingForClaudeCode(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		if file == "npm" {
			return "", errNotFound{}
		}
		return "/usr/bin/" + file, nil
	})
	t.Cleanup(restore)

	step := checkDependenciesStep{
		id:      "prepare:check-dependencies",
		profile: windowsProfile,
		selection: model.Selection{
			Agents: []model.AgentID{model.AgentClaudeCode},
		},
	}

	err := step.Run()
	if err == nil {
		t.Fatal("checkDependenciesStep.Run() expected error for missing npm when claude-code is selected")
	}
	if !strings.Contains(err.Error(), "npm") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, want npm remediation hint", err.Error())
	}
	if !strings.Contains(err.Error(), "Node.js") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, want Node.js remediation hint", err.Error())
	}
}

// TestCheckDependenciesStepFailsWhenNpmMissingForOpenCode verifies the same gate for
// OpenCode, which also uses npm on Windows.
func TestCheckDependenciesStepFailsWhenNpmMissingForOpenCode(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		if file == "npm" {
			return "", errNotFound{}
		}
		return "/usr/bin/" + file, nil
	})
	t.Cleanup(restore)

	step := checkDependenciesStep{
		id:      "prepare:check-dependencies",
		profile: windowsProfile,
		selection: model.Selection{
			Agents: []model.AgentID{model.AgentOpenCode},
		},
	}

	err := step.Run()
	if err == nil {
		t.Fatal("checkDependenciesStep.Run() expected error for missing npm when opencode is selected")
	}
	if !strings.Contains(err.Error(), "npm") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, want npm remediation hint", err.Error())
	}
}

// TestCheckDependenciesStepNpmHintContainsWingetOnWindows verifies that the install
// hint on Windows points the user to the `winget install OpenJS.NodeJS.LTS` command.
func TestCheckDependenciesStepNpmHintContainsWingetOnWindows(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		if file == "npm" {
			return "", errNotFound{}
		}
		return "/usr/bin/" + file, nil
	})
	t.Cleanup(restore)

	step := checkDependenciesStep{
		id:      "prepare:check-dependencies",
		profile: windowsProfile,
		selection: model.Selection{
			Agents: []model.AgentID{model.AgentClaudeCode},
		},
	}

	err := step.Run()
	if err == nil {
		t.Fatal("checkDependenciesStep.Run() expected error for missing npm on Windows")
	}
	if !strings.Contains(err.Error(), "winget") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, want winget install hint for Windows", err.Error())
	}
}

// TestCheckDependenciesStepPassesWhenNpmPresent verifies that npm-based agents do not
// fail the preflight when npm is found on PATH.
func TestCheckDependenciesStepPassesWhenNpmPresent(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		// npm is present, everything else is too
		return "/usr/bin/" + file, nil
	})
	t.Cleanup(restore)

	for _, agent := range []model.AgentID{
		model.AgentClaudeCode,
		model.AgentOpenCode,
		model.AgentKilocode,
	} {
		agent := agent
		t.Run(string(agent), func(t *testing.T) {
			step := checkDependenciesStep{
				id:      "prepare:check-dependencies",
				profile: windowsProfile,
				selection: model.Selection{
					Agents: []model.AgentID{agent},
				},
			}
			if err := step.Run(); err != nil {
				t.Fatalf("checkDependenciesStep.Run() unexpected error for %s with npm present: %v", agent, err)
			}
		})
	}
}

// TestCheckDependenciesStepFailsWhenNpmMissingForPi verifies that selecting Pi with
// npm absent produces a clear, actionable Node.js / npm error before any npm command
// runs. Pi's install always runs engramInitCommand(), which falls through to
// `npm exec` when pnpm is absent — so npm (and Node.js) is an unconditional requirement.
func TestCheckDependenciesStepFailsWhenNpmMissingForPi(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		// pi binary is present, but npm (and pnpm) are absent.
		if file == "pi" {
			return "/usr/local/bin/pi", nil
		}
		return "", errNotFound{}
	})
	t.Cleanup(restore)

	step := checkDependenciesStep{
		id:      "prepare:check-dependencies",
		profile: windowsProfile,
		selection: model.Selection{
			Agents: []model.AgentID{model.AgentPi},
		},
	}

	err := step.Run()
	if err == nil {
		t.Fatal("checkDependenciesStep.Run() expected error for missing npm when pi is selected")
	}
	if !strings.Contains(err.Error(), "npm") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, want npm remediation hint", err.Error())
	}
	if !strings.Contains(err.Error(), "Node.js") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, want Node.js remediation hint", err.Error())
	}
}

// TestCheckDependenciesStepKimiStillFailsForMissingUVEvenWhenNpmAbsent verifies that
// Kimi's uv preflight is independent of the npm preflight — both can fail, and the
// first failure encountered (npm, since Kimi is not in npmBasedAgents) wins.
func TestCheckDependenciesStepKimiNotBlockedByNpmPreflight(t *testing.T) {
	restore := installcmd.OverrideLookPath(func(file string) (string, error) {
		// npm is missing, uv is also missing
		return "", errNotFound{}
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
		t.Fatal("checkDependenciesStep.Run() expected error for missing uv when kimi is selected")
	}
	// Kimi is NOT in npmBasedAgents, so the error must be about uv, not npm.
	if !strings.Contains(err.Error(), "uv") {
		t.Fatalf("checkDependenciesStep.Run() error = %q, want uv remediation hint for Kimi", err.Error())
	}
}
