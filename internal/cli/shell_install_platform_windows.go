package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

const shellInstallHelp = `gentle-ai shell install --target C:\owned\shell --mode separate
  --channel stable|main     default stable; Main freezes the canonical source snapshot
  --inspect                 print physical-selection confirmation without effects
  --confirm SHA256          approve that exact inspected selection
No flags: existing installer TUI. Owned commands live in TARGET\bin; personal PATH stays unchanged.
Requires Windows 11 x64, a normal unelevated account and a private local NTFS directory.
Separate only. No Shared, update, force, recovery, Windows Server or ARM64 qualification.
`

func shellInstallPlatformFlags(flags *flag.FlagSet, req *shellinstaller.UserInstallRequest) func() error {
	flags.StringVar(&req.Channel, "channel", "stable", "stable or main")
	return func() error {
		channel, err := shellinstaller.UserInstallChannel(req.Channel)
		if err != nil || req.Channel == "" {
			return errors.New("invalid channel; rerun `gentle-ai shell install --channel stable` or `gentle-ai shell install --channel main`")
		}
		req.Channel = channel
		return nil
	}
}

func shellEntryValues(req shellinstaller.UserInstallRequest) []string {
	channel, _ := shellinstaller.UserInstallChannel(req.Channel) // Worker rejects invalid values.
	return []string{req.Destination, req.Mode, req.SharedPrefix, req.SharedAgent, req.Confirmation, "--channel", channel}
}

func shellInstallInspected(stdout io.Writer, req shellinstaller.UserInstallRequest, token string) error {
	_, err := fmt.Fprintf(stdout, "Channel: %s\nConfirmation: %s\nCommands: %s, %s\n", req.Channel, token, filepath.Join(req.Destination, "bin/gentle-shell.cmd"), filepath.Join(req.Destination, "bin/pi.cmd"))
	return err
}

// The Windows worker runs inside the TUI so Ctrl-C can cooperatively cancel
// it and the model can wait for actual stop/reap before quitting.
func runShellInstallTUI(ctx context.Context, cancel context.CancelFunc, self string, stdout io.Writer) error {
	model := shellInstallModel{ctx: ctx, cancel: cancel, self: self, stdout: stdout, req: shellinstaller.UserInstallRequest{Mode: "separate", Channel: "stable"}}
	final, err := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	if err != nil {
		return err
	}
	return final.(shellInstallModel).err
}
