package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

type shellInstallDone struct{ err error }

// Windows Separate review model with a stable/main channel selector.
type shellInstallModel struct {
	ctx           context.Context
	cancel        context.CancelFunc
	self          string
	stdout        io.Writer
	req           shellinstaller.UserInstallRequest
	field         int
	review        bool
	busy          bool
	selectionOnly bool
	confirmed     bool
	err           error
}

func newShellInstallSelectionModel(cancel context.CancelFunc) shellInstallModel {
	return shellInstallModel{cancel: cancel, selectionOnly: true, req: shellinstaller.UserInstallRequest{Mode: "separate", Channel: "stable"}}
}

func (m shellInstallModel) Init() tea.Cmd { return nil }

func (m shellInstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if done, ok := msg.(shellInstallDone); ok {
		m.busy, m.err = false, done.err
		return m, tea.Quit
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" || key.String() == "esc" {
		m.cancel()
		m.confirmed = false
		if m.busy {
			return m, nil // Await actual stop/reap; never abandon the install goroutine.
		}
		return m, tea.Quit
	}
	if m.busy {
		return m, nil
	}
	if key.Type == tea.KeyRunes && strings.IndexFunc(string(key.Runes), unicode.IsControl) >= 0 {
		// Windows console modifier records can carry a NUL character instead of text.
		// Never turn those records into path bytes or let them dismiss physical review.
		if key.Paste || len(key.Runes) > 1 {
			// refusal:by-design operator-knowledge: only the operator can retype the destination without control characters; the TUI stays open for that input and no runnable command can supply it for them
			m.err = errors.New("destination text contains control characters; input refused")
		}
		return m, nil
	}
	if m.review {
		if key.String() != "y" {
			m.review = false
			return m, nil
		}
		if m.selectionOnly {
			m.confirmed = true
			return m, tea.Quit
		}
		m.busy = true
		return m, func() tea.Msg {
			err := shellinstaller.RunUserEntry(m.ctx, m.self, append([]string{"install"}, shellEntryValues(m.req)...), os.Stdin, m.stdout, os.Stderr)
			return shellInstallDone{err}
		}
	}
	switch key.String() {
	case "tab":
		m.field = (m.field + 1) % 3
	case "enter":
		token, err := shellinstaller.InspectUserInstall(m.req)
		m.err = err
		if err == nil {
			m.req.Confirmation, m.review = token, true
		}
	case "left", "right":
		// Separate remains fixed; only the channel field toggles.
		if m.field == 2 {
			if m.req.Channel == "main" {
				m.req.Channel = "stable"
			} else {
				m.req.Channel = "main"
			}
		}
	case " ":
		if m.field == 0 {
			m.req.Destination += " "
		}
	default:
		fields := []*string{&m.req.Destination, nil, nil}
		if field := fields[m.field]; field != nil {
			if key.Type == tea.KeyBackspace && len(*field) > 0 {
				value := []rune(*field)
				*field = string(value[:len(value)-1])
			} else if key.Type == tea.KeyRunes {
				*field += string(key.Runes)
			}
		}
	}
	return m, nil
}

func (m shellInstallModel) View() string {
	if m.busy {
		return "Installing selected Gentle Shell. Ctrl-C cancels; waiting for stop/reap.\n"
	}
	rows := []string{"Gentle Shell Windows 11 x64 user installer", "Target: " + m.req.Destination, "Mode: Separate (private Node/Go/Pi; personal installation preserved)",
		"Channel: " + m.req.Channel + " (Left/Right selects; Main resolves once after confirmation)",
		"Commands: " + filepath.Join(m.req.Destination, "bin/gentle-shell.cmd") + " and " + filepath.Join(m.req.Destination, "bin/pi.cmd"),
		"Tab selects field; Enter reviews; Escape cancels. Personal PATH and configuration are not changed."}
	rows[m.field+1] = "> " + rows[m.field+1]
	if m.review {
		rows = append(rows, "Confirm this physical selection and both command bindings? y installs; any other key edits.", m.req.Confirmation)
	}
	if m.err != nil {
		rows = append(rows, m.err.Error())
	}
	return strings.Join(rows, "\n") + "\n"
}
