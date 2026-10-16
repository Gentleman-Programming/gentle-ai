package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellInstallEmbeddedDefaultsAndCancel(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		t.Run(tea.KeyMsg{Type: key}.String(), func(t *testing.T) {
			cancelled := false
			model := NewShellInstallModel(func() { cancelled = true })
			if model.Init() != nil {
				t.Fatal("selection model starts a command on initialization")
			}
			selection, err := ShellInstallOutcome(model)
			if err != nil || selection.Confirmed || selection.Request.Mode != "separate" || len(ShellInstallArguments(selection)) != 0 {
				t.Fatalf("initial selection = %+v, %v", selection, err)
			}
			model, cmd := model.Update(tea.KeyMsg{Type: key})
			if !cancelled || cmd == nil {
				t.Fatal("cancel did not finish the selector")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("cancel starts a worker instead of quitting")
			}
			selection, err = ShellInstallOutcome(model)
			if err != nil || selection.Confirmed || len(ShellInstallArguments(selection)) != 0 {
				t.Fatalf("cancelled selection = %+v, %v", selection, err)
			}
		})
	}
}

func TestShellInstallEmbeddedConfirmationOnlySelects(t *testing.T) {
	model := NewShellInstallModel(nil).(shellInstallModel)
	model.review = true // Model-only fixture: not a physical consent proof.
	model.req.Destination, model.req.Confirmation = "/owned/fixture", "fixture-token"
	final, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("confirmation did not finish the selector")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("embedded confirmation starts a worker instead of quitting")
	}
	selection, err := ShellInstallOutcome(final)
	if err != nil || !selection.Confirmed || selection.Request.Confirmation != "fixture-token" {
		t.Fatalf("confirmed selection = %+v, %v", selection, err)
	}
	args := ShellInstallArguments(selection)
	if len(args) == 0 || args[0] != "install" {
		t.Fatalf("worker arguments = %q", args)
	}
	req, err := shellinstaller.UserInstallFromEntry(args[1:])
	if err != nil || req != selection.Request {
		t.Fatalf("platform entry lost selection: %+v, %v", req, err)
	}
}

func TestShellInstallEmbeddedOutcomeRejectsOtherModels(t *testing.T) {
	if _, err := ShellInstallOutcome(nil); err == nil {
		t.Fatal("unrelated model accepted as an installation selection")
	}
}
