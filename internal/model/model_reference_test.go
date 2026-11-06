package model

import "testing"

func TestParseModelReference(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input any
		want  ModelAssignment
		ok    bool
	}{
		{"nested string", "openrouter/vendor/model#high", ModelAssignment{"openrouter", "vendor/model", "high"}, true},
		{"legacy colon", "local:coder:latest", ModelAssignment{"local", "coder:latest", ""}, true},
		{"expanded", map[string]any{"providerID": "local", "model": "coder", "variant": "deep"}, ModelAssignment{"local", "coder", "deep"}, true},
		{"wrong catalog key", map[string]any{"providerID": "local", "modelID": "coder"}, ModelAssignment{}, false},
		{"empty variant", "local/model#", ModelAssignment{}, false},
		{"multiple variants", "local/model#high#low", ModelAssignment{}, false},
		{"provider hash", "lo#cal/model", ModelAssignment{}, false},
		{"invalid type", 42, ModelAssignment{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseModelReference(tt.input)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("got (%+v,%v), want (%+v,%v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
