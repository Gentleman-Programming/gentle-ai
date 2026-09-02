package opencode

import (
	"reflect"
	"testing"
)

func TestModelEffortLevels(t *testing.T) {
	tests := []struct {
		name     string
		variants []string
		want     []string
	}{
		{name: "no variants", variants: nil, want: nil},
		{name: "reasoning levels", variants: []string{"high", "low", "medium"}, want: []string{"high", "low", "medium"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Model{Variants: tt.variants}).EffortLevels(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EffortLevels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewPhasesCompleteRuntimeSet(t *testing.T) {
	want := []string{
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
		"review-refuter",
		"review-validator",
	}
	if got := ReviewPhases(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReviewPhases() = %v, want %v", got, want)
	}

	configurable := ConfigurableAgentPhases()
	if got := configurable[len(configurable)-len(want):]; !reflect.DeepEqual(got, want) {
		t.Fatalf("ConfigurableAgentPhases() review suffix = %v, want %v", got, want)
	}
}

func TestNativeFallbackPhasesAreConfigurable(t *testing.T) {
	want := []string{"general", "explore"}
	if got := NativeFallbackPhases(); !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeFallbackPhases() = %v, want %v", got, want)
	}

	configurable := ConfigurableAgentPhases()
	for _, phase := range want {
		found := false
		for _, candidate := range configurable {
			if candidate == phase {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ConfigurableAgentPhases() missing native fallback %q: %v", phase, configurable)
		}
	}
}
