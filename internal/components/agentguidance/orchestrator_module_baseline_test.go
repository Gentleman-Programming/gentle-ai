package agentguidance_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

// #5256 T1 characterizes the orchestrator every non-Pi runtime receives before
// it is split into a core plus on-demand modules: per runtime and profile, the
// raw asset, the rendered orchestrator, the embedded native review contract and
// the guidance a fresh install writes. It measures only; it sets no byte budget
// and proves nothing about lazy loading. Installed paths and bytes are Linux
// measurements only, so the test skips elsewhere; cross-platform installer
// behavior stays with the portable installer tests.
//
// Regenerate the baseline on Linux after an intended change with the whole
// matrix (a subtest filter or any failure never writes the golden)
//
//	go test ./internal/components/agentguidance -run '^TestOrchestratorModuleBaseline$' -update
var update = flag.Bool("update", false, "rewrite "+moduleBaselineFile)

const moduleBaselineFile = "testdata/orchestrator-module-baseline.json"

type moduleVariantInput struct {
	id         string
	agent      model.AgentID
	capability string
	// openCodeVersion is what the fake `opencode --version` prints; empty
	// makes the probe fail, as for a missing or unknown CLI.
	openCodeVersion string
	// kimiCurrent pre-creates ~/.kimi-code so install selects the flattened
	// AGENTS.md hub instead of the legacy Jinja router.
	kimiCurrent bool
	asset       string
}

var moduleVariantInputs = []moduleVariantInput{
	{id: "claude-code", agent: model.AgentClaudeCode, asset: "claude/orchestrator.md"},
	{id: "codex", agent: model.AgentCodex, asset: "codex/orchestrator.md"},
	{id: "opencode/unknown", agent: model.AgentOpenCode, asset: "opencode/orchestrator.md"},
	{id: "opencode/v1", agent: model.AgentOpenCode, openCodeVersion: "1.14.0", asset: "opencode/orchestrator.md"},
	{id: "opencode/v2", agent: model.AgentOpenCode, openCodeVersion: "2.0.4", asset: "opencode/orchestrator.md"},
	{id: "kilocode", agent: model.AgentKilocode, asset: "opencode/orchestrator.md"},
	{id: "gemini-cli", agent: model.AgentGeminiCLI, asset: "gemini/orchestrator.md"},
	{id: "cursor", agent: model.AgentCursor, asset: "cursor/orchestrator.md"},
	{id: "antigravity", agent: model.AgentAntigravity, asset: "antigravity/orchestrator.md"},
	{id: "windsurf", agent: model.AgentWindsurf, asset: "windsurf/orchestrator.md"},
	{id: "kimi/legacy", agent: model.AgentKimi, asset: "kimi/orchestrator.md"},
	{id: "kimi/current", agent: model.AgentKimi, kimiCurrent: true, asset: "kimi/orchestrator.md"},
	{id: "qwen-code", agent: model.AgentQwenCode, asset: "qwen/orchestrator.md"},
	{id: "kiro-ide", agent: model.AgentKiroIDE, asset: "kiro/orchestrator.md"},
	{id: "hermes", agent: model.AgentHermes, asset: "hermes/orchestrator.md"},
	{id: "vscode-copilot/capable", agent: model.AgentVSCodeCopilot, asset: "generic/orchestrator.md"},
	{id: "vscode-copilot/small", agent: model.AgentVSCodeCopilot, capability: "small", asset: "generic/orchestrator.md"},
	{id: "openclaw/capable", agent: model.AgentOpenClaw, asset: "generic/orchestrator.md"},
	{id: "openclaw/small", agent: model.AgentOpenClaw, capability: "small", asset: "generic/orchestrator.md"},
	{id: "trae-ide/capable", agent: model.AgentTrae, asset: "generic/orchestrator.md"},
	{id: "trae-ide/small", agent: model.AgentTrae, capability: "small", asset: "generic/orchestrator.md"},
}

type moduleVariant struct {
	ID             string              `json:"id"`
	Asset          string              `json:"asset"`
	RDD            bool                `json:"rdd"`
	RawBytes       int                 `json:"rawBytes"`
	RawSHA256      string              `json:"rawSha256"`
	RenderedBytes  int                 `json:"renderedBytes"`
	RenderedSHA256 string              `json:"renderedSha256"`
	ContractBytes  int                 `json:"contractBytes"`
	ContractSHA256 string              `json:"contractSha256,omitempty"`
	Installed      []installedGuidance `json:"installed"`
}

// installedGuidance is one file a fresh install writes, as the runtime reads
// it: the whole Markdown file, or the managed agent prompt of a JSON settings
// document. The temporary root is replaced with "~".
type installedGuidance struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// fakeOpenCodeVersion replaces the `opencode --version` probe the OpenCode
// review contract runs, so no installed CLI is executed.
func fakeOpenCodeVersion(t *testing.T, version string) {
	t.Helper()
	original := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = original })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		if version == "" {
			return opencode.CommandOutput{}, errors.New("fake opencode is unavailable")
		}
		return opencode.CommandOutput{Stdout: []byte(version + "\n")}, nil
	}
}

// installedFiles runs the real install carrier into a fresh root.
func installedFiles(t *testing.T, input moduleVariantInput) []installedGuidance {
	t.Helper()
	root := t.TempDir()
	if input.kimiCurrent {
		if err := os.MkdirAll(filepath.Join(root, ".kimi-code"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	result, err := agentguidance.InjectRoutingWithOptions(root, input.agent, agentguidance.RoutingOptions{
		OrchestratorCapability: input.capability,
		ReviewContract:         reviewassets.ReviewExecutionContractFor,
	})
	if err != nil {
		t.Fatalf("InjectRoutingWithOptions(%s) error = %v", input.id, err)
	}
	var files []installedGuidance
	for _, file := range result.Files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content, rel := string(raw), filepath.ToSlash(strings.TrimPrefix(file, root))
		if agentguidance.DeliversThroughOrchestratorPrompt(input.agent) {
			var settings struct {
				Agent map[string]struct {
					Prompt string `json:"prompt"`
				} `json:"agent"`
			}
			if err := json.Unmarshal(raw, &settings); err != nil {
				t.Fatalf("decode %s settings: %v", input.id, err)
			}
			content = settings.Agent[opencodedefault.ManagedAgent].Prompt
			rel += "#agent." + opencodedefault.ManagedAgent + ".prompt"
		}
		content = strings.ReplaceAll(content, root, "~")
		files = append(files, installedGuidance{Path: "~" + rel, Bytes: len(content), SHA256: sha256Hex(content)})
	}
	return files
}

// TestOrchestratorModuleBaseline is not parallel: it swaps the process-wide
// OpenCode version probe and HOME.
func TestOrchestratorModuleBaseline(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("%s pins Linux install paths and bytes; %s is not measured", moduleBaselineFile, runtime.GOOS)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	renders := map[string]string{}
	lines := map[string]string{}
	var order []string
	for _, input := range moduleVariantInputs {
		t.Run(input.id, func(t *testing.T) {
			fakeOpenCodeVersion(t, input.openCodeVersion)
			rendered, err := agentguidance.RenderOrchestratorWithSource(input.agent, reviewassets.ReviewExecutionContractFor, input.capability)
			if err != nil {
				t.Fatalf("RenderOrchestratorWithSource(%s) error = %v", input.id, err)
			}
			raw, err := assets.Read(input.asset)
			if err != nil {
				t.Fatal(err)
			}
			variant := moduleVariant{
				ID: input.id, Asset: input.asset, RDD: model.SupportsReceiptDrivenDevelopment(input.agent),
				RawBytes: len(raw), RawSHA256: sha256Hex(raw),
				RenderedBytes: len(rendered), RenderedSHA256: sha256Hex(rendered),
				Installed: installedFiles(t, input),
			}
			if variant.RDD {
				// The native review authority stays always loaded: the exact
				// current contract is embedded once, with its defect handoff.
				contract, err := reviewassets.ReviewExecutionContractFor(input.agent)
				if err != nil {
					t.Fatal(err)
				}
				contract = strings.TrimSpace(contract)
				if got := strings.Count(rendered, contract); got != 1 {
					t.Fatalf("rendered %s carries its review contract %d times, want 1", input.id, got)
				}
				if !strings.Contains(rendered, "#### Gentle AI Provider Defect Handoff (MANDATORY)") {
					t.Fatalf("rendered %s lost the provider defect handoff", input.id)
				}
				variant.ContractBytes, variant.ContractSHA256 = len(contract), sha256Hex(contract)
			} else {
				for _, leak := range []string{"Native Compact Review Orchestration", "Review Execution Contract", "Provider Defect Handoff", "receipt"} {
					if strings.Contains(rendered, leak) {
						t.Errorf("rendered %s without receipt-driven development carries %q", input.id, leak)
					}
				}
			}
			encoded, err := json.Marshal(variant)
			if err != nil {
				t.Fatal(err)
			}
			renders[input.id], lines[input.id] = rendered, string(encoded)
			order = append(order, input.id)
		})
	}
	if t.Failed() {
		return
	}
	if len(order) != len(moduleVariantInputs) {
		t.Fatalf("partial matrix: %d of %d variants ran; run the whole TestOrchestratorModuleBaseline without a subtest filter", len(order), len(moduleVariantInputs))
	}

	// Every non-Pi runtime with a guidance target and every orchestrator asset
	// is characterized. Pi's prompt is owned by Gentle Shell and Conductor is
	// catalog-only, so neither renders an orchestrator here.
	agentsSeen, assetsSeen := map[model.AgentID]bool{}, map[string]bool{}
	for _, input := range moduleVariantInputs {
		agentsSeen[input.agent], assetsSeen[input.asset] = true, true
	}
	for _, agent := range catalog.AllAgents() {
		if agent.ID != model.AgentPi && agent.ID != model.AgentConductor && !agentsSeen[agent.ID] {
			t.Errorf("runtime %q has no baseline variant", agent.ID)
		}
	}
	if err := fs.WalkDir(assets.FS, ".", func(assetPath string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && path.Base(assetPath) == "orchestrator.md" && !assetsSeen[assetPath] {
			t.Errorf("orchestrator asset %s has no baseline variant", assetPath)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Variant relationships the module split must keep observable.
	if renders["opencode/unknown"] != renders["opencode/v1"] {
		t.Error("an unknown OpenCode version no longer renders the v1 contract")
	}
	if renders["opencode/v2"] == renders["opencode/v1"] || !strings.Contains(renders["opencode/v2"], "`subagent`") {
		t.Error("OpenCode v2 no longer binds the review contract to its own tool names")
	}
	for _, agent := range []string{"vscode-copilot", "openclaw", "trae-ide"} {
		capable, small := renders[agent+"/capable"], renders[agent+"/small"]
		if capable == small || strings.Contains(capable, "(Small Model)") || !strings.Contains(small, "(Small Model)") {
			t.Errorf("%s does not keep distinct capable and small generic variants", agent)
		}
	}

	if t.Failed() {
		return
	}

	// One variant per line, so a review diff names exactly the runtime that moved.
	ordered := make([]string, len(order))
	for i, id := range order {
		ordered[i] = "  " + lines[id]
	}
	encoded := "[\n" + strings.Join(ordered, ",\n") + "\n]\n"
	if *update {
		if err := os.MkdirAll(filepath.Dir(moduleBaselineFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(moduleBaselineFile, []byte(encoded), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	current, err := os.ReadFile(moduleBaselineFile)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update to create it)", moduleBaselineFile, err)
	}
	if string(current) != encoded {
		want := map[string]bool{}
		for _, line := range strings.Split(string(current), "\n") {
			want[strings.TrimSuffix(line, ",")] = true
		}
		for _, line := range ordered {
			if !want[line] {
				t.Logf("changed: %s", strings.TrimSpace(line))
			}
		}
		t.Errorf("%s is stale; rerun with -update only if the drift above is intended", moduleBaselineFile)
	}
}
