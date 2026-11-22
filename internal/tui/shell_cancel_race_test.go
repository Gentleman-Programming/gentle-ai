package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v5/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
)

// blockingRun launches a run whose runner reports the given step as running
// and then blocks until its context is cancelled or release is closed.
func blockingRun(t *testing.T, step int, release <-chan struct{}) (Model, tea.Cmd, <-chan context.Context) {
	t.Helper()
	unsetShellRunEnv(t)
	contexts := make(chan context.Context, 1)
	m := NewModel(system.DetectionResult{}, "cancel-test").WithShellInstaller()
	m.Width, m.Height = 120, 40
	m.Screen = ScreenShellInstall
	m = m.WithShellInstallRunner(func(ctx context.Context, _ shellinstaller.UserInstallRequest, report func(shellinstaller.UserInstallProgress)) error {
		contexts <- ctx
		report(shellinstaller.UserInstallProgress{Index: step, Total: 5, State: "running"})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	})
	next, cmd := m.launchShellInstall(shellinstaller.UserInstallRequest{Destination: "/target", Mode: "separate"}, false)
	m = next.(Model)
	// Pump until the runner's progress event is applied.
	for applied := false; !applied; {
		msg := cmd()
		_, applied = msg.(shellInstallEvent)
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd, contexts
}

func pumpShellRun(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !m.shellInstallRun.done {
		if cmd == nil || time.Now().After(deadline) {
			t.Fatal("run did not finish")
		}
		var next tea.Model
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	return m
}

func key(m Model, msg tea.KeyMsg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// Esc before publication asks for confirmation; confirming cancels the
// worker context and the run fails with a clear cancellation (#6177).
func TestShellRunCancelBeforePublicationAfterConfirmation(t *testing.T) {
	for name, confirm := range map[string]tea.KeyMsg{"enter": {Type: tea.KeyEnter}, "ctrlc": {Type: tea.KeyCtrlC}} {
		t.Run(name, func(t *testing.T) {
			m, cmd, contexts := blockingRun(t, 3, nil)
			ctx := <-contexts
			m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
			if ctx.Err() != nil {
				t.Fatal("Esc cancelled without confirmation")
			}
			if view := m.View(); !strings.Contains(view, "Cancel the installation?") {
				t.Fatalf("cancel confirmation not shown:\n%s", view)
			}
			m = key(m, confirm)
			if ctx.Err() == nil {
				t.Fatal("confirmed cancel did not cancel the worker context")
			}
			m = pumpShellRun(t, m, cmd)
			err := m.shellInstallRun.err
			if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "cancelled before publication") {
				t.Fatalf("cancelled run error = %v", err)
			}
			if m.shellInstallRun.fallbackAvailable {
				t.Fatal("cancelled run offered the Main fallback")
			}
			if view := m.View(); !strings.Contains(view, "Installation failed") || !strings.Contains(view, "cancelled before publication") {
				t.Fatalf("cancelled run screen:\n%s", view)
			}
		})
	}
}

// Declining the confirmation keeps the run going.
func TestShellRunCancelDeclinedKeepsRunning(t *testing.T) {
	release := make(chan struct{})
	m, cmd, contexts := blockingRun(t, 2, release)
	ctx := <-contexts
	m = key(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if ctx.Err() != nil || m.shellInstallRun.confirmCancel {
		t.Fatalf("declined cancel stopped the run: ctx=%v confirm=%v", ctx.Err(), m.shellInstallRun.confirmCancel)
	}
	close(release)
	m = pumpShellRun(t, m, cmd)
	if m.shellInstallRun.err != nil {
		t.Fatalf("run failed after a declined cancel: %v", m.shellInstallRun.err)
	}
}

// After publication starts, Esc/Ctrl+C never cancel and explain why.
func TestShellRunCannotCancelDuringPublication(t *testing.T) {
	release := make(chan struct{})
	m, cmd, contexts := blockingRun(t, 5, release)
	ctx := <-contexts
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyEsc}, {Type: tea.KeyCtrlC}, {Type: tea.KeyEnter}} {
		m = key(m, msg)
	}
	if ctx.Err() != nil || m.shellInstallRun.confirmCancel {
		t.Fatal("publication was cancelled")
	}
	if view := m.View(); !strings.Contains(view, "cannot be cancelled") {
		t.Fatalf("publication screen does not explain why it cannot be cancelled:\n%s", view)
	}
	close(release)
	m = pumpShellRun(t, m, cmd)
	if m.shellInstallRun.err != nil {
		t.Fatalf("publication failed: %v", m.shellInstallRun.err)
	}
}

// A confirmation open when publication starts is withdrawn.
func TestShellRunPublicationWithdrawsPendingConfirmation(t *testing.T) {
	m, _, contexts := blockingRun(t, 4, nil)
	ctx := <-contexts
	m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
	next, _ := m.Update(shellInstallEvent{progress: shellinstaller.UserInstallProgress{Index: 5, Total: 5, State: "running"}})
	m = next.(Model)
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if ctx.Err() != nil || m.shellInstallRun.confirmCancel {
		t.Fatal("a stale confirmation cancelled publication")
	}
}

// A cancel accepted just before the worker reported publication is not
// presented as a clean cancellation.
func TestShellRunCancelRacingPublicationIsReportedAccurately(t *testing.T) {
	m, cmd, contexts := blockingRun(t, 4, nil)
	<-contexts
	m = key(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = key(m, tea.KeyMsg{Type: tea.KeyEnter})
	// The worker had already crossed into publication when the cancel landed.
	next, _ := m.Update(shellInstallEvent{progress: shellinstaller.UserInstallProgress{Index: 5, Total: 5, State: "running"}})
	m = next.(Model)
	m = pumpShellRun(t, m, cmd)
	err := m.shellInstallRun.err
	if err == nil || strings.Contains(err.Error(), "cancelled before publication") || !strings.Contains(err.Error(), "publication had already started") {
		t.Fatalf("racing cancel error = %v", err)
	}
}
