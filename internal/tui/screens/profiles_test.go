package screens_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

// helper to build a simple Profile for tests.
func makeProfile(name string, orchProvider, orchModel string) model.Profile {
	return model.Profile{
		Name: name,
		OrchestratorModel: model.ModelAssignment{
			ProviderID: orchProvider,
			ModelID:    orchModel,
		},
		PhaseAssignments: map[string]model.ModelAssignment{},
	}
}

// ─── RenderProfiles ───────────────────────────────────────────────────────────

func TestRenderProfiles_TitleIsPresent(t *testing.T) {
	profiles := []model.Profile{makeProfile("cheap", "anthropic", "claude-haiku-4")}
	output := screens.RenderProfiles(profiles, 0, nil)

	if !strings.Contains(output, "OpenCode SDD Profiles") {
		t.Errorf("expected title 'OpenCode SDD Profiles' in output, got:\n%s", output)
	}
}

func TestRenderProfiles_ShowsProfileNamesWithProviderModel(t *testing.T) {
	profiles := []model.Profile{
		makeProfile("cheap", "anthropic", "claude-haiku-4"),
		makeProfile("premium", "openai", "gpt-4o"),
	}
	output := screens.RenderProfiles(profiles, 0, nil)

	if !strings.Contains(output, "cheap") {
		t.Errorf("expected 'cheap' profile name in output")
	}
	if !strings.Contains(output, "premium") {
		t.Errorf("expected 'premium' profile name in output")
	}
	if !strings.Contains(output, "anthropic") {
		t.Errorf("expected provider 'anthropic' in output")
	}
	if !strings.Contains(output, "claude-haiku-4") {
		t.Errorf("expected model 'claude-haiku-4' in output")
	}
}

func TestRenderProfiles_ShowsCreateNewProfile(t *testing.T) {
	profiles := []model.Profile{}
	output := screens.RenderProfiles(profiles, 0, nil)

	if !strings.Contains(output, "Create new profile") {
		t.Errorf("expected 'Create new profile' action in output")
	}
}

func TestRenderProfiles_ShowsBackOption(t *testing.T) {
	profiles := []model.Profile{}
	output := screens.RenderProfiles(profiles, 0, nil)

	if !strings.Contains(output, "Back") {
		t.Errorf("expected 'Back' option in output")
	}
}

func TestRenderProfiles_ShowsKeybindingHints(t *testing.T) {
	profiles := []model.Profile{}
	output := screens.RenderProfiles(profiles, 0, nil)

	if !strings.Contains(output, "n: new") {
		t.Errorf("expected 'n: new' keybinding hint in output")
	}
	if !strings.Contains(output, "d: delete") {
		t.Errorf("expected 'd: delete' keybinding hint in output")
	}
	if !strings.Contains(output, "enter: edit") {
		t.Errorf("expected 'enter: edit' keybinding hint in output")
	}
}

func TestRenderProfiles_ShowsDeleteErrorWhenNonNil(t *testing.T) {
	profiles := []model.Profile{makeProfile("cheap", "anthropic", "claude-haiku-4")}
	err := fmt.Errorf("failed to write opencode.json")
	output := screens.RenderProfiles(profiles, 0, err)

	if !strings.Contains(output, "failed to write opencode.json") {
		t.Errorf("expected delete error message in output, got:\n%s", output)
	}
}

// ─── ProfileListOptionCount ───────────────────────────────────────────────────

func TestProfileListOptionCount_EmptyList(t *testing.T) {
	profiles := []model.Profile{}
	count := screens.ProfileListOptionCount(profiles)

	// 0 profiles + "Create new profile" + "Back" = 2
	if count != 2 {
		t.Errorf("expected option count 2 for empty list, got %d", count)
	}
}

func TestProfileListOptionCount_WithProfiles(t *testing.T) {
	profiles := []model.Profile{
		makeProfile("cheap", "anthropic", "claude-haiku-4"),
		makeProfile("premium", "openai", "gpt-4o"),
		makeProfile("smart", "google", "gemini-pro"),
	}
	count := screens.ProfileListOptionCount(profiles)

	// 3 profiles + "Create new profile" + "Back" = 5
	if count != 5 {
		t.Errorf("expected option count 5 for 3 profiles, got %d", count)
	}
}
