package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestProviderRoleCollectInputNamesObeyPublishedPattern pins every provider
// role collection input name to the published transition_input name pattern
// (^[a-z0-9_]+$, status-v5.schema.json and gentle-pi's own decoder). The
// battery found the targeted-validator slot publishing the raw hyphenated
// role token, which no published consumer can admit.
func TestProviderRoleCollectInputNamesObeyPublishedPattern(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile("^[a-z0-9_]+$")
	binding := ReviewTransitionBinding{
		LineageID:         "published-schema-role",
		Revision:          "sha256:" + strings.Repeat("a", 64),
		TargetIdentity:    "sha256:" + strings.Repeat("b", 64),
		RepositoryContext: "rctx1_" + strings.Repeat("c", 64),
	}

	transition := reviewProviderRoleTransition("provider_refuter_required", binding, reviewerprovider.RoleTargetedValidator, model.AgentOpenCode, nil)
	if transition.Kind != "collect" || transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("targeted-validator transition = %#v", transition)
	}
	input := transition.Collect.Inputs[0]
	if !pattern.MatchString(input.Name) {
		t.Fatalf("targeted-validator collect input name %q violates the published ^[a-z0-9_]+$ pattern", input.Name)
	}
	if input.Name != "provider_targeted_validator" {
		t.Fatalf("targeted-validator collect input name = %q, want provider_targeted_validator", input.Name)
	}

	refuter := reviewProviderRoleTransition("provider_refuter_required", binding, reviewerprovider.RoleRefuter, model.AgentOpenCode, nil)
	if refuter.Collect == nil || len(refuter.Collect.Inputs) != 1 || refuter.Collect.Inputs[0].Name != "provider_refuter" {
		t.Fatalf("refuter transition = %#v", refuter)
	}

	validation := &reviewtransaction.TargetedValidationRequest{RequestHash: "sha256:" + strings.Repeat("d", 64)}
	relay, err := reviewProviderHostRelayRoleInput(binding, reviewerprovider.RoleTargetedValidator, model.AgentPi, validation)
	if err != nil {
		t.Fatal(err)
	}
	if !pattern.MatchString(relay.Name) || relay.Name != "provider_targeted_validator" {
		t.Fatalf("host-relay targeted-validator input name = %q, want provider_targeted_validator", relay.Name)
	}
}
