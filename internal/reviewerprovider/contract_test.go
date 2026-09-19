package reviewerprovider

import (
	"strings"
	"testing"
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
