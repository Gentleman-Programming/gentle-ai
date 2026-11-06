//go:build !windows

package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m shellInstallModel) reviewLines() []string {
	return strings.Split(ansi.Hardwrap(m.content(), max(1, m.width), true), "\n")
}

func (m shellInstallModel) reviewHeight() int {
	return max(1, m.height-1) // Reserve one row for navigation when it fits.
}

func (m shellInstallModel) lastReviewOffset() int {
	if m.width == 0 || m.height == 0 {
		return 0
	}
	return max(0, len(m.reviewLines())-m.reviewHeight())
}

// Navigation only changes presentation. Selection and confirmation are untouched.
func (m *shellInstallModel) scrollReview(key string) bool {
	switch key {
	case "pgdown":
		m.scroll += m.reviewHeight()
	case "pgup":
		m.scroll -= m.reviewHeight()
	case "home":
		m.scroll = 0
	case "end":
		m.scroll = m.lastReviewOffset()
	default:
		return false
	}
	m.scroll = min(max(0, m.scroll), m.lastReviewOffset())
	return true
}

func (m shellInstallModel) View() string {
	if m.width == 0 || m.height == 0 {
		return m.content() + "\n" // Preserve pre-layout rendering and pure model use.
	}
	if !m.review {
		return ansi.Hardwrap(m.content(), m.width, true) + "\n"
	}
	lines := m.reviewLines()
	start := min(max(0, m.scroll), m.lastReviewOffset())
	end := min(len(lines), start+m.reviewHeight())
	visible := append([]string{}, lines[start:end]...)
	if m.height > 1 {
		footer := fmt.Sprintf("PgUp/PgDn scroll %d-%d/%d; Home/End; Esc cancels", start+1, end, len(lines))
		// Only navigation hints may shorten on tiny terminals, never review data.
		visible = append(visible, ansi.Truncate(footer, m.width, ""))
	}
	// A trailing newline would create an extra row and let Tea clip real content.
	return strings.Join(visible, "\n")
}
