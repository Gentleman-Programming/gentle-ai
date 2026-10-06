package agentguidance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// claudeModuleOverheadCeiling bounds, in bytes, what the user-global Claude
// pilot adds when every installed module is read on top of its core: pointers,
// module titles and intros. It is an engineering ratchet, not a token, cost or
// latency claim; raise it only as a reviewed decision, never to make a
// failing run pass.
const claudeModuleOverheadCeiling = 4096

// claudeModuleFiles pins, by file name, the exact bytes of every module the
// user-global Claude pilot installs; none embeds the home path. A change to
// the shared Markdown, the fragment specs or the module layout fails here, so
// module drift is a reviewed edit of this table. Keep it hand-written: there
// is no update flag.
var claudeModuleFiles = map[string]struct {
	bytes  int
	sha256 string
}{
	"orchestrator-delegation.md": {1171, "47f6a58cc2e6de22bbb4f3f8c9f5931b9b9678064465950a8bd43099cfa43935"},
	"orchestrator-writer.md":     {2041, "27774cd79f47c2bf4a19ff3d00833746541670d6540c86f9823f989bf2c20d99"},
	"orchestrator-skills.md":     {3018, "ad2be8ac4e5dd4b1913b6c328b9860b61ab080ea5852448c15157bef0e7bdd7c"},
}

// TestOrchestratorModuleBudgetClaudeGlobal measures the real user-global
// Claude pilot against the monolith a default install writes into the same
// home, with the production review contract. The startup core must be smaller
// than the monolith, each installed module no larger than it, and core plus
// every installed module (the ledger is metadata, not prompt) may exceed it by
// at most claudeModuleOverheadCeiling. The installed modules must match
// claudeModuleFiles exactly. It reads the default baseline without
// updating it, measures bytes only, and proves nothing about whether or when
// Claude actually reads a module.
func TestOrchestratorModuleBudgetClaudeGlobal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("%s pins Linux install bytes; %s is not measured", moduleBaselineFile, runtime.GOOS)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	raw, err := os.ReadFile(moduleBaselineFile)
	if err != nil {
		t.Fatal(err)
	}
	var variants []moduleVariant
	if err := json.Unmarshal(raw, &variants); err != nil {
		t.Fatalf("decode %s: %v", moduleBaselineFile, err)
	}
	index := slices.IndexFunc(variants, func(v moduleVariant) bool { return v.ID == "claude-code" })
	if index < 0 || len(variants[index].Installed) != 1 {
		t.Fatalf("%s has no single-file claude-code variant", moduleBaselineFile)
	}
	want := variants[index]

	contract, err := reviewassets.ReviewExecutionContractFor(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	contract = strings.TrimSpace(contract)
	if len(contract) != want.ContractBytes || sha256Hex(contract) != want.ContractSHA256 {
		t.Fatalf("Claude review contract is %d bytes, baseline %d; the budget would not measure the baselined contract", len(contract), want.ContractBytes)
	}

	read := func(path string) string {
		t.Helper()
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}

	// The monolith first, exactly as the default install writes it; the
	// baseline replaces the temporary home with "~" the same way.
	monoResult, err := agentguidance.InjectRoutingWithOptions(home, model.AgentClaudeCode, agentguidance.RoutingOptions{
		ReviewContract: reviewassets.ReviewExecutionContractFor,
	})
	if err != nil {
		t.Fatalf("default InjectRoutingWithOptions error = %v", err)
	}
	if len(monoResult.Files) != 1 {
		t.Fatalf("default install wrote %v, want only CLAUDE.md", monoResult.Files)
	}
	mono := read(monoResult.Files[0])
	if normalized := strings.ReplaceAll(mono, home, "~"); len(normalized) != want.Installed[0].Bytes || sha256Hex(normalized) != want.Installed[0].SHA256 {
		t.Fatalf("default Claude monolith is %d bytes, baseline %d; the baseline must stay unchanged", len(normalized), want.Installed[0].Bytes)
	}

	// Then the pilot into the same home, converting that monolith in place.
	options := agentguidance.RoutingOptions{ReviewContract: reviewassets.ReviewExecutionContractFor, ClaudeGlobalModules: true}
	planned, err := agentguidance.RoutingPathsWithOptions(home, model.AgentClaudeCode, options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := agentguidance.InjectRoutingWithOptions(home, model.AgentClaudeCode, options)
	if err != nil {
		t.Fatalf("pilot InjectRoutingWithOptions error = %v", err)
	}
	// Planned: the core, all seven known module names and the ledger. Written:
	// the core, only the modules the split produces, and the ledger.
	files := result.Files
	if len(planned) != 9 || len(files) < 3 || files[0] != planned[0] || files[len(files)-1] != planned[8] {
		t.Fatalf("pilot wrote %v, planned %v", files, planned)
	}

	core := read(files[0])
	if got := strings.Count(core, contract); got != 1 {
		t.Fatalf("pilot core carries the review contract %d times, want 1", got)
	}
	allLoaded := len(core)
	installed := map[string]bool{}
	for _, module := range files[1 : len(files)-1] {
		if !slices.Contains(planned[1:8], module) {
			t.Fatalf("pilot wrote unplanned module %s", module)
		}
		content := read(module)
		size, name := len(content), filepath.Base(module)
		t.Logf("module %s: %d bytes, sha256 %s", strings.TrimPrefix(module, home), size, sha256Hex(content))
		installed[name] = true
		if want, ok := claudeModuleFiles[name]; !ok || size != want.bytes || sha256Hex(content) != want.sha256 {
			t.Errorf("module %s drifted from claudeModuleFiles; update the table only as a reviewed change", name)
		}
		if size > len(mono) {
			t.Errorf("module %s is %d bytes, larger than the %d-byte monolith", module, size, len(mono))
		}
		allLoaded += size
	}
	if len(installed) != len(files)-2 || len(installed) != len(claudeModuleFiles) {
		t.Errorf("pilot installed modules %v, want exactly the %d in claudeModuleFiles", files[1:len(files)-1], len(claudeModuleFiles))
	}
	ledger, err := os.Stat(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	overhead := allLoaded - len(mono)
	t.Logf("contract %d, monolith %d, core %d, %d modules, all loaded %d, overhead %d bytes; ledger %d bytes not counted",
		len(contract), len(mono), len(core), len(files)-2, allLoaded, overhead, ledger.Size())
	if len(core) >= len(mono) {
		t.Errorf("pilot core is %d bytes, not smaller than the %d-byte monolith", len(core), len(mono))
	}
	if overhead > claudeModuleOverheadCeiling {
		t.Errorf("core plus modules exceed the monolith by %d bytes, ceiling %d", overhead, claudeModuleOverheadCeiling)
	}
}
