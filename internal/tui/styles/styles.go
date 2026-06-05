package styles

import "github.com/charmbracelet/lipgloss"

// Rose Pine color palette.
var (
	ColorBase     = lipgloss.Color("#191724")
	ColorSurface  = lipgloss.Color("#1f1d2e")
	ColorOverlay  = lipgloss.Color("#6e6a86")
	ColorText     = lipgloss.Color("#e0def4")
	ColorSubtext  = lipgloss.Color("#908caa")
	ColorLavender = lipgloss.Color("#c4a7e7")
	ColorGreen    = lipgloss.Color("#9ccfd8")
	ColorPeach    = lipgloss.Color("#f6c177")
	ColorRed      = lipgloss.Color("#eb6f92")
	ColorBlue     = lipgloss.Color("#31748f")
	ColorMauve    = lipgloss.Color("#ebbcba")
	ColorYellow   = lipgloss.Color("#f1ca93")
	ColorTeal     = lipgloss.Color("#9ccfd8")
)

// Cursor is the prefix used for the currently focused item.
const Cursor = "▸ "

// Tagline returns the welcome screen tagline with the given version.
func Tagline(version string) string {
	return "Gentle-AI " + version + " — Ecosystem, Frameworks, Workflows"
}

// Pre-built reusable styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorLavender).
			Bold(true)

	HeadingStyle = lipgloss.NewStyle().
			Foreground(ColorMauve).
			Bold(true)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext)

	SubtextStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(ColorLavender).
			Bold(true)

	UnselectedStyle = lipgloss.NewStyle().
			Foreground(ColorText)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	FrameStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorLavender).
			Padding(1, 2)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(ColorOverlay).
			Padding(0, 1)

	ProgressFilled = lipgloss.NewStyle().
			Foreground(ColorGreen)

	ProgressEmpty = lipgloss.NewStyle().
			Foreground(ColorOverlay)

	PercentStyle = lipgloss.NewStyle().
			Foreground(ColorPeach).
			Bold(true)
)
