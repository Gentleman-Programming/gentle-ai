package providercontractbundle

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// reviewExecutionContractFor uses the non-SDD owner for the bundled runtime.
func reviewExecutionContractFor(agent model.AgentID) (string, error) {
	if agent != model.AgentPi {
		return "", fmt.Errorf("%s has no bundled review execution contract", agent)
	}
	return reviewassets.ReviewExecutionContractFor(agent)
}
