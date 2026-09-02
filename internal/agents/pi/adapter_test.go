package pi

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestAdapterIdentityAndCapabilities(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentPi {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentPi)
	}
	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{"SupportsSkills", a.SupportsSkills(), false},
		{"SupportsMCP", a.SupportsMCP(), true},
		{"SupportsSystemPrompt", a.SupportsSystemPrompt(), false},
		{"SupportsSlashCommands", a.SupportsSlashCommands(), false},
		{"SupportsOutputStyles", a.SupportsOutputStyles(), false},
		{"SupportsSubAgents", a.SupportsSubAgents(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestAdapterPaths(t *testing.T) {
	a := NewAdapter()
	homeDir := t.TempDir()
	piDir := filepath.Join(homeDir, ".pi")
	piAgentDir := filepath.Join(piDir, "agent")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"GlobalConfigDir", a.GlobalConfigDir(homeDir), piDir},
		{"SystemPromptDir", a.SystemPromptDir(homeDir), piAgentDir},
		{"SystemPromptFile", a.SystemPromptFile(homeDir), filepath.Join(piAgentDir, "APPEND_SYSTEM.md")},
		{"SkillsDir", a.SkillsDir(homeDir), ""},
		{"SettingsPath", a.SettingsPath(homeDir), filepath.Join(piAgentDir, "settings.json")},
		{"CommandsDir", a.CommandsDir(homeDir), ""},
		{"MCPConfigPath", a.MCPConfigPath(homeDir, "context7"), filepath.Join(piAgentDir, "mcp.json")},
		{"OutputStyleDir", a.OutputStyleDir(homeDir), ""},
		{"SubAgentsDir", a.SubAgentsDir(homeDir), ""},
		{"EmbeddedSubAgentsDir", a.EmbeddedSubAgentsDir(), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestCodeGraphPathsResolveConfiguredAgentDirectory(t *testing.T) {
	home := t.TempDir()
	configured := filepath.Join(home, "custom-pi")
	t.Setenv("PI_CODING_AGENT_DIR", configured)

	paths := CodeGraphPaths(home)
	if paths.AgentDir != configured {
		t.Fatalf("AgentDir = %q, want %q", paths.AgentDir, configured)
	}
	if paths.MCPConfig != filepath.Join(configured, "mcp.json") {
		t.Fatalf("MCPConfig = %q", paths.MCPConfig)
	}
	if paths.Manifest != filepath.Join(home, ".gentle-ai", "pi-codegraph.json") {
		t.Fatalf("Manifest = %q", paths.Manifest)
	}
}

func TestCodeGraphPathsKeepsAgentDirectoryWhenProjectMCPOverrides(t *testing.T) {
	home := t.TempDir()
	configured := filepath.Join(home, "custom-pi")
	workspace := filepath.Join(home, "project")
	t.Setenv("PI_CODING_AGENT_DIR", configured)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".mcp.json"), []byte(`{"mcpServers":{"codegraph":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := CodeGraphPaths(home)
	effective, err := EffectiveCodeGraphMCPPath(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if paths.AgentDir != configured || effective != filepath.Join(workspace, ".mcp.json") {
		t.Fatalf("agent=%q effective=%q, want configured agent and project config", paths.AgentDir, effective)
	}
}

func TestDiscoverCodeGraphChildrenUsesProjectOverrideAndPreservesPackageSource(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	mustWrite := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(home, ".pi", "agent", "subagents", "worker.md"), "---\ntools: bash\n---\npackage worker\n")
	mustWrite(filepath.Join(workspace, ".pi", "subagents", "worker.md"), "---\ntools: bash, mcp\n---\nproject worker\n")
	mustWrite(filepath.Join(home, ".pi", "agent", "agents", "reader.md"), "---\ntools: read\n---\nreader\n")
	mustWrite(filepath.Join(home, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package-worker.md"), "---\ntools: bash\n---\npackage worker\n")

	children, err := DiscoverCodeGraphChildren(home, workspace)
	if err != nil {
		t.Fatalf("DiscoverCodeGraphChildren() error = %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("children = %#v, want three effective children", children)
	}
	if children[0].Name != "package-worker" || children[1].Name != "reader" || children[2].Name != "worker" {
		t.Fatalf("children = %#v, want sorted package-worker, reader, and worker", children)
	}
	if !children[0].PackageOwned || children[0].Target == children[0].Source {
		t.Fatalf("package worker = %#v, want owned overlay", children[0])
	}
	if children[2].Source != filepath.Join(workspace, ".pi", "subagents", "worker.md") || children[2].PackageOwned {
		t.Fatalf("worker = %#v, want project effective child", children[2])
	}
}

func TestDiscoverCodeGraphChildrenReturnsUnreadableDirectoryError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := piWalkDir
	piWalkDir = func(path string, walkFn fs.WalkDirFunc) error {
		return &fs.PathError{Op: "readdir", Path: path, Err: fs.ErrPermission}
	}
	t.Cleanup(func() { piWalkDir = previous })

	_, err := DiscoverCodeGraphChildren(home, "")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("DiscoverCodeGraphChildren() error = %v, want unreadable-directory error", err)
	}
}

func TestDiscoverCodeGraphChildrenUsesNormalizedRuntimeIdentity(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	for path, body := range map[string]string{
		filepath.Join(home, ".pi", "agent", "subagents", "Worker.md"): "---\ntools: bash\n---\nuser\n",
		filepath.Join(workspace, ".pi", "subagents", "worker.md"):     "---\ntools: bash\n---\nproject\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	children, err := DiscoverCodeGraphChildren(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].Source != filepath.Join(workspace, ".pi", "subagents", "worker.md") {
		t.Fatalf("children = %#v, want one project runtime identity", children)
	}
}

func TestAdapterDetectUsesPiBinaryAndConfigPath(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".pi", "agent")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := &Adapter{
		lookPath: func(file string) (string, error) {
			if file != "pi" {
				t.Fatalf("lookPath called with %q, want pi", file)
			}
			return "/usr/local/bin/pi", nil
		},
		statPath: defaultStat,
	}

	installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), homeDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !installed {
		t.Fatalf("Detect() installed = false, want true")
	}
	if binaryPath != "/usr/local/bin/pi" {
		t.Fatalf("Detect() binaryPath = %q, want /usr/local/bin/pi", binaryPath)
	}
	if configPath != configDir {
		t.Fatalf("Detect() configPath = %q, want %q", configPath, configDir)
	}
	if !configFound {
		t.Fatalf("Detect() configFound = false, want true")
	}
}

func TestAdapterDetectMissingPiBinary(t *testing.T) {
	homeDir := t.TempDir()
	a := &Adapter{
		lookPath: func(file string) (string, error) {
			return "", os.ErrNotExist
		},
		statPath: defaultStat,
	}

	installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), homeDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if installed {
		t.Fatalf("Detect() installed = true, want false")
	}
	if binaryPath != "" {
		t.Fatalf("Detect() binaryPath = %q, want empty", binaryPath)
	}
	if configPath != filepath.Join(homeDir, ".pi", "agent") {
		t.Fatalf("Detect() configPath = %q, want ~/.pi/agent under home", configPath)
	}
	if configFound {
		t.Fatalf("Detect() configFound = true, want false")
	}
}

func TestAdapterInstallCommandSequenceUsesNpmWhenPnpmIsUnavailable(t *testing.T) {
	a := &Adapter{
		lookPath: func(file string) (string, error) {
			if file == "pnpm" {
				return "", os.ErrNotExist
			}
			return "/usr/local/bin/" + file, nil
		},
		statPath: defaultStat,
	}
	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err != nil {
		t.Fatalf("InstallCommand() error = %v", err)
	}

	want := [][]string{
		{"pi", "install", "npm:gentle-pi"},
		{"pi", "install", "npm:gentle-engram"},
		{"pi", "install", "npm:pi-mcp-adapter"},
		{"npm", "exec", "--yes", "--package", "gentle-engram@latest", "--", "pi-engram", "init"},
		piSubagentsInstallCommand(system.PlatformProfile{}),
		{"pi", "install", "npm:@juicesharp/rpiv-ask-user-question"},
		{"pi", "install", "npm:pi-web-access"},
		{"pi", "install", "npm:@juicesharp/rpiv-todo"},
		{"pi", "install", "npm:pi-btw"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("InstallCommand() = %#v, want %#v", commands, want)
	}
}

func TestAdapterInstallCommandSequenceUsesSameSubagentsPackageForWindows(t *testing.T) {
	a := &Adapter{
		lookPath: func(file string) (string, error) {
			if file == "pnpm" {
				return "", os.ErrNotExist
			}
			return "/usr/local/bin/" + file, nil
		},
		statPath: defaultStat,
	}
	commands, err := a.InstallCommand(system.PlatformProfile{OS: "windows"})
	if err != nil {
		t.Fatalf("InstallCommand() error = %v", err)
	}

	want := []string{"pi", "install", "npm:pi-subagents-j0k3r"}
	if !reflect.DeepEqual(commands[4], want) {
		t.Fatalf("InstallCommand()[4] = %#v, want %#v", commands[4], want)
	}
}

func TestAdapterInstallCommandSequenceUsesNpmForEngramInitWhenPnpmIsAvailable(t *testing.T) {
	a := &Adapter{
		lookPath: func(file string) (string, error) {
			if file == "pnpm" {
				return "/usr/local/bin/pnpm", nil
			}
			return "", os.ErrNotExist
		},
		statPath: defaultStat,
	}
	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err != nil {
		t.Fatalf("InstallCommand() error = %v", err)
	}

	want := []string{"npm", "exec", "--yes", "--package", "gentle-engram@latest", "--", "pi-engram", "init"}
	if !reflect.DeepEqual(commands[3], want) {
		t.Fatalf("InstallCommand()[3] = %#v, want %#v", commands[3], want)
	}
}

func TestMergePiSettingsFileRemovesLegacySubagentPackages(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	initial := `{
  "theme": "kanagawa",
  "packages": [
    "npm:pi-subagents",
    "npm:pi-subagents@1.0.0",
    "vendor/pi-subagents",
    "vendor/pi-subagents-fixed@0.0.1",
    "npm:pi-subagents-j0k3r",
    "npm:other@1.0.0"
  ]
}`
	if err := os.WriteFile(settingsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	if _, err := mergePiSettingsFile(settingsPath); err != nil {
		t.Fatalf("mergePiSettingsFile() error = %v", err)
	}

	var settings struct {
		Packages []string `json:"packages"`
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Unmarshal(settings) error = %v", err)
	}

	for _, forbidden := range []string{"npm:pi-subagents", "npm:pi-subagents@1.0.0", "vendor/pi-subagents", "vendor/pi-subagents-fixed@0.0.1"} {
		for _, pkg := range settings.Packages {
			if pkg == forbidden {
				t.Fatalf("packages still contains legacy subagent package %q: %#v", forbidden, settings.Packages)
			}
		}
	}
	if !reflect.DeepEqual(settings.Packages, []string{"npm:pi-subagents-j0k3r", "npm:other@1.0.0", "npm:pi-mcp-adapter"}) {
		t.Fatalf("packages = %#v", settings.Packages)
	}
}

func TestPiLegacyAppendCleanupPreservesUserBytesModeAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	agentDir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	appendFile := filepath.Join(agentDir, "APPEND_SYSTEM.md")

	initial := "# User Preamble\n" +
		"<!-- gentle-ai:persona -->Legacy persona<!-- /gentle-ai:persona -->\n" +
		"<!-- gentle-ai:engram-protocol -->Legacy engram<!-- /gentle-ai:engram-protocol -->\n" +
		"<!-- gentle-ai:sdd-orchestrator -->Legacy SDD<!-- /gentle-ai:sdd-orchestrator -->\n" +
		"<!-- gentle-ai:strict-tdd-mode -->Legacy TDD<!-- /gentle-ai:strict-tdd-mode -->\n" +
		"<!-- gentle-ai:agent-routing -->Legacy routing<!-- /gentle-ai:agent-routing -->\n" +
		"<!-- gentle-ai:trigger-rules -->Legacy trigger<!-- /gentle-ai:trigger-rules -->\n" +
		"<!-- gentle-ai:codegraph-guidance -->Legacy codegraph<!-- /gentle-ai:codegraph-guidance -->\n" +
		"<!-- gentle-ai:custom-user-marker -->\nPreserved custom marker\n<!-- /gentle-ai:custom-user-marker -->\n" +
		"# User Epilogue\n"
	origPerm := os.FileMode(0o640)
	if err := os.WriteFile(appendFile, []byte(initial), origPerm); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	changed, path, err := CleanLegacyAppendSystem(home)
	if err != nil {
		t.Fatalf("CleanLegacyAppendSystem error = %v", err)
	}
	if !changed {
		t.Fatalf("CleanLegacyAppendSystem reported changed = false")
	}
	if path != appendFile {
		t.Fatalf("CleanLegacyAppendSystem path = %q, want %q", path, appendFile)
	}

	info, err := os.Stat(appendFile)
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}
	if info.Mode().Perm() != origPerm {
		t.Fatalf("File mode = %v, want %v", info.Mode().Perm(), origPerm)
	}

	contentBytes, err := os.ReadFile(appendFile)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	cleaned := string(contentBytes)

	for _, sec := range LegacyPiAppendSections {
		if strings.Contains(cleaned, "<!-- gentle-ai:"+sec) {
			t.Errorf("cleaned file still contains legacy section %q", sec)
		}
	}
	if !strings.Contains(cleaned, "# User Preamble") {
		t.Errorf("cleaned file missing user preamble")
	}
	if !strings.Contains(cleaned, "<!-- gentle-ai:custom-user-marker -->\nPreserved custom marker\n<!-- /gentle-ai:custom-user-marker -->") {
		t.Errorf("cleaned file missing non-allowlisted custom marker")
	}
	if !strings.Contains(cleaned, "# User Epilogue") {
		t.Errorf("cleaned file missing user epilogue")
	}

	// Second run is idempotent no-op
	secondChanged, secondPath, err := CleanLegacyAppendSystem(home)
	if err != nil {
		t.Fatalf("second CleanLegacyAppendSystem error = %v", err)
	}
	if secondChanged || secondPath != "" {
		t.Fatalf("second run reported changed = %v, path = %q, want no-op", secondChanged, secondPath)
	}

	secondContent, err := os.ReadFile(appendFile)
	if err != nil {
		t.Fatalf("ReadFile after second run error = %v", err)
	}
	if string(secondContent) != cleaned {
		t.Fatalf("content mutated on second run")
	}

	// Empty file test: only legacy sections should cause file deletion
	onlyLegacy := "<!-- gentle-ai:persona -->Legacy persona<!-- /gentle-ai:persona -->\n<!-- gentle-ai:agent-routing -->Legacy routing<!-- /gentle-ai:agent-routing -->\n"
	if err := os.WriteFile(appendFile, []byte(onlyLegacy), 0o644); err != nil {
		t.Fatalf("WriteFile onlyLegacy error = %v", err)
	}
	delChanged, delPath, err := CleanLegacyAppendSystem(home)
	if err != nil || !delChanged || delPath != appendFile {
		t.Fatalf("CleanLegacyAppendSystem onlyLegacy error = %v, changed = %v, path = %q", err, delChanged, delPath)
	}
	if _, err := os.Stat(appendFile); !os.IsNotExist(err) {
		t.Fatalf("expected empty file to be deleted, stat error = %v", err)
	}
}
