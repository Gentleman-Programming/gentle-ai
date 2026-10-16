//go:build !windows

package cli

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// Linux and macOS review model; shell_install_view.go renders it.
// Confirmation quits the TUI so installation runs on a restored terminal.
type shellInstallModel struct {
	cancel    context.CancelFunc
	req       shellinstaller.UserInstallRequest
	field     int
	review    bool
	confirmed bool
	preview   string
	width     int
	height    int
	scroll    int
	err       error
}

func newShellInstallSelectionModel(cancel context.CancelFunc) shellInstallModel {
	return shellInstallModel{cancel: cancel, req: shellinstaller.UserInstallRequest{Mode: "separate"}}
}

func (m shellInstallModel) Init() tea.Cmd { return nil }

func (m shellInstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = max(0, size.Width), max(0, size.Height)
		m.scroll = min(m.scroll, m.lastReviewOffset())
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" || key.String() == "esc" {
		m.cancel()
		m.confirmed = false
		return m, tea.Quit
	}
	if m.review {
		if m.scrollReview(key.String()) {
			return m, nil
		}
		if key.String() != "y" {
			m.review, m.scroll = false, 0
			return m, nil
		}
		m.confirmed = true
		return m, tea.Quit
	}
	switch key.String() {
	case "tab":
		fields := 2
		if m.req.Mode == "shared" {
			fields = 4
		}
		m.field = (m.field + 1) % fields
	case "enter":
		token, err := shellinstaller.InspectUserInstall(m.req)
		m.preview = ""
		if err == nil {
			m.preview, err = shellinstaller.PreviewUserInstall(m.req, token)
		}
		m.err = err
		if err == nil {
			m.req.Confirmation, m.review, m.scroll = token, true, 0
		}
	case "left", "right":
		if m.field == 1 {
			if m.req.Mode == "separate" {
				m.req.Mode = "shared"
			} else {
				m.req.Mode, m.req.SharedPrefix, m.req.SharedAgent = "separate", "", ""
			}
		}
	default:
		fields := []*string{&m.req.Destination, nil, &m.req.SharedPrefix, &m.req.SharedAgent}
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

func (m shellInstallModel) content() string {
	rows := []string{shellInstallTitle, "Target: " + m.req.Destination, "Mode: " + m.req.Mode, "Shared prefix: " + m.req.SharedPrefix, "Shared agent: " + m.req.SharedAgent,
		"Commands: " + m.req.Destination + "/bin/gentle-shell and " + m.req.Destination + "/bin/pi", "Tab selects field; arrows change mode; Enter reviews; Escape cancels."}
	rows[m.field+1] = "> " + rows[m.field+1]
	if m.review {
		if m.preview != "" {
			rows = append(rows, m.preview)
		}
		rows = append(rows, "Confirm this physical selection and both command bindings? y installs; non-scroll keys edit.", m.req.Confirmation)
	}
	if m.err != nil {
		rows = append(rows, m.err.Error())
	}
	return strings.Join(rows, "\n")
}
