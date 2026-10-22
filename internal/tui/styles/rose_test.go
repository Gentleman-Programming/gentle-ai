package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRoseFramesLoad(t *testing.T) {
	n := RoseFrameCount()
	if n < 2 {
		t.Fatalf("expected an animation, got %d frames", n)
	}
	w, h := RoseSize()
	if w <= 0 || h <= 0 {
		t.Fatalf("invalid rose size %dx%d", w, h)
	}
	for i := 0; i < n; i++ {
		frame := RenderRoseFrame(i)
		if got := lipgloss.Height(frame); got != h {
			t.Fatalf("frame %d: height %d, want %d", i, got, h)
		}
		if got := lipgloss.Width(frame); got > w {
			t.Fatalf("frame %d: width %d exceeds %d", i, got, w)
		}
	}
}

func TestRoseFrameClamps(t *testing.T) {
	n := RoseFrameCount()
	if RenderRoseFrame(-5) != RenderRoseFrame(0) {
		t.Fatal("negative index should clamp to the first frame")
	}
	if RenderRoseFrame(n+10) != RenderRoseFrame(n-1) {
		t.Fatal("index past the end should clamp to the open rose")
	}
}

func TestRoseFinalFrameHasPetalsAndStem(t *testing.T) {
	last := RenderRoseFrame(RoseFrameCount() - 1)
	if !strings.ContainsAny(last, "⣿⣶⣤") {
		t.Fatal("open rose should contain dense braille cells")
	}
}

func TestParseRoseRejectsBadData(t *testing.T) {
	if a := parseRose("not a rose"); len(a.frames) != 0 {
		t.Fatal("expected empty animation for malformed data")
	}
	if a := parseRose("ROSE 2 3 4\nx\tx\n"); len(a.frames) != 0 {
		t.Fatal("expected empty animation for truncated data")
	}
}

func TestRoseFrameDelay(t *testing.T) {
	if RoseFrameDelay(0) <= RoseFrameDelay(1) {
		t.Fatal("the closed bud should linger longer than bloom frames")
	}
}

func TestParseRosePaletteLine(t *testing.T) {
	a := parseRose("ROSE 1 1 2\nPALETTE #ffffff\n⣿⣿\t00\n")
	if len(a.frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(a.frames))
	}
	if strings.Contains(a.frames[0], "PALETTE") || !strings.Contains(a.frames[0], "⣿⣿") {
		t.Fatalf("palette line was not consumed as a header: %q", a.frames[0])
	}
}
