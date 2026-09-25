package screens

import (
	"os"
	"testing"
)

func TestAgentBuilderHasNoSDDSelectionAsset(t *testing.T) {
	for _, name := range []string{"agent_builder_sdd.go", "agent_builder_sdd_test.go"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Errorf("retired SDD builder asset %s remains: %v", name, err)
		}
	}
}
