package agentguidance

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// #5256 T2a proves only the lossless construction of a Claude core plus
// on-demand modules. Separate tests cover installation; nothing here
// proves that a model reads a module when its pointer fires.

const (
	testModuleDir      = "~/.claude/gentle-ai/orchestrator"
	testModuleContract = "Fake native review execution contract for module tests."
)

func fakeModuleContract(model.AgentID) (string, error) { return testModuleContract, nil }

func buildClaudeTestModules(t *testing.T) orchestratorModuleBundle {
	t.Helper()
	bundle, err := buildOrchestratorModules(model.AgentClaudeCode, fakeModuleContract, testModuleDir)
	if err != nil {
		t.Fatalf("buildOrchestratorModules(claude) error = %v", err)
	}
	return bundle
}

func TestBuildOrchestratorModulesReconstructsPublicRender(t *testing.T) {
	bundle := buildClaudeTestModules(t)
	public, err := RenderOrchestratorWithSource(model.AgentClaudeCode, fakeModuleContract, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(public, orchestratorFragmentMarkerToken) {
		t.Fatal("public render carries a module fragment marker")
	}
	got, err := bundle.reconstruct()
	if err != nil {
		t.Fatalf("reconstruct() error = %v", err)
	}
	if got != public {
		t.Fatalf("reconstructed core and modules differ from the public render (%d vs %d bytes)", len(got), len(public))
	}
	annotated, err := renderOrchestrator(model.AgentClaudeCode, fakeModuleContract, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if stripped := stripOrchestratorFragmentMarkers(annotated); stripped != public {
		t.Fatal("stripping the exact annotations from the annotated render does not yield the public render")
	}

	// Measurements with the fake contract, not budgets.
	var ids, names []string
	for _, fragment := range bundle.fragments {
		ids = append(ids, fragment.id)
	}
	for _, module := range bundle.modules {
		names = append(names, module.name)
		t.Logf("module %s: %d bytes", module.file, len(module.content))
	}
	t.Logf("core: %d bytes; public: %d bytes; fake contract: %d bytes", len(bundle.core), len(public), len(testModuleContract))
	wantIDs := []string{"writer.edit-surfaces", "delegation.key-learnings", "skills.discovery", "skills.registry"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("fragment order = %q, want %q", ids, wantIDs)
	}
	// Modules without a relevant body (verification, tracking, memory, prompts)
	// are not built.
	if want := []string{"delegation", "writer", "skills"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("modules = %q, want %q", names, want)
	}

	for _, fragment := range bundle.fragments {
		if strings.Count(bundle.core, fragment.pointer) != 1 {
			t.Errorf("%s pointer appears %d times in core, want 1", fragment.id, strings.Count(bundle.core, fragment.pointer))
		}
		if !strings.HasPrefix(fragment.pointer, "On demand: when ") || !strings.HasSuffix(fragment.pointer, "\".\n") {
			t.Errorf("%s pointer has no triggering reason line: %q", fragment.id, fragment.pointer)
		}
	}
	for file, want := range map[string]int{
		"orchestrator-delegation.md": 1, "orchestrator-verification.md": 0, "orchestrator-writer.md": 1, "orchestrator-skills.md": 2,
	} {
		if got := strings.Count(bundle.core, "read `"+testModuleDir+"/"+file+"`"); got != want {
			t.Errorf("core references %s %d times, want %d", file, got, want)
		}
	}
	for _, forbidden := range []string{orchestratorFragmentMarkerToken, "{{GENTLE_AI_", "@~/", "@" + testModuleDir} {
		if strings.Contains(bundle.core, forbidden) {
			t.Errorf("core carries %q", forbidden)
		}
	}
}

func TestOrchestratorModulesCarryCanonicalBodies(t *testing.T) {
	bundle := buildClaudeTestModules(t)
	shared, err := assets.Read(sharedOrchestratorSectionsAsset)
	if err != nil {
		t.Fatal(err)
	}
	modules := map[string]orchestratorModule{}
	for _, module := range bundle.modules {
		modules[module.name] = module
	}
	for _, fragment := range bundle.fragments {
		module := modules[fragment.module]
		if module.file != "orchestrator-"+fragment.module+".md" {
			t.Errorf("%s module file = %q", fragment.id, module.file)
		}
		if strings.Count(module.content, fragment.body) != 1 {
			t.Errorf("%s body appears %d times in %s, want 1", fragment.id, strings.Count(module.content, fragment.body), module.file)
		}
	}
	for _, fragment := range bundle.fragments {
		if fragment.id == "skills.registry" && fragment.body != sharedOrchestratorSection(shared, "Skill Registry Protocol")+"\n" {
			t.Error("skills.registry body is not the canonical Skill Registry Protocol section")
		}
	}

	delegation := modules["delegation"].content
	if !strings.HasPrefix(delegation, "# Gentle AI Orchestrator — Delegation Module\n") {
		t.Errorf("delegation module heading = %q", strings.SplitN(delegation, "\n", 2)[0])
	}
	for _, module := range bundle.modules {
		if strings.Contains(module.content, orchestratorFragmentMarkerToken) || strings.Contains(module.content, "{{GENTLE_AI_") {
			t.Errorf("%s carries a marker or template token", module.file)
		}
	}
}

func TestOrchestratorModuleCoreKeepsAuthorityAndSafety(t *testing.T) {
	core := buildClaudeTestModules(t).core
	for _, required := range []string{
		"### Identity Contract", "### Lossless Blocking Prompts (MANDATORY)",
		"#### Gentle AI Provider Defect Handoff (MANDATORY)", "When delegating, forward this contract",
		"#### Mandatory Delegation Triggers", "#### Delegated Verification Gate (MANDATORY)",
		"#### Native Checking Contract", "#### Review Execution Contract\n\n" + testModuleContract,
		"#### 1. Inline Direct", "Inline evidence uses one parallel batch", "#### 2. Simple Delegation",
		"### Allowed edit surfaces (MANDATORY)", "### Delivery strategy",
		"### Safety", "Ask before destructive git operations", "### Skill Registry Protocol",
	} {
		if strings.Count(core, required) != 1 {
			t.Errorf("core carries %q %d times, want 1", required, strings.Count(core, required))
		}
	}
	for _, moved := range []string{
		"Before launching a bounded writer through", "Discovery order:",
		"The parent resolves skills once per session",
	} {
		if strings.Contains(core, moved) {
			t.Errorf("core still carries module text %q", moved)
		}
	}
}

// Until a model is proved to read a module when its pointer fires, policy
// that decides RDD/consent fallback, prelaunch verification and handoff,
// mandatory delegation and incident handling must stay always loaded. The
// canonical bodies are compared whole, so no part of them can move behind a
// pointer; the clauses only prove that the derived bodies still carry them.
func TestOrchestratorModuleCoreKeepsCriticalPolicy(t *testing.T) {
	bundle := buildClaudeTestModules(t)
	shared, err := assets.Read(sharedOrchestratorSectionsAsset)
	if err != nil {
		t.Fatal(err)
	}
	routing := sharedOrchestratorSection(shared, "Orchestrator Routing and Delivery")
	start, end, ok := sectionBounds(routing, "#### 2. Simple Delegation", "Allowed edit surfaces (MANDATORY)")
	if !ok || end == len(routing) {
		t.Fatal("canonical Simple Delegation block is not bounded by Allowed edit surfaces")
	}

	for _, tc := range []struct {
		name, body string
		clauses    []string
	}{
		{"Delegated Verification Gate", sharedOrchestratorSection(shared, "Delegated Verification Gate (MANDATORY)"), []string{
			"declines the consent envelope", "the parent follows the RDD off path instead", "**RDD off or unknown**",
			"An unknown outcome is treated as not closed", "**Verify handoff**", "The writer receives `## Verification`",
		}},
		{"Simple Delegation", strings.TrimSpace(routing[start:end]), []string{
			"The delegation trigger stays mandatory", "requires one read-only explorer", "parent-context backstop",
			"After tooling/worktree incident:",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Main's canonical gate now requires the concrete runtime binding.
			// Compare its whole body after binding, not an unresolved template.
			tc.body = strings.ReplaceAll(tc.body, "{{GENTLE_AI_RUNTIME_AGENT_ID}}", string(model.AgentClaudeCode))
			for _, clause := range tc.clauses {
				if !strings.Contains(tc.body, clause) {
					t.Fatalf("canonical body no longer carries %q", clause)
				}
			}
			if got := strings.Count(bundle.core, tc.body); got != 1 {
				t.Errorf("core carries the whole canonical body %d times, want 1", got)
			}
			for _, block := range strings.Split(tc.body, "\n\n") {
				for _, module := range bundle.modules {
					if strings.Contains(module.content, block) {
						t.Errorf("%s carries core block %q", module.file, strings.SplitN(block, "\n", 2)[0])
					}
				}
			}
		})
	}
}

func TestBuildOrchestratorModulesRejectsUnsupportedBindings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent model.AgentID
		dir   string
		want  error
	}{
		{"codex runtime", model.AgentCodex, testModuleDir, errOrchestratorModulesUnsupported},
		{"pi runtime", model.AgentPi, testModuleDir, errOrchestratorModulesUnsupported},
		{"empty binding", model.AgentClaudeCode, "", errInvalidOrchestratorModuleDir},
		{"blank binding", model.AgentClaudeCode, "   ", errInvalidOrchestratorModuleDir},
		{"project-relative binding", model.AgentClaudeCode, ".claude/gentle-ai/orchestrator", errInvalidOrchestratorModuleDir},
		{"dot-relative binding", model.AgentClaudeCode, "./orchestrator", errInvalidOrchestratorModuleDir},
		{"drive-relative binding", model.AgentClaudeCode, `C:orchestrator`, errInvalidOrchestratorModuleDir},
		{"root-relative Windows binding", model.AgentClaudeCode, `\.claude\gentle-ai\orchestrator`, errInvalidOrchestratorModuleDir},
		{"padded Windows binding", model.AgentClaudeCode, ` C:\home\.claude\gentle-ai\orchestrator`, errInvalidOrchestratorModuleDir},
		{"backtick Windows binding", model.AgentClaudeCode, "C:\\home\\.claude\\`orchestrator`", errInvalidOrchestratorModuleDir},
		{"import binding", model.AgentClaudeCode, "@~/.claude/gentle-ai/orchestrator", errInvalidOrchestratorModuleDir},
		{"multiline binding", model.AgentClaudeCode, "~/.claude\n/orchestrator", errInvalidOrchestratorModuleDir},
		{"backtick binding", model.AgentClaudeCode, "~/.claude/`x`", errInvalidOrchestratorModuleDir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle, err := buildOrchestratorModules(tc.agent, fakeModuleContract, tc.dir)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if !reflect.DeepEqual(bundle, orchestratorModuleBundle{}) {
				t.Fatal("rejected binding returned a partial bundle")
			}
		})
	}

	// The public renderer of an unsupported runtime is unchanged and unmarked.
	codex, err := RenderOrchestratorWithSource(model.AgentCodex, fakeModuleContract, "")
	if err != nil || strings.Contains(codex, orchestratorFragmentMarkerToken) || !strings.Contains(codex, testModuleContract) {
		t.Fatalf("public Codex render changed: err=%v", err)
	}
}

// The marker grammar cases live in TestDecodeOrchestratorFragmentsFailsClosed;
// this test covers module assembly and that decoder errors propagate whole.
func TestSplitOrchestratorModulesFailsClosed(t *testing.T) {
	fragment, valid := markedTestFragment, testFragmentDoc
	specs := []orchestratorFragmentSpec{
		{id: "delegation.a", title: "A", reason: "a fires"},
		{id: "skills.c", title: "C", reason: "c fires"},
		{id: "delegation.b", title: "B", reason: "b fires"},
	}

	t.Run("valid", func(t *testing.T) {
		bundle, err := splitOrchestratorModules(valid, specs, testModuleDir)
		if err != nil {
			t.Fatalf("split error = %v", err)
		}
		pointer := func(module, title, reason string) string {
			return "On demand: when " + reason + ", read `" + testModuleDir + "/orchestrator-" + module + ".md`, section \"" + title + "\".\n"
		}
		wantCore := "# Core\n\n" + pointer("delegation", "A", "a fires") + "\nmid\n\n" + pointer("skills", "C", "c fires") +
			"\n" + pointer("delegation", "B", "b fires") + "\ntail\n"
		if bundle.core != wantCore {
			t.Fatalf("core = %q, want %q", bundle.core, wantCore)
		}
		wantModules := []orchestratorModule{
			{name: "delegation", file: "orchestrator-delegation.md", content: "# Gentle AI Orchestrator — Delegation Module\n\n" +
				orchestratorModuleIntro + "\n## A\n\nA body\n\n## B\n\nB body\n\n```text\ncode\n```\n"},
			{name: "skills", file: "orchestrator-skills.md", content: "# Gentle AI Orchestrator — Skills Module\n\n" +
				orchestratorModuleIntro + "\n## C\n\nC body\n"},
		}
		if !reflect.DeepEqual(bundle.modules, wantModules) {
			t.Fatalf("modules = %#v, want %#v", bundle.modules, wantModules)
		}
		got, err := bundle.reconstruct()
		if err != nil || got != stripOrchestratorFragmentMarkers(valid) || strings.Contains(got, orchestratorFragmentMarkerToken) {
			t.Fatalf("reconstruct() = %q, %v", got, err)
		}
	})

	for _, tc := range []struct {
		name, doc, want string
		specs           []orchestratorFragmentSpec
	}{
		{name: "decoder rejects a duplicate fragment", doc: valid + fragment("delegation.a", "again\n"), want: "duplicate"},
		{name: "decoder rejects a missing fragment", doc: "# Core\n" + fragment("delegation.a", "A\n") + fragment("delegation.b", "B\n"), want: "missing"},
		{name: "unresolved placeholder", doc: valid + "{{GENTLE_AI_ODD_SECTION:X}}\n", want: "unresolved"},
		{name: "unknown module", doc: valid, specs: append(append([]orchestratorFragmentSpec{}, specs...), orchestratorFragmentSpec{id: "bogus.x", title: "X", reason: "x"}), want: "unknown module"},
		{name: "duplicate spec", doc: valid, specs: append(append([]orchestratorFragmentSpec{}, specs...), specs[0]), want: "duplicate or incomplete"},
		{name: "spec without reason", doc: valid, specs: []orchestratorFragmentSpec{specs[0], specs[1], {id: "delegation.b", title: "B"}}, want: "duplicate or incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			use := specs
			if tc.specs != nil {
				use = tc.specs
			}
			bundle, err := splitOrchestratorModules(tc.doc, use, testModuleDir)
			if !errors.Is(err, errInvalidOrchestratorFragments) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %v containing %q", err, errInvalidOrchestratorFragments, tc.want)
			}
			if !reflect.DeepEqual(bundle, orchestratorModuleBundle{}) {
				t.Fatal("rejected fragments returned a partial bundle")
			}
		})
	}
}

// reconstruct is the test oracle that puts every fragment body back in place of its pointer, which
// yields the public monolithic render the bundle was split from.
func (b orchestratorModuleBundle) reconstruct() (string, error) {
	var out strings.Builder
	cursor := 0
	for _, fragment := range b.fragments {
		if fragment.offset < cursor || fragment.offset > len(b.core) || !strings.HasPrefix(b.core[fragment.offset:], fragment.pointer) {
			return "", fmt.Errorf("%w: fragment %q pointer is not at offset %d", errInvalidOrchestratorFragments, fragment.id, fragment.offset)
		}
		out.WriteString(b.core[cursor:fragment.offset])
		out.WriteString(fragment.body)
		cursor = fragment.offset + len(fragment.pointer)
	}
	out.WriteString(b.core[cursor:])
	return out.String(), nil
}
