package opencode

import "testing"

func TestNativeConfigPreservesAmbiguousPluralMap(t *testing.T) {
	for _, tt := range []struct {
		name string
		root map[string]any
		want bool
	}{
		{"model only plural", map[string]any{"agents": map[string]any{"worker": map[string]any{"model": "local/coder"}}}, true},
		{"historical plural alias", map[string]any{"agents": map[string]any{"worker": map[string]any{"prompt": "Old", "variant": "high"}}}, false},
		{"native system", map[string]any{"agents": map[string]any{"worker": map[string]any{"system": "Native"}}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := NativeConfig(tt.root); got != tt.want {
				t.Fatalf("native=%v want %v", got, tt.want)
			}
		})
	}
}
