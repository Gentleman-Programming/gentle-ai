package tui

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func shellBackupsModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(system.DetectionResult{}, "shell-backups-test").WithShellInstaller()
	m.Width, m.Height = 120, 40
	m.Screen = ScreenShellInstall
	return m
}

func shellBackupsInstall() shellinstaller.UserShellInstall {
	return shellinstaller.UserShellInstall{Target: "/home/u/.local/share/gentle-shell/install", Mode: "separate"}
}

func shellBackupsCandidates() []shellinstaller.UserReplacedBackup {
	return []shellinstaller.UserReplacedBackup{
		{Path: "/home/u/.local/share/gentle-shell/install.replaced-20260101T110000Z", Stamp: "20260101T110000Z", Bytes: 100, Complete: true},
		{Path: "/home/u/.local/share/gentle-shell/install.replaced-20260101T120000Z", Stamp: "20260101T120000Z", Bytes: 200, Complete: true, Latest: true},
	}
}

// (a) Manage offers exactly one replaced-backups entry point.
func TestShellManageReplacedBackupsAction(t *testing.T) {
	manage := &shellManageState{install: shellBackupsInstall()}
	for _, action := range manageActions(manage) {
		if action == "Replaced backups" {
			return
		}
	}
	t.Fatalf("Manage actions lack %q: %v", "Replaced backups", manageActions(manage))
}

// (b) The injected discovery result replaces the loading state with the list.
func TestShellBackupsLoadedPopulatesList(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{loading: true}}
	next, _ := m.Update(shellBackupsLoaded{list: shellBackupsCandidates()})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups == nil || m.shellManage.backups.loading || len(m.shellManage.backups.list) != 2 {
		t.Fatalf("discovery result did not populate the list: %+v", m.shellManage)
	}
}

// (c) Esc from the list returns to the Manage summary, keeping it intact.
func TestShellBackupsEscReturnsToManage(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{list: shellBackupsCandidates()}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups != nil {
		t.Fatalf("Esc from replaced backups did not return to Manage: %+v", m.shellManage)
	}
}

// (d) Esc from the older-cleanup confirmation cancels back to the list.
func TestShellBackupsConfirmationCancel(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{list: shellBackupsCandidates(), confirm: true}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups == nil || m.shellManage.backups.confirm {
		t.Fatalf("Esc from older-cleanup confirmation did not return to the list: %+v", m.shellManage)
	}
}

// (e) The injected prune result lands on the done state: success has no error,
// a failure is kept as a preservation error.
func TestShellBackupsPruneResult(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantError bool
	}{
		{"success", nil, false},
		{"preservation error", errors.New("uncertain backup preserved"), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := shellBackupsModel(t)
			install := shellBackupsInstall()
			m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{working: true}}
			next, _ := m.Update(shellBackupsPruned{err: test.err})
			m = next.(Model)
			state := m.shellManage
			if state == nil || state.backups == nil || state.backups.working || !state.backups.done {
				t.Fatalf("prune result did not complete: %+v", state)
			}
			if test.wantError {
				if state.backups.err == nil || !strings.Contains(state.backups.err.Error(), "preserved") {
					t.Fatalf("prune failure was not preserved as an error: %+v", state)
				}
			} else if state.backups.err != nil {
				t.Fatalf("clean prune reported an error: %+v", state)
			}
		})
	}
}

// (f) Non-Windows stub contract: the list is empty and batch prune is
// unsupported, so the TUI renders the empty state and never offers cleanup.
func TestShellBackupsNonWindowsStub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows replaced-backup listing is native; this pins the non-Windows stub only")
	}
	install := shellBackupsInstall()
	backups, err := shellinstaller.UserReplacedBackups(install)
	if err != nil || len(backups) != 0 {
		t.Fatalf("non-Windows listing is not empty: %v %+v", err, backups)
	}
	if err := shellinstaller.UserPruneReplacedBackups(install); err == nil {
		t.Fatal("non-Windows batch prune is not reported unsupported")
	}
}

// (g) Enter on the Manage "Replaced backups" action enters the loading state
// and returns a discovery command that yields shellBackupsLoaded.
func TestShellManageReplacedBackupsActionEnter(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install}
	index := -1
	for cursor, action := range manageActions(m.shellManage) {
		if action == "Replaced backups" {
			index = cursor
		}
	}
	if index < 0 {
		t.Fatalf("Manage actions lack %q: %v", "Replaced backups", manageActions(m.shellManage))
	}
	m.shellManage.cursor = index
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups == nil || !m.shellManage.backups.loading {
		t.Fatalf("Enter on %q did not enter loading: %+v", "Replaced backups", m.shellManage)
	}
	if cmd == nil {
		t.Fatal("Enter on the replaced-backups action returned no discovery command")
	}
	msg := cmd()
	if _, ok := msg.(shellBackupsLoaded); !ok {
		t.Fatalf("discovery command yielded %T, want shellBackupsLoaded", msg)
	}
}

// (h) Enter on the "Clean older backups" list action enters the confirmation
// WITHOUT pruning: no prune command may be issued until the operator confirms.
func TestShellBackupsCleanupEnterConfirmation(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{list: shellBackupsCandidates()}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups == nil || !m.shellManage.backups.confirm || m.shellManage.backups.working {
		t.Fatalf("Enter on cleanup did not enter confirmation without working: %+v", m.shellManage)
	}
	if cmd != nil {
		if _, ok := cmd().(shellBackupsPruned); ok {
			t.Fatal("cleanup confirmation already issued a prune command")
		}
	}
}

// (i) Enter on the confirmed cleanup returns a prune command that yields
// shellBackupsPruned carrying the preservation result.
func TestShellBackupsConfirmEnterPrunes(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{list: shellBackupsCandidates(), confirm: true}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups == nil || !m.shellManage.backups.working {
		t.Fatalf("confirmed cleanup did not start working: %+v", m.shellManage)
	}
	if cmd == nil {
		t.Fatal("confirmed cleanup returned no prune command")
	}
	msg := cmd()
	if _, ok := msg.(shellBackupsPruned); !ok {
		t.Fatalf("prune command yielded %T, want shellBackupsPruned", msg)
	}
}

// (j) While the prune is working, keys are suppressed: a second Enter must not
// start another prune or mutate the in-flight state.
func TestShellBackupsWorkingSuppressesKeys(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{list: shellBackupsCandidates(), working: true}}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups == nil || !m.shellManage.backups.working {
		t.Fatalf("working state was mutated by a key press: %+v", m.shellManage)
	}
	if cmd != nil {
		if _, ok := cmd().(shellBackupsPruned); ok {
			t.Fatal("double-enter issued a second prune command")
		}
	}
}

// (k) A failed discovery lands the load error on the state instead of a false
// empty list, so the screen renders the error rather than "No replaced backups".
func TestShellBackupsLoadError(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{loading: true}}
	next, _ := m.Update(shellBackupsLoaded{err: errors.New("discovery failed")})
	m = next.(Model)
	if m.shellManage == nil || m.shellManage.backups == nil {
		t.Fatalf("load error dropped the backups state: %+v", m.shellManage)
	}
	backups := m.shellManage.backups
	if backups.loading {
		t.Fatal("failed discovery left the loading state active")
	}
	if backups.err == nil || !strings.Contains(backups.err.Error(), "discovery failed") {
		t.Fatalf("load error was not preserved: %v", backups.err)
	}
	if len(backups.list) != 0 {
		t.Fatalf("failed discovery fabricated a list: %+v", backups.list)
	}
}

// (l) The Manage view routes an active backups sub-flow to the replaced
// backups screen rather than the Manage summary.
func TestShellBackupsViewRoutesToScreen(t *testing.T) {
	m := shellBackupsModel(t)
	install := shellBackupsInstall()
	m.shellManage = &shellManageState{install: install, backups: &shellBackupsState{list: shellBackupsCandidates()}}
	output := m.View()
	if !strings.Contains(output, "Replaced backups") {
		t.Fatalf("Manage view did not route to the replaced-backups screen: %q", output)
	}
	if !strings.Contains(output, "install.replaced-20260101T110000Z") {
		t.Fatalf("Manage view did not list the candidate: %q", output)
	}
}
