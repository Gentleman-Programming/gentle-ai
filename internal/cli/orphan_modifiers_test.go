package cli

import (
	"os"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestRunInstallRejectsOrphanModifiers(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"persona", []string{"--persona", "neutral"}, "--persona requires component persona in the resolved install plan; rerun gentle-ai install with --component persona added to your existing component list, or remove --persona"},
		{"skill", []string{"--skill", "go-testing"}, "--skill/--skills requires component skills in the resolved install plan; rerun gentle-ai install with --component skills added to your existing component list, or remove --skill/--skills"},
		{"skills", []string{"--skills", "go-testing"}, "--skill/--skills requires component skills in the resolved install plan; rerun gentle-ai install with --component skills added to your existing component list, or remove --skill/--skills"},
	} {
		for _, dryRun := range []bool{true, false} {
			name := tc.name + "/real"
			if dryRun {
				name = tc.name + "/dry-run"
			}
			t.Run(name, func(t *testing.T) {
				home := t.TempDir()
				previous := osUserHomeDir
				osUserHomeDir = func() (string, error) { return home, nil }
				t.Cleanup(func() { osUserHomeDir = previous })
				args := append([]string{"--agent", "claude-code", "--component", "theme"}, tc.args...)
				if dryRun {
					args = append(args, "--dry-run")
				}
				_, err := RunInstall(args, system.DetectionResult{})
				if err == nil || err.Error() != tc.want {
					t.Fatalf("RunInstall() error = %v, want %q", err, tc.want)
				}
				entries, err := os.ReadDir(home)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Fatalf("rejected install wrote home entries: %v", entries)
				}
			})
		}
	}
}

func TestRunInstallModifierConsumersAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []model.ComponentID
	}{
		{"implicit persona with theme", []string{"--component", "theme"}, []model.ComponentID{model.ComponentTheme}},
		{"custom persona opt-out", []string{"--component", "theme", "--persona", "custom"}, []model.ComponentID{model.ComponentTheme}},
		{"explicit persona consumer", []string{"--component", "persona", "--persona", "neutral"}, []model.ComponentID{model.ComponentPersona}},
		{"explicit skill consumer", []string{"--component", "skills", "--skill", "go-testing"}, []model.ComponentID{model.ComponentSkills}},
		{"explicit skills alias consumer", []string{"--components", "skills", "--skills", "go-testing"}, []model.ComponentID{model.ComponentSkills}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			previous := osUserHomeDir
			osUserHomeDir = func() (string, error) { return home, nil }
			t.Cleanup(func() { osUserHomeDir = previous })
			args := append([]string{"--agent", "claude-code", "--dry-run"}, tc.args...)
			result, err := RunInstall(args, system.DetectionResult{})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(result.Resolved.OrderedComponents, tc.want) {
				t.Fatalf("components = %v, want %v", result.Resolved.OrderedComponents, tc.want)
			}
			if len(result.Execution.Apply.Steps) != 0 {
				t.Fatal("dry-run executed install")
			}
		})
	}
}
