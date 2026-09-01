package communitytool

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("GENTLE_AI_CODEGRAPH_TEST_HELPER"); mode != "" {
		runCodeGraphTestHelper(mode)
		os.Exit(0)
	}
	codeGraphPackageLookPath = func(name string) (string, error) {
		if name == "npm" {
			return "/bin/npm", nil
		}
		return "", fmt.Errorf("not found")
	}
	codeGraphPnpmGlobalBin = func() (string, error) {
		return "/bin", nil
	}
	codeGraphCLIUsable = func(string) bool { return true }
	piCodeGraphEffectiveMCPProbe = func(string) (PiCodeGraphMCPProbeResult, error) {
		return PiCodeGraphMCPProbeResult{
			AdapterAvailable: true,
			Initialized:      true,
			Tools: []PiCodeGraphMCPTool{{
				Name: "codegraph_explore",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":       map[string]any{"type": "string"},
						"maxFiles":    map[string]any{"type": "integer"},
						"projectPath": map[string]any{"type": "string"},
					},
					"required": []any{"query"},
				},
			}},
		}, nil
	}
	os.Exit(m.Run())
}

func runCodeGraphTestHelper(mode string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		request := scanner.Text()
		if mode == "stall-initialize" || mode == "stall-tools-list" && strings.Contains(request, `"id":2`) {
			time.Sleep(24 * time.Hour)
		}
		response := os.Getenv("GENTLE_AI_CODEGRAPH_TEST_RESPONSE")
		if response == "" && strings.Contains(request, `"id":1`) {
			response = `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{},"serverInfo":{"name":"fake","version":"1"}}}`
		} else if response == "" && strings.Contains(request, `"id":2`) {
			response = `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"codegraph_explore","inputSchema":{"type":"object","properties":{"query":{"type":"string"},"maxFiles":{"type":"integer"},"projectPath":{"type":"string"}},"required":["query"]}}]}}`
		}
		if response != "" {
			fmt.Fprintln(os.Stdout, response)
		}
	}
}

func TestDefinitionsIncludesCodeGraph(t *testing.T) {
	def, ok := DefinitionFor(model.CommunityToolCodeGraph)
	if !ok {
		t.Fatal("CodeGraph definition not found")
	}
	if def.PackageName != "@colbymchenry/codegraph@latest" || def.CommandName != "codegraph" {
		t.Fatalf("CodeGraph definition = %#v", def)
	}
}

func TestCodeGraphCommands(t *testing.T) {
	want := [][]string{
		{"npm", "install", "-g", "@colbymchenry/codegraph@latest"},
		{"codegraph", "install", "--yes"},
	}
	if got := CodeGraphCommands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CodeGraphCommands() = %#v, want %#v", got, want)
	}
	for _, command := range CodeGraphCommands() {
		if strings.Contains(strings.Join(command, " "), "codegraph init") {
			t.Fatalf("CodeGraphCommands() includes project init command: %#v", command)
		}
	}
}

func TestCodeGraphCommandsForDetectorUsesPnpmWhenNpmIsUnavailable(t *testing.T) {
	commands, err := CodeGraphCommandsForDetector(DetectorFunc(func(name string) (string, error) {
		if name == "pnpm" {
			return "/bin/pnpm", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatalf("CodeGraphCommandsForDetector() error = %v", err)
	}
	want := [][]string{
		{"pnpm", "add", "-g", "@colbymchenry/codegraph@latest"},
		{"codegraph", "install", "--yes"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("CodeGraphCommandsForDetector() = %#v, want %#v", commands, want)
	}
}

func TestCodeGraphCommandsForDetectorReportsUnusablePnpmGlobalBin(t *testing.T) {
	previous := codeGraphPnpmGlobalBin
	codeGraphPnpmGlobalBin = func() (string, error) {
		return "", errors.New(`ERROR The configured global bin directory "/Users/example/Library/pnpm" is not in PATH`)
	}
	t.Cleanup(func() { codeGraphPnpmGlobalBin = previous })

	_, err := CodeGraphCommandsForDetector(DetectorFunc(func(name string) (string, error) {
		if name == "pnpm" {
			return "/bin/pnpm", nil
		}
		return "", errors.New("not found")
	}))
	if err == nil {
		t.Fatal("CodeGraphCommandsForDetector() error = nil, want unusable pnpm global bin error")
	}
	for _, want := range []string{"pnpm global installs are not ready", "pnpm setup", "not in PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestCodeGraphCommandsForDetectorPrefersNpmWhenBothExist(t *testing.T) {
	commands, err := CodeGraphCommandsForDetector(DetectorFunc(func(name string) (string, error) {
		if name == "npm" || name == "pnpm" {
			return "/bin/" + name, nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatalf("CodeGraphCommandsForDetector() error = %v", err)
	}
	if got := commands[0]; !reflect.DeepEqual(got, []string{"npm", "install", "-g", "@colbymchenry/codegraph@latest"}) {
		t.Fatalf("CodeGraphCommandsForDetector()[0] = %#v, want npm install", got)
	}
}

func TestCodeGraphCommandsForDetectorFailsWhenNpmAndPnpmAreMissing(t *testing.T) {
	_, err := CodeGraphCommandsForDetector(DetectorFunc(func(string) (string, error) {
		return "", errors.New("not found")
	}))
	if err == nil || !strings.Contains(err.Error(), "npm") || !strings.Contains(err.Error(), "pnpm") {
		t.Fatalf("CodeGraphCommandsForDetector() error = %v, want npm/pnpm requirement", err)
	}
}

func TestInstallUsesPnpmWhenNpmIsUnavailable(t *testing.T) {
	previous := codeGraphPackageLookPath
	codeGraphPackageLookPath = func(name string) (string, error) {
		if name == "pnpm" {
			return "/bin/pnpm", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { codeGraphPackageLookPath = previous })

	var commands []string
	installed := false
	_, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", t.TempDir(), RunnerFunc(func(name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	want := []string{
		"pnpm add -g @colbymchenry/codegraph@latest",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestInstallWithHomeReportsPiChildClassifications(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".pi", "agent", "subagents", "worker.md"), "---\ntools: bash\n---\nwork\n")
	installed := false
	var commands []string
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "", home, RunnerFunc(func(name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.PiCodeGraph == nil || len(result.PiCodeGraph.Children) != 1 || result.PiCodeGraph.Children[0].Classification != PiChildCompatible {
		t.Fatalf("PiCodeGraph classifications = %#v", result.PiCodeGraph)
	}
	if !reflect.DeepEqual(commands, []string{"npm install -g @colbymchenry/codegraph@latest"}) {
		t.Fatalf("Pi-only commands = %#v, want package install without synthetic target", commands)
	}
}

func TestInstallWithHomeReportsWorkspaceChildAndOwnershipTarget(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "workspace")
	target := filepath.Join(workspace, ".pi", "subagents", "worker.md")
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)
	mustWrite(t, target, "---\ntools: bash\n---\nworkspace work\n")

	installed := false
	result, err := InstallWithHome(model.CommunityToolCodeGraph, workspace, home, RunnerFunc(func(string, ...string) error {
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.PiCodeGraph == nil || len(result.PiCodeGraph.Children) != 1 || result.PiCodeGraph.Children[0].Target != target {
		t.Fatalf("workspace Pi result = %#v, want target %q", result.PiCodeGraph, target)
	}
	manifestData, err := os.ReadFile(filepath.Join(home, ".gentle-ai", "pi-codegraph.json"))
	var manifest piCodeGraphManifest
	if err != nil || json.Unmarshal(manifestData, &manifest) != nil {
		t.Fatalf("ownership manifest = %q, err=%v", manifestData, err)
	}
	if _, ok := manifest.Children[target]; !ok {
		t.Fatalf("ownership manifest children = %v, want workspace target %q", manifest.Children, target)
	}
}

func TestInstallWithHomeReportsEffectiveMCPAdapterSchema(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".pi", "agent", "subagents", "worker.md"), "---\ntools: bash\n---\nwork\n")
	installed := false
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "", home, RunnerFunc(func(string, ...string) error {
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.PiCodeGraph == nil || !result.PiCodeGraph.MCP.Adapter || !result.PiCodeGraph.MCP.ReadOnlyExplore {
		t.Fatalf("effective MCP verification = %#v, want adapter and read-only explore schema", result.PiCodeGraph)
	}
}

func TestInstallWithHomeFailsClosedForEmptyPiSettingsWithoutMCPProcess(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".pi", "agent", "subagents", "worker.md"), "---\ntools: bash\n---\nwork\n")
	previous := piCodeGraphEffectiveMCPProbe
	piCodeGraphEffectiveMCPProbe = probePiCodeGraphMCP
	t.Cleanup(func() { piCodeGraphEffectiveMCPProbe = previous })

	installed := false
	_, err := InstallWithHome(model.CommunityToolCodeGraph, "", home, RunnerFunc(func(string, ...string) error {
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err == nil || !strings.Contains(err.Error(), "capability probe") {
		t.Fatalf("InstallWithHome() error = %v, want failed effective MCP capability probe", err)
	}
}

func TestCodeGraphGuidanceContainsLazyInitAndUsageRules(t *testing.T) {
	guidance := CodeGraphGuidanceMarkdown()
	for _, want := range []string{
		"use CodeGraph before broad filesystem searches",
		"hard ordering rule",
		"Create Git worktrees that may need CodeGraph under the user's home directory",
		"<repo-parent>/<repo-name>-worktrees/<worktree-name>",
		"Never place a CodeGraph-dependent worktree under `/tmp`, `/var/tmp`, or `/tmp/opencode`",
		"generic temporary-work guidance does not override this rule",
		"Every worktree needs its own `.codegraph/` index",
		"Never copy, symlink, or reuse another checkout's index",
		"git rev-parse --show-toplevel || pwd",
		"Do not ask the user before initializing CodeGraph in a real project.",
		"Do not initialize CodeGraph in `$HOME`, temporary directories, or non-project folders",
		"<project-root>/.codegraph/",
		"before any broad Read/Glob/Grep filesystem exploration",
		"immediately run `gentle-ai codegraph init --cwd <project-root>`",
		"gentle-ai codegraph init --cwd <project-root>",
		"codegraph_explore",
		"call paths, and blast-radius context",
		"invoke the upstream CLI directly",
		"`codegraph status`",
		"`codegraph query`",
		"`codegraph explore`",
		"`codegraph node`",
		"`codegraph files`",
		"`codegraph callers`",
		"`codegraph callees`",
		"`codegraph impact`",
		"`codegraph affected`",
		"Do not use `gentle-ai codegraph` as a general proxy",
		"Never run or recommend destructive or administrative lifecycle commands",
		"`codegraph uninit`",
		"`codegraph install`",
		"`codegraph uninstall`",
		"`codegraph upgrade`",
		"Reserve `codegraph index` for explicit index-corruption recovery, never routine use",
		"Missing .codegraph/ is the trigger to initialize, not a reason to skip CodeGraph.",
		"Do not fall back just because `.codegraph/` is missing",
		"missing index is the trigger to lazy-initialize",
		"read-only upstream CLI commands when MCP tools are absent",
		"rely on watcher auto-sync by default",
		"Run `codegraph sync` only when the watcher is disabled or CodeGraph reports stale files",
		"Only fall back to normal filesystem tools after CodeGraph initialization or use fails",
		"Broad Read/Glob/Grep exploration before this CodeGraph check is explicitly discouraged",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("CodeGraphGuidanceMarkdown() missing %q:\n%s", want, guidance)
		}
	}
}

func TestCodeGraphGuidanceInjectsForRepresentativeAgents(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"agent":{"worker":{"prompt":"use codegraph_explore"}}}`)
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".codex", "config.toml"), `[mcp_servers.codegraph]`)
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)

	installed := false
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		installed = true
		mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	if result.StatusAfter == nil {
		t.Fatal("StatusAfter = nil")
	}

	for _, path := range []string{
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".codex", "AGENTS.md"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		text := string(content)
		if !strings.Contains(text, "<!-- gentle-ai:codegraph-guidance -->") || !strings.Contains(text, "gentle-ai codegraph init --cwd <project-root>") {
			t.Fatalf("%q missing CodeGraph guidance:\n%s", path, text)
		}
	}
	for _, path := range CodeGraphGuidancePaths(home) {
		if strings.Contains(path, "node_modules") || strings.Contains(path, string(filepath.Separator)+"agents"+string(filepath.Separator)) || strings.Contains(path, string(filepath.Separator)+"chains"+string(filepath.Separator)) {
			t.Fatalf("guidance mutated forbidden package path %q", path)
		}
	}
}

func TestCodeGraphGuidanceInjectRemovesLegacySkipBlock(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, agentsPath, strings.Join([]string{
		"custom notes",
		"<!-- CODEGRAPH_START -->",
		"## CodeGraph",
		"If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.",
		"<!-- CODEGRAPH_END -->",
		"more notes",
	}, "\n"))

	result, err := InjectCodeGraphGuidanceIfSelected(home, []model.CommunityToolID{model.CommunityToolCodeGraph})
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidanceIfSelected() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}

	body, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", agentsPath, err)
	}
	text := string(body)
	for _, stale := range []string{"<!-- CODEGRAPH_START -->", "<!-- CODEGRAPH_END -->", "skip CodeGraph entirely"} {
		if strings.Contains(text, stale) {
			t.Fatalf("legacy CodeGraph guidance %q was not removed:\n%s", stale, text)
		}
	}
	for _, want := range []string{"custom notes", "more notes", "<!-- gentle-ai:codegraph-guidance -->", "gentle-ai codegraph init --cwd <project-root>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated guidance missing %q:\n%s", want, text)
		}
	}
}

func TestCodeGraphGuidanceInjectRemovesUnmarkedUpstreamDuplicateBlock(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, agentsPath, strings.Join([]string{
		"custom notes",
		"",
		"## CodeGraph",
		"",
		"In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:",
		"",
		"- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow.",
		"- **Shell** (always works): `codegraph explore \"<symbol names or question>\"` prints the same output.",
		"",
		"If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.",
		"",
		"## CodeGraph manual notes",
		"This manual section is unrelated and must stay.",
		"",
		"more notes",
	}, "\n"))

	result, err := InjectCodeGraphGuidanceIfSelected(home, []model.CommunityToolID{model.CommunityToolCodeGraph})
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidanceIfSelected() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}

	body, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", agentsPath, err)
	}
	text := string(body)
	for _, stale := range []string{"BEFORE grep/find or reading files", "skip CodeGraph entirely", "`codegraph explore \"<symbol names or question>\"`"} {
		if strings.Contains(text, stale) {
			t.Fatalf("unmarked upstream CodeGraph guidance %q was not removed:\n%s", stale, text)
		}
	}
	for _, want := range []string{"custom notes", "## CodeGraph manual notes", "This manual section is unrelated and must stay.", "more notes", "<!-- gentle-ai:codegraph-guidance -->", "gentle-ai codegraph init --cwd <project-root>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated guidance missing %q:\n%s", want, text)
		}
	}
}

func TestCodeGraphGuidanceInjectPreservesManualNotesInsideUnmarkedCodeGraphSection(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, agentsPath, strings.Join([]string{
		"custom notes",
		"",
		"## CodeGraph",
		"",
		"In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:",
		"",
		"- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow.",
		"- **Shell** (always works): `codegraph explore \"<symbol names or question>\"` prints the same output.",
		"",
		"If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.",
		"",
		"Manual note: keep CodeGraph indexes outside throwaway directories.",
		"Manual note: rerun `codegraph sync` after large refactors.",
		"",
		"more notes",
	}, "\n"))

	result, err := InjectCodeGraphGuidanceIfSelected(home, []model.CommunityToolID{model.CommunityToolCodeGraph})
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidanceIfSelected() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}

	body, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", agentsPath, err)
	}
	text := string(body)
	for _, stale := range []string{"BEFORE grep/find or reading files", "skip CodeGraph entirely", "`codegraph explore \"<symbol names or question>\"`"} {
		if strings.Contains(text, stale) {
			t.Fatalf("unmarked upstream CodeGraph guidance %q was not removed:\n%s", stale, text)
		}
	}
	for _, want := range []string{"custom notes", "Manual note: keep CodeGraph indexes outside throwaway directories.", "Manual note: rerun `codegraph sync` after large refactors.", "more notes", "<!-- gentle-ai:codegraph-guidance -->", "gentle-ai codegraph init --cwd <project-root>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated guidance missing %q:\n%s", want, text)
		}
	}
}

func TestCodeGraphGuidanceInjectPreservesManualNoteBoundaryBeforeNextHeading(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, agentsPath, strings.Join([]string{
		"custom notes",
		"",
		"## CodeGraph",
		"",
		"In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:",
		"",
		"- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow.",
		"- **Shell** (always works): `codegraph explore \"<symbol names or question>\"` prints the same output.",
		"",
		"If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.",
		"",
		"Manual note: preserve this boundary.",
		"## Next Heading",
		"This section must remain separate.",
	}, "\n"))

	result, err := InjectCodeGraphGuidanceIfSelected(home, []model.CommunityToolID{model.CommunityToolCodeGraph})
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidanceIfSelected() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}

	body, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", agentsPath, err)
	}
	text := string(body)
	for _, stale := range []string{"BEFORE grep/find or reading files", "skip CodeGraph entirely", "`codegraph explore \"<symbol names or question>\"`"} {
		if strings.Contains(text, stale) {
			t.Fatalf("unmarked upstream CodeGraph guidance %q was not removed:\n%s", stale, text)
		}
	}
	if !strings.Contains(text, "Manual note: preserve this boundary.\n## Next Heading") {
		t.Fatalf("manual note was not separated from the next heading by exactly one newline:\n%s", text)
	}
	for _, broken := range []string{"Manual note: preserve this boundary.## Next Heading", "Manual note: preserve this boundary.\n\n## Next Heading"} {
		if strings.Contains(text, broken) {
			t.Fatalf("manual note boundary contains invalid separator %q:\n%s", broken, text)
		}
	}
	for _, want := range []string{"custom notes", "This section must remain separate.", "<!-- gentle-ai:codegraph-guidance -->", "gentle-ai codegraph init --cwd <project-root>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated guidance missing %q:\n%s", want, text)
		}
	}
}

func TestCodeGraphGuidanceInjectPreservesManualNotesBeforeUnmarkedUpstreamDuplicateBlock(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, agentsPath, strings.Join([]string{
		"custom notes",
		"",
		"## CodeGraph",
		"",
		"Manual note: always inspect the project root before using generated indexes.",
		"Manual note: never initialize CodeGraph in scratch directories.",
		"",
		"In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:",
		"",
		"- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow.",
		"- **Shell** (always works): `codegraph explore \"<symbol names or question>\"` prints the same output.",
		"",
		"If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.",
		"",
		"more notes",
	}, "\n"))

	result, err := InjectCodeGraphGuidanceIfSelected(home, []model.CommunityToolID{model.CommunityToolCodeGraph})
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidanceIfSelected() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}

	body, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", agentsPath, err)
	}
	text := string(body)
	for _, stale := range []string{"BEFORE grep/find or reading files", "skip CodeGraph entirely", "`codegraph explore \"<symbol names or question>\"`"} {
		if strings.Contains(text, stale) {
			t.Fatalf("unmarked upstream CodeGraph guidance %q was not removed:\n%s", stale, text)
		}
	}
	for _, want := range []string{"custom notes", "Manual note: always inspect the project root before using generated indexes.", "Manual note: never initialize CodeGraph in scratch directories.", "more notes", "<!-- gentle-ai:codegraph-guidance -->", "gentle-ai codegraph init --cwd <project-root>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated guidance missing %q:\n%s", want, text)
		}
	}
}

func TestCodeGraphGuidanceInjectPreservesManualNotesInterleavedWithUnmarkedUpstreamDuplicateBlock(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, agentsPath, strings.Join([]string{
		"custom notes",
		"",
		"## CodeGraph",
		"",
		"In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:",
		"Manual note: prefer the MCP tool when it returns exact source.",
		"",
		"- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow.",
		"Manual note: shell fallback is okay after CodeGraph initialization fails.",
		"- **Shell** (always works): `codegraph explore \"<symbol names or question>\"` prints the same output.",
		"",
		"If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.",
		"",
		"more notes",
	}, "\n"))

	result, err := InjectCodeGraphGuidanceIfSelected(home, []model.CommunityToolID{model.CommunityToolCodeGraph})
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidanceIfSelected() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}

	body, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", agentsPath, err)
	}
	text := string(body)
	for _, stale := range []string{"BEFORE grep/find or reading files", "skip CodeGraph entirely", "`codegraph explore \"<symbol names or question>\"`"} {
		if strings.Contains(text, stale) {
			t.Fatalf("unmarked upstream CodeGraph guidance %q was not removed:\n%s", stale, text)
		}
	}
	for _, want := range []string{"custom notes", "Manual note: prefer the MCP tool when it returns exact source.", "Manual note: shell fallback is okay after CodeGraph initialization fails.", "more notes", "<!-- gentle-ai:codegraph-guidance -->", "gentle-ai codegraph init --cwd <project-root>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated guidance missing %q:\n%s", want, text)
		}
	}
}

func TestUnselectedCodeGraphDoesNotInjectGuidance(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)

	result, err := InjectCodeGraphGuidanceIfSelected(home, nil)
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidanceIfSelected() error = %v", err)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("result = %#v, want no-op for unselected CodeGraph", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("OpenCode AGENTS.md should not exist when CodeGraph is unselected; stat err = %v", err)
	}
}

func TestInstallRunsCommandsAndReturnsLazyProjectIndexManualAction(t *testing.T) {
	var ran []string
	installed := false
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", t.TempDir(), RunnerFunc(func(name string, args ...string) error {
		ran = append(ran, append([]string{name}, args...)...)
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(ran) == 0 {
		t.Fatal("expected commands to run")
	}
	if len(result.ManualActions) != 1 {
		t.Fatalf("ManualActions = %#v, want one lazy project index instruction", result.ManualActions)
	}
	action := result.ManualActions[0]
	for _, want := range []string{"CodeGraph CLI was installed", "agents were connected", "Project indexes will be created automatically"} {
		if !strings.Contains(action, want) {
			t.Fatalf("ManualActions[0] = %q, want %q", action, want)
		}
	}
	if strings.Contains(action, "codegraph init") {
		t.Fatalf("ManualActions[0] = %q, should not instruct immediate project init", action)
	}
}

func TestInstallLeavesPiPendingWhenAdapterHealthIsNotMachineVerifiable(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".pi", "agent", "npm", "node_modules", "pi-mcp-adapter", "index.ts"), "export default {}\n")
	previousProbe := piCodeGraphEffectiveMCPProbe
	piCodeGraphEffectiveMCPProbe = func(path string) (PiCodeGraphMCPProbeResult, error) {
		result, _ := piProbeForTest(path)
		return result, ErrPiCodeGraphAdapterHealthUnavailable
	}
	t.Cleanup(func() {
		piCodeGraphEffectiveMCPProbe = previousProbe
	})

	installed := false
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "", home, RunnerFunc(func(string, ...string) error {
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	if result.PiCodeGraph == nil || len(result.PiCodeGraph.ManualActions) != 1 {
		t.Fatalf("Pi result = %#v, want pending guidance", result.PiCodeGraph)
	}
	if !strings.Contains(result.PiCodeGraph.ManualActions[0], "pending") {
		t.Fatalf("Pi manual action = %q, want pending state", result.PiCodeGraph.ManualActions[0])
	}
	pi := findAgentStatus(t, *result.StatusAfter, model.AgentPi)
	if pi.Configured || pi.Status != AgentStatusMissing {
		t.Fatalf("Pi status = %#v, want unconfigured pending state", pi)
	}
}

func TestDetectStatusReportsCLIAndPerAgentWiring(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
	mustWrite(t, filepath.Join(home, ".claude", "CLAUDE.md"), strings.Join([]string{
		"existing Claude guidance",
		"<!-- gentle-ai:codegraph-guidance -->",
		"CodeGraph guidance with `gentle-ai codegraph init --cwd <project-root>`",
		"<!-- /gentle-ai:codegraph-guidance -->",
	}, "\n"))
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), "<!-- gentle-ai:codegraph-guidance -->\nmanaged\n<!-- /gentle-ai:codegraph-guidance -->\n")

	status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(name string) (string, error) {
		if name != "codegraph" {
			t.Fatalf("LookPath(%q), want codegraph", name)
		}
		return "/bin/codegraph", nil
	}))

	if status.CLI != AvailabilityAvailable || status.CLIPath != "/bin/codegraph" {
		t.Fatalf("CLI = (%s, %q), want available /bin/codegraph", status.CLI, status.CLIPath)
	}
	claude := findAgentStatus(t, status, model.AgentClaudeCode)
	if !claude.Detected || !claude.Configured || claude.Status != AgentStatusConfigured {
		t.Fatalf("claude status = %#v, want detected configured", claude)
	}
	opencode := findAgentStatus(t, status, model.AgentOpenCode)
	if !opencode.Detected || opencode.Configured || opencode.Status != AgentStatusMissing {
		t.Fatalf("opencode status = %#v, want marker-only agent reported missing", opencode)
	}
	if !strings.Contains(opencode.Reason, "no effective MCP") {
		t.Fatalf("opencode reason = %q, want missing effective MCP wiring", opencode.Reason)
	}
}

func TestDetectStatusRecognizesOpenCodeJSONCWiring(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.jsonc"), `{
  // user comment
  "mcp": {"codegraph": {"type": "local", "command": ["codegraph", "serve", "--mcp"], "enabled": true,},},
}`)
	mustWrite(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), "<!-- gentle-ai:codegraph-guidance -->\nmanaged\n<!-- /gentle-ai:codegraph-guidance -->\n")

	status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(string) (string, error) {
		return "/bin/codegraph", nil
	}))
	opencode := findAgentStatus(t, status, model.AgentOpenCode)
	if !opencode.Detected || !opencode.Configured || opencode.Status != AgentStatusConfigured {
		t.Fatalf("opencode status = %#v, want JSONC wiring configured", opencode)
	}
}

func TestDetectStatusRejectsDisabledOpenCodeWiring(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":false}}}`)
	mustWrite(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), "<!-- gentle-ai:codegraph-guidance -->\nmanaged\n<!-- /gentle-ai:codegraph-guidance -->\n")

	status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(string) (string, error) {
		return "/bin/codegraph", nil
	}))
	opencode := findAgentStatus(t, status, model.AgentOpenCode)
	if opencode.Configured || opencode.Status != AgentStatusMissing {
		t.Fatalf("opencode status = %#v, want disabled MCP reported missing", opencode)
	}
}

func TestCodeGraphEffectiveWiringCapabilityIsOptional(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	var openCodeAdapter agents.Adapter = opencode.NewAdapter()
	if _, ok := openCodeAdapter.(agents.EffectiveCodeGraphWiringDetector); !ok {
		t.Fatal("OpenCode adapter does not expose effective CodeGraph wiring detection")
	}

	var claudeAdapter agents.Adapter = claude.NewAdapter()
	if _, ok := claudeAdapter.(agents.EffectiveCodeGraphWiringDetector); ok {
		t.Fatal("Claude adapter unexpectedly exposes OpenCode-specific wiring detection")
	}
	path := filepath.Join(home, ".claude.json")
	mustWrite(t, path, `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
	if gotPath, configured := hasCodeGraphToolWiring(home, claudeAdapter); !configured || gotPath != path {
		t.Fatalf("Claude global detection = (%q, %v), want (%q, true)", gotPath, configured, path)
	}
}

func TestReconcileOpenCodeCodeGraphUsesUpstreamInstaller(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	mustWrite(t, settingsPath, `{"mcp":{"user":{"type":"remote","url":"https://example.com"}}}`)

	var command string
	result, err := ReconcileOpenCodeCodeGraph(home, RunnerFunc(func(name string, args ...string) error {
		command = strings.Join(append([]string{name}, args...), " ")
		mustWrite(t, settingsPath, `{"mcp":{"user":{"type":"remote","url":"https://example.com"},"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
		return nil
	}))
	if err != nil {
		t.Fatalf("ReconcileOpenCodeCodeGraph() error = %v", err)
	}
	if command != "codegraph install --target opencode --location global --yes" {
		t.Fatalf("command = %q", command)
	}
	if !result.Changed || !reflect.DeepEqual(result.Files, []string{settingsPath}) {
		t.Fatalf("result = %#v, want changed OpenCode settings", result)
	}
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"user"`) || !strings.Contains(string(content), `"codegraph"`) {
		t.Fatalf("OpenCode settings lost user or CodeGraph MCP entry: %s", content)
	}
}

func TestReconcileOpenCodeCodeGraphPreservesJSONCUserContent(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	mustWrite(t, settingsPath, "{\n  // keep this comment\n  \"mcp\": {\"user\": {\"type\": \"remote\", \"url\": \"https://example.com\"},},\n}\n")

	result, err := ReconcileOpenCodeCodeGraph(home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, settingsPath, "{\n  // keep this comment\n  \"mcp\": {\"user\": {\"type\": \"remote\", \"url\": \"https://example.com\"}, \"codegraph\": {\"type\": \"local\", \"command\": [\"codegraph\", \"serve\", \"--mcp\"], \"enabled\": true},},\n}\n")
		return nil
	}))
	if err != nil {
		t.Fatalf("ReconcileOpenCodeCodeGraph() error = %v", err)
	}
	if !result.Changed || !reflect.DeepEqual(result.Files, []string{settingsPath}) {
		t.Fatalf("result = %#v, want changed JSONC settings", result)
	}
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// keep this comment", `"user"`, `"codegraph"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("JSONC content missing %q after reconcile: %s", want, content)
		}
	}
}

func TestReconcileOpenCodeCodeGraphUsesXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "custom-config")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	settingsPath := filepath.Join(xdg, "opencode", "opencode.json")
	mustWrite(t, settingsPath, `{}`)

	result, err := ReconcileOpenCodeCodeGraph(home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, settingsPath, `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
		return nil
	}))
	if err != nil {
		t.Fatalf("ReconcileOpenCodeCodeGraph() error = %v", err)
	}
	if !result.Changed || !reflect.DeepEqual(result.Files, []string{settingsPath}) {
		t.Fatalf("result = %#v, want XDG OpenCode settings", result)
	}
}

func TestInstallUsesUnifiedAntigravityConfigWithoutMigratedMarker(t *testing.T) {
	home := t.TempDir()
	unifiedPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	legacyPath := filepath.Join(home, ".gemini", "antigravity", "mcp_config.json")
	absoluteCodeGraph := filepath.Join(home, "bin", "codegraph")
	mustWrite(t, unifiedPath, `{"mcpServers":{"user":{"command":"other"}}}`)
	mustWrite(t, filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"), `{}`)

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(name string, args ...string) error {
		command := strings.Join(append([]string{name}, args...), " ")
		if command != "codegraph install --target antigravity,gemini --location global --yes" {
			t.Fatalf("command = %q, want targeted Antigravity install", command)
		}
		mustWrite(t, unifiedPath, fmt.Sprintf(`{"mcpServers":{"user":{"command":"other"},"codegraph":{"command":%q,"args":["serve","--mcp"]}}}`, absoluteCodeGraph))
		mustWrite(t, filepath.Join(home, ".gemini", "settings.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	if !reflect.DeepEqual(result.CommandsRun, []string{"codegraph install --target antigravity,gemini --location global --yes"}) {
		t.Fatalf("CommandsRun = %#v, want targeted Antigravity install", result.CommandsRun)
	}
	antigravity := findAgentStatus(t, *result.StatusAfter, model.AgentAntigravity)
	if !antigravity.Detected || !antigravity.Configured {
		t.Fatalf("Antigravity status = %#v, want detected and configured", antigravity)
	}
	content, readErr := os.ReadFile(unifiedPath)
	if readErr != nil || !strings.Contains(string(content), `"user"`) || !strings.Contains(string(content), `"codegraph"`) {
		t.Fatalf("unified config = %q, error = %v; want preserved user and CodeGraph entries", content, readErr)
	}
	if _, statErr := os.Stat(legacyPath); !os.IsNotExist(statErr) {
		t.Fatalf("legacy config should not be created, stat error = %v", statErr)
	}
}

func TestInstallRepairsRelativeAntigravityCommand(t *testing.T) {
	home := t.TempDir()
	unifiedPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	mustWrite(t, unifiedPath, `{"mcpServers":{"user":{"command":"other"},"codegraph":{"command":"./codegraph","args":["serve","--mcp"]}}}`)
	mustWrite(t, filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".gemini", "settings.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(name string, args ...string) error {
		command := strings.Join(append([]string{name}, args...), " ")
		if command != "codegraph install --target antigravity,gemini --location global --yes" {
			t.Fatalf("command = %q, want targeted Antigravity repair", command)
		}
		mustWrite(t, unifiedPath, `{"mcpServers":{"user":{"command":"other"},"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		mustWrite(t, filepath.Join(home, ".gemini", "settings.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	before := findAgentStatus(t, *result.StatusBefore, model.AgentAntigravity)
	if !before.Detected || before.Configured || before.Status != AgentStatusMissing {
		t.Fatalf("Antigravity status before repair = %#v, want detected and missing", before)
	}
	wantCommands := []string{"codegraph install --target antigravity,gemini --location global --yes"}
	if !reflect.DeepEqual(result.CommandsRun, wantCommands) {
		t.Fatalf("CommandsRun = %#v, want repair command %#v", result.CommandsRun, wantCommands)
	}
	after := findAgentStatus(t, *result.StatusAfter, model.AgentAntigravity)
	if !after.Detected || !after.Configured {
		t.Fatalf("Antigravity status after repair = %#v, want detected and configured", after)
	}
}

func TestInstallRecordsTargetedOpenCodeReconciliation(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	mustWrite(t, settingsPath, `{}`)
	mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
	mustWrite(t, filepath.Join(home, ".claude", "CLAUDE.md"), "<!-- gentle-ai:codegraph-guidance -->\nmanaged\n<!-- /gentle-ai:codegraph-guidance -->\n")

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, settingsPath, `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	want := []string{"codegraph install --target opencode --location global --yes"}
	if !reflect.DeepEqual(result.CommandsRun, want) {
		t.Fatalf("CommandsRun = %#v, want %#v", result.CommandsRun, want)
	}
}

func TestInstallRunsFullReconcileWhenAnotherAgentIsMissing(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	mustWrite(t, settingsPath, `{}`)
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{}`)

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, settingsPath, `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
		mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	if !reflect.DeepEqual(result.CommandsRun, []string{"codegraph install --target claude --location global --yes"}) {
		t.Fatalf("CommandsRun = %#v, want full reconciliation", result.CommandsRun)
	}
}

func TestDetectStatusReportsCodexMissingWhenConfigHasCodeGraphButGuidanceIsMissing(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".codex", "config.toml"), strings.Join([]string{
		`[mcp_servers.codegraph]`,
		`command = "codegraph"`,
	}, "\n"))

	status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(name string) (string, error) {
		if name != "codegraph" {
			t.Fatalf("LookPath(%q), want codegraph", name)
		}
		return "/bin/codegraph", nil
	}))

	codex := findAgentStatus(t, status, model.AgentCodex)
	if !codex.Detected || codex.Configured || codex.Status != AgentStatusMissing {
		t.Fatalf("codex status = %#v, want detected missing until AGENTS.md has CodeGraph guidance", codex)
	}
	wantPath := filepath.Join(home, ".codex", "AGENTS.md")
	if codex.Path != wantPath {
		t.Fatalf("codex path = %q, want guidance path %q", codex.Path, wantPath)
	}
}

func TestDetectStatusReportsMissingCLIThroughMock(t *testing.T) {
	status := DetectStatus(model.CommunityToolCodeGraph, t.TempDir(), DetectorFunc(func(string) (string, error) {
		return "", errors.New("not found")
	}))
	if status.CLI != AvailabilityMissing {
		t.Fatalf("CLI = %s, want missing", status.CLI)
	}
}

func TestDetectStatusReportsPiRuntimeMissingWhenAppendSystemHasNoMarker(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)

	status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(string) (string, error) {
		return "/bin/codegraph", nil
	}))
	pi := findAgentStatus(t, status, model.AgentPi)
	if !pi.Detected || pi.Configured || pi.Status != AgentStatusMissing {
		t.Fatalf("Pi status = %#v, want detected missing", pi)
	}
	if pi.Path != filepath.Join(home, ".gentle-ai", "pi-codegraph.json") {
		t.Fatalf("Pi path = %q, want ownership manifest path", pi.Path)
	}
}

func TestDetectStatusRejectsPiParentMarkerAsCapabilityEvidence(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".pi", "agent", "APPEND_SYSTEM.md"), strings.Join([]string{
		"existing Pi guidance",
		"<!-- gentle-ai:codegraph-guidance -->",
		"CodeGraph guidance with `gentle-ai codegraph init --cwd <project-root>`",
		"<!-- /gentle-ai:codegraph-guidance -->",
	}, "\n"))

	status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(string) (string, error) {
		return "/bin/codegraph", nil
	}))
	pi := findAgentStatus(t, status, model.AgentPi)
	if !pi.Detected || pi.Configured || pi.Status != AgentStatusMissing {
		t.Fatalf("Pi status = %#v, want marker-only Pi missing", pi)
	}
}

func TestDetectStatusReportsPiChildClassifications(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)
	mustWrite(t, filepath.Join(home, ".pi", "agent", "subagents", "worker.md"), "---\ntools: bash\n---\nwork\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true}); err != nil {
		t.Fatal(err)
	}
	status := DetectStatus(model.CommunityToolCodeGraph, home, DetectorFunc(func(string) (string, error) { return "/bin/codegraph", nil }))
	pi := findAgentStatus(t, status, model.AgentPi)
	if len(pi.Children) != 1 || pi.Children[0].Classification != PiChildCompatible {
		t.Fatalf("Pi classifications = %#v", pi.Children)
	}
}

func TestInjectCodeGraphGuidanceDoesNotUsePiParentMarker(t *testing.T) {
	home := t.TempDir()
	appendSystemPath := filepath.Join(home, ".pi", "agent", "APPEND_SYSTEM.md")
	mustWrite(t, appendSystemPath, "existing Pi instructions\n")

	result, err := InjectCodeGraphGuidance(home)
	if err != nil {
		t.Fatalf("InjectCodeGraphGuidance() error = %v", err)
	}
	if result.Changed {
		t.Fatalf("InjectCodeGraphGuidance() Changed = true, want Pi parent no-op")
	}

	content, err := os.ReadFile(appendSystemPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", appendSystemPath, err)
	}
	text := string(content)
	if text != "existing Pi instructions\n" {
		t.Fatalf("APPEND_SYSTEM.md changed:\n%s", text)
	}
	for _, path := range result.Files {
		if path != appendSystemPath {
			t.Fatalf("InjectCodeGraphGuidance() file = %q, want only %q", path, appendSystemPath)
		}
		if strings.Contains(path, "node_modules") || strings.Contains(path, string(filepath.Separator)+"agents"+string(filepath.Separator)) || strings.Contains(path, string(filepath.Separator)+"chains"+string(filepath.Separator)) {
			t.Fatalf("guidance mutated forbidden package path %q", path)
		}
	}
}

func TestInstallFailsWhenPostInstallContractStillMissing(t *testing.T) {
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", t.TempDir(), RunnerFunc(func(string, ...string) error {
		return nil
	}), DetectorFunc(func(string) (string, error) {
		return "", errors.New("not found")
	}))
	if err == nil || !strings.Contains(err.Error(), "CLI available") {
		t.Fatalf("InstallWithHome() error = %v, want missing CLI validation", err)
	}
	if result.StatusAfter == nil {
		t.Fatal("StatusAfter = nil, want partial result context")
	}
}

func TestValidateCodeGraphInstallStatusFailsForDetectedMissingAgent(t *testing.T) {
	status := Status{
		Tool: model.CommunityToolCodeGraph,
		CLI:  AvailabilityAvailable,
		Agents: []AgentStatus{
			{Agent: model.AgentOpenCode, Name: "OpenCode", Detected: true, Configured: false},
		},
	}
	err := validateCodeGraphInstallStatus(status)
	if err == nil || !strings.Contains(err.Error(), "OpenCode") {
		t.Fatalf("validateCodeGraphInstallStatus() error = %v, want missing OpenCode", err)
	}
}

func TestInstallSkipsWhenCodeGraphAlreadyReconciled(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)

	calls := 0
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		calls++
		return nil
	}), DetectorFunc(func(string) (string, error) {
		return "/bin/codegraph", nil
	}))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("runner calls = %d, want 0 for already reconciled install", calls)
	}
	if result.StatusAfter == nil || !result.StatusAfter.CodeGraphReconcileSatisfied() {
		t.Fatalf("StatusAfter = %#v, want reconciled", result.StatusAfter)
	}
}

func TestInstallRefreshesOldCodeGraphGuidanceMarker(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	mustWrite(t, agentsPath, strings.Join([]string{
		"user content",
		"<!-- gentle-ai:codegraph-guidance -->",
		"old CodeGraph prompt",
		"<!-- /gentle-ai:codegraph-guidance -->",
	}, "\n"))

	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
		mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`)
		return nil
	}), DetectorFunc(func(string) (string, error) {
		return "/bin/codegraph", nil
	}))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	if !reflect.DeepEqual(result.CommandsRun, []string{"codegraph install --target opencode --location global --yes"}) {
		t.Fatalf("CommandsRun = %#v, want MCP reconciliation only", result.CommandsRun)
	}

	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", agentsPath, err)
	}
	text := string(content)
	if strings.Contains(text, "old CodeGraph prompt") {
		t.Fatalf("old guidance was not replaced:\n%s", text)
	}
	if !strings.Contains(text, "immediately run `gentle-ai codegraph init --cwd <project-root>`") || !strings.Contains(text, "user content") {
		t.Fatalf("latest guidance/user content missing after refresh:\n%s", text)
	}
}

func TestInstallRepairsMissingCLIWhenAgentMarkerExists(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"codegraph":{"command":"codegraph"}}}`)

	var commands []string
	installed := false
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		installed = true
		return nil
	}), DetectorFunc(func(string) (string, error) {
		if installed {
			return "/bin/codegraph", nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatalf("InstallWithHome() error = %v", err)
	}
	want := []string{
		"npm install -g @colbymchenry/codegraph@latest",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
	if result.StatusBefore == nil || result.StatusBefore.CLI != AvailabilityMissing {
		t.Fatalf("StatusBefore = %#v, want missing CLI", result.StatusBefore)
	}
}

func TestDetectStatusRejectsWSLWindowsNPMShimWithoutExecutingIt(t *testing.T) {
	useCodeGraphWSLAdmission(t, true)
	previousVersion := codeGraphInstalledVersion
	t.Cleanup(func() { codeGraphInstalledVersion = previousVersion })
	codeGraphInstalledVersion = func(string) (string, bool) {
		t.Fatal("DetectStatus must not execute a candidate CodeGraph CLI")
		return "", false
	}

	status := DetectStatus(model.CommunityToolCodeGraph, t.TempDir(), DetectorFunc(func(string) (string, error) {
		return "/mnt/c/Users/alan/AppData/Roaming/npm/codegraph", nil
	}))
	if status.CLI != AvailabilityMissing || status.CLIPath != "" {
		t.Fatalf("DetectStatus() = %#v, want WSL Windows npm shim rejected", status)
	}
}

func TestDetectStatusAdmitsLinuxCLIPathsWithoutRequiringFiles(t *testing.T) {
	useCodeGraphWSLAdmission(t, true)
	previousVersion := codeGraphInstalledVersion
	t.Cleanup(func() { codeGraphInstalledVersion = previousVersion })
	codeGraphInstalledVersion = func(string) (string, bool) {
		t.Fatal("DetectStatus must not execute a candidate CodeGraph CLI")
		return "", false
	}

	for _, path := range []string{
		"/bin/codegraph",
		"/home/alan/.local/bin/codegraph",
		"/home/alan/AppData/Roaming/npm/codegraph",
	} {
		t.Run(path, func(t *testing.T) {
			status := DetectStatus(model.CommunityToolCodeGraph, t.TempDir(), DetectorFunc(func(string) (string, error) {
				return path, nil
			}))
			if status.CLI != AvailabilityAvailable || status.CLIPath != path {
				t.Fatalf("DetectStatus() = %#v, want injected Linux path available", status)
			}
		})
	}
}

func TestInstallRejectsWSLWindowsNPMShimAfterPackageInstall(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".claude.json"), `{}`)
	shim := "/mnt/c/Users/alan/AppData/Roaming/npm/codegraph"

	previousPackageLookPath := codeGraphPackageLookPath
	t.Cleanup(func() { codeGraphPackageLookPath = previousPackageLookPath })
	codeGraphPackageLookPath = func(name string) (string, error) {
		if name == "npm" {
			return "/bin/npm", nil
		}
		return "", errors.New("not found")
	}
	useCodeGraphWSLAdmission(t, true)
	previousVersion := codeGraphInstalledVersion
	t.Cleanup(func() { codeGraphInstalledVersion = previousVersion })
	codeGraphInstalledVersion = func(string) (string, bool) {
		t.Fatal("CodeGraph admission must not execute the stale shim")
		return "", false
	}

	var runnerCalls []string
	result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(name string, args ...string) error {
		runnerCalls = append(runnerCalls, strings.Join(append([]string{name}, args...), " "))
		if name != "npm" {
			return errors.New("CodeGraph runner must not receive a rejected shim")
		}
		return nil
	}), DetectorFunc(func(string) (string, error) {
		return shim, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "did not leave a runnable codegraph CLI available") {
		t.Fatalf("InstallWithHome() error = %v, want unavailable CodeGraph outcome after package install", err)
	}
	if !reflect.DeepEqual(runnerCalls, []string{"npm install -g @colbymchenry/codegraph@latest"}) {
		t.Fatalf("runner calls = %#v, want package install before stale-shim re-admission blocks CodeGraph", runnerCalls)
	}
	if result.StatusBefore == nil || result.StatusBefore.CLI != AvailabilityMissing || result.StatusBefore.CLIPath != "" {
		t.Fatalf("StatusBefore = %#v, want truthful absent CLI", result.StatusBefore)
	}
	if result.StatusAfter == nil || result.StatusAfter.CLI != AvailabilityMissing || result.StatusAfter.CLIPath != "" {
		t.Fatalf("StatusAfter = %#v, want rejected shim to remain unavailable", result.StatusAfter)
	}
}

func TestInstallFailurePaths(t *testing.T) {
	t.Run("nil runner", func(t *testing.T) {
		result, err := Install(model.CommunityToolCodeGraph, "/work/project", nil)
		if err == nil {
			t.Fatal("Install() error = nil, want configured runner error")
		}
		if result.Tool != "" || len(result.CommandsRun) != 0 {
			t.Fatalf("result = %#v, want empty result", result)
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		result, err := Install(model.CommunityToolID("missing-tool"), "/work/project", RunnerFunc(func(string, ...string) error { return nil }))
		if err == nil || !strings.Contains(err.Error(), "unknown community tool") {
			t.Fatalf("Install() error = %v, want unknown tool error", err)
		}
		if result.Tool != "" || len(result.CommandsRun) != 0 {
			t.Fatalf("result = %#v, want empty result", result)
		}
	})

	t.Run("command runner failure preserves attempted command", func(t *testing.T) {
		boom := errors.New("npm failed")
		result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", t.TempDir(), RunnerFunc(func(string, ...string) error { return boom }), DetectorFunc(func(string) (string, error) {
			return "", errors.New("not found")
		}))
		if !errors.Is(err, boom) {
			t.Fatalf("Install() error = %v, want wrapped runner error", err)
		}
		if result.Tool != model.CommunityToolCodeGraph {
			t.Fatalf("result tool = %q, want CodeGraph", result.Tool)
		}
		if len(result.CommandsRun) != 1 || !strings.Contains(result.CommandsRun[0], "npm install -g @colbymchenry/codegraph@latest") {
			t.Fatalf("CommandsRun = %#v, want failed CLI install command", result.CommandsRun)
		}
	})

	t.Run("agent wiring failure preserves attempted commands", func(t *testing.T) {
		boom := errors.New("install failed")
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		calls := 0
		result, err := InstallWithHome(model.CommunityToolCodeGraph, "/work/project", home, RunnerFunc(func(string, ...string) error {
			calls++
			if calls == 2 {
				return boom
			}
			return nil
		}), DetectorFunc(func(string) (string, error) {
			if calls > 0 {
				return "/bin/codegraph", nil
			}
			return "", errors.New("not found")
		}))
		if !errors.Is(err, boom) {
			t.Fatalf("Install() error = %v, want wrapped install error", err)
		}
		if calls != 2 {
			t.Fatalf("runner calls = %d, want 2", calls)
		}
		if len(result.CommandsRun) != 2 {
			t.Fatalf("CommandsRun = %#v, want CLI install and failed agent wiring command", result.CommandsRun)
		}
		got := strings.Join(result.CommandsRun, "\n")
		if !strings.Contains(got, "npm install -g @colbymchenry/codegraph@latest") || !strings.Contains(got, "codegraph install --target claude") || strings.Contains(got, "codegraph init") {
			t.Fatalf("CommandsRun = %#v, want CLI install and agent wiring commands only", result.CommandsRun)
		}
	})
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func useCodeGraphWSLAdmission(t *testing.T, wsl bool) {
	t.Helper()
	previousCLIUsable := codeGraphCLIUsable
	previousWSL := codeGraphWSL
	t.Cleanup(func() {
		codeGraphCLIUsable = previousCLIUsable
		codeGraphWSL = previousWSL
	})
	codeGraphCLIUsable = defaultCodeGraphCLIUsable
	codeGraphWSL = func() bool { return wsl }
}

func findAgentStatus(t *testing.T, status Status, id model.AgentID) AgentStatus {
	t.Helper()
	for _, agent := range status.Agents {
		if agent.Agent == id {
			return agent
		}
	}
	t.Fatalf("agent %s not found in %#v", id, status.Agents)
	return AgentStatus{}
}
