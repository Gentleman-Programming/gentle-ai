//go:build !windows

package cli

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func keyRunes(runes ...rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: runes}
}

func reviewModelWithToken(t *testing.T) shellInstallModel {
	t.Helper()
	m := newShellInstallSelectionModel(func() {})
	m.req.Destination = "/tmp/gentle-root-risk-test"
	m.req.Confirmation = "token"
	m.review = true
	return m
}

func TestShellInstallModelRootRiskTypedAcceptance(t *testing.T) {
	t.Setenv(shellinstaller.RootRiskEnv, "")
	original := shellRootRiskGate
	shellRootRiskGate = func() bool { return true }
	t.Cleanup(func() { shellRootRiskGate = original })
	defer func() {
		if err := os.Unsetenv(shellinstaller.RootRiskEnv); err != nil {
			t.Fatalf("unset binding: %v", err)
		}
	}()

	// "y" on the review screen must open the typed phase, never confirm root.
	m := reviewModelWithToken(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(shellInstallModel)
	if !m.rootRisk || m.confirmed {
		t.Fatal("root confirmation must enter the typed acceptance phase instead of confirming")
	}
	if view := m.content(); !strings.Contains(view, "WARNING") || !strings.Contains(view, "adduser") {
		t.Fatalf("the typed phase must render the warning and the remedy, got:\n%s", view)
	}

	// A wrong phrase is refused and keeps the operator in the phase.
	for _, r := range "I accept" {
		updated, _ = m.Update(keyRunes(r))
		m = updated.(shellInstallModel)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(shellInstallModel)
	if m.confirmed || m.err == nil {
		t.Fatal("a partial phrase must be refused without confirming")
	}

	// The exact phrase records the binding and confirms.
	for _, r := range shellinstaller.RootRiskPhrase {
		updated, _ = m.Update(keyRunes(r))
		m = updated.(shellInstallModel)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(shellInstallModel)
	if !m.confirmed || m.err != nil {
		t.Fatalf("the exact phrase must confirm, got confirmed=%v err=%v", m.confirmed, m.err)
	}
	if os.Getenv(shellinstaller.RootRiskEnv) != shellinstaller.RootRiskBinding {
		t.Fatal("confirming a root installation must record the acceptance binding")
	}
}

func TestShellInstallModelRootRiskEscapeCancels(t *testing.T) {
	t.Setenv(shellinstaller.RootRiskEnv, "")
	original := shellRootRiskGate
	shellRootRiskGate = func() bool { return true }
	t.Cleanup(func() { shellRootRiskGate = original })

	m := reviewModelWithToken(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(shellInstallModel)
	cancelled := false
	m.cancel = func() { cancelled = true }
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(shellInstallModel)
	if m.confirmed || !cancelled || cmd == nil {
		t.Fatal("escape on the root warning must cancel without confirming")
	}
}

func TestShellInstallModelNonRootKeepsDirectConfirmation(t *testing.T) {
	original := shellRootRiskGate
	shellRootRiskGate = func() bool { return false }
	t.Cleanup(func() { shellRootRiskGate = original })

	m := reviewModelWithToken(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(shellInstallModel)
	if !m.confirmed || m.rootRisk {
		t.Fatal("non-root operators keep the ordinary y confirmation")
	}
}

func TestShellInstallModelRootRiskPhraseConstMatchesPrompt(t *testing.T) {
	if !strings.Contains(shellinstaller.RootRiskPhrase, "accept") {
		t.Fatal("the typed phrase must state acceptance")
	}
	if shellinstaller.RootRiskBinding == "" || shellinstaller.RootRiskEnv == "" {
		t.Fatal("the binding contract must stay non-empty")
	}
}
