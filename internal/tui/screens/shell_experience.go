package screens

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

func RenderShellExperience(p shellinstaller.UserExperience, cursor int, err error, width, height int, targets ...ShellExperienceEntry) string {
	return RenderShellExperienceWithUninstall(p, cursor, err, width, height, false, targets...)
}

func RenderShellExperienceWithUninstall(p shellinstaller.UserExperience, cursor int, err error, width, height int, uninstall bool, targets ...ShellExperienceEntry) string {
	return RenderShellExperienceWithUninstallAction(p, cursor, err, width, height, uninstall, "Continue", targets...)
}

func RenderShellExperienceWithUninstallAction(p shellinstaller.UserExperience, cursor int, err error, width, height int, uninstall bool, continueLabel string, targets ...ShellExperienceEntry) string {
	width, height = shellViewport(width, height)
	view := renderShellExperienceConfigWithUninstall(p, cursor, width, height, uninstall, continueLabel, targets...)
	if err != nil {
		warning := wrapShellExperienceConfigText(err.Error(), width)
		warning = warning[:min(len(warning), max(1, min(3, height-1)))]
		for i := range warning {
			warning[i] = styles.WarningStyle.Render(warning[i])
		}
		return joinShellExperienceConfigLines(append(warning, strings.Split(view, "\n")...), height)
	}
	return view
}

var shellReadyActions = []string{"Install Gentle-Shell", "Back", "Cancel"}

// shellReadyLines retains the complete current request and truthful scopes.
// Only presentation chrome is clipped; all request/preview data stays pageable.
func shellReadyLines(req shellinstaller.UserInstallRequest, preview string, width int) []string {
	data, err := json.MarshalIndent(req, "", "  ")
	selection := string(data)
	if err != nil {
		selection = "Selected experience unavailable: " + err.Error()
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".cmd"
	}
	logical := []string{
		"Selected experience", shellSDKSourceLabel(req.Experience),
		"Changes + scopes", "New selected runtime; personal Pi/projects unchanged.",
		"Ready reviews choices, not installed health or qualification proof.",
		"Config: " + filepath.Join(req.Destination, "config"),
		"Target HOME/state: " + filepath.Join(req.Destination, "home"),
		"Commands: " + filepath.Join(req.Destination, "bin", "gentle-shell"+suffix) + " and " + filepath.Join(req.Destination, "bin", "pi"+suffix),
		"Project overrides remain; no automatic indexing hooks.",
	}
	if req.Experience != nil && req.Experience.SDKSource == "main-preview" {
		logical = append(logical, "SDK Main + stock provider: On my existing Pi or Separate install.",
			"Main installer is inert DATA; stock native/pins remain.",
			"Source + selected SDK: "+filepath.Join(req.Destination, "runtime"))
	}
	logical = append(logical, "Confirm this physical selection and both command bindings? y installs; n edits target; Ctrl-E configures.",
		"Complete validated selection:", selection, "Inspection preview:", preview)
	return strings.Split(ansi.Hardwrap(strings.Join(logical, "\n"), width, true), "\n")
}

func shellReadyLayout(width, height int) (contentWidth, slots int, wide bool) {
	wide = width >= 100 && height >= 10
	contentWidth, slots = width, max(1, height-2)
	if height >= 6 {
		slots = height - 3
	}
	if wide {
		contentWidth = width - width/3 - 1
	}
	return
}

// RenderShellReady is the historical Plan/About view over the current selector.
// The action cursor is inert; only the containing model may confirm a handoff.
func RenderShellReady(req shellinstaller.UserInstallRequest, preview string, width, height, offset, action int) string {
	width, height = shellViewport(width, height)
	action = min(max(action, 0), len(shellReadyActions)-1)
	contentWidth, slots, wide := shellReadyLayout(width, height)
	data := shellReadyLines(req, preview, contentWidth)
	last := max(0, len(data)-slots)
	offset = min(max(offset, 0), last)
	page := data[offset:min(len(data), offset+slots)]
	var lines []string
	if height >= 6 {
		lines = append(lines, renderShellExperienceConfigLine(styles.TitleStyle, "Ready — review before installation", width))
	}
	if wide {
		leftWidth := width - contentWidth - 1
		left := []string{renderShellExperienceConfigLine(styles.HeadingStyle, "Plan", leftWidth)}
		for i, label := range shellReadyActions {
			style, marker := styles.UnselectedStyle, " "
			if i == action {
				style, marker = styles.SelectedStyle, ">"
			}
			left = append(left, renderShellExperienceConfigLine(style, marker+label, leftWidth))
		}
		left = append(left, "Install: confirm handoff", "Back: edit destination", "Cancel: do not install")
		summary := []string{"Target: " + req.Destination, "Mode: " + req.Mode}
		if req.Experience != nil {
			summary = append(summary, "Persona: "+req.Experience.Persona, "Background subagents: "+req.Experience.BackgroundSubagents)
		}
		for _, text := range summary[:min(len(summary), slots+1-len(left))] {
			left = append(left, renderShellExperienceConfigLine(styles.SubtextStyle, text, leftWidth))
		}
		right := append([]string{renderShellExperienceConfigLine(styles.HeadingStyle, "About: "+shellReadyActions[action], contentWidth)}, page...)
		for i := 0; i < slots+1; i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, padShellExperienceConfigStyled(l, leftWidth)+styles.SubtextStyle.Render("│")+r)
		}
	} else {
		label := shellFlowFit(width, ">"+shellReadyActions[action], ">"+[]string{"Install", "Back", "Cancel"}[action], ">"+[]string{"I", "B", "C"}[action])
		lines = append(lines, renderShellExperienceConfigLine(styles.SelectedStyle, label, width))
		lines = append(lines, page...)
	}
	footer := shellFlowFit(width,
		fmt.Sprintf("[%d/%d] j/k: action • enter: select • pgup/pgdn: review • esc: cancel", offset, last),
		fmt.Sprintf("[%d/%d] pgup/pgdn: review • esc: cancel", offset, last),
		fmt.Sprintf("[%d/%d]", offset, last), "esc", "←")
	lines = append(lines, renderShellExperienceConfigLine(styles.HelpStyle, footer, width))
	return joinShellExperienceConfigLines(lines, height)
}

// ShellReadyPageRows is the exact data stride used by the visible review.
func ShellReadyPageRows(width, height int) int {
	width, height = shellViewport(width, height)
	_, slots, _ := shellReadyLayout(width, height)
	return slots
}

func ShellReadyLastOffset(req shellinstaller.UserInstallRequest, preview string, width, height int) int {
	width, height = shellViewport(width, height)
	contentWidth, slots, _ := shellReadyLayout(width, height)
	return max(0, len(shellReadyLines(req, preview, contentWidth))-slots)
}

func shellViewport(width, height int) (int, int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 30
	}
	return width, height
}

func shellSDKSourceLabel(p *shellinstaller.UserExperience) string {
	if p == nil || p.SDKSource == "" {
		return "SDK source: stock"
	}
	if p.SDKSource == "main-preview" {
		return "SDK Main + stock provider (Pi or Separate)"
	}
	return "SDK source: stock (" + p.SDKSource + ")"
}
