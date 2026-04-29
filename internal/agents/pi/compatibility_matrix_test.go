package pi

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEvaluateRolloutReadiness(t *testing.T) {
	tests := []struct {
		name         string
		matrix       map[Contract]ContractStatus
		wantReady    bool
		wantBlockers []ContractCheck
	}{
		{
			name: "all required contracts pass",
			matrix: map[Contract]ContractStatus{
				ContractBinaryInstallMethod: ContractStatusPass,
				ContractConfigRoot:          ContractStatusPass,
				ContractSettingsSchema:      ContractStatusPass,
				ContractMCPShape:            ContractStatusPass,
				ContractCommandPromptLayout: ContractStatusPass,
				ContractModelCacheAuth:      ContractStatusPass,
				ContractProfileSupport:      ContractStatusPass,
			},
			wantReady: true,
		},
		{
			name: "fail status blocks rollout",
			matrix: map[Contract]ContractStatus{
				ContractBinaryInstallMethod: ContractStatusPass,
				ContractConfigRoot:          ContractStatusFail,
				ContractSettingsSchema:      ContractStatusPass,
				ContractMCPShape:            ContractStatusPass,
				ContractCommandPromptLayout: ContractStatusPass,
				ContractModelCacheAuth:      ContractStatusPass,
				ContractProfileSupport:      ContractStatusPass,
			},
			wantReady: false,
			wantBlockers: []ContractCheck{
				{Contract: ContractConfigRoot, Status: ContractStatusFail, Reason: "status fail blocks rollout"},
			},
		},
		{
			name: "unknown status blocks rollout",
			matrix: map[Contract]ContractStatus{
				ContractBinaryInstallMethod: ContractStatusPass,
				ContractConfigRoot:          ContractStatusPass,
				ContractSettingsSchema:      ContractStatusUnknown,
				ContractMCPShape:            ContractStatusPass,
				ContractCommandPromptLayout: ContractStatusPass,
				ContractModelCacheAuth:      ContractStatusPass,
				ContractProfileSupport:      ContractStatusPass,
			},
			wantReady: false,
			wantBlockers: []ContractCheck{
				{Contract: ContractSettingsSchema, Status: ContractStatusUnknown, Reason: "status unknown blocks rollout"},
			},
		},
		{
			name: "missing contract becomes unknown blocker with deterministic order",
			matrix: map[Contract]ContractStatus{
				ContractSettingsSchema:      ContractStatusPass,
				ContractMCPShape:            ContractStatusPass,
				ContractCommandPromptLayout: ContractStatusFail,
				ContractProfileSupport:      ContractStatusUnknown,
			},
			wantReady: false,
			wantBlockers: []ContractCheck{
				{Contract: ContractBinaryInstallMethod, Status: ContractStatusUnknown, Reason: "missing evidence for required contract"},
				{Contract: ContractConfigRoot, Status: ContractStatusUnknown, Reason: "missing evidence for required contract"},
				{Contract: ContractCommandPromptLayout, Status: ContractStatusFail, Reason: "status fail blocks rollout"},
				{Contract: ContractModelCacheAuth, Status: ContractStatusUnknown, Reason: "missing evidence for required contract"},
				{Contract: ContractProfileSupport, Status: ContractStatusUnknown, Reason: "status unknown blocks rollout"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateRolloutReadiness(tt.matrix)

			if got.Ready != tt.wantReady {
				t.Fatalf("EvaluateRolloutReadiness().Ready = %v, want %v", got.Ready, tt.wantReady)
			}

			if len(got.Blockers) != len(tt.wantBlockers) {
				t.Fatalf("EvaluateRolloutReadiness().Blockers length = %d, want %d, got=%v", len(got.Blockers), len(tt.wantBlockers), got.Blockers)
			}

			for i := range tt.wantBlockers {
				if got.Blockers[i] != tt.wantBlockers[i] {
					t.Fatalf("EvaluateRolloutReadiness().Blockers[%d] = %+v, want %+v", i, got.Blockers[i], tt.wantBlockers[i])
				}
			}
		})
	}
}

func TestPICompatibilityDocDocumentsConditionalCapabilityContract(t *testing.T) {
	root := projectRootFromThisFile(t)

	docPath := filepath.Join(root, "docs", "pi-compatibility.md")
	assertFileContains(t, docPath, "Capability Matrix")
	assertFileContains(t, docPath, "pi install npm:pi-subagents")
	assertFileContains(t, docPath, "profiles")
	assertFileContains(t, docPath, "modelPicker")
	assertFileContains(t, docPath, "generatedMulti")
	assertFileContains(t, docPath, "PI multi-model requires installing the `pi-subagents` extension.")

	settingsFixture := filepath.Join(root, "testdata", "pi", "settings.sample.json")
	mcpFixture := filepath.Join(root, "testdata", "pi", "mcp.sample.json")
	commandFixture := filepath.Join(root, "testdata", "pi", "command.sample.md")
	unknownFixture := filepath.Join(root, "testdata", "pi", "unknown-fields.md")

	assertFileContains(t, settingsFixture, "_meta")
	assertFileContains(t, mcpFixture, "mcpServers")
	assertFileContains(t, commandFixture, "sdd-apply")
	assertFileContains(t, unknownFixture, "unknown")
	assertFileContains(t, unknownFixture, "unsupported")
}

func TestPIDocumentationDriftGuardsForAmendedScope(t *testing.T) {
	root := projectRootFromThisFile(t)

	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "PI_CODING_AGENT_DIR")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "~/.pi/agent")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "Global PI root stores settings/prompt/skills")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "project-root `.pi/agents/*`")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "MCP and Context7 are out of scope")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "packages")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "npm:pi-engram")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "frontmatter + `## <agent-name>` step headings")
	assertFileContains(t, filepath.Join(root, "docs", "pi-compatibility.md"), "`sdd-onboard` remains an optional standalone agent")

	assertFileContains(t, filepath.Join(root, "docs", "platforms.md"), "PI_CODING_AGENT_DIR")
	assertFileContains(t, filepath.Join(root, "docs", "platforms.md"), "PI→Engram extension")
	assertFileContains(t, filepath.Join(root, "docs", "platforms.md"), "MCP/Context7 out of scope")
	assertFileContains(t, filepath.Join(root, "docs", "platforms.md"), "packages")

	assertFileContains(t, filepath.Join(root, "README.md"), "PI_CODING_AGENT_DIR")
	assertFileContains(t, filepath.Join(root, "README.md"), "PI→Engram extension")
	assertFileContains(t, filepath.Join(root, "README.md"), "MCP/Context7 out of scope")
	assertFileContains(t, filepath.Join(root, "README.md"), "npm:pi-engram")
}

func projectRootFromThisFile(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if !strings.Contains(string(data), want) {
		t.Fatalf("%s does not contain %q", path, want)
	}
}
