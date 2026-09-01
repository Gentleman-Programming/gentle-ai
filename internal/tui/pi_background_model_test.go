package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func piSDDReviewModel(background model.PiBackgroundIntent) Model {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenReview
	m.Cursor = 0
	m.Selection.Agents = []model.AgentID{model.AgentPi}
	m.Selection.Components = []model.ComponentID{model.ComponentEngram}
	m.PiBackgroundIntent = background
	return m
}

func TestPiBackgroundPromptVisibility(t *testing.T) {
	t.Setenv(cli.PiBackgroundSubagentsEnv, "")
	m := piSDDReviewModel("")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if state.Screen != ScreenPiBackground {
		t.Fatalf("screen = %v, want ScreenPiBackground", state.Screen)
	}
	if !strings.Contains(state.View(), "Enable managed background subagents") {
		t.Fatalf("pi background prompt missing enable choice:\n%s", state.View())
	}
}

func TestPiBackgroundPriorStateSkipsPrompt(t *testing.T) {
	t.Setenv(cli.PiBackgroundSubagentsEnv, "")
	for _, want := range []model.PiBackgroundIntent{model.PiBackgroundOn, model.PiBackgroundOff} {
		t.Run(string(want), func(t *testing.T) {
			m := piSDDReviewModel(want)
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			state := updated.(Model)
			if state.Screen != ScreenInstalling {
				t.Fatalf("screen = %v, want ScreenInstalling", state.Screen)
			}
			if state.PiBackgroundIntent != want || state.PiBackgroundPersist != "" {
				t.Fatalf("pi background intent/persist = %q/%q, want %q/empty", state.PiBackgroundIntent, state.PiBackgroundPersist, want)
			}
			if cmd == nil {
				t.Fatal("install command = nil")
			}
		})
	}
}

func TestPiBackgroundChoiceFeedsInstall(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cursor int
		want   model.PiBackgroundIntent
	}{
		{name: "enable managed background", cursor: 0, want: model.PiBackgroundOn},
		{name: "keep foreground", cursor: 1, want: model.PiBackgroundOff},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := piSDDReviewModel("")
			m.Screen = ScreenPiBackground
			m.Cursor = tt.cursor
			var got model.PiBackgroundIntent
			var gotPersist model.PiBackgroundIntent
			m.ExecuteFn = func(_ model.Selection, _ planner.ResolvedPlan, _ system.DetectionResult, _, _ model.OpenCodeBackgroundIntent, piBackground, piPersist model.PiBackgroundIntent, _ pipeline.ProgressFunc) pipeline.ExecutionResult {
				got = piBackground
				gotPersist = piPersist
				return pipeline.ExecutionResult{}
			}

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			state := updated.(Model)
			if state.Screen != ScreenInstalling || state.PiBackgroundIntent != tt.want || state.PiBackgroundPersist != tt.want {
				t.Fatalf("screen/pi background = %v/%q/%q, want installing/%q/%q", state.Screen, state.PiBackgroundIntent, state.PiBackgroundPersist, tt.want, tt.want)
			}
			if cmd == nil {
				t.Fatal("install command = nil")
			}
			if result, ok := cmd().(tea.BatchMsg); ok {
				for _, command := range result {
					if command == nil {
						continue
					}
					if _, ok := command().(PipelineDoneMsg); ok {
						break
					}
				}
			}
			if got != tt.want || gotPersist != tt.want {
				t.Fatalf("executor pi background/persist = %q/%q, want %q/%q", got, gotPersist, tt.want, tt.want)
			}
		})
	}
}

func TestPiBackgroundCancellationLeavesChoiceUnchanged(t *testing.T) {
	t.Setenv(cli.PiBackgroundSubagentsEnv, "")
	for _, tt := range []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "escape", key: tea.KeyMsg{Type: tea.KeyEsc}},
		{name: "back option", key: tea.KeyMsg{Type: tea.KeyEnter}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := piSDDReviewModel("")
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			state := updated.(Model)
			if state.Screen != ScreenPiBackground {
				t.Fatalf("screen = %v, want ScreenPiBackground", state.Screen)
			}
			if tt.name == "back option" {
				state.Cursor = len(screens.PiBackgroundOptions())
			}

			updated, _ = state.Update(tt.key)
			state = updated.(Model)
			if state.Screen != ScreenReview || state.PiBackgroundIntent != "" || state.PiBackgroundPersist != "" {
				t.Fatalf("cancelled state = %v/%q/%q, want review/empty/empty", state.Screen, state.PiBackgroundIntent, state.PiBackgroundPersist)
			}
		})
	}
}

func TestOpenCodeBackgroundPromptChainsIntoPiBackground(t *testing.T) {
	t.Setenv(cli.OpenCodeBackgroundSubagentsEnv, "")
	t.Setenv(cli.PiBackgroundSubagentsEnv, "")
	m := piSDDReviewModel("")
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode, model.AgentPi}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if state.Screen != ScreenOpenCodeBackground {
		t.Fatalf("screen = %v, want ScreenOpenCodeBackground first", state.Screen)
	}
	state.Cursor = 0 // enable managed OpenCode background
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)
	if state.Screen != ScreenPiBackground {
		t.Fatalf("screen = %v, want ScreenPiBackground after OpenCode choice", state.Screen)
	}
	if state.BackgroundIntent != model.OpenCodeBackgroundOn || state.BackgroundPersist != model.OpenCodeBackgroundOn {
		t.Fatalf("opencode choice lost: %q/%q", state.BackgroundIntent, state.BackgroundPersist)
	}
}
