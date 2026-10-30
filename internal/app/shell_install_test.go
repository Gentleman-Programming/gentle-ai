package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui"
)

func TestShellInstallWelcomeMainTUI(t *testing.T) {
	assumeInteractiveTTY(t)
	setupMockHome(t, t.TempDir())
	oldOS, oldDetect, oldTUI := ensureCurrentOSSupported, detectSystem, runTUI
	t.Cleanup(func() { ensureCurrentOSSupported, detectSystem, runTUI = oldOS, oldDetect, oldTUI })
	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true}}, nil
	}
	calls := 0
	runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
		calls++
		if !strings.Contains(m.View(), "Install Gentle-Shell, our own agent") {
			t.Fatal("ordinary application does not expose the embedded installer")
		}
		return m, nil
	}
	if err := RunArgs(nil, &bytes.Buffer{}); err != nil || calls != 1 {
		t.Fatalf("application entry = %v, program calls=%d", err, calls)
	}
}

type shellInstallFinalFixture struct {
	tui.Model
	selection cli.ShellInstallSelection
}

func (m shellInstallFinalFixture) ShellInstallSelection() cli.ShellInstallSelection {
	return m.selection
}

func TestShellInstallHandoffAfterTUI(t *testing.T) {
	assumeInteractiveTTY(t)
	setupMockHome(t, t.TempDir())
	oldOS, oldDetect, oldTUI, oldRun := ensureCurrentOSSupported, detectSystem, runTUI, runShellInstallSelection
	t.Cleanup(func() {
		ensureCurrentOSSupported, detectSystem, runTUI, runShellInstallSelection = oldOS, oldDetect, oldTUI, oldRun
	})
	ensureCurrentOSSupported = func() error { return nil }
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Supported: true}}, nil
	}
	failure := errors.New("fixture failure")
	for _, tc := range []struct {
		name                          string
		confirmed, failedTUI, generic bool
	}{
		{name: "confirmed", confirmed: true},
		{name: "unconfirmed"},
		{name: "failed-program", confirmed: true, failedTUI: true},
		{name: "unrelated-model", generic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, inProgram := 0, false
			output := &bytes.Buffer{}
			selection := cli.ShellInstallSelection{Confirmed: tc.confirmed}
			runTUI = func(m tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
				inProgram = true
				defer func() { inProgram = false }()
				if tc.failedTUI {
					return nil, failure
				}
				if tc.generic {
					return nil, nil
				}
				return shellInstallFinalFixture{Model: m.(tui.Model), selection: selection}, nil
			}
			runShellInstallSelection = func(got cli.ShellInstallSelection, stdout io.Writer) error {
				calls++
				if inProgram || got != selection || stdout != output {
					t.Fatal("installer starts before terminal-owner return or loses its selection/output")
				}
				return failure
			}
			err := RunArgs(nil, output)
			wantCall := tc.confirmed && !tc.failedTUI
			if (calls == 1) != wantCall || (err != nil) != (wantCall || tc.failedTUI) {
				t.Fatalf("handoff err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestShellInstallEarlyDispatch(t *testing.T) {
	oldOS, oldDetect := ensureCurrentOSSupported, detectSystem
	t.Cleanup(func() { ensureCurrentOSSupported, detectSystem = oldOS, oldDetect })
	ensureCurrentOSSupported = func() error {
		t.Fatal("shell installer reached generic OS gate")
		return errors.New("unexpected OS detection")
	}
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		t.Fatal("shell installer reached generic system detection")
		return system.DetectionResult{}, errors.New("unexpected detection")
	}
	var output bytes.Buffer
	if err := RunArgs([]string{"shell", "install", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 {
		t.Fatal("dedicated installer help missing")
	}
}
