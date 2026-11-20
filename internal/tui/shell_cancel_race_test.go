package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// A cancel accepted and a publication started are mutually exclusive, however
// the worker's progress callback and the UI's cancel interleave. The cancel
// function runs under the control's lock, so it can read the flag directly.
func TestRunControlMutualExclusion(t *testing.T) {
	for i := 0; i < 20000; i++ {
		control := &shellRunControl{}
		violated := false
		control.cancel = func() {
			if control.publishing {
				violated = true
			}
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			control.markProgress(shellinstaller.UserInstallProgress{Index: len(shellInstallStepLabels), Total: 5, State: "running"})
		}()
		go func() { defer wg.Done(); control.tryCancel() }()
		wg.Wait()
		if violated {
			t.Fatalf("iteration %d: cancel accepted after publication started", i)
		}
	}
}

// Once publication is marked, a cancel is refused and never reaches the context.
func TestRunControlRefusesCancelAfterPublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	control := &shellRunControl{cancel: cancel}
	control.markProgress(shellinstaller.UserInstallProgress{Index: len(shellInstallStepLabels), State: "running"})
	if control.tryCancel() || ctx.Err() != nil {
		t.Fatal("cancel accepted after publication started")
	}
}

// A detail event is progress: it refreshes the watchdog clock.
func TestDetailEventRefreshesWatchdog(t *testing.T) {
	m, _, _ := blockingRun(t, 2, nil)
	stale := time.Now().Add(-time.Hour)
	m.shellInstallRun.lastProgress = stale
	next, _ := m.Update(shellInstallEvent{progress: shellinstaller.UserInstallProgress{Index: 2, Total: 5, State: "detail", Detail: "x"}})
	m = next.(Model)
	if !m.shellInstallRun.lastProgress.After(stale) {
		t.Fatal("detail event did not refresh lastProgress")
	}
}

// The cancel hint disappears once publication starts.
func TestShellRunHidesCancelHintDuringPublication(t *testing.T) {
	release := make(chan struct{})
	m, cmd, _ := blockingRun(t, 3, release)
	if !strings.Contains(m.View(), "esc: cancel before publication") {
		t.Fatalf("hint missing before publication:\n%s", m.View())
	}
	m, cmd2, _ := blockingRun(t, 5, release)
	if strings.Contains(m.View(), "esc: cancel before publication") {
		t.Fatalf("hint shown during publication:\n%s", m.View())
	}
	close(release)
	_, _ = cmd, cmd2
}
