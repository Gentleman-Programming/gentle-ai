package reviewerprovider

import (
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestRuntimeBudgetLeavesContractResultLimitsUnchanged pins the output side of
// the byte policy: the runtime-context cap is an input budget owned by the
// runtime declaration, and it must never shrink Contract.ResultLimit, which
// owns the raw provider OUTPUT admission limit for every role.
func TestRuntimeBudgetLeavesContractResultLimitsUnchanged(t *testing.T) {
	for _, contract := range Contracts() {
		if contract.ResultLimit != 4<<20 {
			t.Fatalf("provider role %q ResultLimit = %d, want the unchanged %d byte output limit", contract.Role, contract.ResultLimit, 4<<20)
		}
	}
}

func TestTargetedValidatorContractDefinesPassedPolarity(t *testing.T) {
	contract, err := ContractFor(RoleTargetedValidator)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{"result schema", string(contract.ResultSchema), "true means the named check passed; false means the named check failed."},
		{"original criteria", contract.PromptInstruction, "Set `original_criteria.passed` to true only when every original criterion is met in the corrected candidate; set it to false when any original criterion remains unmet."},
		{"correction regression", contract.PromptInstruction, "Set `correction_regression.passed` to true only when the correction caused no regression; set it to false when you observe a regression."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.payload, tt.want) {
				t.Fatalf("targeted validator %s does not define passed polarity: missing %q", tt.name, tt.want)
			}
		})
	}
}

// TestTargetedValidatorContractDoesNotPromiseOmittedGeneratedContent pins the
// briefing against the evidence this role is actually handed.
// reviewProviderMaterializeEvidence gives it the same representation a lens
// receives, so a generated path arrives as a metadata summary with its content
// hunks omitted. A briefing that promised the complete patch for every path
// would have this role sign a verified verdict over bytes it never saw.
func TestTargetedValidatorContractDoesNotPromiseOmittedGeneratedContent(t *testing.T) {
	contract, err := ContractFor(RoleTargetedValidator)
	if err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{
		"already carries the complete frozen tree-to-tree patch",
		"for every path in `validation_request.correction_paths`. It is authoritative corrected-candidate content",
	} {
		if strings.Contains(contract.PromptInstruction, forbidden) {
			t.Fatalf("targeted validator briefing still promises complete content for every path: %q", forbidden)
		}
	}

	for _, required := range []string{
		`"content_omitted": true`,
		"Its content hunks are not in this input, so the summary alone never verifies a claim about what those hunks say.",
		"Use it whenever a check turns on a generated path's content, because that content reaches you no other way.",
		"When a check turns on a generated path's omitted content and you cannot run that command, that check carries no verdict: mark it unavailable rather than reading one out of the summary.",
	} {
		if !strings.Contains(contract.PromptInstruction, required) {
			t.Fatalf("targeted validator briefing omits the generated-path route: missing %q", required)
		}
	}
}

// severityRulePhrases are the concrete S12 rules every reviewer surface must
// carry. Concrete rules stay stable across models; abstract ones drift.
var severityRulePhrases = []string{
	"must be caused by this change",
	"does not already happen at the baseline",
	"reachable with realistic input",
	"was not asked to change",
	"out-of-domain values",
	"at most WARNING",
	"ignoring an explicit option or argument while reporting success",
	"unrequested changes to existing command output or messages",
}

func TestRefuterPromptAppliesSeverityRules(t *testing.T) {
	contract, err := ContractFor(RoleRefuter)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range append(slices.Clone(severityRulePhrases), "Refute a BLOCKER or CRITICAL claim that fails these conditions") {
		if !strings.Contains(contract.PromptInstruction, required) {
			t.Fatalf("refuter prompt omits severity rule %q:\n%s", required, contract.PromptInstruction)
		}
	}
}

// TestRefuterProbeInstructionIsRuntimeConditional pins S11: only a runtime
// whose adapter isolates a probe (Codex: a fresh scratch copy under the system
// temp dir, workspace-write confined to it, no network) is told it may run one
// reproducing command; every other runtime is told the probe is unavailable,
// by name, so no refuter ever believes a command ran when none could.
func TestRefuterProbeInstructionIsRuntimeConditional(t *testing.T) {
	codex := RefuterProbeInstruction("codex")
	for _, required := range []string{
		"one reproducing command", "scratch copy", "No network", "no installs",
		"cite the exact command and its observed output",
	} {
		if !strings.Contains(codex, required) {
			t.Fatalf("Codex refuter probe paragraph omits %q:\n%s", required, codex)
		}
	}
	if !RefuterProbeIsolated("codex") {
		t.Fatal("Codex must isolate the refuter probe")
	}
	for _, runtime := range []string{"claude-code", "pi", "opencode", ""} {
		if RefuterProbeIsolated(model.AgentID(runtime)) {
			t.Fatalf("runtime %q must stay no-probe", runtime)
		}
		instruction := RefuterProbeInstruction(model.AgentID(runtime))
		if strings.Contains(instruction, "one reproducing command") {
			t.Fatalf("no-probe runtime %q was offered a probe:\n%s", runtime, instruction)
		}
		if note := RefuterProbeUnavailableNote(model.AgentID(runtime)); !strings.Contains(instruction, note) || !strings.HasPrefix(note, "probe unavailable on ") {
			t.Fatalf("no-probe runtime %q instruction = %q, want the note %q", runtime, instruction, note)
		}
	}
	if note := RefuterProbeUnavailableNote("pi"); note != "probe unavailable on pi" {
		t.Fatalf("pi note = %q", note)
	}
}
