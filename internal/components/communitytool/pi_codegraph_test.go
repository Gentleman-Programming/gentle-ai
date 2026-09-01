package communitytool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	piagent "github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

func TestPiCodeGraphUnselectedIsNoOp(t *testing.T) {
	home := t.TempDir()
	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: false})
	if err != nil {
		t.Fatalf("ReconcilePiCodeGraph() error = %v", err)
	}
	if result.Changed || len(result.Children) != 0 {
		t.Fatalf("result = %#v, want no-op", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".gentle-ai", "pi-codegraph.json")); !os.IsNotExist(err) {
		t.Fatalf("manifest exists after unselected reconciliation: %v", err)
	}
}

func TestPiCodeGraphReconcileInjectsOnlyCompatibleToolsAndGuidanceForEveryChild(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	writePiFile(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{}`)
	writePiFile(t, filepath.Join(home, ".pi", "agent", "subagents", "compatible.md"), "---\ntools: bash\n---\nwork\n")
	writePiFile(t, filepath.Join(home, ".pi", "agent", "subagents", "limited.md"), "---\ntools: read\n---\nwork\n")

	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, WorkspaceDir: workspace, Selected: true, EffectiveMCPProbe: piProbeForTest})
	if err != nil {
		t.Fatalf("ReconcilePiCodeGraph() error = %v", err)
	}
	if len(result.Children) != 2 || result.Children[0].Classification != PiChildCompatible || result.Children[1].Classification != PiChildGuidanceOnly {
		t.Fatalf("children = %#v", result.Children)
	}
	for _, child := range result.Children {
		body, err := os.ReadFile(child.Target)
		if err != nil || !strings.Contains(string(body), piCodeGraphGuidanceMarker) {
			t.Fatalf("child %s has no owned guidance: %v\n%s", child.Name, err, body)
		}
	}
	compatible, _ := os.ReadFile(result.Children[0].Target)
	if !strings.Contains(string(compatible), "mcp") || !strings.Contains(string(compatible), piCodeGraphToolMarker) {
		t.Fatalf("compatible child has no mcp tool block:\n%s", compatible)
	}
	limited, _ := os.ReadFile(result.Children[1].Target)
	if strings.Contains(string(limited), piCodeGraphToolMarker) {
		t.Fatalf("guidance-only child received tools:\n%s", limited)
	}
	var mcp map[string]any
	data, _ := os.ReadFile(filepath.Join(home, ".pi", "agent", "mcp.json"))
	if err := json.Unmarshal(data, &mcp); err != nil || !strings.Contains(string(data), "codegraph") {
		t.Fatalf("mcp config = %s, err=%v", data, err)
	}
}

func TestPiCodeGraphRejectsParentMarkerAndRestoresOnConflict(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".pi", "agent", "mcp.json")
	writePiFile(t, filepath.Join(home, ".pi", "agent", "APPEND_SYSTEM.md"), "<!-- gentle-ai:codegraph-guidance -->")
	writePiFile(t, filepath.Join(home, ".pi", "agent", "subagents", "worker.md"), "---\ntools: bash\n---\nwork\n")
	writePiFile(t, settings, `{"mcpServers":{"codegraph":{"command":"other"}}}`)
	before, _ := os.ReadFile(settings)
	_, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true})
	if err == nil || !strings.Contains(err.Error(), "misconfigured") {
		t.Fatalf("error = %v, want misconfigured", err)
	}
	after, _ := os.ReadFile(settings)
	if string(after) != string(before) {
		t.Fatalf("mcp changed after failed reconcile: %s", after)
	}
}

func TestPiCodeGraphReportsMisconfiguredMalformedChild(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "subagents", "broken.md")
	writePiFile(t, path, "---\ntools: [bash\n---\nwork\n")
	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true})
	if err == nil {
		t.Fatal("ReconcilePiCodeGraph() error = nil, want malformed child failure")
	}
	if len(result.Children) != 1 || result.Children[0].Classification != PiChildMisconfigured {
		t.Fatalf("children = %#v, want misconfigured child report", result.Children)
	}
}

func TestPiCodeGraphReportsMisconfiguredConflictingChild(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "subagents", "conflict.md")
	writePiFile(t, path, "---\ntools: read\n---\nwork\n"+piCodeGraphToolMarker+"\nstale tool block\n"+piCodeGraphEndMarker+"\n")
	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true})
	if err == nil {
		t.Fatal("ReconcilePiCodeGraph() error = nil, want conflicting child failure")
	}
	if len(result.Children) != 1 || result.Children[0].Classification != PiChildMisconfigured {
		t.Fatalf("children = %#v, want misconfigured child report", result.Children)
	}
}

func TestPiCodeGraphUninstallPreservesDriftedOwnedChild(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, path, "---\ntools: bash\n---\nwork\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(mustReadPiFile(t, path), []byte("\nuser edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := UninstallPiCodeGraph(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ManualActions) == 0 {
		t.Fatalf("result = %#v, want drift manual action", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("drifted child was removed: %v", err)
	}
}

func TestPiCodeGraphRootValidationRejectsUnsafeRoots(t *testing.T) {
	home := t.TempDir()
	valid := filepath.Join(home, "repo")
	if output, err := exec.Command("git", "init", valid).CombinedOutput(); err != nil {
		t.Fatalf("git init %q: %v\n%s", valid, err, output)
	}
	for _, path := range []string{home, string(filepath.Separator), os.TempDir()} {
		if ValidatePiCodeGraphRoot(path, home) == nil {
			t.Fatalf("ValidatePiCodeGraphRoot(%q) succeeded", path)
		}
	}
	if err := ValidatePiCodeGraphRoot(valid, home); err != nil {
		t.Fatalf("valid root rejected: %v", err)
	}
	if output, err := exec.Command("git", "-C", valid, "rev-parse", "--show-toplevel").CombinedOutput(); err != nil {
		t.Fatalf("git -C root resolution = %q, %v", output, err)
	} else {
		gotInfo, gotErr := os.Stat(strings.TrimSpace(string(output)))
		wantInfo, wantErr := os.Stat(valid)
		if gotErr != nil || wantErr != nil || !os.SameFile(gotInfo, wantInfo) {
			t.Fatalf("git root %q and %q differ: %v, %v", output, valid, gotErr, wantErr)
		}
	}
	t.Chdir(home)
	if err := ValidatePiCodeGraphRoot("repo", home); err != nil {
		t.Fatalf("relative valid root rejected: %v", err)
	}
}

func TestPiCodeGraphReconcileIsByteIdempotentAndUninstallPreservesUserMCP(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, mcpPath, `{"mcpServers":{"user":{"command":"user-server"}}}`)
	writePiFile(t, childPath, "---\ntools: bash\n---\nwork\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	before := mustReadPiFile(t, childPath)
	second, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest})
	if err != nil || second.Changed {
		t.Fatalf("second reconcile = %#v, err=%v", second, err)
	}
	if string(before) != string(mustReadPiFile(t, childPath)) {
		t.Fatal("idempotent reconciliation changed child bytes")
	}
	if _, err := UninstallPiCodeGraph(home); err != nil {
		t.Fatal(err)
	}
	mcp := string(mustReadPiFile(t, mcpPath))
	if !strings.Contains(mcp, "user-server") || strings.Contains(mcp, `"codegraph"`) {
		t.Fatalf("uninstall MCP = %s", mcp)
	}
	if _, err := UninstallPiCodeGraph(home); err != nil {
		t.Fatalf("repeat uninstall: %v", err)
	}
}

func TestPiChildToolsAcceptsYAMLBlockList(t *testing.T) {
	body := "---\ntools:\n  - read\n  - grep\n  - bash\n---\nwork\n"

	tools, parseable, malformed := piChildTools(body)
	if malformed || !parseable {
		t.Fatalf("piChildTools() = %v, %v, %v; want parseable block list", tools, parseable, malformed)
	}
	if got, want := strings.Join(tools, ","), "read,grep,bash"; got != want {
		t.Fatalf("piChildTools() tools = %q, want %q", got, want)
	}

	rendered := replacePiChildTools(body, append(tools, "mcp"))
	if strings.Contains(rendered, "  - ") || !strings.Contains(rendered, "tools: read, grep, bash, mcp") {
		t.Fatalf("replacePiChildTools() left malformed frontmatter:\n%s", rendered)
	}
}

func TestPiCodeGraphUninstallRestoresAdoptedAndOwnedArtifacts(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	userChild := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	packageChild := filepath.Join(home, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package.md")
	adopted := `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]},"user":{"command":"user"}}}`
	userBody := "---\ntools: bash\n---\nuser instructions\n"
	writePiFile(t, mcpPath, adopted)
	writePiFile(t, userChild, userBody)
	writePiFile(t, packageChild, "---\ntools: bash\n---\npackage instructions\n")

	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallPiCodeGraph(home); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadPiFile(t, mcpPath)); got != adopted {
		t.Fatalf("adopted MCP = %q, want exact original %q", got, adopted)
	}
	if got := string(mustReadPiFile(t, userChild)); got != userBody {
		t.Fatalf("user child = %q, want exact original %q", got, userBody)
	}
	overlay := filepath.Join(home, ".pi", "agent", "subagents", "package.md")
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Fatalf("owned package overlay remains: %v", err)
	}
	if got := string(mustReadPiFile(t, packageChild)); !strings.Contains(got, "package instructions") || strings.Contains(got, piCodeGraphGuidanceMarker) {
		t.Fatalf("package child was modified: %q", got)
	}
}

func TestPiCodeGraphUninstallPreservesPreexistingMarkedUserChildWithoutManifest(t *testing.T) {
	home := t.TempDir()
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	preexisting := "---\ntools: bash, mcp\n---\nuser instructions\n\n" +
		piCodeGraphToolMarker + "\npreexisting tool guidance\n" + piCodeGraphEndMarker + "\n\n" +
		piCodeGraphGuidanceMarker + "\npreexisting lazy-init guidance\n" + piCodeGraphEndMarker + "\n"
	writePiFile(t, childPath, preexisting)

	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallPiCodeGraph(home); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadPiFile(t, childPath)); got != preexisting {
		t.Fatalf("preexisting marked user child = %q, want exact before-image %q", got, preexisting)
	}
}

func TestPiCodeGraphDeselectionRemovesOnlyOwnedIntegration(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, mcpPath, `{"mcpServers":{"user":{"command":"user"}}}`)
	writePiFile(t, childPath, "---\ntools: bash\n---\nuser instructions\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: false})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || strings.Contains(string(mustReadPiFile(t, mcpPath)), `"codegraph"`) || strings.Contains(string(mustReadPiFile(t, childPath)), piCodeGraphGuidanceMarker) {
		t.Fatalf("deselection did not remove owned integration: %#v", result)
	}
	if !strings.Contains(string(mustReadPiFile(t, mcpPath)), `"user"`) {
		t.Fatal("deselection removed user MCP entry")
	}
}

func TestPiCodeGraphRefreshRestoresMissingOwnedChild(t *testing.T) {
	home := t.TempDir()
	previousProbe := piCodeGraphEffectiveMCPProbe
	piCodeGraphEffectiveMCPProbe = piProbeForTest
	t.Cleanup(func() { piCodeGraphEffectiveMCPProbe = previousProbe })
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, childPath, "---\ntools: bash\n---\nwork\n")
	if err := os.Chmod(childPath, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(childPath); err != nil {
		t.Fatal(err)
	}
	result, handled, err := RefreshPiCodeGraphIfConfigured(home, "")
	if err != nil || !handled {
		t.Fatalf("RefreshPiCodeGraphIfConfigured() = %#v, %v, %v", result, handled, err)
	}
	if got := string(mustReadPiFile(t, childPath)); !strings.Contains(got, piCodeGraphGuidanceMarker) || !strings.Contains(got, "mcp") {
		t.Fatalf("refresh did not restore and verify child: %q", got)
	}
	info, err := os.Stat(childPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o640 {
		t.Fatalf("restored child mode = %o, want %o", got, 0o640)
	}
}

func TestPiCodeGraphPathsExcludesUnsafeManifestPaths(t *testing.T) {
	home := t.TempDir()
	paths := piagent.CodeGraphPaths(home)
	outside := filepath.Join(t.TempDir(), "outside.json")
	escapedDir := filepath.Join(paths.AgentDir, "escaped")
	escaped := filepath.Join(escapedDir, "child.md")
	if err := os.MkdirAll(paths.AgentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), escapedDir); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name     string
		mcpPath  string
		children map[string]piCodeGraphOwnedFile
		unsafe   string
	}{
		{name: "path traversal", children: map[string]piCodeGraphOwnedFile{filepath.Join(paths.AgentDir, "subagents", "..", "..", "escape.md"): {}}, unsafe: filepath.Join(paths.AgentDir, "subagents", "..", "..", "escape.md")},
		{name: "arbitrary absolute MCP path", mcpPath: outside, children: map[string]piCodeGraphOwnedFile{}, unsafe: outside},
		{name: "symlink escape", children: map[string]piCodeGraphOwnedFile{escaped: {}}, unsafe: escaped},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manifest := piCodeGraphManifest{MCPPath: tt.mcpPath, Children: tt.children}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(paths.Manifest), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(paths.Manifest, data, 0o600); err != nil {
				t.Fatal(err)
			}

			if got := PiCodeGraphPaths(home, ""); slices.Contains(got, tt.unsafe) {
				t.Fatalf("PiCodeGraphPaths() = %v, contains unsafe manifest path %q", got, tt.unsafe)
			}
		})
	}
}

func TestPiCodeGraphManifestPermissionsSafe(t *testing.T) {
	for _, tt := range []struct {
		name string
		goos string
		mode os.FileMode
		want bool
	}{
		{name: "Windows normal file", goos: "windows", mode: 0o666, want: true},
		{name: "Windows read-only file", goos: "windows", mode: 0o444, want: true},
		{name: "POSIX private file", goos: "linux", mode: 0o600, want: true},
		{name: "POSIX group-readable file", goos: "linux", mode: 0o644, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := piCodeGraphManifestPermissionsSafe(tt.goos, tt.mode); got != tt.want {
				t.Fatalf("piCodeGraphManifestPermissionsSafe(%q, %o) = %v, want %v", tt.goos, tt.mode, got, tt.want)
			}
		})
	}
}

func TestReadPiCodeGraphManifestPreservesFileErrors(t *testing.T) {
	t.Run("read error", func(t *testing.T) {
		if _, err := readPiCodeGraphManifest(t.TempDir()); err == nil {
			t.Fatal("readPiCodeGraphManifest() error = nil, want read error")
		}
	})

	t.Run("stat error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(path, []byte(`{"children":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		wantErr := errors.New("stat failed")
		previousStat := piCodeGraphManifestStat
		piCodeGraphManifestStat = func(string) (os.FileInfo, error) { return nil, wantErr }
		t.Cleanup(func() { piCodeGraphManifestStat = previousStat })

		if _, err := readPiCodeGraphManifest(path); !errors.Is(err, wantErr) {
			t.Fatalf("readPiCodeGraphManifest() error = %v, want wrapped %v", err, wantErr)
		}
	})
}

func TestVerifyPiCodeGraphRejectsNonCanonicalMCP(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, "mcp.json")
	writePiFile(t, mcpPath, `{"mcpServers":{"not-codegraph":{"command":"other codegraph"}}}`)
	if err := verifyPiCodeGraph(mcpPath, nil); err == nil {
		t.Fatal("verifyPiCodeGraph() accepted substring-only MCP evidence")
	}
}

func TestVerifyPiMCPFailsClosedWithoutAdapterOrProcess(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, "mcp.json")
	writePiFile(t, mcpPath, `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
	previous := piCodeGraphEffectiveMCPProbe
	piCodeGraphEffectiveMCPProbe = probePiCodeGraphMCP
	t.Cleanup(func() { piCodeGraphEffectiveMCPProbe = previous })

	if _, err := verifyPiMCP(mcpPath); err == nil {
		t.Fatal("verifyPiMCP() succeeded without a Pi MCP adapter or MCP process")
	}
}

func TestVerifyPiMCPUsesInjectedEffectiveProbe(t *testing.T) {
	mcpPath := filepath.Join(t.TempDir(), "mcp.json")
	writePiFile(t, mcpPath, `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)

	validTool := PiCodeGraphMCPTool{
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
	}

	for _, tt := range []struct {
		name        string
		probe       PiCodeGraphMCPProbeResult
		probeErr    error
		wantErr     bool
		wantPending bool
	}{
		{name: "successful initialized read-only explore", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{validTool}}},
		{name: "successful unindexed initialized read-only explore requires project path", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{Name: validTool.Name, InputSchema: map[string]any{
			"type":       "object",
			"properties": validTool.InputSchema["properties"],
			"required":   []any{"query", "projectPath"},
		}}}}},
		{name: "adapter unavailable", probe: PiCodeGraphMCPProbeResult{Initialized: true, Tools: []PiCodeGraphMCPTool{validTool}}, wantErr: true},
		{name: "handshake failed", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Tools: []PiCodeGraphMCPTool{validTool}}, wantErr: true},
		{name: "missing explore tool", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true}, wantErr: true},
		{name: "malformed explore schema", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{Name: "codegraph_explore", InputSchema: map[string]any{"type": "object"}}}}, wantErr: true},
		{name: "incompatible maxFiles type", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{Name: validTool.Name, InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}, "maxFiles": map[string]any{"type": "boolean"}, "projectPath": map[string]any{"type": "string"}}, "required": []any{"query"}}}}}, wantErr: true},
		{name: "unknown required field", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{Name: validTool.Name, InputSchema: map[string]any{
			"type":       "object",
			"properties": validTool.InputSchema["properties"],
			"required":   []any{"query", "writePath"},
		}}}}, wantErr: true},
		{name: "missing query requirement", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{Name: validTool.Name, InputSchema: map[string]any{
			"type":       "object",
			"properties": validTool.InputSchema["properties"],
			"required":   []any{"projectPath"},
		}}}}, wantErr: true},
		{name: "duplicate required field", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{Name: validTool.Name, InputSchema: map[string]any{
			"type":       "object",
			"properties": validTool.InputSchema["properties"],
			"required":   []any{"query", "query"},
		}}}}, wantErr: true},
		{name: "unexpected writable tool", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{validTool, {Name: "codegraph_write", InputSchema: validTool.InputSchema}}}, wantErr: true},
		{name: "pending health with missing explore tool remains fatal", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true}, probeErr: ErrPiCodeGraphAdapterHealthUnavailable, wantErr: true},
		{name: "pending health with malformed explore schema remains fatal", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{Name: "codegraph_explore", InputSchema: map[string]any{"type": "object"}}}}, probeErr: ErrPiCodeGraphAdapterHealthUnavailable, wantErr: true},
		{name: "validated capability with unavailable adapter health becomes pending", probe: PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{validTool}}, probeErr: ErrPiCodeGraphAdapterHealthUnavailable, wantPending: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			previous := piCodeGraphEffectiveMCPProbe
			piCodeGraphEffectiveMCPProbe = func(string) (PiCodeGraphMCPProbeResult, error) { return tt.probe, tt.probeErr }
			t.Cleanup(func() { piCodeGraphEffectiveMCPProbe = previous })

			verification, err := verifyPiMCP(mcpPath)
			if tt.wantPending {
				if !errors.Is(err, ErrPiCodeGraphAdapterHealthUnavailable) {
					t.Fatalf("verifyPiMCP() error = %v, want pending adapter health", err)
				}
				if !verification.Adapter || !verification.ReadOnlyExplore || len(verification.Tools) != 1 || verification.Tools[0] != "codegraph_explore" {
					t.Fatalf("verification = %#v, want validated capability evidence", verification)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("verifyPiMCP() = %#v, nil error", verification)
				}
				return
			}
			if err != nil {
				t.Fatalf("verifyPiMCP() error = %v", err)
			}
			if !verification.Adapter || !verification.ReadOnlyExplore || len(verification.Tools) != 1 || verification.Tools[0] != "codegraph_explore" {
				t.Fatalf("verification = %#v, want observed read-only codegraph_explore", verification)
			}
		})
	}
}

func TestPiCodeGraphFailureRestoresNewMCPAndChild(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	original := "---\ntools: bash\n---\nwork\n"
	writePiFile(t, childPath, original)
	previous := piCodeGraphAtomicWrite
	piCodeGraphAtomicWrite = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		if path == filepath.Join(home, ".gentle-ai", "pi-codegraph.json") {
			return filemerge.WriteResult{}, os.ErrPermission
		}
		return previous(path, data, mode)
	}
	t.Cleanup(func() { piCodeGraphAtomicWrite = previous })
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err == nil {
		t.Fatal("ReconcilePiCodeGraph() error = nil, want post-write failure")
	}
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Fatalf("new MCP remains after rollback: %v", err)
	}
	if got := string(mustReadPiFile(t, childPath)); got != original {
		t.Fatalf("child after rollback = %q, want %q", got, original)
	}
}

func TestPiCodeGraphFailureRemovesNewPackageOverlayWithoutVerificationSuccess(t *testing.T) {
	home := t.TempDir()
	packageChild := filepath.Join(home, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package.md")
	overlay := filepath.Join(home, ".pi", "agent", "subagents", "package.md")
	writePiFile(t, packageChild, "---\ntools: bash\n---\npackage instructions\n")
	previous := piCodeGraphAtomicWrite
	piCodeGraphAtomicWrite = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		if path == filepath.Join(home, ".gentle-ai", "pi-codegraph.json") {
			return filemerge.WriteResult{}, os.ErrPermission
		}
		return previous(path, data, mode)
	}
	t.Cleanup(func() { piCodeGraphAtomicWrite = previous })

	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest})
	if err == nil {
		t.Fatal("ReconcilePiCodeGraph() error = nil, want post-write failure")
	}
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Fatalf("new package overlay remains after rollback: %v", err)
	}
	if result.MCP.Adapter || result.MCP.ReadOnlyExplore || len(result.MCP.Tools) != 0 {
		t.Fatalf("failed reconcile reported MCP verification success: %#v", result.MCP)
	}
}

func TestPiCodeGraphPendingProbePreservesConfiguredFiles(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	manifestPath := filepath.Join(home, ".gentle-ai", "pi-codegraph.json")
	writePiFile(t, childPath, "---\ntools: bash\n---\nwork\n")
	writePiFile(t, filepath.Join(home, ".pi", "agent", "npm", "node_modules", "pi-mcp-adapter", "index.ts"), "export default {}\n")
	installFakeCodeGraph(t)
	previousProbe := piCodeGraphEffectiveMCPProbe
	piCodeGraphEffectiveMCPProbe = probePiCodeGraphMCP
	t.Cleanup(func() { piCodeGraphEffectiveMCPProbe = previousProbe })

	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true})
	if err != nil {
		t.Fatalf("ReconcilePiCodeGraph() error = %v, want pending success", err)
	}
	if len(result.ManualActions) != 1 {
		t.Fatalf("ManualActions = %#v, want one pending action", result.ManualActions)
	}
	action := result.ManualActions[0]
	for _, want := range []string{"configuration was installed and preserved", "direct MCP capability was verified", "adapter activation health cannot be machine-verified", "remains pending"} {
		if !strings.Contains(action, want) {
			t.Fatalf("ManualActions[0] = %q, want %q", action, want)
		}
	}
	if strings.Contains(strings.ToLower(action), "reinstall") {
		t.Fatalf("ManualActions[0] = %q, should not instruct users to reinstall", action)
	}
	if got := string(mustReadPiFile(t, mcpPath)); !strings.Contains(got, `"codegraph"`) {
		t.Fatalf("pending MCP config = %s, want preserved CodeGraph server", got)
	}
	if got := string(mustReadPiFile(t, childPath)); !strings.Contains(got, piCodeGraphGuidanceMarker) {
		t.Fatalf("pending child = %q, want preserved CodeGraph guidance", got)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("pending manifest was not persisted: %v", err)
	}
}

func TestPiCodeGraphPendingProbeRejectsConflictingChildAndRollsBack(t *testing.T) {
	home := t.TempDir()
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	original := "---\ntools: read\n---\nwork\n" + piCodeGraphToolMarker + "\nstale tool block\n" + piCodeGraphEndMarker + "\n"
	writePiFile(t, childPath, original)

	_, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: func(string) (PiCodeGraphMCPProbeResult, error) {
		result, _ := piProbeForTest("unused")
		return result, ErrPiCodeGraphAdapterHealthUnavailable
	}})
	if err == nil || !strings.Contains(err.Error(), "misconfigured") {
		t.Fatalf("ReconcilePiCodeGraph() error = %v, want conflicting child failure", err)
	}
	if got := string(mustReadPiFile(t, childPath)); got != original {
		t.Fatalf("child after rollback = %q, want %q", got, original)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".gentle-ai", "pi-codegraph.json")); !os.IsNotExist(statErr) {
		t.Fatalf("pending manifest persisted after child failure: %v", statErr)
	}
}

func TestPiCodeGraphUninstallRollsBackWhenManifestRemovalFails(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	packageChild := filepath.Join(home, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package.md")
	overlay := filepath.Join(home, ".pi", "agent", "subagents", "package.md")
	writePiFile(t, mcpPath, `{"mcpServers":{"user":{"command":"user"}}}`)
	writePiFile(t, packageChild, "---\ntools: bash\n---\npackage instructions\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(home, ".gentle-ai", "pi-codegraph.json")
	previousRemove := piCodeGraphRemove
	piCodeGraphRemove = func(path string) error {
		if path == manifestPath {
			return os.ErrPermission
		}
		return previousRemove(path)
	}
	t.Cleanup(func() { piCodeGraphRemove = previousRemove })

	if _, err := UninstallPiCodeGraph(home); err == nil {
		t.Fatal("UninstallPiCodeGraph() error = nil, want manifest removal failure")
	}
	if got := string(mustReadPiFile(t, mcpPath)); !strings.Contains(got, `"codegraph"`) {
		t.Fatalf("MCP was not restored after failed uninstall: %s", got)
	}
	if _, err := os.Stat(overlay); err != nil {
		t.Fatalf("overlay was not restored after failed uninstall: %v", err)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest was removed after failed uninstall: %v", err)
	}
}

func TestPiCodeGraphUninstallFailsClosedOnChildReadFailure(t *testing.T) {
	home := t.TempDir()
	child := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, child, "---\ntools: bash\n---\nwork\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	previousRead := piCodeGraphReadFile
	piCodeGraphReadFile = func(path string) ([]byte, error) {
		if path == child {
			return nil, os.ErrPermission
		}
		return previousRead(path)
	}
	t.Cleanup(func() { piCodeGraphReadFile = previousRead })

	if _, err := UninstallPiCodeGraph(home); err == nil {
		t.Fatal("UninstallPiCodeGraph() error = nil, want child read failure")
	}
	if _, err := os.Stat(filepath.Join(home, ".gentle-ai", "pi-codegraph.json")); err != nil {
		t.Fatalf("manifest missing after failed cleanup: %v", err)
	}
}

func TestPiCodeGraphReconcileJoinsJournalRestoreFailure(t *testing.T) {
	home := t.TempDir()
	child := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, child, "---\ntools: bash\n---\nwork\n")
	manifestPath := filepath.Join(home, ".gentle-ai", "pi-codegraph.json")
	previousWrite := piCodeGraphAtomicWrite
	piCodeGraphAtomicWrite = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		if path == manifestPath {
			return filemerge.WriteResult{}, os.ErrPermission
		}
		if path == child && !strings.Contains(string(data), piCodeGraphGuidanceMarker) {
			return filemerge.WriteResult{}, os.ErrPermission
		}
		return previousWrite(path, data, mode)
	}
	t.Cleanup(func() { piCodeGraphAtomicWrite = previousWrite })

	_, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest})
	if err == nil || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "restore Pi CodeGraph journal") {
		t.Fatalf("ReconcilePiCodeGraph() error = %v, want original and journal restore evidence", err)
	}
}

func TestPiCodeGraphRejectsUnmatchedManagedMarkerWithoutChangingUserContent(t *testing.T) {
	home := t.TempDir()
	child := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	before := "---\ntools: bash\n---\nuser content\n" + piCodeGraphGuidanceMarker + "\nunclosed user content\n"
	writePiFile(t, child, before)

	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err == nil {
		t.Fatal("ReconcilePiCodeGraph() error = nil, want unmatched marker failure")
	}
	if got := string(mustReadPiFile(t, child)); got != before {
		t.Fatalf("child content changed after unmatched marker: %q, want %q", got, before)
	}
}

func TestPiCodeGraphFailsClosedForBrokenProjectMCPOverride(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	writePiFile(t, filepath.Join(home, ".pi", "agent", "subagents", "worker.md"), "---\ntools: bash\n---\nwork\n")
	writePiFile(t, filepath.Join(workspace, ".pi", "mcp.json"), `{"mcpServers":{"codegraph":{"command":"other","args":["serve","--mcp"]}}}`)

	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, WorkspaceDir: workspace, Selected: true, EffectiveMCPProbe: piProbeForTest}); err == nil || !strings.Contains(err.Error(), "MCP transport is not configured") {
		t.Fatalf("ReconcilePiCodeGraph() error = %v, want broken effective project override", err)
	}
}

func TestPiCodeGraphProbeVerifiesDirectMCPWithoutPiProcess(t *testing.T) {
	home := t.TempDir()
	agentDir := filepath.Join(home, "custom-agent")
	mcpPath := filepath.Join(home, "project", ".mcp.json")
	writePiFile(t, filepath.Join(agentDir, "npm", "node_modules", "pi-mcp-adapter", "index.ts"), "export default {}\n")
	writePiFile(t, mcpPath, `{"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`)
	installFakeCodeGraph(t)

	result, err := probePiCodeGraphMCPWithAgentDir(mcpPath, agentDir)
	if !errors.Is(err, ErrPiCodeGraphAdapterHealthUnavailable) {
		t.Fatalf("probe error = %v, want unavailable adapter health", err)
	}
	if !result.AdapterAvailable || !result.Initialized || !isReadOnlyCodeGraphExploreSchema(result.Tools) {
		t.Fatalf("probe result = %#v, want verified direct MCP capability", result)
	}
}

func TestPiCodeGraphProbeClassifiesStalledMCPResponsesAsDeadlineExceeded(t *testing.T) {
	for _, tt := range []struct {
		name               string
		stallInitialize    bool
		responseAtDeadline string
		wantPhase          string
	}{
		{
			name:            "initialize",
			stallInitialize: true,
			wantPhase:       "MCP initialize: read response",
		},
		{
			name:               "tools list cleanup race",
			responseAtDeadline: `{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}` + "\n",
			wantPhase:          "MCP tools/list",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			transport := &piCodeGraphStalledTransport{ctx: ctx, stallInitialize: tt.stallInitialize, responseAtDeadline: []byte(tt.responseAtDeadline)}
			_, err := probePiCodeGraphMCPWithTransport(ctx, transport, transport, func() (error, error) {
				return nil, nil
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("probe error = %v, want context deadline exceeded", err)
			}
			if !strings.Contains(err.Error(), tt.wantPhase) {
				t.Fatalf("probe error = %q, want phase %q", err, tt.wantPhase)
			}
		})
	}
}

type piCodeGraphStalledTransport struct {
	ctx                context.Context
	response           bytes.Buffer
	stallInitialize    bool
	responseAtDeadline []byte
}

func (t *piCodeGraphStalledTransport) Write(request []byte) (int, error) {
	if bytes.Contains(request, []byte(`"id":1`)) {
		if t.stallInitialize {
			return len(request), nil
		}
		t.response.WriteString(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}` + "\n")
	}
	return len(request), nil
}

func (t *piCodeGraphStalledTransport) Read(data []byte) (int, error) {
	if t.response.Len() != 0 {
		return t.response.Read(data)
	}
	<-t.ctx.Done()
	if len(t.responseAtDeadline) != 0 {
		t.response.Write(t.responseAtDeadline)
		t.responseAtDeadline = nil
		return t.response.Read(data)
	}
	return 0, io.EOF
}

func (*piCodeGraphStalledTransport) Close() error { return nil }

func TestPiCodeGraphProbeRejectsInvalidInitializeResponses(t *testing.T) {
	responses := []string{
		`{"id":1,"result":{"protocolVersion":"2025-03-26"}}`,
		`{"jsonrpc":"2.0","id":1}`,
		`{"jsonrpc":"2.0","id":1,"result":[]}`,
		`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":""}}`,
	}
	for _, response := range responses {
		t.Run(response, func(t *testing.T) {
			home := t.TempDir()
			agentDir := filepath.Join(home, "custom-agent")
			writePiFile(t, filepath.Join(agentDir, "npm", "node_modules", "pi-mcp-adapter", "index.ts"), "export default {}\n")
			if runtime.GOOS == "windows" {
				t.Setenv("GENTLE_AI_CODEGRAPH_TEST_RESPONSE", response)
				installFakeCodeGraphHelper(t, "invalid-response")
			} else {
				installFakeCodeGraphScript(t, `while IFS= read -r request; do printf '%s\n' '`+response+`'; done`)
			}

			_, err := probePiCodeGraphMCPWithAgentDir(filepath.Join(home, "mcp.json"), agentDir)
			if err == nil || !strings.Contains(err.Error(), "invalid JSON-RPC 2.0 result") {
				t.Fatalf("probe error = %v, want invalid initialize response", err)
			}
		})
	}
}

func TestPiCodeGraphRejectsMalformedMCPServersWithoutChangingBytes(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	before := []byte(`{"mcpServers":["user-server"]}`)
	writePiFile(t, mcpPath, string(before))

	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err == nil {
		t.Fatal("ReconcilePiCodeGraph() error = nil, want malformed mcpServers rejection")
	}
	if got := mustReadPiFile(t, mcpPath); string(got) != string(before) {
		t.Fatalf("MCP bytes = %q, want %q", got, before)
	}
}

func TestPiCodeGraphPreservesSensitiveFileModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exact POSIX file modes are unavailable on Windows")
	}
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	childPath := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, mcpPath, `{"mcpServers":{"user":{"command":"user"}}}`)
	writePiFile(t, childPath, "---\ntools: bash\n---\nwork\n")
	if err := os.Chmod(mcpPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(childPath, 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path string
		want os.FileMode
	}{{mcpPath, 0o600}, {childPath, 0o640}, {filepath.Join(home, ".gentle-ai", "pi-codegraph.json"), 0o600}} {
		info, err := os.Stat(tt.path)
		if err != nil || info.Mode().Perm() != tt.want {
			t.Fatalf("mode %q = %v, %v; want %v", tt.path, info.Mode(), err, tt.want)
		}
	}
	if _, err := UninstallPiCodeGraph(home); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(mcpPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("restored MCP mode = %v, %v; want 0600", info.Mode(), err)
	}
}

func TestPiCodeGraphUninstallRejectsManifestPathEscapeAndMissingOwnedChildIsSuccess(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	writePiFile(t, outside, "do not modify\n")
	manifestPath := filepath.Join(home, ".gentle-ai", "pi-codegraph.json")
	writePiFile(t, manifestPath, `{"children":{"`+outside+`":{"after":"owned","afterHash":"deadbeef"}}}`)
	if err := os.Chmod(manifestPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallPiCodeGraph(home); err == nil {
		t.Fatal("UninstallPiCodeGraph() error = nil, want manifest path escape rejection")
	}
	if got := string(mustReadPiFile(t, outside)); got != "do not modify\n" {
		t.Fatalf("outside file = %q", got)
	}
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}

	child := filepath.Join(home, ".pi", "agent", "subagents", "missing.md")
	writePiFile(t, child, "---\ntools: bash\n---\nwork\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallPiCodeGraph(home); err != nil {
		t.Fatalf("UninstallPiCodeGraph() missing owned child error = %v", err)
	}
}

func TestPiCodeGraphUninstallRejectsSwappedSymlinkParent(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(home, ".pi", "agent", "subagents")
	child := filepath.Join(parent, "worker.md")
	writePiFile(t, child, "---\ntools: bash\n---\nwork\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	managed := mustReadPiFile(t, child)
	if err := os.Rename(parent, parent+"-original"); err != nil {
		t.Fatal(err)
	}
	externalDir := t.TempDir()
	external := filepath.Join(externalDir, "worker.md")
	if err := os.WriteFile(external, managed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalDir, parent); err != nil {
		t.Skipf("parent symlink unavailable: %v", err)
	}

	if _, err := UninstallPiCodeGraph(home); err == nil {
		t.Fatal("UninstallPiCodeGraph() error = nil, want swapped parent rejection")
	}
	if got := mustReadPiFile(t, external); string(got) != string(managed) {
		t.Fatalf("external child was mutated through swapped parent: %q", got)
	}
}

func TestPiCodeGraphConfiguredRejectsStaleGuidanceWhenBashIsRemoved(t *testing.T) {
	home := t.TempDir()
	child := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	writePiFile(t, child, "---\ntools: bash\n---\nwork\n")
	if _, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piProbeForTest}); err != nil {
		t.Fatal(err)
	}
	writePiFile(t, child, "---\ntools: read\n---\nwork\n\n"+piCodeGraphToolMarker+"\nstale\n"+piCodeGraphEndMarker+"\n\n"+piCodeGraphGuidanceMarker+"\nstale\n"+piCodeGraphEndMarker+"\n")
	previous := piCodeGraphEffectiveMCPProbe
	piCodeGraphEffectiveMCPProbe = piProbeForTest
	t.Cleanup(func() { piCodeGraphEffectiveMCPProbe = previous })
	configured, _ := PiCodeGraphConfigured(home, "")
	if configured {
		t.Fatal("PiCodeGraphConfigured() accepted stale guidance for a bash-less child")
	}
}

func piProbeForTest(string) (PiCodeGraphMCPProbeResult, error) {
	return PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: []PiCodeGraphMCPTool{{
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
	}}}, nil
}

func writePiFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadPiFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func installFakeCodeGraph(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		installFakeCodeGraphHelper(t, "normal")
		return
	}
	installFakeCodeGraphScript(t, `while IFS= read -r request; do
  case "$request" in
    *'"id":1'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{},"serverInfo":{"name":"fake","version":"1"}}}' ;;
    *'"id":2'*) printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"codegraph_explore","inputSchema":{"type":"object","properties":{"query":{"type":"string"},"maxFiles":{"type":"integer"},"projectPath":{"type":"string"}},"required":["query"]}}]}}' ;;
  esac
done`)
}

func installFakeCodeGraphHelper(t *testing.T, mode string) {
	t.Helper()
	binDir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "codegraph.exe"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GENTLE_AI_CODEGRAPH_TEST_HELPER", mode)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func installFakeCodeGraphScript(t *testing.T, body string) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\n" + body + "\n"
	path := filepath.Join(binDir, "codegraph")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
