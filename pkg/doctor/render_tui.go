package doctor

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// TUIRenderer provides an interactive TUI for the doctor report.
type TUIRenderer struct {
	options RenderOptions
}

// NewTUIRenderer creates a new TUI renderer.
func NewTUIRenderer(opts RenderOptions) *TUIRenderer {
	return &TUIRenderer{options: opts}
}

// Render launches the TUI application, or falls back to text renderer if not in a TTY.
func (t *TUIRenderer) Render(report DoctorReport) error {
	// Check if we're in a TTY
	if !isatty.IsTerminal(0) || !isatty.IsTerminal(1) {
		// Fall back to text renderer
		textRenderer := NewTextRenderer(t.options.Writer, t.options)
		return textRenderer.Render(report)
	}

	model := newTUIModel(report, t.options)
	return model.Run()
}

// tuiModel holds the state for the TUI.
type tuiModel struct {
	report    DoctorReport
	options   RenderOptions
	table     table.Model
	styles    tuiStyles
	width     int
	height    int
	quitting  bool
	verbose   bool
}

// tuiStyles holds lipgloss styles for the TUI.
type tuiStyles struct {
	passed   lipgloss.Style
	warning  lipgloss.Style
	failed   lipgloss.Style
	info     lipgloss.Style
	skipped  lipgloss.Style
	header   lipgloss.Style
	category lipgloss.Style
	summary  lipgloss.Style
	help     lipgloss.Style
	selected lipgloss.Style
}

// newTUIModel creates a new TUI model.
func newTUIModel(report DoctorReport, opts RenderOptions) *tuiModel {
	m := &tuiModel{
		report:  report,
		options: opts,
		verbose: opts.Verbose,
	}
	m.initStyles()
	m.initTable()
	return m
}

func (m *tuiModel) initStyles() {
	// Use Rose Pine palette from internal/tui/styles
	m.styles.header = lipgloss.NewStyle().
		Foreground(styles.ColorLavender).
		Bold(true).
		Padding(0, 1)

	m.styles.passed = lipgloss.NewStyle().
		Foreground(styles.ColorGreen)

	m.styles.warning = lipgloss.NewStyle().
		Foreground(styles.ColorYellow)

	m.styles.failed = lipgloss.NewStyle().
		Foreground(styles.ColorRed)

	m.styles.info = lipgloss.NewStyle().
		Foreground(styles.ColorBlue)

	m.styles.skipped = lipgloss.NewStyle().
		Foreground(styles.ColorOverlay)

	m.styles.selected = lipgloss.NewStyle().
		Foreground(styles.ColorLavender).
		Bold(true)

	m.styles.help = lipgloss.NewStyle().
		Foreground(styles.ColorSubtext)

	m.styles.category = lipgloss.NewStyle().
		Foreground(styles.ColorMauve).
		Bold(true)

	m.styles.summary = lipgloss.NewStyle().
		Foreground(styles.ColorText)
}

func (m *tuiModel) initTable() {
	columns := []table.Column{
		{Title: "STATUS", Width: 10},
		{Title: "CATEGORY", Width: 12},
		{Title: "CHECK", Width: 42},
		{Title: "SUMMARY", Width: 60},
	}

	rows := make([]table.Row, 0, len(m.report.Results))
	for _, r := range m.report.Results {
		if !m.shouldShow(r) {
			continue
		}
		status := m.statusString(r.Status)
		category := string(r.Category)
		checkName := r.Name
		summary := r.Summary

		rows = append(rows, table.Row{status, category, checkName, summary})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(len(rows)+2),
	)

	// Style matching gentle-ai TUI
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(styles.ColorLavender).
		BorderBottom(true).
		Bold(true).
		Foreground(styles.ColorLavender)
	s.Selected = s.Selected.
		Foreground(styles.ColorLavender).
		Background(styles.ColorSurface).
		Bold(true)
	s.Cell = s.Cell.
		Foreground(styles.ColorText).
		Padding(0, 1)
	t.SetStyles(s)

	m.table = t
}

func (m *tuiModel) statusString(status Status) string {
	switch status {
	case StatusPass:
		return m.styles.passed.Render("[  ok  ]")
	case StatusWarn:
		return m.styles.warning.Render("[ !!  ]")
	case StatusFail:
		return m.styles.failed.Render("[  xx  ]")
	case StatusInfo:
		return m.styles.info.Render("[  ..  ]")
	case StatusSkip:
		return m.styles.skipped.Render("[  --  ]")
	}
	return "[  ??  ]"
}

func (m *tuiModel) shouldShow(result CheckResult) bool {
	if m.verbose {
		return true
	}
	switch result.Status {
	case StatusPass:
		return m.options.ShowPassed
	case StatusSkip:
		return m.options.ShowSkipped
	default:
		return true
	}
}

// Init initializes the model (bubbletea interface).
func (m *tuiModel) Init() tea.Cmd {
	return nil
}

// Update handles messages (bubbletea interface).
func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "j", "down":
			m.table, cmd = m.table.Update(msg)
		case "k", "up":
			m.table, cmd = m.table.Update(msg)
		case "v":
			m.verbose = !m.verbose
			m.initTable() // Rebuild table with new visibility
			return m, nil
		case "enter":
			return m, m.showDetail()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width - 4)
	}

	return m, cmd
}

// View renders the UI (bubbletea interface).
func (m *tuiModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(m.styles.header.Render("🩺 gentle-ai doctor"))
	b.WriteString("\n")
	b.WriteString(m.styles.summary.Render(fmt.Sprintf("goos=%s goarch=%s  |  %d checks  |  %v",
		m.report.GOOS, m.report.GOARCH, m.report.Summary.Total, m.report.Summary.Duration)))
	b.WriteString("\n\n")

	// Summary bar
	b.WriteString(m.renderSummaryBar(m.report.Summary))
	b.WriteString("\n\n")

	// Table
	b.WriteString(m.table.View())
	b.WriteString("\n\n")

	// Legend / Help
	b.WriteString(m.styles.help.Render("↑/↓ navigate • v toggle verbose • enter details • q/esc quit"))
	if m.report.Summary.Fail > 0 {
		b.WriteString("\n")
		b.WriteString(m.styles.failed.Render("⚠ Some checks failed. Run with --fix for remediation hints."))
	}

	return b.String()
}

func (m *tuiModel) renderSummaryBar(s Summary) string {
	var b strings.Builder

	passStyle := m.styles.passed
	warnStyle := m.styles.warning
	failStyle := m.styles.failed
	infoStyle := m.styles.info
	skipStyle := m.styles.skipped

	b.WriteString("Summary: ")
	b.WriteString(passStyle.Render(fmt.Sprintf("%d passed", s.Pass)))
	b.WriteString(", ")
	b.WriteString(failStyle.Render(fmt.Sprintf("%d failed", s.Fail)))
	b.WriteString(", ")
	b.WriteString(warnStyle.Render(fmt.Sprintf("%d warnings", s.Warn)))
	b.WriteString(", ")
	b.WriteString(infoStyle.Render(fmt.Sprintf("%d info", s.Info)))
	b.WriteString(", ")
	b.WriteString(skipStyle.Render(fmt.Sprintf("%d skipped", s.Skip)))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("Duration: %v  ", s.Duration.Round(time.Millisecond)))

	status := "healthy"
	statusStyle := passStyle
	if s.Fail > 0 {
		status = "unhealthy"
		statusStyle = failStyle
	} else if s.Warn > 0 {
		status = "degraded"
		statusStyle = warnStyle
	}
	b.WriteString("Status: ")
	b.WriteString(statusStyle.Render(status))

	return b.String()
}

func (m *tuiModel) showDetail() tea.Cmd {
	selectedRow := m.table.SelectedRow()
	if len(selectedRow) < 3 {
		return nil
	}

	checkName := selectedRow[2]
	var detail string
	var remediation *Remediation

	for _, r := range m.report.Results {
		if r.Name == checkName {
			detail = r.Detail
			remediation = r.Remediation
			break
		}
	}

	if detail == "" && remediation == nil {
		return nil
	}

	// Could show a modal or detail pane here
	// For now, just return nil (detail view would be a future enhancement)
	return nil
}

// Run starts the TUI application.
func (m *tuiModel) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}