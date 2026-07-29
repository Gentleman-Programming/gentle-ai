package sdd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestSharedPromptDir verifies the expected directory path is returned.
func TestSharedPromptDir(t *testing.T) {
	want := filepath.FromSlash("/home/testuser/.config/opencode/prompts/sdd")
	got := SharedPromptDir(filepath.FromSlash("/home/testuser"))
	if got != want {
		t.Fatalf("SharedPromptDir(%q) = %q, want %q", "/home/testuser", got, want)
	}
}

func TestSharedPromptDirUsesXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdgConfigHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	want := filepath.Join(xdgConfigHome, "opencode", "prompts", "sdd")
	if got := SharedPromptDir(home); got != want {
		t.Fatalf("SharedPromptDir() = %q, want %q", got, want)
	}
}

func TestSharedPromptFileRefFallsBackToAbsolutePath(t *testing.T) {
	home := t.TempDir()
	xdgConfigHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	got, err := sharedPromptFileRef(
		filepath.Join(xdgConfigHome, "opencode", "opencode.json"),
		home,
		"sdd-apply",
		func(string, string) (string, error) { return "", errors.New("different volume") },
	)
	if err != nil {
		t.Fatalf("sharedPromptFileRef() error = %v", err)
	}
	want := "{file:" + filepath.ToSlash(filepath.Join(xdgConfigHome, "opencode", "prompts", "sdd", "sdd-apply.md")) + "}"
	if got != want {
		t.Fatalf("sharedPromptFileRef() = %q, want %q", got, want)
	}
}

func TestSharedPromptFileRef(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		name         string
		settingsPath string
		want         string
	}{
		{
			name:         "OpenCode settings",
			settingsPath: filepath.Join(home, ".config", "opencode", "opencode.json"),
			want:         "{file:./prompts/sdd/sdd-apply.md}",
		},
		{
			name:         "Kilocode settings",
			settingsPath: filepath.Join(home, ".config", "kilo", "opencode.json"),
			want:         "{file:../opencode/prompts/sdd/sdd-apply.md}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SharedPromptFileRef(tt.settingsPath, home, "sdd-apply")
			if err != nil {
				t.Fatalf("SharedPromptFileRef() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("SharedPromptFileRef() = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, filepath.ToSlash(home)) {
				t.Fatalf("SharedPromptFileRef() = %q, contains home path", got)
			}
		})
	}
}

func readOpenCodeAgents(t *testing.T, settingsPath string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", settingsPath, err)
	}
	var settings struct {
		Agent map[string]any `json:"agent"`
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", settingsPath, err)
	}
	return settings.Agent
}

func agentPrompt(t *testing.T, agentsMap map[string]any, agentName string) string {
	t.Helper()
	agentRaw, ok := agentsMap[agentName]
	if !ok {
		t.Fatalf("agent %q missing", agentName)
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		t.Fatalf("agent %q has type %T, want object", agentName, agentRaw)
	}
	prompt, ok := agentMap["prompt"].(string)
	if !ok {
		t.Fatalf("agent %q prompt has type %T, want string", agentName, agentMap["prompt"])
	}
	return prompt
}

func sddInstalledSubAgentsForCodeGraphTest() []string {
	agents := append([]string{}, SharedPromptPhases()...)
	agents = append(agents, sddJudgmentDaySubAgentsForCodeGraphTest()...)
	agents = append(agents, sddReviewSubAgentsForCodeGraphTest()...)
	return agents
}

func sddJudgmentDaySubAgentsForCodeGraphTest() []string {
	return []string{
		"jd-judge-a",
		"jd-judge-b",
		"jd-fix-agent",
	}
}

func sddReviewSubAgentsForCodeGraphTest() []string {
	return []string{
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
		"review-refuter",
	}
}

func sddShellDisabledSubAgentsForCodeGraphTest() []string {
	return []string{"jd-judge-a", "jd-judge-b", "review-refuter"}
}

func assertOpenCodeSubAgentReadOnlyTools(t *testing.T, agentsMap map[string]any, agentName string) {
	t.Helper()
	agent, ok := agentsMap[agentName].(map[string]any)
	if !ok {
		t.Fatalf("agent %q missing or not an object", agentName)
	}
	tools, ok := agent["tools"].(map[string]any)
	if !ok {
		t.Fatalf("agent %q tools have type %T, want object", agentName, agent["tools"])
	}
	for tool, want := range map[string]bool{
		"read":  true,
		"write": false,
		"edit":  false,
		"bash":  false,
		"task":  false,
	} {
		got, ok := tools[tool].(bool)
		if !ok || got != want {
			t.Fatalf("agent %q tool %q = %v, want %t", agentName, tool, tools[tool], want)
		}
	}
}

func nativeMarkdownSubAgentFilesForCodeGraphTest(t *testing.T, adapter agents.Adapter) []string {
	t.Helper()
	entries, err := assets.FS.ReadDir(adapter.EmbeddedSubAgentsDir())
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", adapter.EmbeddedSubAgentsDir(), err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && isMarkdownSubAgentPromptFile(entry.Name()) {
			files = append(files, entry.Name())
		}
	}
	return files
}

func nativeToolsLineForCodeGraphTest(t *testing.T, content string) string {
	t.Helper()
	end := strings.Index(content, "\n---\n")
	if !strings.HasPrefix(content, "---\n") || end < 0 {
		t.Fatal("native prompt missing frontmatter")
	}
	for _, line := range strings.Split(content[:end], "\n") {
		if strings.HasPrefix(line, "tools:") {
			return line
		}
	}
	t.Fatal("native prompt missing tools line")
	return ""
}

func kimiYAMLSubagentFilesForCodeGraphTest() []string {
	return []string{"sdd-apply.yaml", "review-risk.yaml"}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func withoutStrings(values []string, excluded ...string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if containsString(excluded, value) {
			continue
		}
		kept = append(kept, value)
	}
	return kept
}

// TestWriteSharedPromptFilesCreates10Files verifies that WriteSharedPromptFiles
// creates exactly the 10 expected prompt files under {homeDir}/.config/opencode/prompts/sdd/.
func TestWriteSharedPromptFilesCreates10Files(t *testing.T) {
	home := t.TempDir()

	changed, err := WriteSharedPromptFiles(home, nil)
	if err != nil {
		t.Fatalf("WriteSharedPromptFiles() error = %v", err)
	}
	if !changed {
		t.Fatal("WriteSharedPromptFiles() first call changed = false, want true")
	}

	expectedFiles := []string{
		"sdd-init.md",
		"sdd-explore.md",
		"sdd-propose.md",
		"sdd-spec.md",
		"sdd-design.md",
		"sdd-tasks.md",
		"sdd-apply.md",
		"sdd-verify.md",
		"sdd-archive.md",
		"sdd-onboard.md",
	}

	promptDir := SharedPromptDir(home)
	for _, fileName := range expectedFiles {
		path := filepath.Join(promptDir, fileName)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Errorf("prompt file %q not found: %v", path, statErr)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("prompt file %q is empty", path)
		}
	}
}

// TestWriteSharedPromptFilesIdempotent verifies that calling WriteSharedPromptFiles
// twice returns changed=false on the second call.
func TestWriteSharedPromptFilesIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := WriteSharedPromptFiles(home, nil)
	if err != nil {
		t.Fatalf("WriteSharedPromptFiles() first error = %v", err)
	}
	if !first {
		t.Fatal("WriteSharedPromptFiles() first call changed = false, want true")
	}

	second, err := WriteSharedPromptFiles(home, nil)
	if err != nil {
		t.Fatalf("WriteSharedPromptFiles() second error = %v", err)
	}
	if second {
		t.Fatal("WriteSharedPromptFiles() second call changed = true, want false (idempotent)")
	}
}

// TestWriteSharedPromptFilesContent verifies each prompt file contains the
// executor-scoped sub-agent prompt content for the correct phase.
func TestWriteSharedPromptFilesContent(t *testing.T) {
	home := t.TempDir()

	if _, err := WriteSharedPromptFiles(home, nil); err != nil {
		t.Fatalf("WriteSharedPromptFiles() error = %v", err)
	}

	promptDir := SharedPromptDir(home)

	phases := []struct {
		file  string
		phase string
	}{
		{"sdd-init.md", "init"},
		{"sdd-explore.md", "explore"},
		{"sdd-propose.md", "propose"},
		{"sdd-spec.md", "spec"},
		{"sdd-design.md", "design"},
		{"sdd-tasks.md", "tasks"},
		{"sdd-apply.md", "apply"},
		{"sdd-verify.md", "verify"},
		{"sdd-archive.md", "archive"},
		{"sdd-onboard.md", "onboard"},
	}

	for _, tc := range phases {
		path := filepath.Join(promptDir, tc.file)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("ReadFile(%q) error = %v", path, readErr)
			continue
		}

		content := string(data)

		// Each file must contain the phase name (executor-scoped prompt).
		if !strings.Contains(content, tc.phase) {
			t.Errorf("prompt file %q missing phase %q in content", tc.file, tc.phase)
		}

		// Each file must have substantial content (not the old one-liner).
		if len(content) < 200 {
			t.Errorf("prompt file %q content too short (%d bytes), want >= 200", tc.file, len(content))
		}

		// Each file must contain the ORCHESTRATOR gate/note (present in all skill files)
		// or "do not delegate" (present in some skill files).
		hasGate := strings.Contains(content, "ORCHESTRATOR GATE") || strings.Contains(content, "ORCHESTRATOR NOTE")
		hasDoNotDelegate := strings.Contains(strings.ToLower(content), "do not delegate")
		if !hasGate && !hasDoNotDelegate {
			t.Errorf("prompt file %q missing expected skill content (ORCHESTRATOR GATE/NOTE or do not delegate)", tc.file)
		}
	}
}

func TestWriteSharedPromptFilesLanguageContract(t *testing.T) {
	phases := []string{
		"sdd-init.md",
		"sdd-explore.md",
		"sdd-propose.md",
		"sdd-spec.md",
		"sdd-design.md",
		"sdd-tasks.md",
		"sdd-apply.md",
		"sdd-verify.md",
		"sdd-archive.md",
		"sdd-onboard.md",
	}

	for _, capability := range []string{"capable", "small"} {
		t.Run(capability, func(t *testing.T) {
			home := t.TempDir()
			phaseCapabilities := make(map[string]string, len(phases))
			for _, fileName := range phases {
				phaseCapabilities[strings.TrimSuffix(fileName, ".md")] = capability
			}
			if _, err := WriteSharedPromptFiles(home, phaseCapabilities); err != nil {
				t.Fatalf("WriteSharedPromptFiles(%s) error = %v", capability, err)
			}

			for _, fileName := range phases {
				path := filepath.Join(SharedPromptDir(home), fileName)
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("ReadFile(%q) error = %v", path, err)
				}
				text := string(content)
				for _, required := range []string{
					"Generated technical artifacts default to English",
					"If technical artifacts are explicitly requested in another language, use a neutral/professional register",
					"Public/contextual comments follow the target context language",
				} {
					if !strings.Contains(text, required) {
						t.Fatalf("%s/%s missing delegated prompt language contract %q", capability, fileName, required)
					}
				}
			}
		})
	}
}

// TestWriteSharedPromptFilesWithCapabilities verifies that prompt file content
// differs based on model capability (small vs capable).
func TestWriteSharedPromptFilesWithCapabilities(t *testing.T) {
	home := t.TempDir()

	// Write with "capable" for sdd-apply.
	capableMap := map[string]string{"sdd-apply": "capable"}
	_, err := WriteSharedPromptFiles(home, capableMap)
	if err != nil {
		t.Fatalf("WriteSharedPromptFiles(capable) error = %v", err)
	}

	capablePath := filepath.Join(SharedPromptDir(home), "sdd-apply.md")
	capableContent, err := os.ReadFile(capablePath)
	if err != nil {
		t.Fatalf("ReadFile(capable) error = %v", err)
	}

	// Now write with "small" for sdd-apply.
	smallMap := map[string]string{"sdd-apply": "small"}
	_, err = WriteSharedPromptFiles(home, smallMap)
	if err != nil {
		t.Fatalf("WriteSharedPromptFiles(small) error = %v", err)
	}

	smallPath := filepath.Join(SharedPromptDir(home), "sdd-apply.md")
	smallContent, err := os.ReadFile(smallPath)
	if err != nil {
		t.Fatalf("ReadFile(small) error = %v", err)
	}

	// The two contents should differ (different skill sections).
	if string(capableContent) == string(smallContent) {
		t.Fatal("sdd-apply.md content should differ between 'capable' and 'small' sections")
	}

	// Small section should mention "max 3 files" (small model constraint).
	if !strings.Contains(string(smallContent), "max 3 files") {
		t.Error("small section should contain 'max 3 files'")
	}

	// Capable section should NOT mention "max 3 files" (no such constraint).
	if strings.Contains(string(capableContent), "max 3 files") {
		t.Error("capable section should NOT contain 'max 3 files'")
	}
}

// TestInjectOpenCodeMultiModeSubagentPromptsUseFilePaths verifies that after
// injection in multi-mode, each sub-agent's prompt field in opencode.json
// contains a {file:...} reference (not an inline string).
func TestInjectOpenCodeMultiModeSubagentPromptsUseFilePaths(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	if _, err := Inject(home, opencodeAdapter(), "multi"); err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(content)
	for _, phase := range []string{"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"} {
		expectedRef, err := SharedPromptFileRef(settingsPath, home, phase)
		if err != nil {
			t.Fatalf("SharedPromptFileRef() error = %v", err)
		}
		if !strings.Contains(text, expectedRef) {
			t.Errorf("opencode.json sub-agent %q missing {file:...} reference %q", phase, expectedRef)
		}
	}
	if strings.Contains(text, filepath.ToSlash(home)) {
		t.Fatalf("opencode.json contains home-specific absolute path %q", filepath.ToSlash(home))
	}
}

func TestWriteSharedPromptFilesOmitCodeGraphGuidanceByDefault(t *testing.T) {
	home := t.TempDir()

	if _, err := WriteSharedPromptFiles(home, nil); err != nil {
		t.Fatalf("WriteSharedPromptFiles() error = %v", err)
	}

	for _, phase := range SharedPromptPhases() {
		path := filepath.Join(SharedPromptDir(home), phase+".md")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		text := string(content)
		if strings.Contains(text, "<!-- gentle-ai:codegraph-guidance -->") || strings.Contains(text, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s unexpectedly contains CodeGraph guidance by default", phase)
		}
	}
}

func TestWriteSharedPromptFilesIncludeCodeGraphGuidanceWhenEnabled(t *testing.T) {
	home := t.TempDir()
	guidance := communitytool.CodeGraphGuidanceMarkdown()

	if _, err := WriteSharedPromptFiles(home, nil, guidance); err != nil {
		t.Fatalf("WriteSharedPromptFiles() error = %v", err)
	}

	for _, phase := range SharedPromptPhases() {
		path := filepath.Join(SharedPromptDir(home), phase+".md")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		text := string(content)
		if !strings.Contains(text, "<!-- gentle-ai:codegraph-guidance -->") || !strings.Contains(text, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s missing CodeGraph guidance when enabled", phase)
		}
		if count := strings.Count(text, "<!-- gentle-ai:codegraph-guidance -->"); count != 1 {
			t.Fatalf("%s has %d CodeGraph guidance sections, want 1", phase, count)
		}
	}
}

func TestInjectOpenCodeSingleModeSubagentPromptsOmitCodeGraphGuidanceByDefault(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	if _, err := Inject(home, opencodeAdapter(), model.SDDModeSingle); err != nil {
		t.Fatalf("Inject(single) error = %v", err)
	}

	agentsMap := readOpenCodeAgents(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	for _, agentName := range sddInstalledSubAgentsForCodeGraphTest() {
		prompt := agentPrompt(t, agentsMap, agentName)
		if strings.Contains(prompt, "<!-- gentle-ai:codegraph-guidance -->") || strings.Contains(prompt, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s unexpectedly contains CodeGraph guidance by default", agentName)
		}
	}
}

func TestInjectOpenCodeSingleModeSubagentPromptsRespectBashCapabilityWhenCodeGraphEnabled(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	if _, err := Inject(home, opencodeAdapter(), model.SDDModeSingle, InjectOptions{CodeGraphGuidanceMarkdown: communitytool.CodeGraphGuidanceMarkdown()}); err != nil {
		t.Fatalf("Inject(single) error = %v", err)
	}

	agentsMap := readOpenCodeAgents(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	bashCapableAgents := append(SharedPromptPhases(), "jd-fix-agent")
	for _, agentName := range bashCapableAgents {
		prompt := agentPrompt(t, agentsMap, agentName)
		if !strings.Contains(prompt, "<!-- gentle-ai:codegraph-guidance -->") || !strings.Contains(prompt, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s missing CodeGraph guidance when enabled", agentName)
		}
		if count := strings.Count(prompt, "<!-- gentle-ai:codegraph-guidance -->"); count != 1 {
			t.Fatalf("%s has %d CodeGraph guidance sections, want 1", agentName, count)
		}
	}
	for _, agentName := range sddShellDisabledSubAgentsForCodeGraphTest() {
		prompt := agentPrompt(t, agentsMap, agentName)
		if strings.Contains(prompt, "<!-- gentle-ai:codegraph-guidance -->") || strings.Contains(prompt, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s contains shell-based CodeGraph guidance with bash disabled", agentName)
		}
		assertOpenCodeSubAgentReadOnlyTools(t, agentsMap, agentName)
	}
}

func TestInjectOpenCodeMultiModeSubagentPromptFilesIncludeCodeGraphGuidanceWhenEnabled(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	if _, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{CodeGraphGuidanceMarkdown: communitytool.CodeGraphGuidanceMarkdown()}); err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	for _, phase := range SharedPromptPhases() {
		path := filepath.Join(SharedPromptDir(home), phase+".md")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		text := string(content)
		if !strings.Contains(text, "<!-- gentle-ai:codegraph-guidance -->") || !strings.Contains(text, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s missing CodeGraph guidance when enabled", phase)
		}
	}

	agentsMap := readOpenCodeAgents(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	fixPrompt := agentPrompt(t, agentsMap, "jd-fix-agent")
	if !strings.Contains(fixPrompt, "<!-- gentle-ai:codegraph-guidance -->") || !strings.Contains(fixPrompt, "gentle-ai codegraph init --cwd <project-root>") {
		t.Fatal("jd-fix-agent missing CodeGraph guidance in multi-mode inline prompt when enabled")
	}
	for _, agentName := range sddShellDisabledSubAgentsForCodeGraphTest() {
		prompt := agentPrompt(t, agentsMap, agentName)
		if strings.Contains(prompt, "<!-- gentle-ai:codegraph-guidance -->") || strings.Contains(prompt, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s contains shell-based CodeGraph guidance with bash disabled", agentName)
		}
		assertOpenCodeSubAgentReadOnlyTools(t, agentsMap, agentName)
	}
}

func TestInjectNativeSDDSubagentsIncludeCodeGraphGuidanceWhenEnabled(t *testing.T) {
	tests := []struct {
		name      string
		agentID   model.AgentID
		toolGrant string
	}{
		{name: "claude", agentID: model.AgentClaudeCode, toolGrant: claudeCodeGraphToolGrant},
		{name: "cursor", agentID: model.AgentCursor},
		{name: "kiro", agentID: model.AgentKiroIDE, toolGrant: kiroCodeGraphToolGrant},
		{name: "kimi", agentID: model.AgentKimi},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			adapter := mustAdapter(t, tc.agentID)
			guidance := communitytool.CodeGraphGuidanceMarkdown()

			if _, err := Inject(home, adapter, model.SDDModeSingle, InjectOptions{CodeGraphGuidanceMarkdown: guidance}); err != nil {
				t.Fatalf("Inject(%s) error = %v", tc.name, err)
			}

			foundRefuter := false
			for _, fileName := range nativeMarkdownSubAgentFilesForCodeGraphTest(t, adapter) {
				foundRefuter = foundRefuter || fileName == "review-refuter.md"
				path := filepath.Join(adapter.SubAgentsDir(home), fileName)
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("ReadFile(%q) error = %v", path, err)
				}
				text := string(content)
				if count := strings.Count(text, "<!-- gentle-ai:codegraph-guidance -->"); count != 1 {
					t.Fatalf("%s guidance count = %d, want 1", fileName, count)
				}

				source := renderBoundedReviewAsset(adapter.EmbeddedSubAgentsDir() + "/" + fileName)
				if tc.toolGrant != "" {
					sourceTools := nativeToolsLineForCodeGraphTest(t, source)
					wantTools := sourceTools + ", " + tc.toolGrant
					if tc.agentID == model.AgentKiroIDE {
						wantTools = strings.TrimSuffix(sourceTools, "]") + `, "` + tc.toolGrant + `"]`
					}
					if got := nativeToolsLineForCodeGraphTest(t, text); got != wantTools {
						t.Fatalf("%s tools = %q, want %q", fileName, got, wantTools)
					}
				}
				for _, grant := range []string{claudeCodeGraphToolGrant, kiroCodeGraphToolGrant} {
					wantCount := 0
					if grant == tc.toolGrant {
						wantCount = 1
					}
					if count := strings.Count(text, grant); count != wantCount {
						t.Fatalf("%s grant %q count = %d, want %d", fileName, grant, count, wantCount)
					}
				}
				if strings.Count(text, "Bash") != strings.Count(source, "Bash") {
					t.Fatalf("%s CodeGraph grant changed Bash count", fileName)
				}
			}
			if !foundRefuter {
				t.Fatal("dynamic native asset coverage missing review-refuter.md")
			}

			second, err := Inject(home, adapter, model.SDDModeSingle, InjectOptions{CodeGraphGuidanceMarkdown: guidance})
			if err != nil {
				t.Fatalf("Inject(%s) second error = %v", tc.name, err)
			}
			if second.Changed {
				t.Fatalf("Inject(%s) second changed = true, want idempotent output", tc.name)
			}
		})
	}
}

func TestInjectNativeSDDSubagentsOmitCodeGraphGuidanceByDefault(t *testing.T) {
	for _, agentID := range []model.AgentID{model.AgentClaudeCode, model.AgentKiroIDE} {
		t.Run(string(agentID), func(t *testing.T) {
			home := t.TempDir()
			adapter := mustAdapter(t, agentID)
			if _, err := Inject(home, adapter, model.SDDModeSingle); err != nil {
				t.Fatalf("Inject(%s) error = %v", agentID, err)
			}

			for _, fileName := range nativeMarkdownSubAgentFilesForCodeGraphTest(t, adapter) {
				path := filepath.Join(adapter.SubAgentsDir(home), fileName)
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("ReadFile(%q) error = %v", path, err)
				}
				text := string(content)
				if strings.Contains(text, "<!-- gentle-ai:codegraph-guidance -->") || strings.Contains(text, "gentle-ai codegraph init --cwd <project-root>") {
					t.Fatalf("%s native subagent unexpectedly contains CodeGraph guidance by default", fileName)
				}
				for _, grant := range []string{claudeCodeGraphToolGrant, kiroCodeGraphToolGrant} {
					if strings.Contains(text, grant) {
						t.Fatalf("%s native subagent unexpectedly grants %q by default", fileName, grant)
					}
				}
			}
		})
	}

	for _, tc := range []struct {
		agentID model.AgentID
		prompt  string
	}{
		{agentID: model.AgentClaudeCode, prompt: "---\ntools: Read, Grep\n---\nBody\n"},
		{agentID: model.AgentKiroIDE, prompt: "---\ntools: [\"read\"]\n---\nBody\n"},
	} {
		if got := injectCodeGraphToolGrantIntoPrompt(tc.prompt, tc.agentID, ""); got != tc.prompt {
			t.Fatalf("disabled %s grant changed prompt bytes: got %q, want %q", tc.agentID, got, tc.prompt)
		}
	}
}

func TestInjectKimiYAMLSubagentsOmitCodeGraphGuidanceByDefault(t *testing.T) {
	home := t.TempDir()

	if _, err := Inject(home, kimiAdapter(), model.SDDModeSingle); err != nil {
		t.Fatalf("Inject(kimi) error = %v", err)
	}

	for _, fileName := range kimiYAMLSubagentFilesForCodeGraphTest() {
		path := filepath.Join(home, ".kimi", "agents", fileName)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		text := string(content)
		if strings.Contains(text, "  instructions: |-") || strings.Contains(text, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s YAML unexpectedly contains CodeGraph guidance by default", fileName)
		}
	}
}

func TestInjectKimiYAMLSubagentsRemainControlFilesWhenCodeGraphEnabled(t *testing.T) {
	home := t.TempDir()

	if _, err := Inject(home, kimiAdapter(), model.SDDModeSingle, InjectOptions{CodeGraphGuidanceMarkdown: communitytool.CodeGraphGuidanceMarkdown()}); err != nil {
		t.Fatalf("Inject(kimi) error = %v", err)
	}

	for _, fileName := range kimiYAMLSubagentFilesForCodeGraphTest() {
		path := filepath.Join(home, ".kimi", "agents", fileName)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		text := string(content)
		for _, want := range []string{"  system_prompt_path: ./", "  exclude_tools:"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s YAML missing %q:\n%s", fileName, want, text)
			}
		}
		for _, forbidden := range []string{"  instructions: |-", "<!-- gentle-ai:codegraph-guidance -->", "gentle-ai codegraph init --cwd <project-root>"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s YAML unexpectedly contains %q:\n%s", fileName, forbidden, text)
			}
		}

		markdownPath := strings.TrimSuffix(path, ".yaml") + ".md"
		markdownContent, err := os.ReadFile(markdownPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", markdownPath, err)
		}
		markdownText := string(markdownContent)
		if !strings.Contains(markdownText, "<!-- gentle-ai:codegraph-guidance -->") || !strings.Contains(markdownText, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%s referenced Markdown prompt missing CodeGraph guidance when enabled", markdownPath)
		}
	}
}

// TestInjectOpenCodeMultiModeOrchestratorPromptIsStillInlined verifies that
// the orchestrator prompt is still inlined (not a file reference) after injection.
func TestInjectOpenCodeMultiModeOrchestratorPromptIsStillInlined(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	if _, err := Inject(home, opencodeAdapter(), "multi"); err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(content)

	// The orchestrator still uses {file:./AGENTS.md} from the overlay (not from prompts/).
	// We check that there's NO file reference inside the prompts/sdd/ dir for orchestrator.
	promptDir := SharedPromptDir(home)
	if strings.Contains(text, filepath.Join(promptDir, "sdd-orchestrator.md")) {
		t.Fatal("orchestrator should NOT use a file reference from prompts/sdd/")
	}
}

// TestInjectOpenCodeMultiModeIdempotentWithPromptFiles verifies that the second
// Inject call returns changed=false when prompt files are already on disk.
func TestInjectOpenCodeMultiModeIdempotentWithPromptFiles(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	first, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(multi) first changed = false")
	}

	second, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject(multi) second changed = true — should be idempotent with prompt files")
	}
}
