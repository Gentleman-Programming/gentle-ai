package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func openInstallationPlan(t *testing.T) (Model, shellinstaller.UserInstallRequest) {
	t.Helper()
	t.Setenv("HOME", shellTestHome(t))
	m := NewModel(system.DetectionResult{}, "plan-test").WithShellInstaller()
	m.Width, m.Height = 120, 30
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m, request := experiencePlan(t, m)
	return m, request
}

func TestShellPlanListsStepsAndDefaultTarget(t *testing.T) {
	m, request := openInstallationPlan(t)
	view := m.View()
	for _, part := range []string{"Installation plan", "1. Validate the selection and target", "2. Prepare the private stage and preferences", "3. Install the runtime and pinned suppliers", "4. Apply optional runtimes and the SDK channel", "5. Publish, verify and register the installation", "Enter: install"} {
		if !strings.Contains(view, part) {
			t.Fatalf("installation plan omits %q:\n%s", part, view)
		}
	}
	wantDestination := shellDefaultTestDestination()
	if request.Destination != wantDestination || request.Mode != "separate" || request.Confirmation == "" {
		t.Fatalf("plan request differs: %+v", request)
	}
	assertShellPrivateParent(t, filepath.Dir(request.Destination))
	if _, err := os.Lstat(request.Destination); !os.IsNotExist(err) {
		t.Fatal("plan created the destination before confirmation")
	}
}

func TestShellPlanEnterRunsInstallationInsideTUI(t *testing.T) {
	m, request := openInstallationPlan(t)
	var calls int
	m = m.WithShellInstallRunner(func(ctx context.Context, got shellinstaller.UserInstallRequest, report func(shellinstaller.UserInstallProgress)) error {
		calls++
		if got.Confirmation != request.Confirmation || got.Destination != request.Destination {
			return errors.New("runner received a different inspected request")
		}
		for index, label := range shellInstallStepLabels {
			report(shellinstaller.UserInstallProgress{Index: index + 1, Total: 5, Label: label, State: "running"})
			report(shellinstaller.UserInstallProgress{Index: index + 1, Total: 5, Label: label, State: "done"})
		}
		return nil
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	selection := m.ShellInstallSelection()
	if !selection.Confirmed || !selection.Executed || cmd == nil || m.shellInstallRun == nil {
		t.Fatalf("plan enter did not start the in-TUI worker: %+v", selection)
	}
	if m.shellPlan != nil || m.shellInstallRun.done {
		t.Fatal("plan transition exposed an unexpected state")
	}
	for m.shellInstallRun != nil && !m.shellInstallRun.done {
		if cmd == nil {
			t.Fatal("progress pump stopped before the worker result")
		}
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	if calls != 1 || m.shellInstallRun == nil || m.shellInstallRun.err != nil {
		t.Fatalf("in-TUI worker did not finish cleanly: calls=%d err=%v", calls, m.shellInstallRun.err)
	}
	for _, step := range m.shellInstallRun.steps {
		if step.State != "done" {
			t.Fatalf("step was not completed in order: %+v", m.shellInstallRun.steps)
		}
	}
	if view := m.View(); !strings.Contains(view, "Installation complete") || strings.Contains(view, "Welcome to the gentle-tailored experience") {
		t.Fatalf("completion screen differs:\n%s", view)
	}
	if cmd == nil {
		t.Fatal("completion did not start the rose animation")
	}
	for m.shellInstallRun.bloom < screens.ShellBloomFrames-1 {
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	if view := m.View(); !strings.Contains(view, "Welcome to the gentle-tailored experience") || !strings.Contains(view, "gentle-shell") || !strings.Contains(view, "Enter: finish") {
		t.Fatalf("completion copy did not appear after the animation:\n%s", view)
	}
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("completion enter did not leave the TUI")
	}
}

func TestShellInstallShowsCurrentInternalTask(t *testing.T) {
	m, _ := openInstallationPlan(t)
	release := make(chan struct{})
	m = m.WithShellInstallRunner(func(ctx context.Context, got shellinstaller.UserInstallRequest, report func(shellinstaller.UserInstallProgress)) error {
		for index := 1; index <= 2; index++ {
			report(shellinstaller.UserInstallProgress{Index: index, Total: 5, Label: shellInstallStepLabels[index-1], State: "running"})
			report(shellinstaller.UserInstallProgress{Index: index, Total: 5, Label: shellInstallStepLabels[index-1], State: "done"})
		}
		report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, Label: shellInstallStepLabels[2], State: "running"})
		report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, State: "detail", Detail: "Downloading Node 24.18.0"})
		<-release
		for index := 3; index <= 5; index++ {
			report(shellinstaller.UserInstallProgress{Index: index, Total: 5, Label: shellInstallStepLabels[index-1], State: "done"})
		}
		return nil
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(m.View(), "Downloading Node 24.18.0") {
		if cmd == nil {
			t.Fatal("progress pump stopped before the internal task")
		}
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	view := m.View()
	if !strings.Contains(view, "CURRENT TASK") || !strings.Contains(view, "Downloading Node 24.18.0") || !strings.Contains(view, "2/5") || !strings.Contains(view, "running") {
		t.Fatalf("internal task or progress is hidden:\n%s", view)
	}
	close(release)
	for !m.shellInstallRun.done {
		if cmd == nil {
			t.Fatal("progress pump stopped before completion")
		}
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	if !strings.Contains(m.View(), "Installation complete") {
		t.Fatalf("completion screen missing:\n%s", m.View())
	}
}

func TestShellMainFailureOffersQualifiedFallback(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("GENTLE_AI_MAIN_FALLBACK") })
	m, request := openInstallationPlan(t)
	profile := *request.Experience
	profile.SDKSource = "main-preview"
	request.Experience = &profile
	m.shellPlanRequest = request
	calls := 0
	m = m.WithShellInstallRunner(func(ctx context.Context, got shellinstaller.UserInstallRequest, report func(shellinstaller.UserInstallProgress)) error {
		calls++
		report(shellinstaller.UserInstallProgress{Index: 1, Total: 5, Label: shellInstallStepLabels[0], State: "running"})
		return errors.New("Main SDK + stock provider: private installation: nonzero")
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	for !m.shellInstallRun.done {
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	if !m.shellInstallRun.fallbackAvailable || !strings.Contains(m.View(), "try last qualified Main") {
		t.Fatalf("Main failure did not offer the qualified fallback:\n%s", m.View())
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	for cmd != nil && m.shellInstallRun != nil && !m.shellInstallRun.done {
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	if calls != 2 || os.Getenv("GENTLE_AI_MAIN_FALLBACK") != "1" {
		t.Fatalf("fallback did not launch exactly once: calls=%d env=%q", calls, os.Getenv("GENTLE_AI_MAIN_FALLBACK"))
	}
}

func TestShellInstallFailureStaysInTUI(t *testing.T) {
	m, _ := openInstallationPlan(t)
	m = m.WithShellInstallRunner(func(ctx context.Context, got shellinstaller.UserInstallRequest, report func(shellinstaller.UserInstallProgress)) error {
		report(shellinstaller.UserInstallProgress{Index: 1, Total: 5, Label: shellInstallStepLabels[0], State: "running"})
		report(shellinstaller.UserInstallProgress{Index: 1, Total: 5, Label: shellInstallStepLabels[0], State: "done"})
		report(shellinstaller.UserInstallProgress{Index: 2, Total: 5, Label: shellInstallStepLabels[1], State: "running"})
		return errors.New("private installation: uncertain; Preserve evidence: workspace=\"/tmp/ws\"")
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	for !m.shellInstallRun.done {
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	if m.shellInstallRun.err == nil || m.shellInstallRun.steps[0].State != "done" || m.shellInstallRun.steps[1].State != "failed" {
		t.Fatalf("failure did not preserve ordered step state: %+v", m.shellInstallRun)
	}
	view := m.View()
	if !strings.Contains(view, "Installation failed") || !strings.Contains(view, "workspace=") || !strings.Contains(view, "Enter: back to menu") || !strings.Contains(view, "No installation was applied") {
		t.Fatalf("failure screen hides evidence or next action:\n%s", view)
	}
	next, quit := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if quit != nil {
		t.Fatal("failure enter quit the TUI instead of returning to the menu")
	}
	if m.Screen != ScreenWelcome || m.ShellInstallSelection().Confirmed {
		t.Fatalf("failure did not stay owned by the TUI: screen=%v selection=%+v", m.Screen, m.ShellInstallSelection())
	}
}

func TestShellPlanEscapeAndCancel(t *testing.T) {
	m, _ := openInstallationPlan(t)
	back := experienceKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if back.shellExperience == nil || back.shellPlan != nil || back.shellPlanRequest.Confirmation != "" {
		t.Fatalf("plan escape did not return to Configure: %+v", back.shellExperience)
	}
	if back.ShellInstallSelection().Confirmed {
		t.Fatal("plan escape retained consent")
	}
	cancel := experienceKey(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cancel.Screen != ScreenWelcome || len(cli.ShellInstallArguments(cancel.ShellInstallSelection())) != 0 {
		t.Fatal("plan ctrl-c did not cancel to Welcome")
	}
}
