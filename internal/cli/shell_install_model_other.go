//go:build !windows

package cli

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// shellRootRiskGate reports that this operator must pass the typed root
// acceptance phase before the review can confirm. Injectable for tests.
var shellRootRiskGate = func() bool {
	return runtime.GOOS == "linux" && os.Getuid() == 0 && !shellinstaller.RootRiskAccepted()
}

// Linux and macOS review model; shell_install_view.go renders it.
// Confirmation quits the TUI so installation runs on a restored terminal.
type shellInstallModel struct {
	cancel        context.CancelFunc
	req           shellinstaller.UserInstallRequest
	field         int
	review        bool
	confirmed     bool
	preview       string
	width         int
	height        int
	scroll        int
	err           error
	rootRisk      bool
	rootRiskInput string
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
	if m.rootRisk {
		switch key.String() {
		case "ctrl+c", "esc":
			m.cancel()
			m.confirmed = false
			return m, tea.Quit
		case "enter":
			if strings.TrimSpace(m.rootRiskInput) != shellinstaller.RootRiskPhrase {
				m.err = errors.New("the typed phrase does not match exactly; nothing is installed — type " + shellinstaller.RootRiskPhrase + " exactly to accept, or press Escape to cancel and create a regular user instead")
				m.rootRiskInput = ""
				return m, nil
			}
			if err := os.Setenv(shellinstaller.RootRiskEnv, shellinstaller.RootRiskBinding); err != nil {
				m.err = err
				return m, nil
			}
			m.err = nil
			m.confirmed = true
			return m, tea.Quit
		case "backspace":
			if input := []rune(m.rootRiskInput); len(input) > 0 {
				m.rootRiskInput = string(input[:len(input)-1])
			}
		default:
			if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
				m.rootRiskInput += key.String()
			}
		}
		return m, nil
	}
	if m.review {
		if m.scrollReview(key.String()) {
			return m, nil
		}
		if key.String() != "y" {
			m.review, m.scroll = false, 0
			return m, nil
		}
		if shellRootRiskGate() {
			// Root must restate the exact acceptance phrase; "y" alone never
			// accepts a root installation.
			m.rootRisk, m.rootRiskInput, m.err = true, "", nil
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
	if m.rootRisk {
		rows = append(rows,
			"WARNING: you are installing as root. The private sandbox (capability drop,",
			"NoNewPrivileges, delegated unit limits) is disabled for root, and every command",
			"the agent runs will run with root powers.",
			"Recommended alternative: create a regular user (adduser <name> && su - <name>)",
			"and install from that session.",
			"Type \""+shellinstaller.RootRiskPhrase+"\" (without quotes) to accept the risk; Escape cancels.",
			"> "+m.rootRiskInput)
	} else if m.review {
		if m.preview != "" {
			rows = append(rows, m.preview)
		}
		prompt := "Confirm this physical selection and both command bindings? y installs; non-scroll keys edit."
		if os.Getuid() == 0 {
			prompt = "Root detected: confirming asks for the typed risk acceptance. y continues; non-scroll keys edit."
		}
		rows = append(rows, prompt, m.req.Confirmation)
	}
	if m.err != nil {
		rows = append(rows, m.err.Error())
	}
	return strings.Join(rows, "\n")
}
