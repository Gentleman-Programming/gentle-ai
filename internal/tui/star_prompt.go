package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gentleman-programming/gentle-ai/v4/internal/starprompt"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// Star prompt messages.
//
// The offer follows the t3code GitHubStarPrompt courtesies (issue #5969):
// it may appear only after more than starprompt.Threshold successful completed
// operations, only after starprompt.IdleDelay without any keypress and with no
// operation running, at most once per session, and never again after any
// dismissal. When the state file cannot be read or written the offer is
// disabled entirely: a dismissal that cannot be saved must not be replaced by
// nagging.

// StarIdleElapsedMsg fires when the idle window elapses without keypresses.
type StarIdleElapsedMsg struct{}

// StarVisibleTimeoutMsg fires when the visible offer times out unanswered.
type StarVisibleTimeoutMsg struct{}

// StarGhDoneMsg carries the outcome of the Ctrl+G star action.
type StarGhDoneMsg struct {
	Starred bool
	Opened  bool
	Err     error
}

// Injectable seams for tests.
var (
	starTimeNow = time.Now
	// starIdleTick schedules the idle/visible timers; tests replace it with an
	// immediate command so no real waiting happens.
	starIdleTick = func(d time.Duration) tea.Cmd {
		return tea.Tick(d, func(time.Time) tea.Msg { return StarIdleElapsedMsg{} })
	}
	starVisibleTick = func(d time.Duration) tea.Cmd {
		return tea.Tick(d, func(time.Time) tea.Msg { return StarVisibleTimeoutMsg{} })
	}
	starStateDirFn = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return state.Dir(home)
	}
	starGhRunner starprompt.GhRunner = starprompt.DefaultGhRunner
	starGhPath   string              = "gh"
)

// starInit lazily loads the persisted state; an unusable file disables the
// offer for the whole session.
func (m *Model) starInit() {
	read := starprompt.Read(starStateDirFn())
	m.StarState = read.State
	m.StarUnusable = read.Unusable
}

// starRecordOperation counts one successful completed operation and persists
// the counter. A failed write must disable asking (fail closed), and the
// idle wait is armed from here: the operation just finished, so nothing is
// running and the user is likely reading the result screen.
func (m *Model) starRecordOperation() tea.Cmd {
	if !m.StarLoaded {
		m.starInit()
		m.StarLoaded = true
	}
	m.StarState.CompletedOps++
	if err := starprompt.Write(starStateDirFn(), m.StarState); err != nil {
		m.StarUnusable = true
		return nil
	}
	return m.starArmIdle()
}

// starArmIdle schedules the idle wait that precedes the offer. Calling it
// again while waiting restarts the window.
func (m *Model) starArmIdle() tea.Cmd {
	if !starprompt.ShouldOffer(m.StarState, m.StarUnusable, m.StarOffered, m.OperationRunning) {
		return nil
	}
	if m.StarVisible {
		return nil
	}
	m.StarWaitingIdle = true
	m.StarLastKey = starTimeNow()
	return starIdleTick(starprompt.IdleDelay)
}

// starObserveKey records activity and, while the offer is visible, handles the
// two chords. It returns a model/command pair plus whether the key was
// consumed (chords are never passed through to the screens; every other key
// only restarts the idle wait).
func (m Model) starObserveKey(keyStr string) (Model, tea.Cmd, bool) {
	m.StarLastKey = starTimeNow()
	if m.StarVisible {
		switch keyStr {
		case "ctrl+g":
			return m, m.starStarNow(), true
		case "alt+d":
			m.starDismiss()
			return m, nil, true
		}
	}
	// Plain activity only records the timestamp; the pending idle tick sees it
	// via StarLastKey and re-arms itself, so no key ever needs a returned cmd.
	return m, nil, false
}

// starStarNow runs the star action in a goroutine: direct starring through gh
// when opted in, the browser otherwise (and as fallback whenever gh is
// unavailable or ambiguous).
func (m Model) starStarNow() tea.Cmd {
	m.StarVisible = false
	m.StarOffered = true
	direct := m.StarState.DirectStar
	return func() tea.Msg {
		if direct {
			outcome := starprompt.GhStar(context.Background(), starGhRunner, starGhPath, starprompt.Repository)
			if outcome == starprompt.OutcomeYes {
				return StarGhDoneMsg{Starred: true}
			}
			// Unknown or failed gh: fall through to the browser so the chord
			// never dead-ends.
		}
		err := tuiOpenBrowserFn(starprompt.RepositoryURL)
		return StarGhDoneMsg{Opened: err == nil, Err: err}
	}
}

// starDismiss marks the state dismissed forever and persists it; a failed
// write keeps the session-level dismissal and disables future asks.
func (m *Model) starDismiss() {
	m.StarVisible = false
	m.StarOffered = true
	m.StarState.Dismissed = true
	if err := starprompt.Write(starStateDirFn(), m.StarState); err != nil {
		m.StarUnusable = true
	}
}

// starHandleMsg processes the star prompt's own messages inside Update.
// It returns a command when one must be scheduled.
func (m *Model) starHandleMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case StarIdleElapsedMsg:
		if !m.StarWaitingIdle || m.StarVisible || m.OperationRunning {
			m.StarWaitingIdle = false
			return nil
		}
		if starTimeNow().Sub(m.StarLastKey) < starprompt.IdleDelay {
			// A near-simultaneous keystroke won the race: wait out the new window.
			return starIdleTick(starprompt.IdleDelay)
		}
		if !starprompt.ShouldOffer(m.StarState, m.StarUnusable, m.StarOffered, m.OperationRunning) {
			m.StarWaitingIdle = false
			return nil
		}
		// The direct flow may read starred state with stored credentials; the
		// browser-only default never touches credentials. An already-starred
		// account or an ambiguous check marks the offer done for this session.
		if m.StarState.DirectStar {
			outcome := starprompt.GhStarred(context.Background(), starGhRunner, starGhPath, starprompt.Repository)
			switch outcome {
			case starprompt.OutcomeYes:
				m.StarOffered = true
				m.StarWaitingIdle = false
				m.starDismiss()
				return nil
			case starprompt.OutcomeUnknown:
				m.StarOffered = true
				m.StarWaitingIdle = false
				return nil
			}
		}
		m.StarWaitingIdle = false
		m.StarVisible = true
		return starVisibleTick(starprompt.VisibleTimeout)
	case StarVisibleTimeoutMsg:
		if m.StarVisible {
			m.StarVisible = false
			m.StarOffered = true
		}
		return nil
	case StarGhDoneMsg:
		if msg.Starred {
			m.starDismiss()
			m.StarThanksAt = starTimeNow()
			return nil
		}
		if msg.Opened {
			m.starDismiss()
			return nil
		}
		// Neither starring nor the browser worked: surface the URL manually and
		// keep the session eligible (a real answer is still possible).
		m.StarManualURL = starprompt.RepositoryURL
		return nil
	}
	return nil
}

// starFooterLine renders the non-blocking footer line for the current star
// state, or "" when nothing must be shown.
func (m Model) starFooterLine() string {
	if m.StarVisible {
		return fmt.Sprintf("Enjoying Gentle AI? Ctrl+G stars %s on GitHub, Alt+D never asks again", starprompt.RepositoryURL)
	}
	if !m.StarThanksAt.IsZero() && starTimeNow().Sub(m.StarThanksAt) < 5*time.Second {
		return fmt.Sprintf("Thanks for the star! It really helps %s grow.", starprompt.RepositoryURL)
	}
	if m.StarManualURL != "" {
		return "Could not open the browser; star us at " + m.StarManualURL
	}
	return ""
}
