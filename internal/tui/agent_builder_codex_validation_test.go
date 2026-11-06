package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestAgentBuilderValidatesCodexHomeBeforeInstalling(t *testing.T) {
	for _, includeCodex := range []bool{true, false} {
		name := "Codex selected"
		if !includeCodex {
			name = "non-Codex install is unaffected"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			invalid := filepath.Join(t.TempDir(), "missing")
			t.Setenv("CODEX_HOME", invalid)
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen = ScreenAgentBuilderPreview
			m.AgentBuilder.Generated = &agentbuilder.GeneratedAgent{Name: "validation-probe", Content: "# Validation probe\n"}
			m.AgentBuilder.AvailableEngines = []model.AgentID{model.AgentClaudeCode}
			if includeCodex {
				m.AgentBuilder.AvailableEngines = append(m.AgentBuilder.AvailableEngines, model.AgentCodex)
			}
			want := system.ValidateCodexHome(home)
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("install command missing")
			}
			batch, ok := cmd().(tea.BatchMsg)
			if !ok || len(batch) < 2 {
				t.Fatal("expected installation batch")
			}
			done, ok := batch[len(batch)-1]().(AgentBuilderInstallDoneMsg)
			if !ok {
				t.Fatal("installation did not return its result")
			}
			if !includeCodex {
				if done.Err != nil || len(done.Results) != 1 || !done.Results[0].Success {
					t.Fatalf("non-Codex install failed: %#v", done)
				}
				return
			}
			if done.Err == nil || !strings.Contains(done.Err.Error(), want.Error()) {
				t.Errorf("error = %v, want CODEX_HOME validation error %q", done.Err, want)
			}
			if len(done.Results) != 0 {
				t.Errorf("invalid Codex home returned installation results: %#v", done.Results)
			}
			if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
				t.Errorf("refused batch wrote another adapter or registry first: entries=%v err=%v", entries, err)
			}
			if _, err := os.Stat(invalid); !os.IsNotExist(err) {
				t.Errorf("invalid override was created: %v", err)
			}
		})
	}
}
