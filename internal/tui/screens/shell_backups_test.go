package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func shellBackupsView() ShellBackupsView {
	return ShellBackupsView{
		Target: "/home/u/.local/share/gentle-shell/install",
		Mode:   "separate",
		Backups: []shellinstaller.UserReplacedBackup{
			{Path: "/home/u/.local/share/gentle-shell/install.replaced-20260101T110000Z", Stamp: "20260101T110000Z", Bytes: 100, Complete: true},
			{Path: "/home/u/.local/share/gentle-shell/install.replaced-20260101T120000Z", Stamp: "20260101T120000Z", Bytes: 200, Complete: true, Latest: true},
		},
	}
}

// TestRenderShellBackups pins every render state of the Manage → Replaced
// backups screen. The stub returns the empty string, so each assertion is RED
// until the screen is implemented.
func TestRenderShellBackups(t *testing.T) {
	mutate := func(change func(*ShellBackupsView)) ShellBackupsView {
		view := shellBackupsView()
		change(&view)
		return view
	}
	tests := []struct {
		name      string
		view      ShellBackupsView
		want      []string
		noCleanup bool
	}{
		{"loading", mutate(func(v *ShellBackupsView) { v.Loading = true }), []string{"Loading replaced backups"}, false},
		{"list", shellBackupsView(), []string{"Replaced backups", "install.replaced-20260101T110000Z", "install.replaced-20260101T120000Z", "Clean older backups"}, false},
		{"empty", mutate(func(v *ShellBackupsView) { v.Backups = nil }), []string{"No replaced backups"}, true},
		{"confirm", mutate(func(v *ShellBackupsView) { v.Confirm = true }), []string{"Clean older backups", "Back"}, false},
		{"working", mutate(func(v *ShellBackupsView) { v.Working = true }), []string{"Working"}, false},
		{"success", mutate(func(v *ShellBackupsView) { v.Done = true }), []string{"Older backups removed"}, false},
		{"preservation error", mutate(func(v *ShellBackupsView) { v.Done = true; v.Err = "uncertain backup preserved" }), []string{"uncertain backup preserved"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := RenderShellBackups(test.view, 0, 120, 40)
			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("lacks %q: %q", want, output)
				}
			}
			if test.noCleanup && strings.Contains(output, "Clean older backups") {
				t.Fatalf("offers cleanup: %q", output)
			}
		})
	}
}
