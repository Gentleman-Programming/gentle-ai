package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v5/internal/shellinstaller"
)

// failedMainRun drives a Main-preview plan into a failed run whose runner
// returns err, and returns the model on the failure screen.
func failedMainRun(t *testing.T, err error) Model {
	t.Helper()
	m, request := openInstallationPlan(t)
	profile := *request.Experience
	profile.SDKSource = "main-preview"
	request.Experience = &profile
	m.shellPlanRequest = request
	m = m.WithShellInstallRunner(func(context.Context, shellinstaller.UserInstallRequest, func(shellinstaller.UserInstallProgress)) error {
		return err
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	for !m.shellInstallRun.done {
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	return m
}

// shellCancelKeys names subtests with path-safe words: t.TempDir embeds the
// subtest name and the installer refuses non-canonical target characters.
var shellCancelKeys = map[string]tea.KeyMsg{"esc": {Type: tea.KeyEsc}, "ctrlc": {Type: tea.KeyCtrlC}}

func unsetShellRunEnv(t *testing.T) {
	t.Helper()
	// t.Setenv restores each variable when the test ends, including the
	// registry home the flow sets process-wide.
	for _, name := range []string{"GENTLE_AI_MAIN_FALLBACK", "GENTLE_AI_RECONFIGURE_TARGET", "GENTLE_AI_SHELL_REGISTRY_HOME"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
}

// Esc and Ctrl+C on the Main fallback offer decline it and return to the
// menu, clearing the fallback and reconfigure flags (#6176).
func TestShellMainFallbackEscAndCtrlCDecline(t *testing.T) {
	for name, key := range shellCancelKeys {
		t.Run(name, func(t *testing.T) {
			unsetShellRunEnv(t)
			m := failedMainRun(t, fmt.Errorf("Main SDK + stock provider: %w", shellinstaller.ErrMainSDKInstall))
			if !m.shellInstallRun.fallbackAvailable {
				t.Fatalf("typed Main failure did not offer the fallback:\n%s", m.View())
			}
			_ = os.Setenv("GENTLE_AI_MAIN_FALLBACK", "1")
			_ = os.Setenv("GENTLE_AI_RECONFIGURE_TARGET", "target")
			next, cmd := m.Update(key)
			m = next.(Model)
			if cmd != nil {
				t.Fatalf("%s launched a command instead of declining", key)
			}
			if m.shellInstallRun != nil || m.Screen != ScreenWelcome || m.shellInstallSelection.Confirmed {
				t.Fatalf("%s did not return to the menu: screen=%v run=%+v", key, m.Screen, m.shellInstallRun)
			}
			if _, set := os.LookupEnv("GENTLE_AI_MAIN_FALLBACK"); set {
				t.Fatal("declined fallback kept GENTLE_AI_MAIN_FALLBACK")
			}
			if _, set := os.LookupEnv("GENTLE_AI_RECONFIGURE_TARGET"); set {
				t.Fatal("declined fallback kept GENTLE_AI_RECONFIGURE_TARGET")
			}
		})
	}
}

// The fallback is recognized through the typed error in-process and through
// the worker's "Main SDK" output across a process boundary.
func TestShellMainFallbackRecognition(t *testing.T) {
	unsetShellRunEnv(t)
	if m := failedMainRun(t, errors.New("unrelated install failure")); m.shellInstallRun.fallbackAvailable {
		t.Fatal("unrelated error offered the fallback")
	}
	if m := failedMainRun(t, errors.New("exit status 1; worker output: Main SDK + stock provider: nonzero")); !m.shellInstallRun.fallbackAvailable {
		t.Fatal("worker output naming Main SDK did not offer the fallback")
	}
	reworded := fmt.Errorf("composition refused: %w", shellinstaller.ErrMainSDKInstall)
	if m := failedMainRun(t, reworded); !m.shellInstallRun.fallbackAvailable || !strings.Contains(m.View(), "try last qualified Main") {
		t.Fatalf("typed Main failure with other wording did not offer the fallback:\n%s", m.View())
	}
}

// A failed fallback that returns to the menu clears the fallback flag, so a
// later run in the same TUI session does not inherit it.
func TestShellFailedRunBackToMenuClearsFlags(t *testing.T) {
	unsetShellRunEnv(t)
	m := failedMainRun(t, errors.New("private installation: nonzero"))
	_ = os.Setenv("GENTLE_AI_MAIN_FALLBACK", "1")
	_ = os.Setenv("GENTLE_AI_RECONFIGURE_TARGET", "target")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellInstallRun != nil || m.Screen != ScreenWelcome {
		t.Fatalf("failure screen did not return to the menu: %+v", m.shellInstallRun)
	}
	for _, name := range []string{"GENTLE_AI_MAIN_FALLBACK", "GENTLE_AI_RECONFIGURE_TARGET"} {
		if _, set := os.LookupEnv(name); set {
			t.Fatalf("return to menu kept %s", name)
		}
	}
}

// Esc on the plan returns to configuration without leaking the reconfigure
// target (previously only Ctrl+C cleared it).
func TestShellPlanEscClearsReconfigureTarget(t *testing.T) {
	unsetShellRunEnv(t)
	m, _ := openInstallationPlan(t)
	_ = os.Setenv("GENTLE_AI_RECONFIGURE_TARGET", "target")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.shellPlan != nil || m.shellExperience == nil {
		t.Fatalf("Esc did not return to configuration: plan=%v", m.shellPlan)
	}
	if _, set := os.LookupEnv("GENTLE_AI_RECONFIGURE_TARGET"); set {
		t.Fatal("Esc on the plan kept GENTLE_AI_RECONFIGURE_TARGET")
	}
}

// channelOverlayPlan opens the Update plan for a Stable -> Main channel-only
// change and returns the model with the previous persisted channel.
func channelOverlayPlan(t *testing.T) (Model, shellinstaller.UserShellInstall, string) {
	t.Helper()
	skipUnqualifiedMainPreview(t)
	_, target := managedInstallFixture(t)
	install := shellinstaller.UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}
	loaded, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		t.Fatal(err)
	}
	selected := loaded
	selected.SDKSource = "main-preview"
	m := shellUpdateModel(t, install, selected)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellPlan == nil {
		t.Fatal("channel-only delta did not prepare the plan")
	}
	return m, install, loaded.SDKSource
}

func assertShellChannel(t *testing.T, install shellinstaller.UserShellInstall, want string) {
	t.Helper()
	got, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil || got.SDKSource != want {
		t.Fatalf("persisted channel = %q (%v), want %q", got.SDKSource, err, want)
	}
}

// Cancelling the channel plan restores the previously persisted channel.
func TestShellChannelPlanCancelRestoresPreference(t *testing.T) {
	for name, key := range shellCancelKeys {
		t.Run(name, func(t *testing.T) {
			unsetShellRunEnv(t)
			m, install, previous := channelOverlayPlan(t)
			next, _ := m.Update(key)
			m = next.(Model)
			if m.shellPlan != nil {
				t.Fatal("cancel kept the plan")
			}
			assertShellChannel(t, install, previous)
		})
	}
}

// A failed channel run that returns to the menu restores the previous
// channel; a successful run keeps the new one.
func TestShellChannelRunRestoresPreferenceOnlyOnFailure(t *testing.T) {
	for _, fail := range []bool{true, false} {
		t.Run(map[bool]string{true: "failure", false: "success"}[fail], func(t *testing.T) {
			unsetShellRunEnv(t)
			m, install, previous := channelOverlayPlan(t)
			m = m.WithShellInstallRunner(func(context.Context, shellinstaller.UserInstallRequest, func(shellinstaller.UserInstallProgress)) error {
				if fail {
					return errors.New("overlay refused")
				}
				return nil
			})
			m.shellRootRisk = nil
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			for m.shellInstallRun != nil && !m.shellInstallRun.done {
				next, cmd = m.Update(cmd())
				m = next.(Model)
			}
			if m.shellInstallRun == nil {
				t.Fatal("plan confirmation did not run the update")
			}
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			if fail {
				assertShellChannel(t, install, previous)
			} else {
				assertShellChannel(t, install, "main-preview")
			}
		})
	}
}
