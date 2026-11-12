package screens

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// ShellBackupsView is the complete render state of the Manage → Replaced
// backups screen. Exactly one of Loading, Confirm, Working or Done is active;
// otherwise the list (or its empty state) is rendered. LoadErr carries a
// discovery failure and must render as an error, never as the empty list; Err
// carries the preservation error shown on the result state.
type ShellBackupsView struct {
	Target  string
	Mode    string
	Backups []shellinstaller.UserReplacedBackup
	Loading bool
	Confirm bool
	Working bool
	Done    bool
	LoadErr string
	Err     string
}

// ShellBackupsActions returns the selectable actions of the list or
// confirmation state. Cleanup is offered only when a successfully loaded list
// has an older candidate to remove; the confirmation asks before any removal.
func ShellBackupsActions(view ShellBackupsView) []string {
	if view.Confirm {
		return []string{"Clean older backups", "Back"}
	}
	if view.LoadErr == "" && len(view.Backups) > 1 {
		return []string{"Clean older backups", "Back"}
	}
	return []string{"Back"}
}

// RenderShellBackups renders the Manage → Replaced backups screen: loading,
// the sorted candidate list (or the empty state), the older-cleanup
// confirmation, and the working/result states.
func RenderShellBackups(view ShellBackupsView, cursor, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := []string{
		styles.TitleStyle.Render("Replaced backups"),
		detectedSubtleStyle.Render(shellDetectedTarget(view.Target)),
		"",
	}
	errorLines := func(text string) {
		for _, paragraph := range strings.Split(text, "\n") {
			for _, line := range wrapShellExperienceConfigText(paragraph, width-2) {
				lines = append(lines, styles.ErrorStyle.Render(line))
			}
		}
	}
	if view.Loading {
		lines = append(lines, detectedSubtleStyle.Render("Loading replaced backups..."))
		return strings.Join(lines, "\n")
	}
	if view.LoadErr != "" {
		errorLines(view.LoadErr)
		lines = append(lines, "")
	} else if len(view.Backups) == 0 {
		lines = append(lines, "No replaced backups beside this installation.", "")
	} else {
		for _, backup := range view.Backups {
			marker := ""
			if backup.Latest {
				marker = detectedAccentStyle.Render("  latest, kept")
			}
			lines = append(lines, fmt.Sprintf("  %-42s %12s", filepath.Base(backup.Path), shellBackupsSize(backup))+marker)
		}
		lines = append(lines, "")
	}
	switch {
	case view.Working:
		lines = append(lines, styles.WarningStyle.Render("Working: removing older backups..."), detectedSubtleStyle.Render("Do not close this terminal."))
		return strings.Join(lines, "\n")
	case view.Done:
		if view.Err != "" {
			errorLines(view.Err)
		} else {
			lines = append(lines, styles.SuccessStyle.Render("Older backups removed. The latest backup was kept."))
		}
		lines = append(lines, "", detectedSubtleStyle.Render("Enter: back to the list"))
		return strings.Join(lines, "\n")
	case view.Confirm:
		lines = append(lines, "Remove every backup except the latest? Each is validated first;",
			"any uncertain backup is preserved and reported.", "")
	}
	for index, action := range ShellBackupsActions(view) {
		marker, style := "   ", detectedValueStyle
		if index == cursor {
			marker, style = " ▸ ", detectedAccentStyle
		}
		lines = append(lines, marker+style.Render(action))
	}
	lines = append(lines, "", detectedSubtleStyle.Render("j/k: navigate  •  enter: select  •  esc: back"))
	return strings.Join(lines, "\n")
}

// shellBackupsSize renders a candidate's byte total; an incomplete walk is a
// lower bound and is marked with ≥.
func shellBackupsSize(backup shellinstaller.UserReplacedBackup) string {
	size := fmt.Sprintf("%d B", backup.Bytes)
	for index, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		scale := int64(1) << (10 * (index + 1))
		if backup.Bytes >= scale {
			size = fmt.Sprintf("%.1f %s", float64(backup.Bytes)/float64(scale), unit)
		}
	}
	if !backup.Complete {
		return "≥ " + size
	}
	return size
}
