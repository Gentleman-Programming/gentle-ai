package opencode

import (
	"os"
	"path/filepath"
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

func TestDefaultSettingsPathForHomeRejectsRelativeXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative-config")

	want := filepath.Join(home, ".config", "opencode", "opencode.json")
	if got := DefaultSettingsPathForHome(home); got != want {
		t.Fatalf("DefaultSettingsPathForHome() = %q, want %q", got, want)
	}
}

func TestDefaultSettingsPathForHomeEmptyHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(os.TempDir(), "xdg-config"))

	if got := DefaultSettingsPathForHome(""); got != "" {
		t.Fatalf("DefaultSettingsPathForHome(empty) = %q, want empty", got)
	}
}
