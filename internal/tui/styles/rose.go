package styles

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// rose_frames.txt holds the pre-rendered braille frames of the blooming rose.
//
// Format:
//
//	line 1: "ROSE <frames> <rows> <cols>"
//	line 2 (optional): "PALETTE #rrggbb #rrggbb ..." (defaults to rosePalette)
//	then, for every frame, <rows> lines of "glyphs\tcolors", where colors has
//	one rune per glyph: '.' for an empty cell, otherwise an index (0-9a-z)
//	into the palette.
//
//go:embed rose_frames.txt
var roseFramesData string

// rosePalette is the default palette when the frames file does not declare
// one: the same top-to-bottom bands as the static logo.
var rosePalette = gradientColors

const (
	roseFirstFrameDelay = 700 * time.Millisecond
	roseFrameDelay      = 45 * time.Millisecond
)

type roseAnimation struct {
	frames []string
	width  int
	height int
}

var (
	roseOnce sync.Once
	roseAnim roseAnimation
)

func loadRose() roseAnimation {
	roseOnce.Do(func() { roseAnim = parseRose(roseFramesData) })
	return roseAnim
}

func parseRose(data string) roseAnimation {
	lines := strings.Split(strings.TrimRight(data, "\n"), "\n")
	if len(lines) == 0 {
		return roseAnimation{}
	}
	header := strings.Fields(lines[0])
	if len(header) != 4 || header[0] != "ROSE" {
		return roseAnimation{}
	}
	count, err1 := strconv.Atoi(header[1])
	rows, err2 := strconv.Atoi(header[2])
	cols, err3 := strconv.Atoi(header[3])
	if err1 != nil || err2 != nil || err3 != nil || count <= 0 || rows <= 0 || len(lines)-1 < count*rows {
		return roseAnimation{}
	}

	body := lines[1:]
	palette := rosePalette
	if len(body) > 0 && strings.HasPrefix(body[0], "PALETTE ") {
		palette = nil
		for _, hex := range strings.Fields(body[0])[1:] {
			palette = append(palette, lipgloss.Color(hex))
		}
		body = body[1:]
	}
	if len(body) < count*rows {
		return roseAnimation{}
	}

	styles := make([]lipgloss.Style, len(palette))
	for i, c := range palette {
		styles[i] = lipgloss.NewStyle().Foreground(c)
	}

	anim := roseAnimation{frames: make([]string, count), width: cols, height: rows}
	for f := 0; f < count; f++ {
		var b strings.Builder
		for r := 0; r < rows; r++ {
			glyphs, colors, _ := strings.Cut(body[f*rows+r], "\t")
			b.WriteString(renderRoseLine([]rune(glyphs), []rune(colors), styles))
			if r < rows-1 {
				b.WriteByte('\n')
			}
		}
		anim.frames[f] = b.String()
	}
	return anim
}

// renderRoseLine groups consecutive cells sharing a color into a single styled run.
func renderRoseLine(glyphs, colors []rune, styles []lipgloss.Style) string {
	var b strings.Builder
	var run []rune
	runColor := -1
	flush := func() {
		if len(run) == 0 {
			return
		}
		if runColor < 0 {
			b.WriteString(string(run))
		} else {
			b.WriteString(styles[runColor].Render(string(run)))
		}
		run = run[:0]
	}
	for i, g := range glyphs {
		c := -1
		if i < len(colors) && colors[i] != '.' {
			if v, err := strconv.ParseInt(string(colors[i]), 36, 0); err == nil && int(v) < len(styles) {
				c = int(v)
			}
		}
		if g == ' ' || g == '⠀' {
			c = -1
		}
		if c != runColor {
			flush()
			runColor = c
		}
		run = append(run, g)
	}
	flush()
	return b.String()
}

// RoseFrameCount returns the number of frames in the blooming-rose animation.
func RoseFrameCount() int { return len(loadRose().frames) }

// RoseSize returns the width and height, in cells, of every rose frame.
func RoseSize() (width, height int) {
	a := loadRose()
	return a.width, a.height
}

// RenderRoseFrame returns frame i of the blooming rose, clamped to the valid
// range. The last frame is the fully open rose. It returns the static logo if
// the embedded frames could not be loaded.
func RenderRoseFrame(i int) string {
	a := loadRose()
	if len(a.frames) == 0 {
		return RenderLogo()
	}
	if i < 0 {
		i = 0
	}
	if i >= len(a.frames) {
		i = len(a.frames) - 1
	}
	return a.frames[i]
}

// RoseFrameDelay is how long frame i stays on screen before advancing: the
// closed bud lingers a moment, then the bloom plays at ~22 fps.
func RoseFrameDelay(i int) time.Duration {
	if i <= 0 {
		return roseFirstFrameDelay
	}
	return roseFrameDelay
}
