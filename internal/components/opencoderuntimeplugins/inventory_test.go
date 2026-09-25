package opencoderuntimeplugins

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestManagedOpenCodePluginInventory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		agent     model.AgentID
		managed   []string
		lifecycle []string
		eligible  bool
	}{
		{"OpenCode", model.AgentOpenCode, []string{"model-variants.ts", "opencode-review-transport.ts", "skill-registry.ts"}, []string{"model-variants.ts", "opencode-review-transport.ts", "skill-registry.ts", "review-result-artifacts.ts"}, true},
		{"Kilocode", model.AgentKilocode, []string{"model-variants.ts", "skill-registry.ts"}, []string{"model-variants.ts", "skill-registry.ts", "review-result-artifacts.ts"}, true},
		{"Claude", model.AgentClaudeCode, nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ManagedPluginNames(tc.agent); !reflect.DeepEqual(got, tc.managed) {
				t.Errorf("managed = %v, want %v", got, tc.managed)
			}
			if got := OpenCodePluginLifecycleNames(tc.agent); !reflect.DeepEqual(got, tc.lifecycle) {
				t.Errorf("lifecycle = %v, want %v", got, tc.lifecycle)
			}
			if got := AgentReceivesManagedOpenCodePlugins(tc.agent); got != tc.eligible {
				t.Errorf("eligible = %v, want %v", got, tc.eligible)
			}
		})
	}
	if got := ManagedOpenCodePluginNames(); !reflect.DeepEqual(got, []string{"model-variants.ts", "opencode-review-transport.ts", "skill-registry.ts"}) {
		t.Errorf("default inventory = %v", got)
	}
}
