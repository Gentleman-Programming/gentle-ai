package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/screens"
)

func TestRunningScreensRejectInputAndExposeNoOptions(t *testing.T) {
	for _, screen := range []Screen{ScreenRestoreConfirm, ScreenSync, ScreenUpgradeSync, ScreenOpenCodePlugins, ScreenUninstallConfirm, ScreenReviewStoreResetConfirm} {
		t.Run(screenName(screen), func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen = screen
			m.OperationRunning = true
			m.Cursor = 1
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := updated.(Model)
			if cmd != nil || got.Screen != screen || got.Cursor != 1 {
				t.Fatalf("running input changed state: screen=%v cursor=%d cmd=%v", got.Screen, got.Cursor, cmd)
			}
			if got.optionCount() != 0 {
				t.Fatalf("optionCount() = %d, want 0 while running", got.optionCount())
			}
			if got.View() == "" {
				t.Fatal("running screen should render progress")
			}
		})
	}
}

func TestResultScreensDoNotExposePhantomCursorRows(t *testing.T) {
	for _, screen := range []Screen{ScreenRestoreResult, ScreenDeleteResult, ScreenUninstallResult, ScreenOpenCodePluginResult, ScreenCommunityToolResult, ScreenComplete, ScreenReviewStoreResetResult} {
		m := NewModel(system.DetectionResult{}, "dev")
		m.Screen = screen
		if got := m.optionCount(); got != 0 {
			t.Errorf("%s optionCount() = %d, want 0", screenName(screen), got)
		}
	}
}

func TestConditionalPickerNavigationResetsState(t *testing.T) {
	t.Run("empty model picker back returns to configuration", func(t *testing.T) {
		m := NewModel(system.DetectionResult{}, "dev")
		m.Screen, m.ModelConfigMode, m.Cursor = ScreenModelPicker, true, 1
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if got := updated.(Model).Screen; got != ScreenModelConfig {
			t.Fatalf("screen = %v, want %v", got, ScreenModelConfig)
		}
	})

	for _, tc := range []struct {
		name   string
		screen Screen
	}{
		{"Kiro", ScreenKiroModelPicker},
		{"Codex", ScreenCodexModelPicker},
	} {
		t.Run(tc.name+" custom starts at first phase", func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen, m.Cursor = tc.screen, 3
			if tc.screen == ScreenKiroModelPicker {
				m.Cursor = 4 // Kiro has an additional Open Weight preset before Custom.
				m.KiroModelPicker = screens.NewKiroModelPickerState()
			} else {
				m.CodexModelPicker = screens.NewCodexModelPickerState()
			}
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if got := updated.(Model).Cursor; got != 0 {
				t.Fatalf("cursor = %d, want 0", got)
			}
		})
	}
}

func TestBackAndEscShareCleanupContracts(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyEsc} {
		t.Run(key.String(), func(t *testing.T) {
			refreshes := 0
			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen, m.DeleteErr = ScreenDeleteResult, errors.New("old")
			m.ListBackupsFn = func() []backup.Manifest { refreshes++; return nil }
			updated, _ := m.Update(tea.KeyMsg{Type: key})
			got := updated.(Model)
			if got.Screen != ScreenBackups || got.DeleteErr != nil || refreshes != 1 {
				t.Fatalf("delete result cleanup mismatch: screen=%v err=%v refreshes=%d", got.Screen, got.DeleteErr, refreshes)
			}
		})
	}

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen, m.UninstallMode = ScreenUninstall, model.UninstallModePartial
	m.Cursor = len(screens.UninstallAgentOptions()) + 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(Model).Screen; got != ScreenUninstallMode {
		t.Fatalf("uninstall Back screen = %v, want %v", got, ScreenUninstallMode)
	}

	m.Screen = ScreenUpdatePrompt
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := updated.(Model).Screen; got != ScreenWelcome {
		t.Fatalf("update prompt Esc screen = %v, want %v", got, ScreenWelcome)
	}
}

func TestAsyncCompletionCannotReenterAbandonedFlow(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen, m.OperationRunning = ScreenWelcome, true
	updated, _ := m.Update(BackupRestoreMsg{Err: errors.New("late")})
	got := updated.(Model)
	if got.Screen != ScreenWelcome || got.RestoreErr != nil {
		t.Fatalf("late restore changed abandoned flow: screen=%v err=%v", got.Screen, got.RestoreErr)
	}
	updated, _ = m.Update(OpenCodePluginRegistrationDoneMsg{})
	if got := updated.(Model); got.Screen != ScreenWelcome {
		t.Fatalf("late plugin completion changed screen to %v", got.Screen)
	}
}
